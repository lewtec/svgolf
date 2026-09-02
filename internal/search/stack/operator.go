package stack

import (
	"context"
	"image"
	"image/color"
	"sync"
	"time"

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
	OpNone     = search.OpNone
	OpAbsorb   = search.OpAbsorb
	OpTriangle = search.OpTriangle
	OpRing     = search.OpRing
	OpGrow     = search.OpGrow
	OpCarve    = search.OpCarve
	OpSlide    = search.OpSlide
	OpBend     = search.OpBend
	OpSimplify = search.OpSimplify
	OpWash     = search.OpWash
	OpJoin     = search.OpJoin
	OpSubtract = search.OpSubtract
	OpSwap     = search.OpSwap
	OpDelete   = search.OpDelete
	OpUnhole   = search.OpUnhole
	opCount    = search.OpCount
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
	g.ring = ring
	g.fill = modeFill(s.want, g.work)
	g = s.seedGrow(g)
	return s.addLayer(filledPath(ring, g.fill), g, OpTriangle)
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
		gr := grow{i: i, work: work, fill: s.fills[i], dirty0: dirty0, oldErr: ScoreRectOn(s.gotP, s.wantP, dirty0.Inset(-2))}
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
			hole := hullRing(c.left.island)
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
	gr := grow{i: -1, work: c.left.island, fill: c.left.col, dirty0: dirty0, oldErr: ScoreRectOn(s.gotP, s.wantP, dirty0.Inset(-2))}
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

// Simplify drops one vertex.
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
	type job struct {
		next svg.Document
		node svg.Node
		g    grow
	}
	var jobs []job
	for i := 0; i < w.paths; i++ {
		node := w.doc.Children()[i+1]
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := parsePathRings(p)
		if len(rings) == 0 {
			continue
		}
		g := w.seedGrow(grow{i: i, work: s.buckets[i], fill: w.fills[i]})
		lin, hasLin := node.LinearFill()
		paint := func(outer pathRing, holes []pathRing) svg.Path {
			cand := filledRings(outer, holes, w.fills[i])
			if hasLin {
				cand = cand.WithLinearFill(lin)
			}
			return cand
		}
		propose := func(outer pathRing, holes []pathRing, dirty image.Rectangle) {
			if len(outer.verts) < 3 || ringCrosses(outer.points()) {
				return
			}
			cand := paint(outer, holes)
			lg := g
			if !dirty.Empty() {
				lg.dirty0 = dirty
			}
			jobs = append(jobs, job{next: replaceAt(w.doc, i+1, cand.Node()), node: cand.Node(), g: lg})
		}
		outer := rings[0]
		work := outer.collapseColinearLines()
		if len(work.verts) < len(outer.verts) {
			propose(work, rings[1:], g.dirty0)
		}
		if len(work.verts) >= 4 {
			n := len(work.verts)
			for v := 0; v < n; v++ {
				prev := (v - 1 + n) % n
				next := (v + 1) % n
				fan := [][2]float64{work.verts[prev], work.verts[v], work.verts[next]}
				if !regionWorthTrying(fan, w.gotP, w.wantP) {
					continue
				}
				propose(work.dropVertex(v), rings[1:], pointsRect(fan))
			}
		}
	}
	if len(jobs) == 0 {
		return nonePick(), nil
	}
	var mu sync.Mutex
	best := nonePick()
	eg := new(errgroup.Group)
	for _, job := range jobs {
		job := job
		eg.Go(func() error {
			pick, err := w.scoreCand(job.next, job.node, job.g, OpSimplify)
			if err != nil {
				return err
			}
			if pick.ok && pick.errSum > w.errSum {
				pick.ok = false
			}
			mu.Lock()
			if betterPick(pick, best) {
				best = pick
			}
			mu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nonePick(), err
	}
	return best, nil
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
	best := nonePick()
	outers := pathOuters(s.doc, s.paths)
	for i := 0; i < s.paths; i++ {
		for jn := i + 1; jn < s.paths; jn++ {
			if !sameRampFamily(s.fills[i], s.fills[jn]) {
				continue
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
						if pick, err := j.scoreJoin(s, i, jn, work, stitched, nil); err != nil {
							return nonePick(), err
						} else if betterPick(pick, best) {
							best = pick
						}
						continue
					}
				}
			}
			if !ringsNear(outers[i], outers[jn]) && !bucketsTouch(j.buckets[i], j.buckets[jn]) {
				continue
			}
			if !oneBlob(work) {
				continue
			}
			ring := coverRing(work)
			if len(ring) < 3 {
				continue
			}
			if pick, err := j.scoreJoin(s, i, jn, work, polylineRing(ring), nil); err != nil {
				return nonePick(), err
			} else if betterPick(pick, best) {
				best = pick
			}
		}
	}
	return best, nil
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
	best := nonePick()
	outers := pathOuters(w.doc, w.paths)
	for i := 0; i < w.paths; i++ {
		for j := 0; j < w.paths; j++ {
			if i == j || !ringsOverlap(outers[i], outers[j]) {
				continue
			}
			if len(outers[i]) < 3 || len(outers[j]) < 3 {
				continue
			}
			node := w.doc.Children()[j+1]
			bounds := nodeRect(node).Intersect(image.Rect(0, 0, w.w, w.h))
			rem := ringSubtract(outers[j], outers[i], bounds)
			if !hasInterior(rem) {
				continue
			}
			var rings [][][2]float64
			if h := coverRing(rem); len(h) >= 3 {
				rings = append(rings, h)
			}
			overlap := ringAnd(outers[j], outers[i], bounds)
			if shrunk := shrinkOuter(outers[j], overlap); len(shrunk) >= 3 {
				rings = append(rings, shrunk)
			}
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
	gotP := acquirePlane(ngot)
	dirty := nodeRect(s.doc.Children()[d.i+1]).Inset(-2)
	nerr := s.scoreAfter(gotP, dirty)
	releasePlane(gotP)
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
	best := nonePick()
	hot := islandRect(sl.left.island)
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
		vi, bestD := 0, -1.0
		for v, q := range outer.verts {
			pull := nearest(target, q)
			d := (q[0]-pull[0])*(q[0]-pull[0]) + (q[1]-pull[1])*(q[1]-pull[1])
			if bestD < 0 || d < bestD {
				vi, bestD = v, d
			}
		}
		pull := nearest(target, outer.verts[vi])
		if pull[0] == outer.verts[vi][0] && pull[1] == outer.verts[vi][1] {
			pull = leftoverCenter(sl.left.island)
		}
		if pull[0] == outer.verts[vi][0] && pull[1] == outer.verts[vi][1] {
			continue
		}
		moved := outer.moveVertex(vi, pull)
		if len(moved.verts) < 3 {
			continue
		}
		cand := filledRings(moved, rings[1:], s.fills[i])
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		g := s.seedGrow(grow{i: i, work: sl.left.island, fill: s.fills[i]})
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, OpSlide)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
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
	gotP := acquirePlane(ngot)
	dirty := nodeRect(s.doc.Children()[sw.i+1]).Union(nodeRect(s.doc.Children()[sw.j+1])).Inset(-2)
	nerr := s.scoreAfter(gotP, dirty)
	releasePlane(gotP)
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

func leftoverAddOperators(s *world, left leftover) []Operator {
	return []Operator{
		op{id: OpTriangle, world: s, left: left},
		op{id: OpRing, world: s, left: left},
	}
}

func (s *world) leftoverOperators(left leftover, band int) []Operator {
	var add []Operator
	if !left.region && left.big() {
		add = leftoverAddOperators(s, left)
	}
	if left.deltaHi > 0 && left.deltaLo < 64 {
		if left.deltaHi <= 16 {
			switch band {
			case 1, 3:
				return add
			case 2, 4:
				return append(add, op{id: OpAbsorb, world: s, left: left})
			default:
				return nil
			}
		}
		switch band {
		case 1:
			return append(add,
				op{id: OpSlide, world: s, left: left},
				op{id: OpBend, world: s, left: left},
			)
		case 2:
			return append(add, op{id: OpAbsorb, world: s, left: left})
		case 4:
			return append(add,
				op{id: OpSlide, world: s, left: left},
				op{id: OpBend, world: s, left: left},
				op{id: OpAbsorb, world: s, left: left},
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
			return append(add, op{id: OpAbsorb, world: s, left: left})
		case 4:
			return append(add,
				op{id: OpAbsorb, world: s, left: left},
				op{id: OpGrow, world: s, left: left},
				op{id: OpCarve, world: s, left: left},
			)
		default:
			return nil
		}
	}
	switch band {
	case 1:
		return append(add,
			op{id: OpSlide, world: s, left: left},
			op{id: OpBend, world: s, left: left},
		)
	case 2:
		return append(add, op{id: OpAbsorb, world: s, left: left})
	case 3:
		return add
	case 4:
		return append(add,
			op{id: OpSlide, world: s, left: left},
			op{id: OpBend, world: s, left: left},
			op{id: OpAbsorb, world: s, left: left},
			op{id: OpGrow, world: s, left: left},
			op{id: OpCarve, world: s, left: left},
		)
	default:
		return nil
	}
}

func (s *world) worldOperators(band int) []Operator {
	if band != 1 && band != 2 {
		return nil
	}
	var buckets [][]pix
	if s.paths > 0 {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	if band == 1 {
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
	for i := 0; i < s.paths; i++ {
		for j := i + 1; j < s.paths; j++ {
			ops = append(ops, op{id: OpSwap, world: s, i: i, j: j})
		}
	}
	for i := 0; i < s.paths; i++ {
		ops = append(ops, op{id: OpDelete, world: s, i: i})
	}
	return ops
}

type namedPick struct {
	pick    formPick
	elapsed time.Duration
}

func (s *world) choose(ctx context.Context, lefts []leftover, parent snapshot, band int) ([]formPick, []search.Rated, error) {
	type job struct {
		op    Operator
		left  leftover
		bound bool
	}
	var jobs []job
	for _, left := range lefts {
		for _, op := range s.leftoverOperators(left, band) {
			if op.Applies() {
				jobs = append(jobs, job{op: op, left: left, bound: true})
			}
		}
	}
	for _, op := range s.worldOperators(band) {
		if op.Applies() {
			jobs = append(jobs, job{op: op})
		}
	}
	bestByOp := make(map[Op]*namedPick, opCount)
	var mu sync.Mutex
	var pool []formPick
	g, _ := errgroup.WithContext(ctx)
	for _, job := range jobs {
		job := job
		g.Go(func() error {
			started := time.Now()
			p, err := job.op.Run()
			if err != nil {
				return err
			}
			if p.ok {
				p.parent = parent
				if job.bound {
					p.island = job.left.glow
					if len(p.island) == 0 {
						p.island = job.left.island
					}
				}
			}
			elapsed := time.Since(started)
			mu.Lock()
			defer mu.Unlock()
			id := job.op.ID()
			st := bestByOp[id]
			if st == nil {
				st = &namedPick{}
				bestByOp[id] = st
			}
			if elapsed > st.elapsed {
				st.elapsed = elapsed
			}
			if betterPick(p, st.pick) {
				st.pick = p
			}
			if p.ok {
				pool = append(pool, p)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	for id := OpNone; id < opCount; id++ {
		if st, ok := bestByOp[id]; ok {
			s.logCandidate(id, st.elapsed, st.pick)
		}
	}
	return pool, collectRated(bestByOp), nil
}

func collectRated(bestByOp map[Op]*namedPick) []search.Rated {
	var out []search.Rated
	for id := OpNone + 1; id < opCount; id++ {
		st, ok := bestByOp[id]
		if !ok {
			continue
		}
		r := search.Rated{Name: id.String(), Ok: st.pick.ok}
		if st.pick.scored {
			score := st.pick.errSum
			r.Score = &score
		}
		out = append(out, r)
	}
	return out
}

func mergeRated(dst, src []search.Rated) []search.Rated {
	by := make(map[string]search.Rated, len(dst)+len(src))
	for _, r := range dst {
		by[r.Name] = r
	}
	for _, r := range src {
		old, ok := by[r.Name]
		if !ok {
			by[r.Name] = r
			continue
		}
		if r.Score != nil && (old.Score == nil || *r.Score < *old.Score) {
			by[r.Name] = r
		}
	}
	var out []search.Rated
	for id := OpNone + 1; id < opCount; id++ {
		if r, ok := by[id.String()]; ok {
			out = append(out, r)
		}
	}
	return out
}
