package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lewtec/lewkit/x/driver/ndeval/vulkan"
	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/lewtec/lewkit/x/taskgroup"
	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
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
	trying  map[int]map[straightKey]struct{}
	seen    map[Op]map[int]map[uint64]struct{}
	width   int
	cost    float64
}

func newBeam(first snapshot) *beam {
	b := &beam{
		best:   []snapshot{first},
		served: map[Op]map[int]bool{},
		busy:   map[int]int{},
		trying: map[int]map[straightKey]struct{}{},
		seen:   map[Op]map[int]map[uint64]struct{}{},
		head:   first.id,
		width:  beamWidth,
		cost:   commandCost,
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *beam) lessSnap(a, c snapshot) bool {
	return lessObjectiveCost(a.errSum, a.commands, a.paths, c.errSum, c.commands, c.paths, b.cost)
}

func (b *beam) list() []snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]snapshot(nil), b.best...)
}

func (b *beam) holds(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sn := range b.best {
		if sn.id == id {
			return true
		}
	}
	return false
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

// remember reports whether this operator has already proposed this
// document for this corpus entry. A repeat is not sent again.
func (b *beam) remember(op Op, id int, key uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen[op] == nil {
		b.seen[op] = map[int]map[uint64]struct{}{}
	}
	if b.seen[op][id] == nil {
		b.seen[op][id] = map[uint64]struct{}{}
	}
	if _, ok := b.seen[op][id][key]; ok {
		return false
	}
	b.seen[op][id][key] = struct{}{}
	return true
}

func (b *beam) abandon(id int) {
	b.mu.Lock()
	if b.busy[id] > 0 {
		b.busy[id]--
	}
	b.mu.Unlock()
	b.cond.Broadcast()
}

func (b *beam) waitStraight(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.trying[id]) > 0 && !b.closed {
		b.cond.Wait()
	}
}

// reserveStraight claims up to n keys that this document has not
// already refused or sent. The caller shuffles keys first.
func (b *beam) reserveStraight(id int, keys []straightKey, n int) []straightKey {
	b.mu.Lock()
	defer b.mu.Unlock()
	var miss map[straightKey]struct{}
	for _, sn := range b.best {
		if sn.id == id {
			miss = sn.simplifyMiss
			break
		}
	}
	if b.trying[id] == nil {
		b.trying[id] = map[straightKey]struct{}{}
	}
	var out []straightKey
	for _, key := range keys {
		if _, skip := miss[key]; skip {
			continue
		}
		if _, skip := b.trying[id][key]; skip {
			continue
		}
		b.trying[id][key] = struct{}{}
		out = append(out, key)
		if len(out) == n {
			break
		}
	}
	return out
}

func (b *beam) settleStraight(id int, key straightKey, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.trying[id] != nil {
		delete(b.trying[id], key)
	}
	for i := range b.best {
		if ok && b.best[i].id != id {
			continue
		}
		miss := cloneStraightMiss(b.best[i].simplifyMiss)
		if miss == nil {
			miss = map[straightKey]struct{}{}
		}
		miss[key] = struct{}{}
		b.best[i].simplifyMiss = miss
	}
	b.cond.Broadcast()
}

func (b *beam) straightBusy(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.trying[id]) > 0
}

func (b *beam) noteMiss(keys ...straightKey) {
	if len(keys) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.best {
		miss := cloneStraightMiss(b.best[i].simplifyMiss)
		if miss == nil {
			miss = map[straightKey]struct{}{}
		}
		for _, key := range keys {
			miss[key] = struct{}{}
		}
		b.best[i].simplifyMiss = miss
	}
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
		for j > 0 && b.lessSnap(next[j], next[j-1]) {
			next[j], next[j-1] = next[j-1], next[j]
			j--
		}
	}
	limit := b.width
	if limit <= 0 {
		limit = beamWidth
	}
	if len(next) > limit {
		next = next[:limit]
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
		want:         want,
		wantP:        wantP,
		got:          sn.got,
		doc:          sn.doc,
		fills:        append([]color.NRGBA(nil), sn.fills...),
		owner:        append([]uint16(nil), sn.owner...),
		errSum:       sn.errSum,
		paths:        sn.paths,
		w:            w,
		h:            h,
		simplifyMiss: cloneStraightMiss(sn.simplifyMiss),
		facts:        sn.facts,
		knob:         sn.knob,
	}
	if sn.got != nil {
		// The plane converts on first read. A proposer that only emits
		// geometry never reads it, so the claim does not allocate a table.
		s.gotP = loss.NewPlane(sn.got)
	}
	return s
}

type agentNote struct {
	pick    formPick
	err     error
	elapsed time.Duration
	verdict chan bool
}

// proposalBatch is how many scored notes one ranker turn packs, and how
// many notes may sit unscored. send waits at that count, so proposers
// stop handing over documents until the ranker takes some.
// proposalLog is how many scored rows the page keeps. Older rows drop
// off the log; the search does not drop the documents they described.
const (
	proposalBatch  = 32
	proposalBuffer = proposalBatch
	proposalLog    = 40
	renderAhead    = proposalBatch
	// simplifyWindow is how many straight vertices one simplify worker
	// hands the ranker before it waits for the decisions. The rest stay
	// unsent, so a long run of refusals does not fill the queue.
	simplifyWindow = 8
)

func workerLimit() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		n = 1
	}
	if n > proposalBatch {
		return proposalBatch
	}
	return n
}

type pixmapNote struct {
	note agentNote
	img  *image.NRGBA
}

// pendingKeep is a scored document waiting for its islands. The ranker
// offers it after the CPU pass, and keeps scoring on the device meanwhile.
type pendingKeep struct {
	seq      int
	sn       snapshot
	pick     formPick
	proposal int
}

// runAgents runs three actors. Proposers claim a parent and send geometry.
// Painters turn that geometry into pixmaps. This goroutine is the ranker:
// it scores, keeps the beam, and the proposers claim any parent that offer publishes.
func (s *world) runAgents(ctx context.Context, emit func(Op, []pix, []pix, []search.Rated) bool) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	first := s.snap()
	first.lefts = s.watchLeftovers(ctx)
	board := newBeam(first)
	board.width = s.knob.beamWidth()
	board.cost = s.knob.cost()
	proposals := make(chan agentNote, proposalBuffer)
	pixmaps := make(chan pixmapNote, renderAhead)
	ready := make(chan pendingKeep, simplifyWindow)
	var proposers, painters sync.WaitGroup
	defer func() {
		cancel()
		board.close()
		proposers.Wait()
		painters.Wait()
	}()
	// slots is how many proposers may build at once. A proposer holds a
	// slot through the send, and send blocks when the batch queue is full,
	// so the rest wait until the ranker has taken work.
	slots := make(chan struct{}, workerLimit())
	for _, kind := range agentKinds() {
		copies := 1
		if kind == OpSimplify || fuzzKind(kind) {
			copies = fuzzCopies
		}
		for range copies {
			s.startProposer(ctx, &proposers, board, kind, proposals, slots)
		}
	}
	go func() {
		proposers.Wait()
		close(proposals)
	}()
	paintersN := runtime.GOMAXPROCS(0)
	if paintersN < 1 {
		paintersN = 1
	}
	for range paintersN {
		painters.Add(1)
		go paintProposals(ctx, proposals, pixmaps, &painters)
	}
	go func() {
		painters.Wait()
		close(pixmaps)
	}()
	st := rankState{head: first.id, bestByOp: map[Op]*namedPick{}}
	var fold keepFold
	var inFlight int
	var seq int
	go func() {
		<-ctx.Done()
		board.close()
	}()
loop:
	for {
		for {
			select {
			case kept := <-ready:
				s.foldKept(ctx, board, &st, &fold, &inFlight, kept, emit)
				if st.stop {
					board.close()
					break loop
				}
			default:
				goto take
			}
		}
	take:
		if inFlight >= simplifyWindow {
			select {
			case <-ctx.Done():
				board.close()
				break loop
			case kept := <-ready:
				s.foldKept(ctx, board, &st, &fold, &inFlight, kept, emit)
				if st.stop {
					board.close()
					break loop
				}
			}
			continue
		}
		var painted pixmapNote
		var kept pendingKeep
		var ok bool
		select {
		case <-ctx.Done():
			board.close()
			break loop
		case kept = <-ready:
			s.foldKept(ctx, board, &st, &fold, &inFlight, kept, emit)
			if st.stop {
				board.close()
				break loop
			}
			continue
		case painted, ok = <-pixmaps:
			if !ok {
				break loop
			}
		}
		batch := []pixmapNote{painted}
		pixels := pixmapPixels(painted)
		closed := false
	drain:
		for len(batch) < proposalBatch && pixels < maxDevicePixels {
			select {
			case <-ctx.Done():
				board.close()
				break loop
			case next, ok := <-pixmaps:
				if !ok {
					closed = true
					break drain
				}
				batch = append(batch, next)
				pixels += pixmapPixels(next)
			default:
				break drain
			}
		}
		notes := make([]agentNote, len(batch))
		imgs := make([]*image.NRGBA, len(batch))
		for i, item := range batch {
			notes[i] = item.note
			imgs[i] = item.img
		}
		queued := s.rankNotes(ctx, board, &st, notes, imgs, ready, &seq, emit)
		inFlight += queued
		if closed || ctx.Err() != nil || st.stop {
			board.close()
			break loop
		}
	}
	for inFlight > 0 && ctx.Err() == nil && !st.stop {
		kept := <-ready
		s.foldKept(ctx, board, &st, &fold, &inFlight, kept, emit)
	}
	if !st.stop && st.yielded && len(st.bestByOp) > st.flushed {
		top := board.top()
		s.load(top)
		rated := collectRated(st.bestByOp)
		markKept(rated, []formPick{{op: top.operator}})
		blob, fitted := leftoverView(top, st.pool)
		emit(top.operator, blob, fitted, rated)
	}
	if err := ctx.Err(); err != nil && !st.yielded {
		return false, err
	}
	return st.yielded, nil
}

type rankState struct {
	yielded  bool
	head     int
	bestByOp map[Op]*namedPick
	flushed  int
	pool     []formPick
	recent   []candResult
	loss     []float64
	log      []search.Proposal
	logBase  int
	stop     bool
}

func (st *rankState) pushProposal(p search.Proposal) int {
	abs := st.logBase + len(st.log)
	st.log = append(st.log, p)
	if len(st.log) > proposalLog {
		drop := len(st.log) - proposalLog
		st.log = append([]search.Proposal(nil), st.log[drop:]...)
		st.logBase += drop
	}
	return abs
}

func (st *rankState) proposalAt(abs int) *search.Proposal {
	i := abs - st.logBase
	if i < 0 || i >= len(st.log) {
		return nil
	}
	return &st.log[i]
}

func paintNote(note agentNote) *image.NRGBA {
	if note.err != nil || note.pick.scored || note.pick.doc.Width() == 0 {
		return nil
	}
	img, err := render.Render(note.pick.doc)
	if err != nil {
		return nil
	}
	return img
}

func paintProposals(ctx context.Context, in <-chan agentNote, out chan<- pixmapNote, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		var note agentNote
		var ok bool
		select {
		case <-ctx.Done():
			return
		case note, ok = <-in:
			if !ok {
				return
			}
		}
		painted := pixmapNote{note: note, img: paintNote(note)}
		select {
		case <-ctx.Done():
			return
		case out <- painted:
		}
	}
}

func pixmapPixels(item pixmapNote) int {
	if item.img == nil {
		return 1
	}
	b := item.img.Bounds()
	if n := b.Dx() * b.Dy(); n > 0 {
		return n
	}
	return 1
}

func (s *world) rankNotes(ctx context.Context, board *beam, st *rankState, notes []agentNote, imgs []*image.NRGBA, ready chan<- pendingKeep, seq *int, emit func(Op, []pix, []pix, []search.Rated) bool) int {
	board.beginChoose()
	defer board.endChoose()
	for range notes {
		board.account(-1)
	}
	if st.stop || ctx.Err() != nil {
		return 0
	}
	for _, note := range notes {
		if note.err != nil {
			board.close()
			st.stop = true
			return 0
		}
	}
	drafts := make([]formPick, len(notes))
	for i, note := range notes {
		drafts[i] = note.pick
	}
	var scored []formPick
	if imgs != nil {
		scored = s.scoreRendered(ctx, drafts, imgs)
	} else {
		scored = s.scoreBatch(ctx, drafts)
	}
	if ctx.Err() != nil {
		return 0
	}
	queued := 0
	for i, pick := range scored {
		note := notes[i]
		if ctx.Err() != nil {
			replyVerdict(note, false)
			return queued
		}
		if !pick.scored {
			replyVerdict(note, false)
			continue
		}
		// A simplify run drops vertices of one document. Once one drop
		// takes the slot, the rest of that run is the old ring.
		if pick.op == OpSimplify && pick.parent.id != 0 && !board.holds(pick.parent.id) {
			replyVerdict(note, false)
			continue
		}
		if pick.hasStraight && !pick.ok {
			board.noteMiss(pick.straightMiss)
		}
		replyVerdict(note, pick.ok)
		id := pick.op
		row := st.bestByOp[id]
		if row == nil {
			row = &namedPick{}
			st.bestByOp[id] = row
		}
		if note.elapsed > row.elapsed {
			row.elapsed = note.elapsed
		}
		if betterPick(pick, row.pick) {
			row.pick = pick
		}
		s.logCandidate(id, note.elapsed, pick)
		st.recent = append(st.recent, candResult{name: id.String(), pick: pick})
		if len(st.recent) > 12 {
			st.recent = st.recent[len(st.recent)-12:]
		}
		if s.rank != nil {
			rows := append([]candResult(nil), st.recent...)
			sortCandResults(rows)
			s.rank.show(ctx, rankSummary(rows, s.knob.beamWidth()))
		}
		abs := st.pushProposal(search.Proposal{
			Operator: id.String(),
			Score:    objectiveCost(pick.errSum, pick.commands, s.knob.cost()),
			Paths:    pick.paths,
			Commands: pick.commands,
			Vertices: docVertices(pick.doc),
			Keep:     pick.ok,
		})
		if !pick.ok || st.stop {
			s.publishRank(ctx, board, st)
			continue
		}
		s.load(pick.parent)
		s.apply(pick)
		sn := s.snap()
		sn.operator = pick.op
		if ready != nil {
			s.publishRank(ctx, board, st)
			board.beginChoose()
			item := pendingKeep{seq: *seq, sn: sn, pick: pick, proposal: abs}
			*seq++
			queued++
			go s.fillKeep(ctx, ready, item)
			continue
		}
		sn.lefts = s.watchLeftovers(ctx)
		if !s.retainKept(ctx, board, st, sn, pick, abs, emit) {
			return queued
		}
	}
	s.publishRank(ctx, board, st)
	return queued
}

func replyVerdict(note agentNote, ok bool) {
	if note.verdict == nil {
		return
	}
	select {
	case note.verdict <- ok:
	default:
	}
}

func (s *world) fillKeep(ctx context.Context, ready chan<- pendingKeep, item pendingKeep) {
	item.sn.lefts = s.leftoversOf(item.sn)
	select {
	case <-ctx.Done():
		return
	case ready <- item:
	}
}

func (s *world) leftoversOf(sn snapshot) []leftover {
	w := &world{
		want:  s.want,
		wantP: s.wantP,
		got:   sn.got,
		doc:   sn.doc,
		fills: sn.fills,
		owner: sn.owner,
		paths: sn.paths,
		w:     s.w,
		h:     s.h,
		knob:  s.knob,
	}
	if sn.got != nil {
		w.gotP = loss.NewPlane(sn.got)
	}
	if w.wantP != nil {
		w.wantP.Ensure()
	}
	return w.leftovers(nil)
}

type keepFold struct {
	next int
	wait map[int]pendingKeep
}

func (f *keepFold) push(item pendingKeep) []pendingKeep {
	if f.wait == nil {
		f.wait = map[int]pendingKeep{}
	}
	f.wait[item.seq] = item
	var out []pendingKeep
	for {
		item, ok := f.wait[f.next]
		if !ok {
			break
		}
		delete(f.wait, f.next)
		f.next++
		out = append(out, item)
	}
	return out
}

func (s *world) foldKept(ctx context.Context, board *beam, st *rankState, fold *keepFold, inFlight *int, item pendingKeep, emit func(Op, []pix, []pix, []search.Rated) bool) {
	for _, ready := range fold.push(item) {
		board.endChoose()
		*inFlight--
		if !s.retainKept(ctx, board, st, ready.sn, ready.pick, ready.proposal, emit) {
			return
		}
	}
}

func (s *world) retainKept(ctx context.Context, board *beam, st *rankState, sn snapshot, pick formPick, abs int, emit func(Op, []pix, []pix, []search.Rated) bool) bool {
	if !board.offer(sn) {
		s.publishRank(ctx, board, st)
		return true
	}
	if row := st.proposalAt(abs); row != nil {
		row.Slot = true
	}
	st.loss = append(st.loss, objectiveCost(sn.errSum, sn.commands, s.knob.cost()))
	jobNote(ctx, archiveLine(board.list()))
	st.pool = append(st.pool, pick)
	s.publishRank(ctx, board, st)
	top, changed := board.takeHead(st.head)
	if !changed {
		return true
	}
	st.head = top.id
	s.load(top)
	rated := collectRated(st.bestByOp)
	st.flushed = len(st.bestByOp)
	markKept(rated, []formPick{{op: top.operator}})
	blob, fitted := leftoverView(top, st.pool)
	st.yielded = true
	if !emit(top.operator, blob, fitted, rated) {
		board.close()
		st.stop = true
		return false
	}
	return true
}

func (s *world) publishRank(ctx context.Context, board *beam, st *rankState) {
	emitFrame(ctx, search.Frame{
		Beam:      beamSlots(board.list(), s.knob.cost()),
		Proposals: append([]search.Proposal(nil), st.log...),
		Loss:      append([]float64(nil), st.loss...),
	})
}

func (s *world) fuzzSimplifyParent(ctx context.Context, st *taskgroup.Status, board *beam, parent snapshot, proposals chan<- agentNote) (stop bool) {
	if st != nil {
		st.Update("running")
	}
	fork := parent.fork(s.want, s.wantP, s.w, s.h)
	sent, busy, err := fork.fuzzSimplify(ctx, board, parent, proposals, time.Now())
	if err != nil {
		board.finish(OpSimplify, parent.id)
		if ctx.Err() != nil {
			return true
		}
		if st != nil {
			st.Update(err.Error())
		}
		board.send(ctx, proposals, agentNote{err: err})
		return true
	}
	if sent == 0 && !busy {
		board.finish(OpSimplify, parent.id)
		if st != nil {
			st.Update("waiting")
		}
		return false
	}
	board.abandon(parent.id)
	if sent == 0 && busy {
		board.waitStraight(parent.id)
	}
	if st != nil {
		st.Update("proposed")
	}
	return false
}

func (s *world) startProposer(ctx context.Context, wg *sync.WaitGroup, board *beam, kind Op, proposals chan<- agentNote, slots chan struct{}) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		// The task row stays open for the whole claim loop. The work runs on
		// this goroutine, so a proposer waiting for a parent does not occupy
		// a CPU slot and the row remains a sibling of rank under the job.
		watchStep(ctx, kind.String(), func(st *taskgroup.Status) {
			if st != nil {
				st.Progress(0, -1)
				st.Update("waiting")
			}
			for {
				parent, ok := board.claim(kind)
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					board.abandon(parent.id)
					return
				case slots <- struct{}{}:
				}
				stop := s.proposeParent(ctx, st, board, kind, parent, proposals)
				<-slots
				if stop {
					return
				}
			}
		})
	}()
}

func (s *world) proposeParent(ctx context.Context, st *taskgroup.Status, board *beam, kind Op, parent snapshot, proposals chan<- agentNote) (stop bool) {
	if kind == OpSimplify {
		return s.fuzzSimplifyParent(ctx, st, board, parent, proposals)
	}
	again := false
	defer func() {
		if again {
			board.abandon(parent.id)
			return
		}
		board.finish(kind, parent.id)
	}()
	if st != nil {
		st.Update("running")
	}
	started := time.Now()
	fork := parent.fork(s.want, s.wantP, s.w, s.h)
	fork.rank = s.rank
	fork.candidateLog = s.candidateLog
	fork.stashOn = true
	fork.oneShot = fuzzKind(kind)
	var sent atomic.Bool
	send := func(p formPick) {
		if ctx.Err() != nil {
			return
		}
		if !board.remember(kind, parent.id, docKey(p.doc)) {
			return
		}
		p.parent = parent
		fork.gotNovel = true
		sent.Store(true)
		board.send(ctx, proposals, agentNote{pick: p, elapsed: time.Since(started)})
	}
	fork.emitNow = send
	tries := 1
	switch kind {
	case OpSwap, OpDelete, OpSlide, OpBend:
		tries = 8
	}
	var err error
	for range tries {
		err = fork.emitKind(ctx, kind, parent.lefts, send)
		if err != nil || sent.Load() {
			break
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return true
		}
		if st != nil {
			st.Update(err.Error())
		}
		board.send(ctx, proposals, agentNote{err: err})
		return true
	}
	if fuzzKind(kind) && sent.Load() {
		again = true
	}
	if st != nil {
		if sent.Load() {
			st.Update("proposed")
		} else {
			st.Update("waiting")
		}
	}
	return false
}

func fuzzKind(kind Op) bool {
	switch kind {
	case OpSuggest, OpSquash, OpSpline, OpSwap, OpDelete, OpSlide, OpBend,
		OpWash, OpJoin, OpSubtract, OpUnhole:
		return true
	default:
		return false
	}
}

func beamSlots(list []snapshot, cost float64) []search.Slot {
	out := make([]search.Slot, len(list))
	for i, sn := range list {
		name := sn.operator.String()
		if name == "" {
			name = "plate"
		}
		out[i] = search.Slot{Operator: name, Score: objectiveCost(sn.errSum, sn.commands, cost), Paths: sn.paths, Commands: sn.commands, Vertices: docVertices(sn.doc)}
	}
	return out
}

type frameKey struct{}

// WithFrames publishes the beam and the latest scored batch.
func WithFrames(ctx context.Context, fn func(search.Frame)) context.Context {
	if ctx == nil || fn == nil {
		return ctx
	}
	return context.WithValue(ctx, frameKey{}, fn)
}

func emitFrame(ctx context.Context, frame search.Frame) {
	if ctx == nil {
		return
	}
	fn, ok := ctx.Value(frameKey{}).(func(search.Frame))
	if ok && fn != nil {
		fn(frame)
	}
}

func (b *beam) send(ctx context.Context, proposals chan<- agentNote, note agentNote) bool {
	b.account(1)
	if ctx == nil {
		proposals <- note
		return true
	}
	select {
	case <-ctx.Done():
		b.account(-1)
		return false
	case proposals <- note:
		return true
	}
}

func (s *world) watchLeftovers(ctx context.Context) []leftover {
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	var lefts []leftover
	watchStep(ctx, "leftovers", func(st *taskgroup.Status) {
		lefts = s.leftovers(ctx)
		if st != nil {
			st.Update(fmt.Sprintf("%d islands", len(lefts)))
		}
	})
	return lefts
}

func (s *world) scoreDraft(ctx context.Context, p formPick) (formPick, error) {
	s.load(p.parent)
	node := svgNodeAt(p.doc, p.replace)
	g := grow{i: p.replace, work: p.work, fill: p.fill, dirty0: p.dirty}
	if g.dirty0.Empty() && len(p.work) > 0 {
		g.dirty0 = islandRect(p.work)
	}
	scored, err := s.scoreCand(ctx, p.doc, node, g, p.op)
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
