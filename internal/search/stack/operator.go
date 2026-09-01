package stack

import (
	"context"
	"image"
	"image/color"
	"sync"
	"time"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
	"golang.org/x/sync/errgroup"
)

// Operator is one edit. choose starts every applicable Operator
// and waits once. Score ranks the pool.
type Operator interface {
	Name() string
	Applies() bool
	Run() (formPick, error)
}

// Cover places a rough leftover hull. Slide and Bend walk
// the leftover outline later.
type Cover struct {
	world *world
	left  leftover
}

func (Cover) Name() string { return "cover" }
func (c Cover) Applies() bool {
	return c.left.big() && !c.left.paper && c.world.paths < maxPaths
}

func (c Cover) Run() (formPick, error) {
	s, g := c.world, c.left.fresh
	if len(g.work) < minIsland {
		return nonePick(), nil
	}
	ring := hullRing(g.work)
	if len(ring) < 3 {
		return nonePick(), nil
	}
	g.ring = ring
	return s.addLayer(filledPath(ring, g.fill), g, c.Name())
}

// Rect places the biggest axis-aligned rectangle inside the leftover.
type Rect struct {
	world *world
	left  leftover
}

func (Rect) Name() string { return "rect" }
func (r Rect) Applies() bool {
	return r.left.big() && !r.left.paper && r.world.paths < maxPaths
}

func (r Rect) Run() (formPick, error) {
	s, g := r.world, r.left.fresh
	if len(g.work) < minIsland {
		return nonePick(), nil
	}
	ring := largestRect(g.work)
	if len(ring) < 4 {
		return nonePick(), nil
	}
	work := rectPix(ring)
	if len(work) < minIsland {
		return nonePick(), nil
	}
	g.work = work
	g.ring = ring
	g = s.seedGrow(g)
	return s.addLayer(filledPath(ring, g.fill), g, r.Name())
}

// Ring is a leftover with a painted interior: evenodd outer plus holes
// so a visor does not fill the face.
type Ring struct {
	world *world
	left  leftover
}

func (Ring) Name() string { return "ring" }
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
	g.ring = hullRing(g.work)
	if len(g.ring) < 3 {
		return nonePick(), nil
	}
	return s.addLayer(withHoles(filledPath(g.ring, g.fill), holes), g, r.Name())
}

// Absorb writes leftover error back into a touching path as a 2-stop.
// That is the backprop step: residual updates an existing plate
// instead of stacking another flat.
type Absorb struct {
	world   *world
	left    leftover
	scratch scratch
}

func (Absorb) Name() string { return "absorb" }
func (a Absorb) Applies() bool {
	return a.left.big() && !a.left.paper && a.world.paths > 0
}

func (a *Absorb) Run() (formPick, error) {
	s := a.world
	works := a.left.grows
	if works == nil {
		works = s.connecting(a.left.island, a.scratch.seen)
	}
	curA := s.currentScore()
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
		pick, err := s.scoreCand(replaceAt(s.doc, g.i+1, cand.Node()), cand.Node(), g, s.paths, a.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Grow expands an existing path over the leftover with a hull union.
type Grow struct {
	world   *world
	left    leftover
	scratch scratch
}

func (g Grow) Name() string { return "grow" }
func (g Grow) Applies() bool {
	return g.left.big() && !g.left.paper
}

func (g *Grow) Run() (formPick, error) {
	s := g.world
	works := g.left.grows
	if works == nil {
		works = s.connecting(g.left.island, g.scratch.seen)
	}
	curA := s.currentScore()
	best := nonePick()
	for _, work := range works {
		ring := hullRing(work.work)
		if len(ring) < 3 {
			continue
		}
		work.ring = ring
		cand := filledPath(ring, work.fill)
		pick, err := s.scoreCand(replaceAt(s.doc, work.i+1, cand.Node()), cand.Node(), work, s.paths, g.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Carve cuts a painted leftover out of a covering path.
// Paper leftover shrinks the covering ring instead of stacking
// an evenodd white hole.
type Carve struct {
	world   *world
	left    leftover
	scratch scratch
}

func (c Carve) Name() string { return "carve" }
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
	curA := s.currentScore()
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
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), gr, s.paths, c.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok && pick.errSum >= s.errSum {
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
	curA := s.currentScore()
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
		rings := pathRings(p)
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
			outer := shrinkOuter(rings[0], c.left.island)
			if len(outer) < 3 {
				continue
			}
			cand = filledPath(outer, s.fills[i])
			if len(rings) > 1 {
				cand = withHoles(cand, rings[1:])
			}
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
	pick, err := s.scoreCand(next, last, gr, s.paths, c.Name(), curA)
	if err != nil {
		return nonePick(), err
	}
	if pick.ok && pick.errSum >= s.errSum {
		pick.ok = false
	}
	if pick.ok {
		pick.reclaims = reclaims
		pick.replace = -1
	}
	return pick, nil
}

// Simplify drops one vertex or one hole. Score picks the drop.
type Simplify struct {
	world   *world
	buckets [][]pix
}

func (s Simplify) Name() string { return "simplify" }
func (s Simplify) Applies() bool {
	return s.world.paths > 0
}

func (s Simplify) Run() (formPick, error) {
	w := s.world
	curA := w.currentScore()
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
		rings := pathRings(p)
		if len(rings) == 0 {
			continue
		}
		g := w.seedGrow(grow{i: i, work: s.buckets[i], fill: w.fills[i]})
		lin, hasLin := node.LinearFill()
		paint := func(outer [][2]float64, holes [][][2]float64) svg.Path {
			cand := filledPath(outer, w.fills[i])
			if len(holes) > 0 {
				cand = withHoles(cand, holes)
			}
			if hasLin {
				cand = cand.WithLinearFill(lin)
			}
			return cand
		}
		outer := rings[0]
		if len(outer) >= 4 {
			for v := 0; v < len(outer); v++ {
				shorter := append([][2]float64{}, outer[:v]...)
				shorter = append(shorter, outer[v+1:]...)
				if ringCrosses(shorter) {
					continue
				}
				cand := paint(shorter, rings[1:])
				jobs = append(jobs, job{next: replaceAt(w.doc, i+1, cand.Node()), node: cand.Node(), g: g})
			}
		}
		for h := 1; h < len(rings); h++ {
			keep := append([][][2]float64{}, rings[1:h]...)
			keep = append(keep, rings[h+1:]...)
			cand := paint(rings[0], keep)
			jobs = append(jobs, job{next: replaceAt(w.doc, i+1, cand.Node()), node: cand.Node(), g: g})
		}
	}
	var mu sync.Mutex
	best := nonePick()
	eg := new(errgroup.Group)
	for _, job := range jobs {
		job := job
		eg.Go(func() error {
			pick, err := w.scoreCand(job.next, job.node, job.g, w.paths, s.Name(), curA)
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

// Wash fits a 2-stop linear on one path, or on two owners that form a ramp.
type Wash struct {
	world   *world
	buckets [][]pix
	scratch scratch
}

func (Wash) Name() string { return "wash" }
func (w Wash) Applies() bool {
	return w.world.paths > 0
}

func (w *Wash) Run() (formPick, error) {
	s := w.world
	curA := s.currentScore()
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
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, s.paths, w.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	if s.paths < 2 {
		return best, nil
	}
	for i := 0; i < s.paths; i++ {
		for j := i + 1; j < s.paths; j++ {
			w.scratch.work = w.scratch.work[:0]
			w.scratch.work = append(w.scratch.work, w.buckets[i]...)
			w.scratch.work = append(w.scratch.work, w.buckets[j]...)
			if len(w.scratch.work) < minIsland {
				continue
			}
			grad, ok := fitLinearStops(w.scratch.work, s.want)
			if !ok {
				continue
			}
			work := append([]pix{}, w.scratch.work...)
			g := s.seedGrow(grow{i: i, work: work, fill: s.fills[i], ring: hullRing(work)})
			if len(g.ring) < 3 {
				continue
			}
			cand := filledPath(g.ring, s.fills[i]).WithLinearFill(grad)
			next := replaceAt(s.doc, i+1, cand.Node())
			next = dropAt(next, j+1)
			pick, err := s.scoreCand(next, cand.Node(), g, s.paths-1, w.Name(), curA)
			if err != nil {
				return nonePick(), err
			}
			if pick.ok {
				pick.mergeJ = j
			}
			if betterPick(pick, best) {
				best = pick
			}
		}
	}
	return best, nil
}

// Join unifies two same-family paths that overlap or have
// a vertex next to the other's vertex or edge. A single
// blob keeps the union outline; a gap uses the hull.
type Join struct {
	world   *world
	buckets [][]pix
	scratch scratch
}

func (Join) Name() string { return "join" }
func (j Join) Applies() bool {
	return j.world.paths >= 2
}

func (j *Join) Run() (formPick, error) {
	s := j.world
	curA := s.currentScore()
	best := nonePick()
	outers := pathOuters(s.doc, s.paths)
	for i := 0; i < s.paths; i++ {
		for jn := i + 1; jn < s.paths; jn++ {
			if !sameRampFamily(s.fills[i], s.fills[jn]) {
				continue
			}
			if !ringsNear(outers[i], outers[jn]) {
				continue
			}
			j.scratch.work = j.scratch.work[:0]
			j.scratch.work = append(j.scratch.work, j.buckets[i]...)
			j.scratch.work = append(j.scratch.work, j.buckets[jn]...)
			work := append([]pix{}, j.scratch.work...)
			var ring [][2]float64
			if oneBlob(work) {
				ring = coverRing(work)
			} else {
				pts := append([][2]float64{}, outers[i]...)
				pts = append(pts, outers[jn]...)
				ring = uncross(convexHull(pts))
			}
			if len(ring) < 3 {
				continue
			}
			fills := []color.NRGBA{s.fills[i], s.fills[jn]}
			if fills[0] == fills[1] {
				fills = fills[:1]
			}
			for _, fill := range fills {
				g := s.seedGrow(grow{i: i, work: work, fill: fill, ring: ring})
				cand := filledPath(ring, fill)
				next := replaceAt(s.doc, i+1, cand.Node())
				next = dropAt(next, jn+1)
				pick, err := s.scoreCand(next, cand.Node(), g, s.paths-1, j.Name(), curA)
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
		}
	}
	return best, nil
}

// Subtract punches one overlapping path out of another. Score
// keeps it when the lower plate should not paint under the upper.
type Subtract struct {
	world   *world
	buckets [][]pix
}

func (Subtract) Name() string { return "subtract" }
func (s Subtract) Applies() bool {
	return s.world.paths >= 2
}

func (s Subtract) Run() (formPick, error) {
	w := s.world
	curA := w.currentScore()
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
			ring := hullRing(rem)
			if len(ring) < 3 {
				continue
			}
			cand := filledPath(ring, w.fills[j])
			if lin, ok := node.LinearFill(); ok {
				cand = cand.WithLinearFill(lin)
			}
			g := w.seedGrow(grow{i: j, work: rem, fill: w.fills[j], ring: ring})
			pick, err := w.scoreCand(replaceAt(w.doc, j+1, cand.Node()), cand.Node(), g, w.paths, s.Name(), curA)
			if err != nil {
				return nonePick(), err
			}
			if betterPick(pick, best) {
				best = pick
			}
		}
	}
	return best, nil
}

// Delete removes path i if Score improves.
type Delete struct {
	world *world
	i     int
}

func (Delete) Name() string { return "delete" }
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
	wantP := s.wantP
	if wantP == nil {
		wantP = loss.NewPlane(s.want)
	}
	gotP := acquirePlane(ngot)
	nerr := ScoreOn(gotP, wantP, 0)
	releasePlane(gotP)
	curA := s.currentScore()
	raw := nerr + pathCost*float64(s.paths-1) + cmdCost*float64(docCmdLen(next))
	a := s.streak(raw, d.Name())
	ok := a < curA && nerr <= s.errSum
	var got *image.NRGBA
	if ok {
		got = render.Keep(ngot)
	}
	return formPick{doc: next, got: got, errSum: nerr, a: a, raw: raw, replace: -1, insert: -1, dropIdx: d.i, mergeJ: -1, op: d.Name(), ok: ok, scored: true}, nil
}

// Slide moves one vertex of a touching path toward the leftover outline.
type Slide struct {
	world *world
	left  leftover
}

func (Slide) Name() string { return "slide" }
func (sl Slide) Applies() bool {
	return sl.left.big() && sl.world.paths > 0
}

func (sl Slide) Run() (formPick, error) {
	s := sl.world
	target := outline(sl.left.island)
	if len(target) < 1 {
		return nonePick(), nil
	}
	curA := s.currentScore()
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
		rings := pathRings(p)
		if len(rings) == 0 || len(rings[0]) < 3 {
			continue
		}
		outer := rings[0]
		vi, bestD := 0, -1.0
		for v, q := range outer {
			pull := nearest(target, q)
			d := (q[0]-pull[0])*(q[0]-pull[0]) + (q[1]-pull[1])*(q[1]-pull[1])
			if bestD < 0 || d < bestD {
				vi, bestD = v, d
			}
		}
		pull := nearest(target, outer[vi])
		if pull[0] == outer[vi][0] && pull[1] == outer[vi][1] {
			pull = leftoverCenter(sl.left.island)
		}
		if pull[0] == outer[vi][0] && pull[1] == outer[vi][1] {
			continue
		}
		moved := append([][2]float64{}, outer...)
		moved[vi] = pull
		cand := filledPath(moved, s.fills[i])
		if len(rings) > 1 {
			cand = withHoles(cand, rings[1:])
		}
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		g := s.seedGrow(grow{i: i, work: sl.left.island, fill: s.fills[i]})
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, s.paths, sl.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Bend pulls the leftover-facing edge into a cubic toward the residual.
type Bend struct {
	world *world
	left  leftover
}

func (Bend) Name() string { return "bend" }
func (b Bend) Applies() bool {
	return b.left.big() && b.world.paths > 0
}

func (b Bend) Run() (formPick, error) {
	s := b.world
	target := outline(b.left.island)
	if len(target) < 1 {
		return nonePick(), nil
	}
	curA := s.currentScore()
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
		rings := pathRings(p)
		if len(rings) == 0 || len(rings[0]) < 3 {
			continue
		}
		outer := rings[0]
		n := len(outer)
		if n < 3 {
			continue
		}
		ctr := leftoverCenter(b.left.island)
		ei, bestD := 0, -1.0
		for e := 0; e < n; e++ {
			a, c := outer[e], outer[(e+1)%n]
			mid := [2]float64{(a[0] + c[0]) / 2, (a[1] + c[1]) / 2}
			d := (mid[0]-ctr[0])*(mid[0]-ctr[0]) + (mid[1]-ctr[1])*(mid[1]-ctr[1])
			if bestD < 0 || d < bestD {
				ei, bestD = e, d
			}
		}
		a, c := outer[ei], outer[(ei+1)%n]
		mid := [2]float64{(a[0] + c[0]) / 2, (a[1] + c[1]) / 2}
		pull := nearest(target, mid)
		if pull[0] == mid[0] && pull[1] == mid[1] {
			pull = ctr
		}
		c1 := [2]float64{(a[0] + pull[0]) / 2, (a[1] + pull[1]) / 2}
		c2 := [2]float64{(c[0] + pull[0]) / 2, (c[1] + pull[1]) / 2}
		cand := bentEdge(outer, rings[1:], s.fills[i], ei, c1, c2)
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		g := s.seedGrow(grow{i: i, work: b.left.island, fill: s.fills[i]})
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, s.paths, b.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

func bentEdge(outer [][2]float64, holes [][][2]float64, col color.NRGBA, ei int, c1, c2 [2]float64) svg.Path {
	n := len(outer)
	if n < 3 || ei < 0 || ei >= n {
		return filledPath(outer, col)
	}
	cmds := []svg.PathCmd{{Kind: svg.CmdMove, X: outer[0][0], Y: outer[0][1]}}
	for i := 0; i < n; i++ {
		dest := outer[(i+1)%n]
		if i == ei {
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdCubic, X1: c1[0], Y1: c1[1], X2: c2[0], Y2: c2[1], X: dest[0], Y: dest[1]})
			continue
		}
		cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: dest[0], Y: dest[1]})
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	p, _ := svg.NewPath().WithCommands(cmds)
	p = p.WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
	if len(holes) > 0 {
		p = withHoles(p, holes)
	}
	return p
}

// HullPath replaces one path's outer ring with the convex hull of
// its owned pixels. Score keeps it only when that is cheaper.
type HullPath struct {
	world   *world
	buckets [][]pix
}

func (HullPath) Name() string { return "hull" }
func (h HullPath) Applies() bool {
	return h.world.paths > 0
}

func (h HullPath) Run() (formPick, error) {
	s := h.world
	curA := s.currentScore()
	best := nonePick()
	for i := 0; i < s.paths; i++ {
		if len(h.buckets[i]) < 3 {
			continue
		}
		ring := hullRing(h.buckets[i])
		if len(ring) < 3 {
			continue
		}
		node := s.doc.Children()[i+1]
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := pathRings(p)
		cand := filledPath(ring, s.fills[i])
		if len(rings) > 1 {
			cand = withHoles(cand, rings[1:])
		}
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		g := s.seedGrow(grow{i: i, work: h.buckets[i], fill: s.fills[i], ring: ring})
		pick, err := s.scoreCand(replaceAt(s.doc, i+1, cand.Node()), cand.Node(), g, s.paths, h.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

// Swap exchanges paths i and j. Score judges the painted order.
type Swap struct {
	world *world
	i, j  int
}

func (Swap) Name() string { return "swap" }
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
	wantP := s.wantP
	if wantP == nil {
		wantP = loss.NewPlane(s.want)
	}
	gotP := acquirePlane(ngot)
	nerr := ScoreOn(gotP, wantP, 0)
	releasePlane(gotP)
	curA := s.currentScore()
	raw := nerr + pathCost*float64(s.paths) + cmdCost*float64(docCmdLen(next))
	a := s.streak(raw, sw.Name())
	ok = a < curA
	var got *image.NRGBA
	if ok {
		got = render.Keep(ngot)
	}
	return formPick{
		doc: next, got: got, errSum: nerr, a: a, raw: raw,
		replace: -1, insert: -1, dropIdx: -1, mergeJ: -1,
		op: sw.Name(), ok: ok, scored: true,
		fills: fills, owner: owner,
	}, nil
}

func (s *world) leftoverOps(left leftover) []Operator {
	return []Operator{
		&Absorb{world: s, left: left},
		Cover{world: s, left: left},
		Rect{world: s, left: left},
		Ring{world: s, left: left},
		&Grow{world: s, left: left},
		&Carve{world: s, left: left},
		Slide{world: s, left: left},
		Bend{world: s, left: left},
	}
}

func (s *world) worldOps() []Operator {
	var buckets [][]pix
	if s.paths > 0 {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	ops := []Operator{
		// Simplify{world: s, buckets: buckets},
		HullPath{world: s, buckets: buckets},
		&Wash{world: s, buckets: buckets},
		&Join{world: s, buckets: buckets},
		Subtract{world: s, buckets: buckets},
	}
	for i := 0; i < s.paths; i++ {
		for j := i + 1; j < s.paths; j++ {
			ops = append(ops, Swap{world: s, i: i, j: j})
		}
	}
	for i := 0; i < s.paths; i++ {
		ops = append(ops, Delete{world: s, i: i})
	}
	return ops
}

type namedPick struct {
	pick    formPick
	elapsed time.Duration
}

var operatorNames = []string{
	"absorb", "cover", "rect", "hull", "ring", "grow", "carve",
	"slide", "bend", "simplify", "wash", "join", "subtract", "swap", "delete",
}

func (s *world) choose(ctx context.Context, lefts []leftover, parent snapshot, world bool) ([]formPick, []search.Rated, error) {
	type job struct {
		op    Operator
		left  leftover
		bound bool
	}
	var jobs []job
	for _, left := range lefts {
		for _, op := range s.leftoverOps(left) {
			if op.Applies() {
				jobs = append(jobs, job{op: op, left: left, bound: true})
			}
		}
	}
	if world {
		for _, op := range s.worldOps() {
			if op.Applies() {
				jobs = append(jobs, job{op: op})
			}
		}
	}
	bestByName := make(map[string]*namedPick, len(operatorNames))
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
					p.island = job.left.island
				}
			}
			elapsed := time.Since(started)
			mu.Lock()
			defer mu.Unlock()
			st := bestByName[job.op.Name()]
			if st == nil {
				st = &namedPick{}
				bestByName[job.op.Name()] = st
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
	for _, name := range operatorNames {
		if st, ok := bestByName[name]; ok {
			s.logCandidate(name, st.elapsed, st.pick)
		}
	}
	return pool, collectRated(bestByName), nil
}

func collectRated(bestByName map[string]*namedPick) []search.Rated {
	var out []search.Rated
	for _, name := range operatorNames {
		st, ok := bestByName[name]
		if !ok {
			continue
		}
		r := search.Rated{Name: name, Ok: st.pick.ok}
		if st.pick.scored {
			s := st.pick.a
			r.Score = &s
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
	for _, name := range operatorNames {
		if r, ok := by[name]; ok {
			out = append(out, r)
		}
	}
	return out
}
