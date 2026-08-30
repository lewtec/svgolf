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

func (s *world) operators(left leftover) []Operator {
	var buckets [][]pix
	if s.paths > 0 {
		buckets = fillBuckets(s.owner, s.w, s.paths, nil)
	}
	return []Operator{
		Rectangle{world: s, left: left},
		&Grow{world: s, left: left},
		&Carve{world: s, left: left},
		Simplify{world: s, buckets: buckets},
		&Wash{world: s, buckets: buckets},
		&Join{world: s, buckets: buckets},
		Drop{world: s},
	}
}

func (s *world) choose(ctx context.Context, left leftover) (formPick, error) {
	var mu sync.Mutex
	best := nonePick()
	g, _ := errgroup.WithContext(ctx)
	for _, op := range s.operators(left) {
		if !op.Applies() {
			continue
		}
		op := op
		g.Go(func() error {
			started := time.Now()
			p, err := op.Run()
			s.logCandidate(op.Name(), time.Since(started), p)
			if err != nil {
				return err
			}
			if !p.ok {
				return nil
			}
			mu.Lock()
			defer mu.Unlock()
			if !best.ok || p.a < best.a {
				best = p
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nonePick(), err
	}
	return best, nil
}
