package stack

import (
	"context"
	"math/rand/v2"
	"runtime"

	"github.com/lewtec/svgolf/pkg/svg"
	"golang.org/x/sync/errgroup"
)

// parallelEach runs fn across n items on GOMAXPROCS workers.
// One returned error stops the rest. n is a fan-out, not a quality cutoff.
func parallelEach(ctx context.Context, n int, fn func(context.Context, int) error) error {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return fn(ctx, 0)
	}
	group, ctx := errgroup.WithContext(ctx)
	limit := runtime.GOMAXPROCS(0)
	if limit < 1 {
		limit = 1
	}
	group.SetLimit(limit)
	for i := range n {
		group.Go(func() error {
			return fn(ctx, i)
		})
	}
	return group.Wait()
}

// Suggest submits one fill per path: the solid mean and each covariance axis.
type Suggest struct {
	world   *world
	buckets [][]pix
}

func (Suggest) ID() Op { return OpSuggest }

func (g Suggest) Applies() bool { return g.world != nil && g.world.paths > 0 }

func (g Suggest) Run(ctx context.Context) (formPick, error) {
	return firstPick(ctx, g.Stream)
}

func (g Suggest) Stream(ctx context.Context, emit func(formPick)) error {
	s := g.world
	buckets := g.buckets
	if buckets == nil {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	one := func(ctx context.Context, i int) error {
		if err := live(ctx); err != nil {
			return err
		}
		node := s.doc.Children()[i+1]
		path, ok := node.Path()
		if !ok {
			return nil
		}
		work := buckets[i]
		if len(work) == 0 {
			return nil
		}
		for _, choice := range paintChoices(work, s.want) {
			cand := path
			if choice.linear {
				cand = cand.WithLinearFill(choice.grad)
			} else {
				cand = cand.WithFill(choice.solid)
			}
			emit(formPick{
				doc: replaceAt(s.doc, i+1, cand.Node()), replace: i, work: work, fill: s.fills[i],
				dropIdx: -1, mergeJ: -1, insert: -1, op: OpSuggest,
				dirty: nodeRect(node),
			})
			if s.oneShot && s.gotNovel {
				return nil
			}
		}
		return nil
	}
	if s.oneShot {
		order := rand.Perm(s.paths)
		for _, i := range order {
			if s.gotNovel {
				return nil
			}
			if err := one(ctx, i); err != nil {
				return err
			}
		}
		return nil
	}
	if s.emitNow != nil && s.paths > 1 {
		return parallelEach(ctx, s.paths, one)
	}
	for i := 0; i < s.paths; i++ {
		if err := one(ctx, i); err != nil {
			return err
		}
	}
	return nil
}

// Squash welds one stitched pair into one path and tries each paint on the union.
// The ring is the stitch. A cover of the union would fill gaps between the paths.
type Squash struct {
	world   *world
	buckets [][]pix
}

func (Squash) ID() Op { return OpSquash }

func (q Squash) Applies() bool { return q.world != nil && q.world.paths >= 2 }

func (q Squash) Run(ctx context.Context) (formPick, error) {
	return firstPick(ctx, q.Stream)
}

func (q Squash) Stream(ctx context.Context, emit func(formPick)) error {
	s := q.world
	buckets := q.buckets
	if buckets == nil {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	type pair struct{ i, j int }
	var pairs []pair
	for i := 0; i < s.paths; i++ {
		for j := i + 1; j < s.paths; j++ {
			pairs = append(pairs, pair{i, j})
		}
	}
	one := func(ctx context.Context, k int) error {
		if err := live(ctx); err != nil {
			return err
		}
		i, j := pairs[k].i, pairs[k].j
		pa, oka := s.doc.Children()[i+1].Path()
		pb, okb := s.doc.Children()[j+1].Path()
		if !oka || !okb {
			return nil
		}
		ra, rb := parsePathRings(pa), parsePathRings(pb)
		if len(ra) == 0 || len(rb) == 0 {
			return nil
		}
		stitched, ok := stitchNearEdge(ra[0], rb[0])
		if !ok {
			return nil
		}
		work := append(append([]pix{}, buckets[i]...), buckets[j]...)
		dirty := nodeRect(s.doc.Children()[i+1]).Union(nodeRect(s.doc.Children()[j+1]))
		for _, choice := range paintChoices(work, s.want) {
			cand := filledRings(stitched, nil, choice.solid)
			if choice.linear {
				cand = cand.WithLinearFill(choice.grad)
			}
			next := dropAt(replaceAt(s.doc, i+1, cand.Node()), j+1)
			emit(formPick{
				doc: next, replace: i, mergeJ: j, work: work, fill: choice.solid,
				dropIdx: -1, insert: -1, op: OpSquash, dirty: dirty,
			})
			if s.oneShot && s.gotNovel {
				return nil
			}
		}
		return nil
	}
	if s.oneShot {
		rand.Shuffle(len(pairs), func(a, b int) { pairs[a], pairs[b] = pairs[b], pairs[a] })
		for k := range pairs {
			if s.gotNovel {
				return nil
			}
			if err := one(ctx, k); err != nil {
				return err
			}
		}
		return nil
	}
	if s.emitNow != nil && len(pairs) > 1 {
		return parallelEach(ctx, len(pairs), one)
	}
	for k := range pairs {
		if err := one(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

// Spline replaces one monotone run with one cubic, and also submits the
// farthest-point split of that run when both halves still have two edges.
type Spline struct {
	world   *world
	buckets [][]pix
}

func (Spline) ID() Op { return OpSpline }

func (sp Spline) Applies() bool { return sp.world != nil && sp.world.paths > 0 }

func (sp Spline) Run(ctx context.Context) (formPick, error) {
	return firstPick(ctx, sp.Stream)
}

func (sp Spline) Stream(ctx context.Context, emit func(formPick)) error {
	s := sp.world
	buckets := sp.buckets
	if buckets == nil {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	one := func(ctx context.Context, i int) error {
		if err := live(ctx); err != nil {
			return err
		}
		node := s.doc.Children()[i+1]
		path, ok := node.Path()
		if !ok {
			return nil
		}
		rings := parsePathRings(path)
		if len(rings) == 0 {
			return nil
		}
		work := buckets[i]
		for ri, ring := range rings {
			for _, span := range monotoneSpans(ring) {
				samples := spanPoints(ring.verts, span.from, span.to)
				cmds := spanCommands(samples)
				if len(cmds) == 0 {
					continue
				}
				sp.emitRing(emit, s, i, node, rings, ri, ring.replaceSpan(span.from, span.to, cmds), work)
				if s.oneShot && s.gotNovel {
					return nil
				}
				if far, ok := farthestOnSpan(ring.verts, span.from, span.to); ok {
					split := splitCommands(ring.verts, span.from, far, span.to)
					if len(split) == 2 {
						sp.emitRing(emit, s, i, node, rings, ri, ring.replaceSpan(span.from, span.to, split), work)
						if s.oneShot && s.gotNovel {
							return nil
						}
					}
				}
			}
		}
		return nil
	}
	if s.oneShot {
		order := rand.Perm(s.paths)
		for _, i := range order {
			if s.gotNovel {
				return nil
			}
			if err := one(ctx, i); err != nil {
				return err
			}
		}
		return nil
	}
	if s.emitNow != nil && s.paths > 1 {
		return parallelEach(ctx, s.paths, one)
	}
	for i := 0; i < s.paths; i++ {
		if err := one(ctx, i); err != nil {
			return err
		}
	}
	return nil
}

func (sp Spline) emitRing(emit func(formPick), s *world, i int, node svg.Node, rings []pathRing, ri int, next pathRing, work []pix) {
	if len(next.verts) < 3 || ringLoops(next) {
		return
	}
	outer := rings[0]
	holes := rings[1:]
	if ri == 0 {
		outer = next
	} else {
		holes = append([]pathRing{}, rings[1:]...)
		holes[ri-1] = next
	}
	cand := filledRings(outer, holes, s.fills[i])
	if lin, ok := node.LinearFill(); ok {
		cand = cand.WithLinearFill(lin)
	}
	emit(formPick{
		doc: replaceAt(s.doc, i+1, cand.Node()), replace: i, work: work, fill: s.fills[i],
		dropIdx: -1, mergeJ: -1, insert: -1, op: OpSpline, dirty: nodeRect(node),
	})
}

func firstPick(ctx context.Context, stream func(context.Context, func(formPick)) error) (formPick, error) {
	best := nonePick()
	err := stream(ctx, func(p formPick) {
		if !best.scored {
			best = p
		}
	})
	return best, err
}

func (s *world) emitKind(ctx context.Context, kind Op, lefts []leftover, emit func(formPick)) error {
	if err := live(ctx); err != nil {
		return err
	}
	buckets := fillBuckets(s.owner, s.w, s.paths, nil)
	switch kind {
	case OpSuggest:
		return Suggest{world: s, buckets: buckets}.Stream(ctx, emit)
	case OpSquash:
		return Squash{world: s, buckets: buckets}.Stream(ctx, emit)
	case OpSpline:
		return Spline{world: s, buckets: buckets}.Stream(ctx, emit)
	case OpWash, OpJoin, OpSubtract, OpSimplify, OpUnhole:
		return proposeOne(ctx, op{id: kind, world: s, buckets: buckets}, emit)
	case OpSwap:
		if s.paths < 2 {
			return nil
		}
		i := rand.IntN(s.paths)
		j := rand.IntN(s.paths - 1)
		if j >= i {
			j++
		}
		if i > j {
			i, j = j, i
		}
		return proposeOne(ctx, op{id: OpSwap, world: s, i: i, j: j}, emit)
	case OpDelete:
		if s.paths < 2 {
			return nil
		}
		return proposeOne(ctx, op{id: OpDelete, world: s, i: rand.IntN(s.paths)}, emit)
	default:
		for _, left := range lefts {
			if err := live(ctx); err != nil {
				return err
			}
			for _, oper := range s.leftoverOperators(left, 4) {
				if oper.ID() != kind {
					continue
				}
				if err := proposeOne(ctx, oper, emit); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func proposeOne(ctx context.Context, oper Operator, emit func(formPick)) error {
	if !oper.Applies() {
		return nil
	}
	if stream, ok := oper.(interface {
		Stream(context.Context, func(formPick)) error
	}); ok {
		return stream.Stream(ctx, emit)
	}
	if holder, ok := oper.(interface{ proposeWorld() *world }); ok {
		if w := holder.proposeWorld(); w != nil && w.stashOn {
			w.stash = w.stash[:0]
			if _, err := oper.Run(ctx); err != nil {
				return err
			}
			for _, p := range w.stash {
				emit(p)
			}
			w.stash = w.stash[:0]
			return nil
		}
	}
	pick, err := oper.Run(ctx)
	if err != nil {
		return err
	}
	if !pick.scored && pick.doc.Width() == 0 {
		return nil
	}
	emit(pick)
	return nil
}

func agentKinds() []Op {
	return []Op{
		OpTriangle, OpRectangle, OpRing, OpOutline,
		OpGrow, OpCarve, OpSlide, OpBend,
		OpSimplify, OpUnhole,
		OpWash, OpJoin, OpSubtract, OpSwap, OpDelete,
		OpSuggest, OpSquash, OpSpline,
	}
}
