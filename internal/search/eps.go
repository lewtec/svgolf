package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Eps is the simplest-that-fits Search: cut RMSE to loss.Eps, then minimize Parts.
// Phase 1 adds blob plates (and residual rects) only to drop RMSE.
// Phase 2 deletes or merges while RMSE stays ≤ loss.Eps.
// Accept uses loss.EpsFit: 1+RMSE/255 while over the threshold, else k.
type Eps struct {
	Colors  int // 0 = escalate 1,2,4,8 until quant RMSE ≤ Eps
	Renders int // Render calls used by the last Search
}

var _ Search = (*Eps)(nil)

func init() {
	Register("eps", func() Search { return &Eps{} })
}

const (
	epsRenderBudget = 200
	epsMaxKids      = 4096
	epsPhase1Cap    = 40 // stay under Residual #9's 50 when RMSE cannot hit Eps
)

func (e *Eps) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if e == nil {
		return svg.Document{}, fmt.Errorf("search: nil Eps")
	}
	e.Renders = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	want := FitCanvas(FromImage(target), MaxCanvas)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return doc, nil
	}
	cmap, pal, err := choosePalette(want, e.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}

	s := newEpsSess(ctx, want)
	if s.nScored == 0 {
		e.Renders = s.used
		return doc, nil
	}

	// One dominant full-canvas plate. If that already fits, it is the simplest tree.
	// Full canvas avoids bbox-edge AA on interior marks (launcher / lewtec).
	plate := rectNode(0, 0, w, h, pal[0])
	s.paint(plate, true)
	kids := []svg.Node{plate}
	if s.rmse() <= loss.Eps {
		e.Renders = s.used
		return doc.Append(kids...), nil
	}

	// Phase 1: add blob plates (largest first) only to cut RMSE.
	blobs := colorBlobs(want, cmap, pal, speckleMin(w, h))
	for _, bl := range blobs {
		if err := ctx.Err(); err != nil {
			e.Renders = s.used
			return doc.Append(kids...), err
		}
		if s.rmse() <= loss.Eps || len(kids) >= epsPhase1Cap {
			break
		}
		node := s.bestBlobNode(bl)
		if node.Kind() == svg.KindInvalid {
			continue
		}
		s.paint(node, true)
		kids = append(kids, node)
	}

	// Residual rects only when blobs were scarce (photos, not outline soup).
	if s.rmse() > loss.Eps && len(kids) < 12 {
		kids = s.residual(kids, cmap)
	}

	// Phase 2: RMSE ≤ Eps → EpsFit is k. Delete, then merge, while it still fits.
	if s.rmse() <= loss.Eps && len(kids) > 1 {
		kids = s.simplify(kids)
	}
	if len(kids) > epsMaxKids {
		kids = kids[:epsMaxKids]
	}
	e.Renders = s.used
	return doc.Append(kids...), nil
}

func choosePalette(want *image.NRGBA, colors int) (palette.ColorMap, []color.NRGBA, error) {
	if colors > 0 {
		return palette.Auto(want, colors)
	}
	var (
		bestC palette.ColorMap
		bestP []color.NRGBA
	)
	for _, n := range []int{1, 2, 4, 8} {
		cmap, pal, err := palette.Auto(want, n)
		if err != nil {
			return nil, nil, err
		}
		bestC, bestP = cmap, pal
		if quantRMSE(want, cmap) <= loss.Eps {
			return cmap, pal, nil
		}
	}
	return bestC, bestP, nil
}

func quantRMSE(want *image.NRGBA, cmap palette.ColorMap) float64 {
	got := image.NewNRGBA(want.Rect)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := want.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			m := cmap.Map(c)
			m.A = 255
			got.SetNRGBA(x, y, m)
		}
	}
	return loss.RMSE(got, want)
}

type epsSess struct {
	ctx     context.Context
	want    *image.NRGBA
	got     *image.NRGBA
	w, h    int
	nScored int
	sse     float64
	left    int
	used    int
}

func newEpsSess(ctx context.Context, want *image.NRGBA) *epsSess {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	s := &epsSess{
		ctx:  ctx,
		want: want,
		got:  image.NewNRGBA(image.Rect(0, 0, w, h)),
		w:    w,
		h:    h,
		left: epsRenderBudget,
	}
	for y := 0; y < h; y++ {
		off := y * want.Stride
		for x := 0; x < w; x++ {
			i := off + 4*x
			if want.Pix[i+3] == 0 {
				continue
			}
			s.nScored++
			r, g, b := uint32(want.Pix[i]), uint32(want.Pix[i+1]), uint32(want.Pix[i+2])
			s.sse += float64(r*r + g*g + b*b)
		}
	}
	return s
}

func (s *epsSess) rmse() float64 {
	if s.nScored == 0 {
		return 0
	}
	return math.Sqrt(s.sse / (3 * float64(s.nScored)))
}

// bestBlobNode picks the rect/circle/ellipse for bl with the lowest SSE drop.
func (s *epsSess) bestBlobNode(bl blob) svg.Node {
	W, H := float64(s.w), float64(s.h)
	cands := []svg.Node{
		seedRect(bl),
		seedCircle(bl, W, H),
		seedEllipse(bl, W, H),
	}
	var best svg.Node
	var bestD float64
	found := false
	for _, n := range cands {
		if n.Kind() == svg.KindInvalid {
			continue
		}
		d := s.paint(n, false)
		if d < 0 && (!found || d < bestD) {
			best, bestD, found = n, d, true
		}
	}
	return best
}

func (s *epsSess) residual(kids []svg.Node, cmap palette.ColorMap) []svg.Node {
	// A few high-error rects. Local paint; no Render.
	for n := 0; n < 8 && s.rmse() > loss.Eps && len(kids) < epsPhase1Cap; n++ {
		if s.ctx.Err() != nil {
			break
		}
		x, y, ok := s.hottest()
		if !ok {
			break
		}
		c := s.want.NRGBAAt(x, y)
		fill := cmap.Map(c)
		fill.A = 255
		best := svg.Node{}
		var bestD float64
		found := false
		for _, span := range []int{2, 4, 8, 16, 32, 64} {
			nx := x - span/2
			ny := y - span/2
			if nx < 0 {
				nx = 0
			}
			if ny < 0 {
				ny = 0
			}
			nw, nh := span, span
			if nx+nw > s.w {
				nw = s.w - nx
			}
			if ny+nh > s.h {
				nh = s.h - ny
			}
			if nw < 1 || nh < 1 {
				continue
			}
			node := rectNode(nx, ny, nw, nh, fill)
			d := s.paint(node, false)
			if d < 0 && (!found || d < bestD) {
				best, bestD, found = node, d, true
			}
		}
		if !found {
			break
		}
		s.paint(best, true)
		kids = append(kids, best)
	}
	return kids
}

func (s *epsSess) hottest() (int, int, bool) {
	bestE := 0
	bx, by := 0, 0
	found := false
	for y := 0; y < s.h; y++ {
		woff := y * s.want.Stride
		goff := y * s.got.Stride
		for x := 0; x < s.w; x++ {
			wi := woff + 4*x
			if s.want.Pix[wi+3] == 0 {
				continue
			}
			gi := goff + 4*x
			e := int(u8sq(s.got.Pix[gi], s.want.Pix[wi]) + u8sq(s.got.Pix[gi+1], s.want.Pix[wi+1]) + u8sq(s.got.Pix[gi+2], s.want.Pix[wi+2]))
			if e > bestE {
				bestE, bx, by, found = e, x, y, true
			}
		}
	}
	return bx, by, found && bestE > 0
}

func (s *epsSess) simplify(kids []svg.Node) []svg.Node {
	kids = s.deleteWhileFit(kids)
	return s.mergeWhileFit(kids)
}

func (s *epsSess) deleteWhileFit(kids []svg.Node) []svg.Node {
	if len(kids) < 2 {
		return kids
	}
	// Smallest first: speckles are the first things a fit can drop.
	order := make([]int, len(kids))
	for i := range kids {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		ai := nodeArea(kids[order[i]])
		aj := nodeArea(kids[order[j]])
		if ai != aj {
			return ai < aj
		}
		return order[i] > order[j]
	})
	removed := make([]bool, len(kids))
	alive := len(kids)
	for _, i := range order {
		if alive < 2 || s.ctx.Err() != nil {
			break
		}
		trial := make([]svg.Node, 0, alive-1)
		for j, n := range kids {
			if j == i || removed[j] {
				continue
			}
			trial = append(trial, n)
		}
		if s.fits(trial) {
			removed[i] = true
			alive--
		}
	}
	out := make([]svg.Node, 0, alive)
	for i, n := range kids {
		if !removed[i] {
			out = append(out, n)
		}
	}
	s.replay(out)
	return out
}

func (s *epsSess) mergeWhileFit(kids []svg.Node) []svg.Node {
	for s.ctx.Err() == nil {
		type pair struct{ i, j, waste int }
		var pairs []pair
		for i := 0; i < len(kids); i++ {
			ri, ok := asRect(kids[i])
			if !ok {
				continue
			}
			for j := i + 1; j < len(kids); j++ {
				rj, ok := asRect(kids[j])
				if !ok || ri.fill != rj.fill || !rectsTouch(ri, rj) {
					continue
				}
				pairs = append(pairs, pair{i: i, j: j, waste: unionWaste(ri, rj)})
			}
		}
		if len(pairs) == 0 {
			return kids
		}
		sort.Slice(pairs, func(a, b int) bool {
			if pairs[a].waste != pairs[b].waste {
				return pairs[a].waste < pairs[b].waste
			}
			return pairs[a].i < pairs[b].i
		})
		merged := false
		for _, p := range pairs {
			ri, _ := asRect(kids[p.i])
			rj, _ := asRect(kids[p.j])
			u := unionOf(ri, rj)
			trial := make([]svg.Node, 0, len(kids)-1)
			for k, n := range kids {
				if k == p.i || k == p.j {
					continue
				}
				trial = append(trial, n)
			}
			trial = append(trial, u.node())
			if s.fits(trial) {
				kids = trial
				merged = true
				break
			}
		}
		if !merged {
			return kids
		}
	}
	return kids
}

// fits is true when trial still has RMSE ≤ Eps. Prefers a real Render so AA
// matches OfEps; falls back to local opaque paint when the budget is gone.
func (s *epsSess) fits(trial []svg.Node) bool {
	if s.ctx.Err() != nil {
		return false
	}
	if sc, ok := s.renderRMSE(trial); ok {
		return sc <= loss.Eps
	}
	saved := cloneNRGBA(s.got)
	savedSSE := s.sse
	s.replay(trial)
	ok := s.rmse() <= loss.Eps
	s.got = saved
	s.sse = savedSSE
	return ok
}

func (s *epsSess) renderRMSE(kids []svg.Node) (float64, bool) {
	if s.left <= 0 || s.ctx.Err() != nil {
		return 0, false
	}
	doc := svg.NewDocument(float64(s.w), float64(s.h)).WithViewBox(0, 0, float64(s.w), float64(s.h)).Append(kids...)
	got, err := render.Render(doc)
	s.left--
	s.used++
	if err != nil {
		return math.Inf(1), true
	}
	return loss.RMSE(got, s.want), true
}

func (s *epsSess) replay(kids []svg.Node) {
	s.got = image.NewNRGBA(image.Rect(0, 0, s.w, s.h))
	s.sse = emptySSE(s.want)
	for _, n := range kids {
		s.paint(n, true)
	}
}

// paint is opaque source-over SSE change on scored pixels. commit writes got.
func (s *epsSess) paint(n svg.Node, commit bool) float64 {
	fill, ok := nodeFill(n)
	if !ok {
		return 0
	}
	x0, y0, x1, y1 := nodeBBox(n, s.w, s.h)
	var delta float64
	for y := y0; y < y1; y++ {
		woff := y * s.want.Stride
		goff := y * s.got.Stride
		for x := x0; x < x1; x++ {
			wi := woff + 4*x
			if s.want.Pix[wi+3] == 0 {
				continue
			}
			if !covers(n, float64(x)+0.5, float64(y)+0.5) {
				continue
			}
			gi := goff + 4*x
			delta -= float64(u8sq(s.got.Pix[gi], s.want.Pix[wi]) + u8sq(s.got.Pix[gi+1], s.want.Pix[wi+1]) + u8sq(s.got.Pix[gi+2], s.want.Pix[wi+2]))
			delta += float64(u8sq(fill.R, s.want.Pix[wi]) + u8sq(fill.G, s.want.Pix[wi+1]) + u8sq(fill.B, s.want.Pix[wi+2]))
			if commit {
				s.got.Pix[gi] = fill.R
				s.got.Pix[gi+1] = fill.G
				s.got.Pix[gi+2] = fill.B
				s.got.Pix[gi+3] = 255
			}
		}
	}
	if commit {
		s.sse += delta
	}
	return delta
}

func rectNode(x, y, w, h int, fill color.NRGBA) svg.Node {
	r := svg.NewRect().WithX(float64(x)).WithY(float64(y)).WithWidth(float64(w)).WithHeight(float64(h))
	return applyFill(r.Node(), fill)
}

func applyFill(n svg.Node, c color.NRGBA) svg.Node {
	col := color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
	fade := c.A != 255
	op := float64(c.A) / 255
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		r = r.WithFill(col)
		if fade {
			r = r.WithFillOpacity(op)
		}
		return r.Node()
	case svg.KindCircle:
		cir, _ := n.Circle()
		cir = cir.WithFill(col)
		if fade {
			cir = cir.WithFillOpacity(op)
		}
		return cir.Node()
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		e = e.WithFill(col)
		if fade {
			e = e.WithFillOpacity(op)
		}
		return e.Node()
	case svg.KindPolygon:
		p, _ := n.Polygon()
		p = p.WithFill(col)
		if fade {
			p = p.WithFillOpacity(op)
		}
		return p.Node()
	default:
		return n
	}
}

func nodeFill(n svg.Node) (color.NRGBA, bool) {
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		return r.Fill()
	case svg.KindCircle:
		c, _ := n.Circle()
		return c.Fill()
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		return e.Fill()
	case svg.KindPolygon:
		p, _ := n.Polygon()
		return p.Fill()
	default:
		return color.NRGBA{}, false
	}
}

func nodeBBox(n svg.Node, W, H int) (x0, y0, x1, y1 int) {
	x0, y0, x1, y1 = W, H, 0, 0
	grow := func(x, y float64) {
		ix, iy := int(math.Floor(x)), int(math.Floor(y))
		if ix < x0 {
			x0 = ix
		}
		if iy < y0 {
			y0 = iy
		}
		if ix+1 > x1 {
			x1 = ix + 1
		}
		if iy+1 > y1 {
			y1 = iy + 1
		}
	}
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		grow(r.X(), r.Y())
		grow(r.X()+r.Width(), r.Y()+r.Height())
	case svg.KindCircle:
		c, _ := n.Circle()
		grow(c.CX()-c.R(), c.CY()-c.R())
		grow(c.CX()+c.R(), c.CY()+c.R())
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		grow(e.CX()-e.RX(), e.CY()-e.RY())
		grow(e.CX()+e.RX(), e.CY()+e.RY())
	case svg.KindPolygon:
		p, _ := n.Polygon()
		for _, pt := range p.Points() {
			grow(pt[0], pt[1])
		}
	default:
		return 0, 0, 0, 0
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > W {
		x1 = W
	}
	if y1 > H {
		y1 = H
	}
	if x0 >= x1 || y0 >= y1 {
		return 0, 0, 0, 0
	}
	return x0, y0, x1, y1
}

func nodeArea(n svg.Node) float64 {
	x0, y0, x1, y1 := nodeBBox(n, 1<<20, 1<<20)
	return float64((x1 - x0) * (y1 - y0))
}

func covers(n svg.Node, x, y float64) bool {
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		return x >= r.X() && x < r.X()+r.Width() && y >= r.Y() && y < r.Y()+r.Height()
	case svg.KindCircle:
		c, _ := n.Circle()
		dx, dy := x-c.CX(), y-c.CY()
		return dx*dx+dy*dy <= c.R()*c.R()
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		if e.RX() <= 0 || e.RY() <= 0 {
			return false
		}
		dx, dy := (x-e.CX())/e.RX(), (y-e.CY())/e.RY()
		return dx*dx+dy*dy <= 1
	case svg.KindPolygon:
		p, _ := n.Polygon()
		return polyContains(p.Points(), x, y)
	default:
		return false
	}
}

func polyContains(pts [][2]float64, x, y float64) bool {
	wn := 0
	n := len(pts)
	for i := 0; i < n; i++ {
		x1, y1 := pts[i][0], pts[i][1]
		j := i + 1
		if j == n {
			j = 0
		}
		x2, y2 := pts[j][0], pts[j][1]
		if y1 <= y {
			if y2 > y && isLeft(x1, y1, x2, y2, x, y) > 0 {
				wn++
			}
		} else if y2 <= y && isLeft(x1, y1, x2, y2, x, y) < 0 {
			wn--
		}
	}
	return wn != 0
}

func isLeft(x1, y1, x2, y2, x, y float64) float64 {
	return (x2-x1)*(y-y1) - (x-x1)*(y2-y1)
}

func speckleMin(w, h int) int {
	return max(32, w*h/5000)
}

type blob struct {
	x0, y0, bw, bh int
	pix            []bool
	n              int
	fill           color.NRGBA
	colorN         int
}

func (b blob) at(x, y int) bool {
	lx, ly := x-b.x0, y-b.y0
	if lx < 0 || ly < 0 || lx >= b.bw || ly >= b.bh {
		return false
	}
	return b.pix[ly*b.bw+lx]
}

func colorBlobs(want *image.NRGBA, cmap palette.ColorMap, pal []color.NRGBA, speckle int) []blob {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	idx := make([]int16, w*h)
	counts := make([]int, len(pal))
	type rgb struct{ r, g, b uint8 }
	m := make(map[rgb]int16, len(pal))
	for i, c := range pal {
		m[rgb{c.R, c.G, c.B}] = int16(i)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := want.NRGBAAt(x, y)
			if c.A == 0 {
				idx[y*w+x] = -1
				continue
			}
			mp := cmap.Map(c)
			i, ok := m[rgb{mp.R, mp.G, mp.B}]
			if !ok {
				idx[y*w+x] = -1
				continue
			}
			idx[y*w+x] = i
			counts[i]++
		}
	}
	visited := make([]bool, w*h)
	var blobs []blob
	var biggest blob
	dirs := [4]int{1, -1, w, -w}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			start := y*w + x
			if visited[start] || idx[start] < 0 {
				continue
			}
			lab := idx[start]
			q := []int{start}
			visited[start] = true
			minX, minY, maxX, maxY := x, y, x+1, y+1
			hist := map[color.NRGBA]int{}
			for qi := 0; qi < len(q); qi++ {
				p := q[qi]
				px, py := p%w, p/w
				hist[want.NRGBAAt(px, py)]++
				if px < minX {
					minX = px
				}
				if py < minY {
					minY = py
				}
				if px+1 > maxX {
					maxX = px + 1
				}
				if py+1 > maxY {
					maxY = py + 1
				}
				for _, d := range dirs {
					n := p + d
					if d == 1 && px+1 >= w {
						continue
					}
					if d == -1 && px <= 0 {
						continue
					}
					if n < 0 || n >= w*h || visited[n] || idx[n] != lab {
						continue
					}
					visited[n] = true
					q = append(q, n)
				}
			}
			bw, bh := maxX-minX, maxY-minY
			pix := make([]bool, bw*bh)
			for _, p := range q {
				pix[(p/w-minY)*bw+(p%w-minX)] = true
			}
			fill := pal[lab]
			fillN := 0
			for c, n := range hist {
				if n > fillN || (n == fillN && lessNRGBA(c, fill)) {
					fill, fillN = c, n
				}
			}
			bl := blob{
				x0: minX, y0: minY, bw: bw, bh: bh,
				pix: pix, n: len(q), fill: fill, colorN: counts[lab],
			}
			if bl.n > biggest.n {
				biggest = bl
			}
			if bl.n < speckle {
				continue
			}
			blobs = append(blobs, bl)
		}
	}
	if len(blobs) == 0 && biggest.n > 0 {
		blobs = []blob{biggest}
	}
	sort.Slice(blobs, func(i, j int) bool {
		if blobs[i].n != blobs[j].n {
			return blobs[i].n > blobs[j].n
		}
		if blobs[i].colorN != blobs[j].colorN {
			return blobs[i].colorN > blobs[j].colorN
		}
		if blobs[i].y0 != blobs[j].y0 {
			return blobs[i].y0 < blobs[j].y0
		}
		return blobs[i].x0 < blobs[j].x0
	})
	return blobs
}

func lessNRGBA(a, b color.NRGBA) bool {
	if a.R != b.R {
		return a.R < b.R
	}
	if a.G != b.G {
		return a.G < b.G
	}
	if a.B != b.B {
		return a.B < b.B
	}
	return a.A < b.A
}

func seedRect(b blob) svg.Node {
	r := svg.NewRect().WithX(float64(b.x0)).WithY(float64(b.y0)).
		WithWidth(float64(b.bw)).WithHeight(float64(b.bh))
	return applyFill(r.Node(), b.fill)
}

func seedCircle(b blob, W, H float64) svg.Node {
	w := float64(b.bw)
	h := float64(b.bh)
	cx := float64(b.x0) + w/2
	cy := float64(b.y0) + h/2
	r := math.Min(w, h) / 2
	r = clampRad(r, cx, cy, W, H)
	if r <= 0 {
		return svg.Node{}
	}
	cir := svg.NewCircle().WithCX(cx).WithCY(cy).WithR(r)
	return applyFill(cir.Node(), b.fill)
}

func seedEllipse(b blob, W, H float64) svg.Node {
	w := float64(b.bw)
	h := float64(b.bh)
	cx := float64(b.x0) + w/2
	cy := float64(b.y0) + h/2
	rx := clampAxis(w/2, cx, W)
	ry := clampAxis(h/2, cy, H)
	if rx <= 0 || ry <= 0 {
		return svg.Node{}
	}
	e := svg.NewEllipse().WithCX(cx).WithCY(cy).WithRX(rx).WithRY(ry)
	return applyFill(e.Node(), b.fill)
}

func clampRad(r, cx, cy, W, H float64) float64 {
	r = math.Min(r, cx)
	r = math.Min(r, cy)
	r = math.Min(r, W-cx)
	r = math.Min(r, H-cy)
	if r < 0 {
		return 0
	}
	return r
}

func clampAxis(rad, c, lim float64) float64 {
	rad = math.Min(rad, c)
	rad = math.Min(rad, lim-c)
	if rad < 0 {
		return 0
	}
	return rad
}

type epsRect struct {
	x, y, w, h int
	fill       color.NRGBA
}

func asRect(n svg.Node) (epsRect, bool) {
	if n.Kind() != svg.KindRect {
		return epsRect{}, false
	}
	r, _ := n.Rect()
	fill, ok := r.Fill()
	if !ok {
		return epsRect{}, false
	}
	return epsRect{
		x: int(math.Round(r.X())), y: int(math.Round(r.Y())),
		w: int(math.Round(r.Width())), h: int(math.Round(r.Height())),
		fill: fill,
	}, r.RX() == 0 && r.RY() == 0 && r.Width() > 0 && r.Height() > 0
}

func (p epsRect) node() svg.Node {
	return rectNode(p.x, p.y, p.w, p.h, p.fill)
}

func (p epsRect) area() int { return p.w * p.h }

func rectsTouch(a, b epsRect) bool {
	return a.x <= b.x+b.w && b.x <= a.x+a.w && a.y <= b.y+b.h && b.y <= a.y+a.h
}

func unionOf(a, b epsRect) epsRect {
	x0 := min(a.x, b.x)
	y0 := min(a.y, b.y)
	x1 := max(a.x+a.w, b.x+b.w)
	y1 := max(a.y+a.h, b.y+b.h)
	return epsRect{x: x0, y: y0, w: x1 - x0, h: y1 - y0, fill: a.fill}
}

func overlapArea(a, b epsRect) int {
	x0 := max(a.x, b.x)
	y0 := max(a.y, b.y)
	x1 := min(a.x+a.w, b.x+b.w)
	y1 := min(a.y+a.h, b.y+b.h)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	return (x1 - x0) * (y1 - y0)
}

func unionWaste(a, b epsRect) int {
	return unionOf(a, b).area() - a.area() - b.area() + overlapArea(a, b)
}

func emptySSE(want *image.NRGBA) float64 {
	var sum float64
	w, h := want.Rect.Dx(), want.Rect.Dy()
	for y := 0; y < h; y++ {
		off := y * want.Stride
		for x := 0; x < w; x++ {
			i := off + 4*x
			if want.Pix[i+3] == 0 {
				continue
			}
			r, g, b := uint32(want.Pix[i]), uint32(want.Pix[i+1]), uint32(want.Pix[i+2])
			sum += float64(r*r + g*g + b*b)
		}
	}
	return sum
}

func u8sq(a, b uint8) uint32 {
	var d uint32
	if a >= b {
		d = uint32(a - b)
	} else {
		d = uint32(b - a)
	}
	return d * d
}

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst
}
