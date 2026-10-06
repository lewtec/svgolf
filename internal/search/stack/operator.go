package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/lewtec/lewkit/x/taskgroup"
	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
	"golang.org/x/sync/errgroup"
)

// Operator is one edit. choose starts every applicable Operator
// in the current neighborhood and waits once.
type Operator interface {
	ID() Op
	Applies() bool
	Run() (formPick, error)
}

type Op = search.Op

const (
	OpNone      = search.OpNone
	OpAbsorb    = search.OpAbsorb
	OpTriangle  = search.OpTriangle
	OpRing      = search.OpRing
	OpRectangle = search.OpRectangle
	OpGrow      = search.OpGrow
	OpCarve     = search.OpCarve
	OpSlide     = search.OpSlide
	OpBend      = search.OpBend
	OpSimplify  = search.OpSimplify
	OpWash      = search.OpWash
	OpJoin      = search.OpJoin
	OpSubtract  = search.OpSubtract
	OpSwap      = search.OpSwap
	OpDelete    = search.OpDelete
	OpUnhole    = search.OpUnhole
	OpOutline   = search.OpOutline
	opCount     = search.OpCount
)

type op struct {
	id      Op
	world   *world
	left    leftover
	buckets [][]pix
	i, j    int
}

func (o op) ID() Op { return o.id }

func (o op) impl() Operator {
	switch o.id {
	case OpAbsorb:
		return &Absorb{world: o.world, left: o.left}
	case OpTriangle:
		return Triangle{world: o.world, left: o.left}
	case OpRing:
		return Ring{world: o.world, left: o.left}
	case OpRectangle:
		return Rectangle{world: o.world, left: o.left}
	case OpGrow:
		return &Grow{world: o.world, left: o.left}
	case OpCarve:
		return &Carve{world: o.world, left: o.left}
	case OpSlide:
		return Slide{world: o.world, left: o.left}
	case OpBend:
		return Bend{world: o.world, left: o.left}
	case OpSimplify:
		return Simplify{world: o.world, buckets: o.buckets}
	case OpWash:
		return Wash{world: o.world, buckets: o.buckets}
	case OpJoin:
		return &Join{world: o.world, buckets: o.buckets}
	case OpSubtract:
		return Subtract{world: o.world, buckets: o.buckets}
	case OpSwap:
		return Swap{world: o.world, i: o.i, j: o.j}
	case OpDelete:
		return Delete{world: o.world, i: o.i}
	case OpUnhole:
		return Unhole{world: o.world, buckets: o.buckets}
	case OpOutline:
		return Outline{world: o.world, left: o.left}
	default:
		return nil
	}
}

func (o op) Applies() bool {
	im := o.impl()
	if im == nil {
		return false
	}
	return im.Applies()
}

func (o op) Run() (formPick, error) {
	im := o.impl()
	if im == nil {
		return nonePick(), nil
	}
	return im.Run()
}

// Triangle places one leftover ear.
type Triangle struct {
	world *world
	left  leftover
}

func (Triangle) ID() Op { return OpTriangle }
func (tr Triangle) Applies() bool {
	return tr.left.big() && !tr.left.region && hasInterior(tr.left.island) && tr.world.paths < maxPaths
}

func (tr Triangle) Run() (formPick, error) {
	s, g := tr.world, tr.left.fresh
	if len(g.work) < 3 {
		return nonePick(), nil
	}
	ring := oneMaskTriangle(g.work)
	if len(ring) < 3 {
		return nonePick(), nil
	}
	g.fill = modeFill(s.want, g.work)
	g.work = trianglePix(ring)
	if len(g.work) == 0 {
		return nonePick(), nil
	}
	g.ring = ring
	g = s.seedGrow(g)
	return s.addLayer(filledPath(ring, g.fill), g, OpTriangle)
}

// Rectangle places one leftover plate from the four strongest
// mask vertices.
type Rectangle struct {
	world *world
	left  leftover
}

func (Rectangle) ID() Op { return OpRectangle }
func (rc Rectangle) Applies() bool {
	return rc.left.big() && !rc.left.region && hasInterior(rc.left.island) && rc.world.paths < maxPaths
}

func (rc Rectangle) Run() (formPick, error) {
	s, g := rc.world, rc.left.fresh
	if len(g.work) < 4 {
		return nonePick(), nil
	}
	ring := oneMaskRectangle(g.work)
	if len(ring) < 4 {
		return nonePick(), nil
	}
	g.fill = modeFill(s.want, g.work)
	g.work = rectanglePix(ring)
	if len(g.work) == 0 {
		return nonePick(), nil
	}
	g.ring = ring
	g = s.seedGrow(g)
	return s.addLayer(filledPath(ring, g.fill), g, OpRectangle)
}

// Outline paints one leftover mask. The color-region flood is
// included: despeckle drops axis tips off a disk, and the flood
// still has them. Triangle and rectangle stay in the neighborhood
// and win when they cover that mask with fewer commands. A
// shorter curve that changes pixels loses on Score. Enclosed
// voids stay solid; carve opens them after the plate exists.
type Outline struct {
	world *world
	left  leftover
}

func (Outline) ID() Op { return OpOutline }
func (o Outline) Applies() bool {
	// Interior is not required. A trace or a fringe has no pixel
	// with four neighbors, and skipping it leaves a real miss.
	// Score still rejects a rim whose plate does not help.
	return o.left.big() && o.world.paths < maxPaths
}

func (o Outline) Run() (formPick, error) {
	s := o.world
	g := o.left.fresh
	if len(g.work) < 3 {
		return nonePick(), nil
	}
	ring := coverRing(g.work)
	if len(ring) < 3 {
		return nonePick(), nil
	}
	g.fill = modeFill(s.want, g.work)
	g.ring = ring
	g = s.seedGrow(g)
	layer, err := s.addLayer(filledPath(ring, g.fill), g, OpOutline)
	if err != nil {
		return nonePick(), err
	}
	if s.paths == 0 || (!o.left.paper && !paperLeftover(g.fill)) {
		return layer, nil
	}
	// The pane already is this color. Punching the same outline out
	// of the plate matches the stacked white path and stays one shape.
	hole, err := (&Carve{world: s, left: o.left}).Run()
	if err != nil {
		return nonePick(), err
	}
	// scoreAfter can leave a fraction of a pixel between two
	// rasters of the same outline. Lex would then keep the extra
	// path. The full sum is the same Score the archive stores.
	if s.want != nil && hole.scored && layer.scored {
		if sum, err := scored(s.ctx, hole.doc, s.want); err == nil {
			hole.errSum = sum
			hole.ok = acceptLexicographic(sum, hole.paths, hole.commands, s.errSum, s.paths, docCmdLen(s.doc))
		}
		if sum, err := scored(s.ctx, layer.doc, s.want); err == nil {
			layer.errSum = sum
			layer.ok = acceptLexicographic(sum, layer.paths, layer.commands, s.errSum, s.paths, docCmdLen(s.doc))
		}
	}
	if betterPick(hole, layer) {
		return hole, nil
	}
	return layer, nil
}

func scored(ctx context.Context, doc svg.Document, want *image.NRGBA) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return 0, err
	}
	return Score(ctx, got, want), nil
}

// Ring places a leftover that already surrounds painted pixels.
type Ring struct {
	world *world
	left  leftover
}

func (Ring) ID() Op { return OpRing }
func (r Ring) Applies() bool {
	if !r.left.big() || r.left.paper || r.world.paths >= maxPaths {
		return false
	}
	return len(leftoverRings(r.left.island, r.world.got, r.world.want, r.left.col)) > 0
}

func (r Ring) Run() (formPick, error) {
	s := r.world
	holes := leftoverRings(r.left.island, s.got, s.want, r.left.col)
	if len(holes) == 0 {
		return nonePick(), nil
	}
	g := r.left.fresh
	g.ring = coverRing(g.work)
	if len(g.ring) < 3 {
		return nonePick(), nil
	}
	return s.addLayer(withHoles(filledPath(g.ring, g.fill), holes), g, OpRing)
}

// Absorb paints leftover into a touching path as a 2-stop linear.
// Not scheduled: leftover is another ear or Wash.
type Absorb struct {
	world   *world
	left    leftover
	scratch scratch
}

func (Absorb) ID() Op { return OpAbsorb }
func (a Absorb) Applies() bool {
	return a.left.big() && !a.left.paper && a.world.paths > 0
}

func (a *Absorb) Run() (formPick, error) {
	s := a.world
	works := a.left.grows
	if works == nil {
		works = s.connecting(a.left.island, a.scratch.seen)
	}
	best := nonePick()
	for _, g := range works {
		if !sameRampFamily(g.fill, a.left.col) {
			continue
		}
		grad, ok := fitLinearStops(g.work, s.want)
		if !ok {
			continue
		}
		node := s.doc.Children()[g.i+1]
		p, ok := node.Path()
		if !ok {
			continue
		}
		cand := p.WithLinearFill(grad)
		pick, err := s.scoreCand(replaceAt(s.doc, g.i+1, cand.Node()), cand.Node(), g, OpAbsorb)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Grow expands a touching path over leftover.
type Grow struct {
	world   *world
	left    leftover
	scratch scratch
}

func (g Grow) ID() Op { return OpGrow }
func (g Grow) Applies() bool {
	return g.left.big() && !g.left.paper
}

func (g *Grow) Run() (formPick, error) {
	s := g.world
	works := g.left.grows
	if works == nil {
		works = s.connecting(g.left.island, g.scratch.seen)
	}
	best := nonePick()
	for _, work := range works {
		ring := hullRing(work.work)
		if len(ring) < 3 {
			continue
		}
		work.ring = ring
		cand := filledPath(ring, work.fill)
		if lin, ok := s.doc.Children()[work.i+1].LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		pick, err := s.scoreCand(replaceAt(s.doc, work.i+1, cand.Node()), cand.Node(), work, OpGrow)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Carve removes leftover from a covering path.
type Carve struct {
	world   *world
	left    leftover
	scratch scratch
}

func (c Carve) ID() Op { return OpCarve }
func (c Carve) Applies() bool {
	return c.left.big() && c.world.paths > 0
}

func (c *Carve) Run() (formPick, error) {
	s := c.world
	c.scratch.ensure(s.w * s.h)
	hole := c.left.fresh.ring
	if len(hole) < 3 {
		hole = hullRing(c.left.island)
	}
	if len(hole) < 3 {
		return nonePick(), nil
	}
	if c.left.paper {
		return c.paper(hole)
	}
	best := nonePick()
	for i := 0; i < s.paths; i++ {
		node := s.doc.Children()[i+1]
		if !ownsAny(s.owner, c.left.island, s.w, uint16(i+1)) && !s.paintsIsland(node, c.left.island) {
			continue
		}
		p, ok := node.Path()
		if !ok {
			continue
		}
		cand := withHoles(p, [][][2]float64{hole})
		work := ownedMinus(s.owner, c.left.island, s.w, uint16(i+1), c.scratch.seen)
		dirty0 := islandRect(c.left.island).Union(nodeRect(node))
		gr := grow{i: i, work: work, fill: s.fills[i], dirty0: dirty0}
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), gr, OpCarve)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok && pick.errSum > s.errSum {
			pick.ok = false
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

func (c *Carve) paper(_ [][2]float64) (formPick, error) {
	s := c.world
	next := s.doc
	reclaims := make([][]pix, s.paths)
	any := false
	dirty0 := islandRect(c.left.island)
	var last svg.Node
	for i := 0; i < s.paths; i++ {
		node := s.doc.Children()[i+1]
		if !ownsAny(s.owner, c.left.island, s.w, uint16(i+1)) && !s.paintsIsland(node, c.left.island) {
			continue
		}
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := parsePathRings(p)
		if len(rings) == 0 {
			continue
		}
		owned := ownerBucket(s.owner, s.w, uint16(i+1))
		var cand svg.Path
		if leftoverIsHole(owned, c.left.island) {
			// The mask outline, not the convex hull. The hull of an
			// L fills the missing corner, Score rejects the spill,
			// and the letter stays a second path.
			hole := coverRing(c.left.island)
			if len(hole) < 3 {
				continue
			}
			cand = withHoles(p, [][][2]float64{hole})
		} else {
			outer := shrinkOuterRing(rings[0], c.left.island)
			if len(outer.verts) < 3 {
				continue
			}
			cand = filledRings(outer, rings[1:], s.fills[i])
			if lin, ok := node.LinearFill(); ok {
				cand = cand.WithLinearFill(lin)
			}
		}
		next = replaceAt(next, i+1, cand.Node())
		last = cand.Node()
		reclaims[i] = ownedMinus(s.owner, c.left.island, s.w, uint16(i+1), c.scratch.seen)
		dirty0 = dirty0.Union(nodeRect(node))
		any = true
	}
	if !any {
		return nonePick(), nil
	}
	gr := grow{i: -1, work: c.left.island, fill: c.left.col, dirty0: dirty0}
	pick, err := s.scoreCand(next, last, gr, OpCarve)
	if err != nil {
		return nonePick(), err
	}
	if pick.ok && pick.errSum > s.errSum {
		pick.ok = false
	}
	if pick.ok {
		pick.reclaims = reclaims
		pick.replace = -1
	}
	return pick, nil
}

// Simplify drops one vertex that already lies on the line
// through its neighbors. A mask outline is a staircase, and
// rewriting that whole ring at once is one shape. Each epoch
// takes the next straight vertex, Score keeps the step only
// when the picture does not get worse, and the run shrinks
// until a corner is left. A bend is not straight, so the
// corner stays. One walk, no search over pairs of vertices.
type Simplify struct {
	world   *world
	buckets [][]pix
}

func (s Simplify) ID() Op { return OpSimplify }
func (s Simplify) Applies() bool {
	return s.world.paths > 0
}

func (s Simplify) Run() (formPick, error) {
	w := s.world
	missed := cloneStraightMiss(w.simplifyMiss)
	for i := 0; i < w.paths; i++ {
		node := w.doc.Children()[i+1]
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := parsePathRings(p)
		lin, hasLin := node.LinearFill()
		for ri, ring := range rings {
			n := len(ring.verts)
			for v := 0; v < n; v++ {
				if !ring.straightVertex(v) {
					continue
				}
				key := ringStraightKey(ring, v)
				if _, seen := missed[key]; seen {
					continue
				}
				moved := ring.dropVertex(v)
				if len(moved.verts) < 3 || ringCrosses(moved.points()) {
					continue
				}
				next := append([]pathRing{}, rings...)
				next[ri] = moved
				cand := filledRings(next[0], next[1:], w.fills[i])
				if hasLin {
					cand = cand.WithLinearFill(lin)
				}
				g := w.seedGrow(grow{i: i, work: s.buckets[i], fill: w.fills[i]})
				// seedGrow dirties the whole path. One straight
				// drop only repaints this triangle; scoreCand
				// insets the rect.
				g.dirty0 = key.scoreRect()
				pick, err := w.scoreCand(replaceAt(w.doc, i+1, cand.Node()), cand.Node(), g, OpSimplify)
				if err != nil {
					return nonePick(), err
				}
				if !pick.scored {
					continue
				}
				if pick.ok && pick.errSum > w.errSum {
					pick.ok = false
				}
				if pick.ok {
					forgetTouching(missed, key)
					pick.simplifyMiss = missed
					return pick, nil
				}
				if missed == nil {
					missed = map[straightKey]struct{}{}
				}
				missed[key] = struct{}{}
			}
		}
	}
	out := nonePick()
	out.simplifyMiss = missed
	return out, nil
}

// Unhole drops one evenodd hole.
type Unhole struct {
	world   *world
	buckets [][]pix
}

func (Unhole) ID() Op { return OpUnhole }
func (u Unhole) Applies() bool {
	return u.world.paths > 0
}

func (u Unhole) Run() (formPick, error) {
	w := u.world
	best := nonePick()
	for i := 0; i < w.paths; i++ {
		node := w.doc.Children()[i+1]
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := parsePathRings(p)
		if len(rings) < 2 {
			continue
		}
		g := w.seedGrow(grow{i: i, work: u.buckets[i], fill: w.fills[i]})
		lin, hasLin := node.LinearFill()
		for h := 1; h < len(rings); h++ {
			if !regionWorthTrying(rings[h].points(), w.gotP, w.wantP) {
				continue
			}
			keep := append([]pathRing{}, rings[1:h]...)
			keep = append(keep, rings[h+1:]...)
			cand := filledRings(rings[0], keep, w.fills[i])
			if hasLin {
				cand = cand.WithLinearFill(lin)
			}
			lg := g
			lg.dirty0 = pointsRect(rings[h].points())
			pick, err := w.scoreCand(replaceAt(w.doc, i+1, cand.Node()), cand.Node(), lg, OpUnhole)
			if err != nil {
				return nonePick(), err
			}
			if pick.ok && pick.errSum > w.errSum {
				pick.ok = false
			}
			if betterPick(pick, best) {
				best = pick
			}
		}
	}
	return best, nil
}

// Wash paints a 2-stop linear on one path.
type Wash struct {
	world   *world
	buckets [][]pix
}

func (Wash) ID() Op { return OpWash }
func (w Wash) Applies() bool {
	return w.world.paths > 0
}

func (w Wash) Run() (formPick, error) {
	s := w.world
	best := nonePick()
	for i := 0; i < s.paths; i++ {
		work := w.buckets[i]
		grad, ok := fitLinearStops(work, s.want)
		if !ok {
			continue
		}
		p, ok := s.doc.Children()[i+1].Path()
		if !ok {
			continue
		}
		cand := p.WithLinearFill(grad)
		g := s.seedGrow(grow{i: i, work: work, fill: s.fills[i]})
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, OpWash)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Join welds two touching same-family paths.
type Join struct {
	world   *world
	buckets [][]pix
	scratch scratch
}

func (Join) ID() Op { return OpJoin }
func (j Join) Applies() bool {
	return j.world.paths >= 2
}

func (j *Join) Run() (formPick, error) {
	s := j.world
	if s.paths < 2 {
		return nonePick(), nil
	}
	i := rand.IntN(s.paths)
	jn := rand.IntN(s.paths - 1)
	if jn >= i {
		jn++
	}
	if i > jn {
		i, jn = jn, i
	}
	if !sameRampFamily(s.fills[i], s.fills[jn]) {
		return nonePick(), nil
	}
	j.scratch.work = j.scratch.work[:0]
	j.scratch.work = append(j.scratch.work, j.buckets[i]...)
	j.scratch.work = append(j.scratch.work, j.buckets[jn]...)
	work := append([]pix{}, j.scratch.work...)
	nodeI := s.doc.Children()[i+1]
	nodeJ := s.doc.Children()[jn+1]
	pa, oka := nodeI.Path()
	pb, okb := nodeJ.Path()
	if oka && okb {
		ra, rb := parsePathRings(pa), parsePathRings(pb)
		if len(ra) > 0 && len(rb) > 0 {
			if stitched, ok := stitchNearEdge(ra[0], rb[0]); ok {
				return j.scoreJoin(s, i, jn, work, stitched, nil)
			}
		}
	}
	outers := pathOuters(s.doc, s.paths)
	if !ringsNear(outers[i], outers[jn]) && !bucketsTouch(j.buckets[i], j.buckets[jn]) {
		return nonePick(), nil
	}
	if !oneBlob(work) {
		return nonePick(), nil
	}
	ring := coverRing(work)
	if len(ring) < 3 {
		return nonePick(), nil
	}
	return j.scoreJoin(s, i, jn, work, polylineRing(ring), nil)
}

func (j *Join) scoreJoin(s *world, i, jn int, work []pix, outer pathRing, holes []pathRing) (formPick, error) {
	best := nonePick()
	fills := []color.NRGBA{s.fills[i], s.fills[jn]}
	if fills[0] == fills[1] {
		fills = fills[:1]
	}
	node := s.doc.Children()[i+1]
	lin, hasLin := node.LinearFill()
	if !hasLin {
		if l, ok := s.doc.Children()[jn+1].LinearFill(); ok {
			lin, hasLin = l, true
		}
	}
	for _, fill := range fills {
		g := s.seedGrow(grow{i: i, work: work, fill: fill, ring: outer.points()})
		g.dirty0 = g.dirty0.Union(nodeRect(s.doc.Children()[jn+1]))
		cand := filledRings(outer, holes, fill)
		if hasLin {
			cand = cand.WithLinearFill(lin)
		}
		next := replaceAt(s.doc, i+1, cand.Node())
		next = dropAt(next, jn+1)
		pick, err := s.scoreCand(next, cand.Node(), g, OpJoin)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok {
			pick.mergeJ = jn
			pick.work = work
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

func bucketsTouch(a, b []pix) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := pixSet(b)
	defer releaseBits(set)
	dirs := [5]pix{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, p := range a {
		for _, d := range dirs {
			if set.has(pix{p.x + d.x, p.y + d.y}) {
				return true
			}
		}
	}
	return false
}

// Subtract cuts one path out of another.
type Subtract struct {
	world   *world
	buckets [][]pix
}

func (Subtract) ID() Op { return OpSubtract }
func (s Subtract) Applies() bool {
	return s.world.paths >= 2
}

func (s Subtract) Run() (formPick, error) {
	w := s.world
	if w.paths < 2 {
		return nonePick(), nil
	}
	i := rand.IntN(w.paths)
	j := rand.IntN(w.paths - 1)
	if j >= i {
		j++
	}
	outers := pathOuters(w.doc, w.paths)
	if !ringsOverlap(outers[i], outers[j]) || len(outers[i]) < 3 || len(outers[j]) < 3 {
		return nonePick(), nil
	}
	node := w.doc.Children()[j+1]
	bounds := nodeRect(node).Intersect(image.Rect(0, 0, w.w, w.h))
	rem := ringSubtract(outers[j], outers[i], bounds)
	if !hasInterior(rem) {
		return nonePick(), nil
	}
	var rings [][][2]float64
	if h := coverRing(rem); len(h) >= 3 {
		rings = append(rings, h)
	}
	overlap := ringAnd(outers[j], outers[i], bounds)
	if shrunk := shrinkOuter(outers[j], overlap); len(shrunk) >= 3 {
		rings = append(rings, shrunk)
	}
	best := nonePick()
	dropCutter := paperLeftover(w.fills[i])
	for _, ring := range rings {
		cand := filledPath(ring, w.fills[j])
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		g := w.seedGrow(grow{i: j, work: rem, fill: w.fills[j], ring: ring})
		next := replaceAt(w.doc, j+1, cand.Node())
		if dropCutter {
			next = dropAt(next, i+1)
		}
		pick, err := w.scoreCand(next, cand.Node(), g, OpSubtract)
		if err != nil {
			return nonePick(), err
		}
		if dropCutter {
			pick.dropIdx = i
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Delete removes one path.
type Delete struct {
	world *world
	i     int
}

func (Delete) ID() Op { return OpDelete }
func (d Delete) Applies() bool {
	return d.world.paths >= 2 && d.i >= 0 && d.i < d.world.paths
}

func (d Delete) Run() (formPick, error) {
	s := d.world
	next := dropAt(s.doc, d.i+1)
	ngot, err := render.Scratch(next)
	if err != nil {
		return nonePick(), err
	}
	defer render.Release(ngot)
	gotP := loss.Acquire(ngot)
	dirty := nodeRect(s.doc.Children()[d.i+1]).Inset(-2)
	nerr := s.scoreAfter(gotP, dirty)
	loss.Release(gotP)
	npaths := s.paths - 1
	ncmds := docCmdLen(next)
	ok := acceptLexicographic(nerr, npaths, ncmds, s.errSum, s.paths, docCmdLen(s.doc)) && nerr <= s.errSum
	return formPick{doc: next, errSum: nerr, paths: npaths, commands: ncmds, replace: -1, insert: -1, dropIdx: d.i, mergeJ: -1, op: OpDelete, ok: ok, scored: true}, nil
}

// Slide moves one vertex toward leftover.
type Slide struct {
	world *world
	left  leftover
}

func (Slide) ID() Op { return OpSlide }
func (sl Slide) Applies() bool {
	return sl.left.big() && sl.world.paths > 0
}

func (sl Slide) Run() (formPick, error) {
	s := sl.world
	target := outline(sl.left.island)
	if len(target) < 1 {
		return nonePick(), nil
	}
	hot := islandRect(sl.left.island)
	type candPath struct {
		i     int
		outer pathRing
		holes []pathRing
	}
	var hits []candPath
	for i := 0; i < s.paths; i++ {
		node := s.doc.Children()[i+1]
		if !nodeRect(node).Overlaps(hot) {
			continue
		}
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := parsePathRings(p)
		if len(rings) == 0 || len(rings[0].verts) < 3 {
			continue
		}
		hits = append(hits, candPath{i: i, outer: rings[0], holes: rings[1:]})
	}
	if len(hits) == 0 {
		return nonePick(), nil
	}
	hit := hits[rand.IntN(len(hits))]
	vi := rand.IntN(len(hit.outer.verts))
	pull := nearest(target, hit.outer.verts[vi])
	if pull[0] == hit.outer.verts[vi][0] && pull[1] == hit.outer.verts[vi][1] {
		pull = leftoverCenter(sl.left.island)
	}
	if pull[0] == hit.outer.verts[vi][0] && pull[1] == hit.outer.verts[vi][1] {
		return nonePick(), nil
	}
	moved := hit.outer.moveVertex(vi, pull)
	if len(moved.verts) < 3 {
		return nonePick(), nil
	}
	node := s.doc.Children()[hit.i+1]
	cand := filledRings(moved, hit.holes, s.fills[hit.i])
	if lin, ok := node.LinearFill(); ok {
		cand = cand.WithLinearFill(lin)
	}
	g := s.seedGrow(grow{i: hit.i, work: sl.left.island, fill: s.fills[hit.i]})
	pick, err := s.scoreCand(replaceAt(s.doc, hit.i+1, cand.Node()), cand.Node(), g, OpSlide)
	if err != nil {
		return nonePick(), err
	}
	return pick, nil
}

// Bend turns one leftover-facing edge into a cubic.
type Bend struct {
	world *world
	left  leftover
}

func (Bend) ID() Op { return OpBend }
func (b Bend) Applies() bool {
	return b.left.big() && b.world.paths > 0
}

func (b Bend) Run() (formPick, error) {
	s := b.world
	shape := b.left.glow
	if len(shape) == 0 {
		shape = b.left.island
	}
	target := outline(shape)
	if len(target) < 1 {
		return nonePick(), nil
	}
	best := nonePick()
	hot := islandRect(b.left.island)
	for i := 0; i < s.paths; i++ {
		node := s.doc.Children()[i+1]
		if !nodeRect(node).Overlaps(hot) {
			continue
		}
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := parsePathRings(p)
		if len(rings) == 0 || len(rings[0].verts) < 3 {
			continue
		}
		outer := rings[0]
		n := len(outer.verts)
		if n < 3 {
			continue
		}
		ei, bestD := -1, -1.0
		var pull [2]float64
		for e := 0; e < n; e++ {
			a, c := outer.verts[e], outer.verts[(e+1)%n]
			mid := [2]float64{(a[0] + c[0]) / 2, (a[1] + c[1]) / 2}
			near := nearest(target, mid)
			if near[0] == mid[0] && near[1] == mid[1] {
				continue
			}
			d := (mid[0]-near[0])*(mid[0]-near[0]) + (mid[1]-near[1])*(mid[1]-near[1])
			if bestD < 0 || d < bestD {
				ei, bestD, pull = e, d, near
			}
		}
		if ei < 0 {
			continue
		}
		a, c := outer.verts[ei], outer.verts[(ei+1)%n]
		c1 := [2]float64{(a[0] + pull[0]) / 2, (a[1] + pull[1]) / 2}
		c2 := [2]float64{(c[0] + pull[0]) / 2, (c[1] + pull[1]) / 2}
		cmd := svg.PathCmd{Kind: svg.CmdCubic, X1: c1[0], Y1: c1[1], X2: c2[0], Y2: c2[1], X: c[0], Y: c[1]}
		bent := outer.setEdge(ei, cmd)
		cand := filledRings(bent, rings[1:], s.fills[i])
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		g := s.seedGrow(grow{i: i, work: b.left.island, fill: s.fills[i]})
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, OpBend)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Swap exchanges two paths.
type Swap struct {
	world *world
	i, j  int
}

func (Swap) ID() Op { return OpSwap }
func (sw Swap) Applies() bool {
	return sw.i >= 0 && sw.j > sw.i && sw.j < sw.world.paths
}

func (sw Swap) Run() (formPick, error) {
	s := sw.world
	next, fills, owner, ok := s.swapPaths(sw.i, sw.j)
	if !ok {
		return nonePick(), nil
	}
	ngot, err := render.Scratch(next)
	if err != nil {
		return nonePick(), err
	}
	defer render.Release(ngot)
	gotP := loss.Acquire(ngot)
	dirty := nodeRect(s.doc.Children()[sw.i+1]).Union(nodeRect(s.doc.Children()[sw.j+1])).Inset(-2)
	nerr := s.scoreAfter(gotP, dirty)
	loss.Release(gotP)
	npaths := s.paths
	ncmds := docCmdLen(next)
	ok = acceptLexicographic(nerr, npaths, ncmds, s.errSum, s.paths, docCmdLen(s.doc))
	return formPick{
		doc: next, errSum: nerr, paths: npaths, commands: ncmds,
		replace: -1, insert: -1, dropIdx: -1, mergeJ: -1,
		op: OpSwap, ok: ok, scored: true,
		fills: fills, owner: owner,
	}, nil
}

// onBlend reports that c is a mix of two paints, not a third flat.
// The match is minErr, the residual cutoff the search already uses
// to decide that a pixel is a paint. ColorAt jumps to 180 when only
// one side still has hue, so a channel gap of that same size counts
// too. An endpoint is the paint itself and is not a mix.
func onBlend(c color.NRGBA, paints []color.NRGBA) bool {
	for _, paint := range paints {
		if loss.ColorAt(c, paint) <= float64(minErr) {
			return false
		}
	}
	limit := minErr * 255 / 180
	for i := range paints {
		for j := i + 1; j < len(paints); j++ {
			t, blend := projectBlend(paints[i], paints[j], c)
			if t <= 0 || t >= 1 {
				continue
			}
			if loss.ColorAt(c, blend) <= float64(minErr) || channelGap(c, blend) <= limit {
				return true
			}
		}
	}
	return false
}

func projectBlend(a, b, c color.NRGBA) (float64, color.NRGBA) {
	ar, ag, ab := float64(a.R), float64(a.G), float64(a.B)
	dx := float64(b.R) - ar
	dy := float64(b.G) - ag
	dz := float64(b.B) - ab
	den := dx*dx + dy*dy + dz*dz
	if den == 0 {
		return 0, a
	}
	t := ((float64(c.R)-ar)*dx + (float64(c.G)-ag)*dy + (float64(c.B)-ab)*dz) / den
	return t, color.NRGBA{
		R: roundChannel(ar + t*dx),
		G: roundChannel(ag + t*dy),
		B: roundChannel(ab + t*dz),
		A: 255,
	}
}

func roundChannel(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func channelGap(a, b color.NRGBA) int {
	gap := channelAbs(a.R, b.R)
	if n := channelAbs(a.G, b.G); n > gap {
		gap = n
	}
	if n := channelAbs(a.B, b.B); n > gap {
		gap = n
	}
	return gap
}

func channelAbs(a, b uint8) int {
	n := int(a) - int(b)
	if n < 0 {
		return -n
	}
	return n
}

// seam is a leftover whose color is only the blend between paints
// already in the document. The letter's soft edge is that blend.
// A flat that merely sits near a mix is not a seam: onBlend rejects
// endpoints and colors off the segment.
// frontierHits counts want pixels just outside the island that already
// match each paint. Paper is paints[len-1] when the caller appended it.
func (s *world) frontierHits(island []pix, paints []color.NRGBA) []int {
	hit := make([]int, len(paints))
	if s.want == nil || len(island) == 0 {
		return hit
	}
	set := pixSet(island)
	defer releaseBits(set)
	w, h := s.worldSize()
	ox, oy := s.want.Rect.Min.X, s.want.Rect.Min.Y
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, p := range island {
		for _, d := range dirs {
			nx, ny := p.x+d.x, p.y+d.y
			if uint(nx) >= uint(w) || uint(ny) >= uint(h) || set.has(pix{nx, ny}) {
				continue
			}
			c := s.want.NRGBAAt(ox+nx, oy+ny)
			for i, paint := range paints {
				if loss.ColorAt(c, paint) <= float64(minErr) {
					hit[i]++
				}
			}
		}
	}
	return hit
}

// seam is a leftover whose color is only the blend between paints
// already in the document. An exact mix is a seam anywhere. A JPEG
// blur sits a little off that line, so a mix that meets the pane (the
// letter, or the empty field) still counts when it is closer to the
// segment than to either paint. A ramp that never meets the pane is
// not that blur: its bands stay plates and Wash can fit them.
func (s *world) seam(left leftover) bool {
	if s == nil || s.paths == 0 || len(left.island) < minIsland {
		return false
	}
	mode := left.col
	if mode.A == 0 {
		mode = modeFill(s.want, left.island)
	}
	paints := make([]color.NRGBA, 0, len(s.fills)+1)
	paints = append(paints, s.fills...)
	paints = append(paints, paper)
	if onBlend(mode, paints) {
		return true
	}
	hit := s.frontierHits(left.island, paints)
	if hit[len(hit)-1] == 0 {
		return false
	}
	return closerBlend(mode, s.fills, paper)
}

// closerBlend reports that c lies on the open segment from a plate to
// the pane and the mix beats either end. The 180 from the hue knee is
// not a measured miss, so it does not count as "closer".
func closerBlend(c color.NRGBA, fills []color.NRGBA, pane color.NRGBA) bool {
	if loss.ColorAt(c, pane) <= float64(minErr) {
		return false
	}
	for _, fill := range fills {
		if loss.ColorAt(c, fill) <= float64(minErr) {
			return false
		}
		t, blend := projectBlend(fill, pane, c)
		if t <= 0 || t >= 1 {
			continue
		}
		along := loss.ColorAt(c, blend)
		if along < 180 && along < loss.ColorAt(c, fill) && along < loss.ColorAt(c, pane) {
			return true
		}
	}
	return false
}

func leftoverAddOperators(s *world, left leftover) []Operator {
	if left.paper && s.paths > 0 {
		// The pane is already this color. A white rectangle on a
		// short sibling survives the archive, and the letter
		// outline on that sibling then beats the hole. Revealing
		// the pane is a carve, not another path.
		return []Operator{op{id: OpCarve, world: s, left: left}}
	}
	return []Operator{
		op{id: OpTriangle, world: s, left: left},
		op{id: OpRectangle, world: s, left: left},
		op{id: OpRing, world: s, left: left},
		op{id: OpOutline, world: s, left: left},
	}
}

func (s *world) leftoverOperators(left leftover, band int) []Operator {
	if s.seam(left) {
		// The leftover color is a mix of two paints already on the
		// document. A new plate would trace that seam, and a soft
		// letter would become a stack of rims. Slide and carve can
		// still seat the edge. Score rejects a move that changes
		// pixels for the worse.
		if band != 4 {
			return nil
		}
		return []Operator{
			op{id: OpSlide, world: s, left: left},
			op{id: OpBend, world: s, left: left},
			op{id: OpCarve, world: s, left: left},
		}
	}
	var add []Operator
	if !left.region && left.big() {
		add = leftoverAddOperators(s, left)
	}
	if left.deltaHi > 0 && left.deltaLo < 64 {
		if left.deltaHi <= 16 {
			switch band {
			case 1, 2, 3, 4:
				return add
			default:
				return nil
			}
		}
		switch band {
		case 1, 2, 3:
			return add
		case 4:
			return append(add,
				op{id: OpSlide, world: s, left: left},
				op{id: OpBend, world: s, left: left},
			)
		default:
			return nil
		}
	}
	if len(add) == 0 {
		add = leftoverAddOperators(s, left)
	}
	// A color-region leftover is a cover hypothesis. Slide/bend
	// stay on the residual miss so they are not pulled onto the
	// already-painted plate.
	if left.region {
		switch band {
		case 1, 3:
			return add
		case 2:
			return add
		case 4:
			return append(add,
				op{id: OpGrow, world: s, left: left},
				op{id: OpCarve, world: s, left: left},
			)
		default:
			return nil
		}
	}
	switch band {
	case 1, 2, 3:
		return add
	case 4:
		return append(add,
			op{id: OpSlide, world: s, left: left},
			op{id: OpBend, world: s, left: left},
			op{id: OpGrow, world: s, left: left},
			op{id: OpCarve, world: s, left: left},
		)
	default:
		return nil
	}
}

func (s *world) worldOperators(band int) []Operator {
	if band != 2 && band != polishBand {
		return nil
	}
	var buckets [][]pix
	if s.paths > 0 {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	if band == polishBand {
		return []Operator{
			op{id: OpSimplify, world: s, buckets: buckets},
			op{id: OpUnhole, world: s, buckets: buckets},
		}
	}
	ops := []Operator{
		op{id: OpWash, world: s, buckets: buckets},
		op{id: OpJoin, world: s, buckets: buckets},
		op{id: OpSubtract, world: s, buckets: buckets},
	}
	if s.paths >= 2 {
		i := rand.IntN(s.paths)
		j := rand.IntN(s.paths - 1)
		if j >= i {
			j++
		}
		if i > j {
			i, j = j, i
		}
		ops = append(ops, op{id: OpSwap, world: s, i: i, j: j})
	}
	if s.paths >= 2 {
		ops = append(ops, op{id: OpDelete, world: s, i: rand.IntN(s.paths)})
	}
	return ops
}

type namedPick struct {
	pick    formPick
	elapsed time.Duration
	miss    map[straightKey]struct{}
}

func (s *world) choose(ctx context.Context, lefts []leftover, parent snapshot, band int) ([]formPick, []search.Rated, map[straightKey]struct{}, error) {
	var jobs []candJob
	for _, left := range lefts {
		for _, op := range s.leftoverOperators(left, band) {
			if op.Applies() {
				jobs = append(jobs, candJob{op: op, left: left, bound: true})
			}
		}
	}
	for _, op := range s.worldOperators(band) {
		if op.Applies() {
			jobs = append(jobs, candJob{op: op})
		}
	}
	bestByOp := make(map[Op]*namedPick, opCount)
	var pool []formPick
	record := func(id Op, p formPick, elapsed time.Duration) {
		st := bestByOp[id]
		if st == nil {
			st = &namedPick{}
			bestByOp[id] = st
		}
		if elapsed > st.elapsed {
			st.elapsed = elapsed
		}
		if id == OpSimplify {
			st.miss = p.simplifyMiss
		}
		if betterPick(p, st.pick) {
			st.pick = p
		}
		if p.ok {
			pool = append(pool, p)
		}
	}
	if taskgroup.FromContext(ctx) == nil {
		var mu sync.Mutex
		g, _ := errgroup.WithContext(ctx)
		for _, job := range jobs {
			job := job
			g.Go(func() error {
				pick, elapsed, err := s.runCandidate(ctx, job, nil, parent)
				if err != nil {
					return err
				}
				mu.Lock()
				defer mu.Unlock()
				record(job.op.ID(), pick, elapsed)
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, nil, nil, err
		}
	} else {
		jobNote(ctx, fmt.Sprintf("%s · %d generators", bandLabel(band), len(jobs)))
		rows, err := s.runCandidateTasks(ctx, jobs, parent, band)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, row := range rows {
			record(row.id, row.pick, row.elapsed)
		}
	}
	for id := OpNone; id < opCount; id++ {
		if st, ok := bestByOp[id]; ok {
			s.logCandidate(id, st.elapsed, st.pick)
		}
	}
	var miss map[straightKey]struct{}
	if st := bestByOp[OpSimplify]; st != nil {
		miss = st.miss
	}
	return pool, collectRated(bestByOp), miss, nil
}

func collectRated(bestByOp map[Op]*namedPick) []search.Rated {
	var out []search.Rated
	for id := OpNone + 1; id < opCount; id++ {
		st, ok := bestByOp[id]
		if !ok {
			continue
		}
		r := search.Rated{Op: id, Ok: st.pick.ok}
		if st.pick.scored {
			score := st.pick.errSum
			r.Score = &score
		}
		out = append(out, r)
	}
	return out
}

func mergeRated(dst, src []search.Rated) []search.Rated {
	by := make(map[Op]search.Rated, len(dst)+len(src))
	for _, r := range dst {
		by[r.Op] = r
	}
	for _, r := range src {
		old, ok := by[r.Op]
		if !ok {
			by[r.Op] = r
			continue
		}
		if r.Score != nil && (old.Score == nil || *r.Score < *old.Score) {
			by[r.Op] = r
		}
	}
	var out []search.Rated
	for id := OpNone + 1; id < opCount; id++ {
		if r, ok := by[id]; ok {
			out = append(out, r)
		}
	}
	return out
}
