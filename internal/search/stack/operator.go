package stack

import (
	"context"
	"image"
	"image/color"
	"sync"
	"time"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
	"golang.org/x/sync/errgroup"
)

// Operator is one iterative edit. It carries the world it reads and
// its own scratch. choose starts every applicable Operator and waits
// once.
type Operator interface {
	Name() string
	Applies() bool
	Run() (formPick, error)
}

// Rectangle places a four-sided leftover cover.
type Rectangle struct {
	world *world
	left  leftover
}

func (r Rectangle) Name() string { return "rectangle" }
func (r Rectangle) Applies() bool {
	return r.left.big() && !r.left.paper && r.world.paths < maxPaths
}

func (r Rectangle) Run() (formPick, error) {
	s, g := r.world, r.left.fresh
	if len(g.work) < minIsland {
		return nonePick(), nil
	}
	g.ring = quadRing(g.work)
	cand := filledPath(g.ring, g.fill)
	return s.scoreCand(s.doc.Append(cand.Node()), cand.Node(), g, s.paths+1, r.Name(), s.currentScore())
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
	g.ring = quadRing(g.work)
	if len(g.ring) < 3 {
		return nonePick(), nil
	}
	cand := withHoles(filledPath(g.ring, g.fill), holes)
	return s.scoreCand(s.doc.Append(cand.Node()), cand.Node(), g, s.paths+1, r.Name(), s.currentScore())
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
		g.ring = quadRing(g.work)
		if len(g.ring) < 3 {
			continue
		}
		cand := filledPath(g.ring, g.fill).WithLinearFill(grad)
		pick, err := s.scoreCand(replaceAt(s.doc, g.i+1, cand.Node()), cand.Node(), g, s.paths, a.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok && (!best.ok || pick.a < best.a) {
			best = pick
		}
	}
	return best, nil
}

// Grow expands an existing path over the leftover with a four-sided union.
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
		work.ring = quadRing(work.work)
		if len(work.ring) < 3 {
			continue
		}
		cand := filledPath(work.ring, work.fill)
		pick, err := s.scoreCand(replaceAt(s.doc, work.i+1, cand.Node()), cand.Node(), work, s.paths, g.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok && (!best.ok || pick.a < best.a) {
			best = pick
		}
	}
	return best, nil
}

// Carve cuts this leftover out of a covering path.
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
		hole = quadRing(c.left.island)
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
			continue
		}
		if pick.ok && (!best.ok || pick.a < best.a) {
			best = pick
		}
	}
	return best, nil
}

func (c *Carve) paper(hole [][2]float64) (formPick, error) {
	s := c.world
	curA := s.currentScore()
	next := s.doc
	reclaims := make([][]pix, s.paths)
	any := false
	dirty0 := islandRect(c.left.island)
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
		next = replaceAt(next, i+1, cand.Node())
		reclaims[i] = ownedMinus(s.owner, c.left.island, s.w, uint16(i+1), c.scratch.seen)
		dirty0 = dirty0.Union(nodeRect(node))
		any = true
	}
	if !any {
		return nonePick(), nil
	}
	gr := grow{i: -1, work: c.left.island, fill: c.left.col, dirty0: dirty0, oldErr: ScoreRectOn(s.gotP, s.wantP, dirty0.Inset(-2))}
	pick, err := s.scoreCand(next, next.Children()[1], gr, s.paths, c.Name(), curA)
	if err != nil {
		return nonePick(), err
	}
	if pick.ok && pick.errSum >= s.errSum {
		return nonePick(), nil
	}
	if pick.ok {
		pick.reclaims = reclaims
		pick.replace = -1
	}
	return pick, nil
}

// Simplify shortens one existing path with RDP.
type Simplify struct {
	world   *world
	buckets [][]pix
}

func (Simplify) Name() string { return "simplify" }
func (s Simplify) Applies() bool {
	return s.world.paths > 0
}

func (s Simplify) Run() (formPick, error) {
	w := s.world
	curA := w.currentScore()
	best := nonePick()
	for i := 0; i < w.paths; i++ {
		node := w.doc.Children()[i+1]
		p, ok := node.Path()
		if !ok {
			continue
		}
		rings := pathRings(p)
		if len(rings) == 0 || len(rings[0]) < 4 {
			continue
		}
		shorter := rdpClosed(rings[0], polyFit)
		if len(shorter) >= len(rings[0]) {
			continue
		}
		cand := filledPath(shorter, w.fills[i])
		if len(rings) > 1 {
			cand = withHoles(cand, rings[1:])
		}
		if lin, ok := node.LinearFill(); ok {
			cand = cand.WithLinearFill(lin)
		}
		if pathLen(cand.Node()) >= pathLen(node) {
			continue
		}
		g := w.seedGrow(grow{i: i, work: s.buckets[i], fill: w.fills[i]})
		pick, err := w.scoreCand(replaceAt(w.doc, i+1, cand.Node()), cand.Node(), g, w.paths, s.Name(), curA)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok && pick.errSum > w.errSum {
			continue
		}
		if pick.ok && (!best.ok || pick.a < best.a) {
			best = pick
		}
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
		grad, ok := fitLinearFill(work, s.want)
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
		if pick.ok && (!best.ok || pick.a < best.a) {
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
			grad, ok := fitLinearFill(w.scratch.work, s.want)
			if !ok {
				continue
			}
			work := append([]pix{}, w.scratch.work...)
			g := s.seedGrow(grow{i: i, work: work, fill: s.fills[i], ring: quadRing(work)})
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
			if pick.ok && (!best.ok || pick.a < best.a) {
				best = pick
			}
		}
	}
	return best, nil
}

// Join merges two paths into one four-sided solid.
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
	rects := make([]image.Rectangle, s.paths)
	for i := 0; i < s.paths; i++ {
		rects[i] = islandRect(j.buckets[i])
	}
	kids := s.doc.Children()
	for i := 0; i < s.paths; i++ {
		for jn := i + 1; jn < s.paths; jn++ {
			if !rects[i].Inset(-1).Overlaps(rects[jn]) {
				continue
			}
			if !sameRampFamily(s.fills[i], s.fills[jn]) {
				continue
			}
			need := len(j.buckets[i]) + len(j.buckets[jn])
			if need < minIsland {
				continue
			}
			ring := joinRing(kids[i+1], kids[jn+1])
			if len(ring) < 3 {
				continue
			}
			j.scratch.work = j.scratch.work[:0]
			j.scratch.work = append(j.scratch.work, j.buckets[i]...)
			j.scratch.work = append(j.scratch.work, j.buckets[jn]...)
			g := s.seedGrow(grow{i: i, work: j.scratch.work, fill: meanTwo(s.fills[i], s.fills[jn]), ring: ring})
			cand := filledPath(ring, g.fill)
			next := replaceAt(s.doc, i+1, cand.Node())
			next = dropAt(next, jn+1)
			pick, err := s.scoreCand(next, cand.Node(), g, s.paths-1, j.Name(), curA)
			if err != nil {
				return nonePick(), err
			}
			if pick.ok {
				pick.mergeJ = jn
				pick.work = append([]pix{}, j.scratch.work...)
			}
			if pick.ok && (!best.ok || pick.a < best.a) {
				best = pick
			}
		}
	}
	return best, nil
}

func joinRing(a, b svg.Node) [][2]float64 {
	var pts [][2]float64
	if p, ok := a.Path(); ok {
		for _, r := range pathRings(p) {
			pts = append(pts, r...)
		}
	}
	if p, ok := b.Path(); ok {
		for _, r := range pathRings(p) {
			pts = append(pts, r...)
		}
	}
	if len(pts) < 3 {
		return nil
	}
	return collapseToSides(convexHull(pts), 4)
}

func meanTwo(a, b color.NRGBA) color.NRGBA {
	return color.NRGBA{
		R: uint8((int(a.R) + int(b.R)) / 2),
		G: uint8((int(a.G) + int(b.G)) / 2),
		B: uint8((int(a.B) + int(b.B)) / 2),
		A: 255,
	}
}

// Drop removes the smallest owned path if the pixmap does not get worse.
type Drop struct {
	world *world
}

func (Drop) Name() string { return "drop" }
func (d Drop) Applies() bool {
	return d.world.paths >= 2
}

func (d Drop) Run() (formPick, error) {
	s := d.world
	idx, ok := smallestOwner(s.owner, s.paths)
	if !ok {
		return nonePick(), nil
	}
	next := dropAt(s.doc, idx+1)
	ngot, err := render.Render(next)
	if err != nil {
		return nonePick(), err
	}
	wantP := s.wantP
	if wantP == nil {
		wantP = loss.NewPlane(s.want)
	}
	nerr := ScoreOn(loss.NewPlane(ngot), wantP, 0)
	curA := s.currentScore()
	a := nerr + pathCost*float64(s.paths-1) + cmdCost*float64(docCmdLen(next))
	if a >= curA || nerr > s.errSum {
		return nonePick(), nil
	}
	return formPick{doc: next, got: ngot, errSum: nerr, a: a, replace: -1, dropIdx: idx, mergeJ: -1, op: d.Name(), ok: true}, nil
}

// Restack sorts existing paths by drawn area: large under small.
// Score judges the reorder; apply never shuffles after accept.
type Restack struct {
	world *world
}

func (Restack) Name() string { return "restack" }
func (r Restack) Applies() bool {
	return r.world.paths >= 2
}

func (r Restack) Run() (formPick, error) {
	s := r.world
	next, fills, owner, ok := s.restackOrder()
	if !ok {
		return nonePick(), nil
	}
	ngot, err := render.Render(next)
	if err != nil {
		return nonePick(), err
	}
	wantP := s.wantP
	if wantP == nil {
		wantP = loss.NewPlane(s.want)
	}
	nerr := ScoreOn(loss.NewPlane(ngot), wantP, 0)
	curA := s.currentScore()
	a := nerr + pathCost*float64(s.paths) + cmdCost*float64(docCmdLen(next))
	if a >= curA {
		return nonePick(), nil
	}
	return formPick{
		doc: next, got: ngot, errSum: nerr, a: a,
		replace: -1, dropIdx: -1, mergeJ: -1,
		op: r.Name(), ok: true,
		fills: fills, owner: owner,
	}, nil
}

func (s *world) leftoverOps(left leftover) []Operator {
	return []Operator{
		&Absorb{world: s, left: left},
		Rectangle{world: s, left: left},
		Ring{world: s, left: left},
		&Grow{world: s, left: left},
		&Carve{world: s, left: left},
	}
}

func (s *world) worldOps() []Operator {
	var buckets [][]pix
	if s.paths > 0 {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	return []Operator{
		Simplify{world: s, buckets: buckets},
		&Wash{world: s, buckets: buckets},
		&Join{world: s, buckets: buckets},
		Drop{world: s},
		Restack{world: s},
	}
}

var operatorNames = []string{
	"absorb", "rectangle", "ring", "grow", "carve",
	"simplify", "wash", "join", "drop", "restack",
}

func (s *world) choose(ctx context.Context, lefts []leftover) (formPick, error) {
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
	for _, op := range s.worldOps() {
		if op.Applies() {
			jobs = append(jobs, job{op: op})
		}
	}
	type named struct {
		pick    formPick
		elapsed time.Duration
	}
	bestByName := make(map[string]*named, len(operatorNames))
	var mu sync.Mutex
	best := nonePick()
	g, _ := errgroup.WithContext(ctx)
	for _, job := range jobs {
		job := job
		g.Go(func() error {
			started := time.Now()
			p, err := job.op.Run()
			if err != nil {
				return err
			}
			if p.ok && job.bound {
				p.island = job.left.island
			}
			elapsed := time.Since(started)
			mu.Lock()
			defer mu.Unlock()
			st := bestByName[job.op.Name()]
			if st == nil {
				st = &named{}
				bestByName[job.op.Name()] = st
			}
			if elapsed > st.elapsed {
				st.elapsed = elapsed
			}
			if p.ok && (!st.pick.ok || p.a < st.pick.a) {
				st.pick = p
			}
			if p.ok && (!best.ok || p.a < best.a) {
				best = p
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nonePick(), err
	}
	for _, name := range operatorNames {
		if st, ok := bestByName[name]; ok {
			s.logCandidate(name, st.elapsed, st.pick)
		}
	}
	return best, nil
}
