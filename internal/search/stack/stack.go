package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"iter"
	"time"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

const (
	maxPaths  = 512
	minIsland = 8
	minErr    = 8
)

// Stack scores every applicable operator on the hottest leftover
// (and merge/drop) and keeps the best Score. Expand: grow hull, new
// hull, leftover ring. Contract: merge to a 2-stop, punch paper,
// cubics/linear refit, drop. Want stays native.
type Stack struct{}

var _ search.Search = Stack{}

func init() {
	search.Register("stack", func() search.Search { return Stack{} })
}

// world is the accepted document and the pixmap it paints.
// leftover is this epoch's hottest miss. grow is one existing
// path union that leftover. formPick is one scored operator.
type world struct {
	want, got   *image.NRGBA
	wantP, gotP *loss.Plane
	doc         svg.Document
	skip        []byte
	owner       []uint16
	fills       []color.NRGBA
	scratch     scratch
	errSum      float64
	paths       int
	w, h        int
}

// leftover is the hottest residual blob and the paths that already
// touch it. paper leftovers punch; others expand or refine.
type leftover struct {
	island []pix
	col    color.NRGBA
	paper  bool
	grows  []grow
}

type grow struct {
	i    int
	work []pix
	fill color.NRGBA
}

type formPick struct {
	doc      svg.Document
	got      *image.NRGBA
	errSum   float64
	a        float64
	replace  int
	work     []pix
	fill     color.NRGBA
	reclaims [][]pix
	dropIdx  int
	mergeJ   int
	ok       bool
}

func nonePick() formPick {
	return formPick{replace: -1, dropIdx: -1, mergeJ: -1}
}

func newWorld(target *image.NRGBA) (*world, error) {
	if target == nil {
		return nil, fmt.Errorf("search: nil pixmap")
	}
	b := target.Bounds()
	w, h := b.Dx(), b.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	doc = doc.Append(whitePane(w, h).Node())
	got, err := render.Render(doc)
	if err != nil {
		return nil, err
	}
	wantP := loss.NewPlane(target)
	gotP := loss.NewPlane(got)
	wantP.Ensure()
	gotP.Ensure()
	return &world{
		want:   target,
		got:    got,
		wantP:  wantP,
		gotP:   gotP,
		doc:    doc,
		skip:   make([]byte, w*h),
		owner:  make([]uint16, w*h),
		w:      w,
		h:      h,
		errSum: ScoreOn(gotP, wantP, 0),
	}, nil
}

func (Stack) Search(ctx context.Context, target *image.NRGBA) iter.Seq2[search.Epoch, error] {
	return func(yield func(search.Epoch, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(search.Epoch{}, err)
			return
		}
		s, err := newWorld(target)
		if err != nil {
			yield(search.Epoch{}, err)
			return
		}
		started := time.Now()
		emit := func(phase string) bool {
			ep := epochOf(s.doc, phase)
			ep.Elapsed = time.Since(started)
			started = time.Now()
			return yield(ep, nil)
		}
		yielded := false
		for {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(search.Epoch{}, err)
				}
				return
			}
			left := s.leftover()
			pick, phase, err := s.choose(left)
			if err != nil {
				yield(search.Epoch{}, err)
				return
			}
			if !pick.ok {
				if !left.big() {
					break
				}
				s.ignore(left)
				continue
			}
			s.apply(pick)
			yielded = true
			if !emit(phase) {
				return
			}
		}
		if !yielded {
			emit("")
		}
	}
}

func epochOf(doc svg.Document, phase string) search.Epoch {
	return search.Epoch{Document: doc, Scale: 1, Phase: phase}
}

func (left leftover) big() bool {
	return len(left.island) >= minIsland
}

func (s *world) leftover() leftover {
	col, island := s.hottest()
	left := leftover{island: island, col: col, paper: len(island) >= minIsland && paperLeftover(col)}
	if left.big() {
		left.grows = s.connecting(island)
	}
	return left
}

func (s *world) currentScore() float64 {
	return s.errSum + pathCost*float64(s.paths) + cmdCost*float64(docCmdLen(s.doc))
}

func (s *world) ignore(left leftover) {
	markSkip(s.skip, left.island, s.w)
}

func (s *world) apply(pick formPick) {
	s.doc, s.got, s.errSum = pick.doc, pick.got, pick.errSum
	if pick.dropIdx >= 0 {
		dropOwner(s.owner, uint16(pick.dropIdx+1), s.paths)
		s.fills = append(s.fills[:pick.dropIdx], s.fills[pick.dropIdx+1:]...)
		s.paths--
	} else if pick.mergeJ >= 0 {
		j := pick.mergeJ
		i := pick.replace
		for k, v := range s.owner {
			if v == uint16(j+1) {
				s.owner[k] = uint16(i + 1)
			}
		}
		dropOwner(s.owner, uint16(j+1), s.paths)
		s.fills[i] = pick.fill
		s.fills = append(s.fills[:j], s.fills[j+1:]...)
		clearOwner(s.owner, uint16(i+1))
		claim(s.owner, pick.work, s.w, uint16(i+1))
		s.paths--
	} else if len(pick.reclaims) > 0 {
		for i, work := range pick.reclaims {
			if work == nil {
				continue
			}
			id := uint16(i + 1)
			clearOwner(s.owner, id)
			claim(s.owner, work, s.w, id)
		}
	} else if pick.replace >= 0 {
		id := uint16(pick.replace + 1)
		clearOwner(s.owner, id)
		claim(s.owner, pick.work, s.w, id)
		s.fills[pick.replace] = pick.fill
	} else {
		claim(s.owner, pick.work, s.w, uint16(s.paths+1))
		s.fills = append(s.fills, pick.fill)
		s.paths++
	}
	if s.gotP == nil {
		s.gotP = loss.NewPlane(s.got)
	} else {
		s.gotP.Reset(s.got)
	}
	s.gotP.Ensure()
}

// choose scores every applicable operator and keeps the lowest Score.
func (s *world) choose(left leftover) (formPick, string, error) {
	best := nonePick()
	phase := ""
	take := func(p formPick, ph string) {
		if !p.ok {
			return
		}
		if !best.ok || p.a < best.a {
			best = p
			phase = ph
		}
	}
	if left.big() && !left.paper && s.paths < maxPaths {
		pick, err := s.pickForm(left, false)
		if err != nil {
			return nonePick(), "", err
		}
		take(pick, "expand")
	}
	if left.big() && s.paths > 0 && !left.paper {
		pick, err := s.pickForm(left, true)
		if err != nil {
			return nonePick(), "", err
		}
		take(pick, "contract")
	}
	if left.paper && s.paths > 0 {
		pick, err := s.punch(left)
		if err != nil {
			return nonePick(), "", err
		}
		take(pick, "contract")
	}
	if s.paths >= 2 {
		pick, err := s.mergeLinear()
		if err != nil {
			return nonePick(), "", err
		}
		take(pick, "contract")
		pick, err = s.drop()
		if err != nil {
			return nonePick(), "", err
		}
		take(pick, "contract")
	}
	return best, phase, nil
}

func (s *world) connecting(island []pix) []grow {
	var out []grow
	for i := range s.fills {
		work := ownedUnion(s.owner, island, s.w, s.h, uint16(i+1), s.scratch.seen)
		if len(work) <= len(island) && !s.paintsIsland(s.doc.Children()[i+1], island) {
			continue
		}
		out = append(out, grow{i: i, work: work, fill: meanFill(s.want, work)})
	}
	return out
}

// pickForm: cover (refine=false) grows a hull or adds a hull.
// refine rewrites a connecting path (filledFit / linear). Score picks.
func (s *world) pickForm(left leftover, refine bool) (formPick, error) {
	best := nonePick()
	curA := s.currentScore()
	bestA := curA
	var bestLen int
	consider := func(work []pix, fill color.NRGBA, replace int) error {
		pick, plen, err := s.scoreForm(work, fill, replace, refine, curA)
		if err != nil || !pick.ok {
			return err
		}
		if pick.a > bestA || pick.a > curA {
			return nil
		}
		if pick.a == bestA && (!best.ok || plen >= bestLen) {
			return nil
		}
		bestA = pick.a
		bestLen = plen
		best = pick
		return nil
	}
	for _, g := range left.grows {
		if err := consider(g.work, g.fill, g.i); err != nil {
			return nonePick(), err
		}
	}
	if !refine {
		if err := consider(left.island, left.col, -1); err != nil {
			return nonePick(), err
		}
	}
	return best, nil
}

func (s *world) scoreForm(work []pix, fill color.NRGBA, replace int, refine bool, curA float64) (formPick, int, error) {
	parts := s.paths
	dirty0 := islandRect(work)
	if replace >= 0 {
		dirty0 = dirty0.Union(nodeRect(s.doc.Children()[replace+1]))
	} else {
		parts = s.paths + 1
	}
	best := nonePick()
	var bestLen int
	bestA := curA
	for _, cand := range s.formPaths(work, fill, refine, !refine && replace < 0) {
		var next svg.Document
		if replace >= 0 {
			next = replaceAt(s.doc, replace+1, cand.Node())
		} else {
			next = s.doc.Append(cand.Node())
		}
		ngot, err := render.Render(next)
		if err != nil {
			return nonePick(), 0, err
		}
		dirty := dirty0.Union(nodeRect(cand.Node())).Inset(-2)
		ngotP := loss.NewPlane(ngot)
		nerr := s.errSum + ScoreRectOn(ngotP, s.wantP, dirty) - ScoreRectOn(s.gotP, s.wantP, dirty)
		plen := pathLen(cand.Node())
		cmds := docCmdLen(next)
		if replace >= 0 && cand.FillRule() == svg.FillEvenOdd {
			cmds = docCmdLen(s.doc)
		}
		a := nerr + pathCost*float64(parts) + cmdCost*float64(cmds)
		if a > bestA || a > curA {
			continue
		}
		if a == bestA && (!best.ok || plen >= bestLen) {
			continue
		}
		bestA = a
		bestLen = plen
		best = formPick{doc: next, got: ngot, errSum: nerr, a: a, replace: replace, work: work, fill: fill, dropIdx: -1, mergeJ: -1, ok: true}
	}
	return best, bestLen, nil
}

// punch shrinks every path that covers a paper leftover so the hole
// opens to the pane. Punching only the top layer reveals the plate
// underneath and Score gets worse.
func (s *world) paintsIsland(node svg.Node, island []pix) bool {
	if !nodeRect(node).Overlaps(islandRect(island)) {
		return false
	}
	d := svg.NewDocument(float64(s.w), float64(s.h)).WithViewBox(0, 0, float64(s.w), float64(s.h))
	d = d.Append(whitePane(s.w, s.h).Node()).Append(node)
	img, err := render.Render(d)
	if err != nil {
		return false
	}
	for _, p := range island {
		if colorErr(img.NRGBAAt(p.x, p.y), paper) > minErr {
			return true
		}
	}
	return false
}

func (s *world) punch(left leftover) (formPick, error) {
	curA := s.currentScore()
	next := s.doc
	reclaims := make([][]pix, s.paths)
	any := false
	for i := 0; i < s.paths; i++ {
		if !ownsAny(s.owner, left.island, s.w, uint16(i+1)) && !s.paintsIsland(s.doc.Children()[i+1], left.island) {
			continue
		}
		work := ownedMinus(s.owner, left.island, s.w, uint16(i+1), s.scratch.seen)
		ring := convexHull(islandPoints(work))
		if len(ring) < 3 {
			continue
		}
		// Punch only this leftover. holeRings(work) would also
		// carve every other void in the plate (trees, scale blocks).
		cand := filledPath(ring, s.fills[i])
		if hole := convexHull(islandPoints(left.island)); len(hole) >= 3 {
			cand = withHoles(cand, [][][2]float64{hole})
		}
		next = replaceAt(next, i+1, cand.Node())
		reclaims[i] = work
		any = true
	}
	if !any {
		return nonePick(), nil
	}
	ngot, err := render.Render(next)
	if err != nil {
		return nonePick(), err
	}
	nerr := Score(ngot, s.want, 0)
	a := nerr + pathCost*float64(s.paths) + cmdCost*float64(docCmdLen(s.doc))
	if a >= curA || nerr >= s.errSum {
		return nonePick(), nil
	}
	return formPick{doc: next, got: ngot, errSum: nerr, a: a, replace: -1, work: left.island, reclaims: reclaims, dropIdx: -1, mergeJ: -1, ok: true}, nil
}

func fillBuckets(owner []uint16, w, n int, buckets [][]pix) [][]pix {
	if cap(buckets) < n {
		buckets = make([][]pix, n)
	} else {
		buckets = buckets[:n]
		for i := range buckets {
			buckets[i] = buckets[i][:0]
		}
	}
	for i, v := range owner {
		if v == 0 || int(v) > n {
			continue
		}
		id := int(v) - 1
		buckets[id] = append(buckets[id], pix{i % w, i / w})
	}
	return buckets
}

func (s *world) mergeLinear() (formPick, error) {
	best := nonePick()
	if s.paths < 2 {
		return best, nil
	}
	s.scratch.buckets = fillBuckets(s.owner, s.w, s.paths, s.scratch.buckets)
	curA := s.currentScore()
	for i := 0; i < s.paths; i++ {
		for j := i + 1; j < s.paths; j++ {
			need := len(s.scratch.buckets[i]) + len(s.scratch.buckets[j])
			if need < minIsland {
				continue
			}
			s.scratch.work = s.scratch.work[:0]
			s.scratch.work = append(s.scratch.work, s.scratch.buckets[i]...)
			s.scratch.work = append(s.scratch.work, s.scratch.buckets[j]...)
			gradient, ok := fitLinearFill(s.scratch.work, s.want)
			if !ok {
				continue
			}
			ring := convexHull(islandPoints(s.scratch.work))
			if len(ring) < 3 {
				continue
			}
			next := replaceAt(s.doc, i+1, filledPath(ring, s.fills[i]).WithLinearFill(gradient).Node())
			next = dropAt(next, j+1)
			ngot, err := render.Render(next)
			if err != nil {
				return nonePick(), err
			}
			nerr := ScoreOn(loss.NewPlane(ngot), s.wantP, 0)
			a := nerr + pathCost*float64(s.paths-1) + cmdCost*float64(docCmdLen(next))
			if a >= curA {
				continue
			}
			if best.ok && a >= best.a {
				continue
			}
			work := append([]pix{}, s.scratch.work...)
			best = formPick{doc: next, got: ngot, errSum: nerr, a: a, replace: i, work: work, fill: meanFill(s.want, work), dropIdx: -1, mergeJ: j, ok: true}
		}
	}
	return best, nil
}

func (s *world) drop() (formPick, error) {
	if s.paths < 2 {
		return nonePick(), nil
	}
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
	return formPick{doc: next, got: ngot, errSum: nerr, a: a, replace: -1, dropIdx: idx, mergeJ: -1, ok: true}, nil
}

func smallestOwner(owner []uint16, n int) (int, bool) {
	if n <= 0 {
		return 0, false
	}
	cnt := make([]int, n+1)
	for _, id := range owner {
		if id > 0 && int(id) <= n {
			cnt[id]++
		}
	}
	best, bestN := 0, -1
	for i := 1; i <= n; i++ {
		if bestN < 0 || cnt[i] < bestN {
			best, bestN = i-1, cnt[i]
		}
	}
	return best, bestN >= 0
}

func dropOwner(owner []uint16, id uint16, n int) {
	for i, v := range owner {
		switch {
		case v == id:
			owner[i] = 0
		case v > id:
			owner[i]--
		}
	}
}

func dropAt(d svg.Document, i int) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	for j, k := range kids {
		if j == i {
			continue
		}
		out = out.Append(k)
	}
	return out
}

func markSkip(skip []byte, island []pix, w int) {
	for _, p := range island {
		skip[p.y*w+p.x] = 1
	}
}

func acceptSum(err0, err1 float64, parts, nparts int, old, cand svg.Node) bool {
	a := err0 + pathCost*float64(parts)
	b := err1 + pathCost*float64(nparts)
	if b < a {
		return true
	}
	if b > a || old.Kind() == svg.KindInvalid {
		return false
	}
	return pathLen(cand) < pathLen(old)
}

func whitePane(w, h int) svg.Path {
	return filledPath([][2]float64{
		{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)},
	}, paper)
}

func (s *world) formPaths(island []pix, col color.NRGBA, refine, holes bool) []svg.Path {
	ring := convexHull(islandPoints(island))
	if len(ring) < 3 {
		return nil
	}
	out := []svg.Path{filledPath(ring, col)}
	if refine {
		out = append(out, filledFit(island, ring, col))
	}
	if holes {
		if hs := leftoverRings(island, s.got, s.want, col); len(hs) > 0 {
			// A solid hull over a ring is a cover plate. Only the
			// evenodd ring is on the menu.
			out = []svg.Path{withHoles(filledPath(ring, col), hs)}
		} else if sameColorHollow(island, s.want, col) {
			return nil
		}
	}
	// Linear is a contract of stairs, not an expand cover.
	if refine {
		if gradient, ok := fitLinearFill(island, s.want); ok {
			n := len(out)
			for i := 0; i < n; i++ {
				out = append(out, out[i].WithLinearFill(gradient))
			}
		}
	}
	return out
}

// leftoverRings are enclosed voids of this leftover that are not paper
// and are already painted. A paper hole waits for contract punch. A
// painted interior is a ring (visor on a plate). An unpainted void is
// another leftover, not a hole in this plate.
func leftoverRings(island []pix, got, want *image.NRGBA, col color.NRGBA) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		if paperLeftover(meanFill(want, h)) {
			continue
		}
		if loss.ColorAt(meanFill(want, h), col) <= minErr {
			continue
		}
		if !holePainted(got, want, h) {
			continue
		}
		r := convexHull(islandPoints(h))
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func sameColorHollow(island []pix, want *image.NRGBA, col color.NRGBA) bool {
	for _, h := range voids(island) {
		if paperLeftover(meanFill(want, h)) {
			continue
		}
		if loss.ColorAt(meanFill(want, h), col) <= minErr {
			return true
		}
	}
	return false
}

func holePainted(got, want *image.NRGBA, hole []pix) bool {
	if got == nil || want == nil || len(hole) == 0 {
		return false
	}
	w := want.Bounds().Dx()
	for _, p := range hole {
		if residual(got, want, nil, p.x, p.y, w) {
			return false
		}
	}
	return true
}

func holeRings(island []pix) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		r := convexHull(islandPoints(h))
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func withHoles(p svg.Path, holes [][][2]float64) svg.Path {
	for _, h := range holes {
		p = appendRing(p, h)
	}
	return p.WithFillRule(svg.FillEvenOdd)
}

func pathLen(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	return len(p.Commands())
}

func pathCommandWeight(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	w := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdLine {
			w += 2
			continue
		}
		w++
	}
	return w
}

func docCmdLen(d svg.Document) int {
	n := 0
	for _, c := range d.Children() {
		n += pathCommandWeight(c)
	}
	return n
}

func thinIsland(island []pix) bool {
	if len(island) == 0 {
		return false
	}
	bb := bbox(island)
	w := bb[1][0] - bb[0][0]
	h := bb[2][1] - bb[1][1]
	return w <= 1 || h <= 1
}

func appendRing(p svg.Path, ring [][2]float64) svg.Path {
	if len(ring) < 3 {
		return p
	}
	cmds := p.Commands()
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdMove, X: ring[0][0], Y: ring[0][1]})
	for _, q := range ring[1:] {
		cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: q[0], Y: q[1]})
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	p, _ = p.WithCommands(cmds)
	return p
}

func filledPath(ring [][2]float64, col color.NRGBA) svg.Path {
	return appendRing(svg.NewPath(), ring).WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
}

func islandRect(island []pix) image.Rectangle {
	if len(island) == 0 {
		return image.Rectangle{}
	}
	r := image.Rect(island[0].x, island[0].y, island[0].x+1, island[0].y+1)
	for _, p := range island[1:] {
		r = r.Union(image.Rect(p.x, p.y, p.x+1, p.y+1))
	}
	return r
}

func nodeRect(ns ...svg.Node) image.Rectangle {
	var r image.Rectangle
	for _, n := range ns {
		p, ok := n.Path()
		if !ok {
			continue
		}
		for _, c := range p.Commands() {
			q := image.Rect(int(c.X)-1, int(c.Y)-1, int(c.X)+2, int(c.Y)+2)
			if c.Kind == svg.CmdCubic {
				q = q.Union(image.Rect(int(c.X1)-1, int(c.Y1)-1, int(c.X1)+2, int(c.Y1)+2))
				q = q.Union(image.Rect(int(c.X2)-1, int(c.Y2)-1, int(c.X2)+2, int(c.Y2)+2))
			}
			if r.Empty() {
				r = q
			} else {
				r = r.Union(q)
			}
		}
	}
	return r
}

func replaceAt(d svg.Document, i int, n svg.Node) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	for j, k := range kids {
		if j == i {
			out = out.Append(n)
			continue
		}
		out = out.Append(k)
	}
	return out
}
