package stack

import (
	"context"
	"image"
	"image/color"
	"runtime"
	"sync"
	"time"

	_ "github.com/lewtec/lewkit/x/driver/ndeval/vulkan"
	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
)

// beam is the chooser's list of the best documents. Proposers read it and
// claim the parent that currently has the fewest proposers working on it.
type beam struct {
	mu      sync.Mutex
	cond    *sync.Cond
	best    []snapshot
	served  map[Op]map[int]bool
	busy    map[int]int
	pending int
	holding int
	closed  bool
	head    int
}

func newBeam(first snapshot) *beam {
	b := &beam{
		best:   []snapshot{first},
		served: map[Op]map[int]bool{},
		busy:   map[int]int{},
		head:   first.id,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *beam) list() []snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]snapshot(nil), b.best...)
}

func (b *beam) claim(op Op) (snapshot, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		if b.closed {
			return snapshot{}, false
		}
		if parent, ok := b.pick(op); ok {
			b.busy[parent.id]++
			return parent, true
		}
		if b.quiescent() {
			return snapshot{}, false
		}
		b.cond.Wait()
	}
}

func (b *beam) pick(op Op) (snapshot, bool) {
	chosen := snapshot{}
	found := false
	load := 0
	for _, sn := range b.best {
		if b.served[op] != nil && b.served[op][sn.id] {
			continue
		}
		n := b.busy[sn.id]
		if !found || n < load {
			chosen = sn
			load = n
			found = true
		}
	}
	return chosen, found
}

func (b *beam) finish(op Op, id int) {
	b.mu.Lock()
	if b.served[op] == nil {
		b.served[op] = map[int]bool{}
	}
	b.served[op][id] = true
	if b.busy[id] > 0 {
		b.busy[id]--
	}
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (b *beam) offer(sn snapshot) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	for _, old := range b.best {
		if samePoint(old, sn) {
			return false
		}
	}
	next := append(append([]snapshot{}, b.best...), sn)
	for i := 1; i < len(next); i++ {
		j := i
		for j > 0 && lexicographicLessSnapshot(next[j], next[j-1]) {
			next[j], next[j-1] = next[j-1], next[j]
			j--
		}
	}
	if len(next) > beamWidth {
		next = next[:beamWidth]
	}
	kept := false
	for _, old := range next {
		if old.id == sn.id {
			kept = true
			break
		}
	}
	b.best = next
	if kept {
		b.cond.Broadcast()
	}
	return kept
}

func (b *beam) top() snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.best) == 0 {
		return snapshot{}
	}
	return b.best[0]
}

func (b *beam) takeHead(prev int) (snapshot, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.best) == 0 || b.best[0].id == prev {
		return snapshot{}, false
	}
	b.head = b.best[0].id
	return b.best[0], true
}

func (b *beam) account(d int) {
	b.mu.Lock()
	b.pending += d
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (b *beam) beginChoose() {
	b.mu.Lock()
	b.holding++
	b.mu.Unlock()
}

func (b *beam) endChoose() {
	b.mu.Lock()
	if b.holding > 0 {
		b.holding--
	}
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (b *beam) quiescent() bool {
	if b.pending > 0 || b.holding > 0 {
		return false
	}
	for _, n := range b.busy {
		if n > 0 {
			return false
		}
	}
	for _, sn := range b.best {
		for _, op := range agentKinds() {
			if b.served[op] == nil || !b.served[op][sn.id] {
				return false
			}
		}
	}
	return true
}

func (b *beam) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (sn snapshot) fork(want *image.NRGBA, wantP *loss.Plane, w, h int) *world {
	s := &world{
		want:   want,
		wantP:  wantP,
		got:    sn.got,
		doc:    sn.doc,
		fills:  append([]color.NRGBA(nil), sn.fills...),
		owner:  append([]uint16(nil), sn.owner...),
		errSum: sn.errSum,
		paths:  sn.paths,
		w:      w,
		h:      h,
	}
	if sn.got != nil {
		s.gotP = loss.NewPlane(sn.got)
		s.gotP.Ensure()
	}
	return s
}

type agentNote struct {
	pick    formPick
	err     error
	elapsed time.Duration
}

// runAgents starts one goroutine per proposer. Each reads the beam, claims the
// least-busy parent it has not served, and sends proposals to the chooser.
// The chooser scores an unscored proposal and offers it back when it stays in
// the best beamWidth documents. Worker slots leave one processor for the chooser
// so scoring does not deadlock the render pool.
func (s *world) runAgents(ctx context.Context, emit func(Op, []pix, []pix, []search.Rated) bool) (bool, error) {
	evaluator, err := openEvaluator(ctx)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	first := s.snap()
	first.lefts = s.leftovers()
	board := newBeam(first)
	notes := make(chan agentNote)
	var wg sync.WaitGroup
	slots := runtime.GOMAXPROCS(0) - 1
	if slots < 1 {
		slots = 1
	}
	ticket := make(chan struct{}, slots)
	for _, kind := range agentKinds() {
		wg.Add(1)
		go func(kind Op) {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				parent, ok := board.claim(kind)
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					board.finish(kind, parent.id)
					return
				case ticket <- struct{}{}:
				}
				started := time.Now()
				fork := parent.fork(s.want, s.wantP, s.w, s.h)
				err := fork.emitKind(kind, parent.lefts, func(p formPick) {
					p.parent = parent
					note := agentNote{pick: p, elapsed: time.Since(started)}
					board.account(1)
					select {
					case <-ctx.Done():
						board.account(-1)
					case notes <- note:
					}
				})
				<-ticket
				board.finish(kind, parent.id)
				if err != nil {
					board.account(1)
					select {
					case <-ctx.Done():
						board.account(-1)
					case notes <- agentNote{err: err}:
					}
					return
				}
			}
		}(kind)
	}
	go func() {
		wg.Wait()
		close(notes)
	}()

	yielded := false
	head := first.id
	bestByOp := map[Op]*namedPick{}
	flushed := 0
	var pool []formPick
	stop := false
	go func() {
		<-ctx.Done()
		board.close()
	}()
	for note := range notes {
		board.beginChoose()
		board.account(-1)
		if stop || ctx.Err() != nil {
			board.endChoose()
			continue
		}
		if note.err != nil {
			board.close()
			stop = true
			board.endChoose()
			continue
		}
		pick := note.pick
		var err error
		if !pick.scored {
			pick, err = s.scoreDraft(pick)
			if err != nil {
				board.endChoose()
				continue
			}
		}
		id := pick.op
		st := bestByOp[id]
		if st == nil {
			st = &namedPick{}
			bestByOp[id] = st
		}
		if note.elapsed > st.elapsed {
			st.elapsed = note.elapsed
		}
		if betterPick(pick, st.pick) {
			st.pick = pick
		}
		s.logCandidate(id, note.elapsed, pick)
		if !pick.ok {
			board.endChoose()
			continue
		}
		s.load(pick.parent)
		s.apply(pick)
		_ = s.dispatchResidual(ctx, evaluator, pick.dirty)
		sn := s.snap()
		sn.operator = pick.op
		sn.lefts = s.leftovers()
		if !board.offer(sn) {
			board.endChoose()
			continue
		}
		pool = append(pool, pick)
		if top, changed := board.takeHead(head); changed {
			head = top.id
			s.load(top)
			rated := collectRated(bestByOp)
			flushed = len(bestByOp)
			markKept(rated, []formPick{{op: top.operator}})
			blob, fitted := leftoverView(top, pool)
			yielded = true
			if !emit(top.operator, blob, fitted, rated) {
				board.close()
				stop = true
				board.endChoose()
				continue
			}
		}
		board.endChoose()
	}
	if !stop && yielded && len(bestByOp) > flushed {
		top := board.top()
		s.load(top)
		rated := collectRated(bestByOp)
		markKept(rated, []formPick{{op: top.operator}})
		blob, fitted := leftoverView(top, pool)
		emit(top.operator, blob, fitted, rated)
	}
	if err := ctx.Err(); err != nil && !yielded {
		return false, err
	}
	return yielded, nil
}

func (s *world) scoreDraft(p formPick) (formPick, error) {
	s.load(p.parent)
	node := svgNodeAt(p.doc, p.replace)
	g := grow{i: p.replace, work: p.work, fill: p.fill, dirty0: p.dirty}
	if g.dirty0.Empty() && len(p.work) > 0 {
		g.dirty0 = islandRect(p.work)
	}
	scored, err := s.scoreCand(p.doc, node, g, p.op)
	if err != nil {
		return nonePick(), err
	}
	scored.mergeJ = p.mergeJ
	scored.work = p.work
	scored.fill = p.fill
	scored.replace = p.replace
	scored.parent = p.parent
	scored.op = p.op
	scored.dirty = g.dirty0
	return scored, nil
}

func (s *world) dispatchResidual(ctx context.Context, evaluator ndarray.Evaluator, dirty image.Rectangle) error {
	if s.got == nil || s.want == nil || dirty.Empty() {
		return nil
	}
	_, err := residualSum(ctx, evaluator, s.got, s.want, dirty.Inset(-2))
	return err
}

func svgNodeAt(doc svg.Document, index int) svg.Node {
	kids := doc.Children()
	if index < 0 || index+1 >= len(kids) {
		return svg.Node{}
	}
	return kids[index+1]
}
