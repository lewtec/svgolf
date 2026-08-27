package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Components is a Search adapter: one primitive per 4-connected color blob.
// Speckles under 32 px are dropped. Color is not a gene.
type Components struct {
	Colors  int // 0 = auto, cap 8
	Renders int // Render calls used by the last Search
}

var _ Search = (*Components)(nil)

const (
	compRenderBudget = 200
	compRenderCap    = 4096
	compMaxKids      = 4096 // Encode child cap
	compSpeckle      = 32
	compMaxPolyVerts = 8
	compNudgeExtra   = 4
)

func (c *Components) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if c == nil {
		return svg.Document{}, fmt.Errorf("search: nil Components")
	}
	c.Renders = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	want := capLongEdge(origin0(target), compRenderCap)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return doc, nil
	}
	cmap, pal, err := palette.Auto(want, c.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}
	blobs := colorBlobs(want, cmap, pal)
	if len(blobs) > compMaxKids {
		blobs = blobs[:compMaxKids]
	}
	if len(blobs) == 0 {
		return doc, nil
	}
	s := &compSess{
		ctx:  ctx,
		want: want,
		w:    float64(w),
		h:    float64(h),
		left: compRenderBudget,
	}
	var kids []svg.Node
	for i, b := range blobs {
		if err := ctx.Err(); err != nil {
			c.Renders = s.used
			return doc.Append(kids...), err
		}
		if s.left <= 0 {
			kids = append(kids, seedRect(b))
			continue
		}
		extra := 0
		if s.left > (len(blobs)-i)+12 {
			extra = compNudgeExtra
		}
		kids = s.fitBlob(kids, b, extra)
	}
	kids = s.prune(kids)
	c.Renders = s.used
	return doc.Append(kids...), nil
}

type compSess struct {
	ctx     context.Context
	want    *image.NRGBA
	w, h    float64
	left    int
	used    int
	lastSSE float64
}

func (s *compSess) eval(nodes []svg.Node) (float64, bool) {
	if s.ctx.Err() != nil || s.left <= 0 {
		return 0, false
	}
	doc := svg.NewDocument(s.w, s.h).WithViewBox(0, 0, s.w, s.h).Append(nodes...)
	got, err := render.Render(doc)
	s.left--
	s.used++
	if err != nil {
		s.lastSSE = math.Inf(1)
		return s.lastSSE, true
	}
	s.lastSSE = ssePixels(got, s.want)
	return s.lastSSE, true
}

func (s *compSess) fitBlob(kids []svg.Node, b compBlob, extra int) []svg.Node {
	rectN, rectSSE, rectOK := s.tryFamily(kids, seedRect(b), extra)
	best := seedRect(b)
	bestSSE := math.Inf(1)
	if rectOK {
		best = rectN
		bestSSE = rectSSE
	}

	cir := seedCircle(b)
	cirSSE := math.Inf(1)
	triedCir := false
	if s.left > 0 && cir.Kind() != svg.KindInvalid {
		if n, sc, ok := s.tryFamily(kids, cir, extra); ok {
			triedCir = true
			cirSSE = sc
			if sc < bestSSE {
				best, bestSSE = n, sc
			}
		}
	}

	ell := seedEllipse(b)
	ellSSE := math.Inf(1)
	if s.left > 0 && triedCir && ell.Kind() != svg.KindInvalid {
		if n, sc, ok := s.tryFamily(kids, ell, extra); ok {
			ellSSE = sc
			if sc < rectSSE && sc < cirSSE {
				best, bestSSE = n, sc
			}
		}
	}

	poly := seedPolygon(b)
	if s.left > 0 && poly.Kind() != svg.KindInvalid {
		if n, sc, ok := s.tryFamily(kids, poly, extra); ok {
			if sc < bestSSE && sc < rectSSE && sc < cirSSE && sc < ellSSE {
				best, bestSSE = n, sc
			}
		}
	}
	return append(kids, best)
}

func (s *compSess) tryFamily(kids []svg.Node, cand svg.Node, extra int) (svg.Node, float64, bool) {
	if cand.Kind() == svg.KindInvalid || s.left <= 0 || s.ctx.Err() != nil {
		return cand, math.Inf(1), false
	}
	trial := append(append([]svg.Node(nil), kids...), cand)
	before := s.used
	trial = s.shortNudge(trial, extra)
	if s.used == before {
		return cand, math.Inf(1), false
	}
	return trial[len(trial)-1], s.lastSSE, true
}

func (s *compSess) shortNudge(nodes []svg.Node, extra int) []svg.Node {
	if len(nodes) == 0 {
		return nodes
	}
	sc, ok := s.eval(nodes)
	if !ok {
		return nodes
	}
	best := nodes
	bestSSE := sc
	if extra <= 0 {
		return best
	}
	step := 2.0
	i := len(best) - 1
	used := 0
	for step >= 1 && used < extra && s.left > 0 {
		improved := false
		n := paramCount(best[i])
		for p := 0; p < n && used < extra; p++ {
			for _, dir := range [2]float64{-step, step} {
				if used >= extra || s.left <= 0 {
					break
				}
				cand, ok := nudgeNode(best[i], p, dir, s.w, s.h)
				if !ok {
					continue
				}
				trial := append(append([]svg.Node(nil), best[:i]...), cand)
				if i+1 < len(best) {
					trial = append(trial, best[i+1:]...)
				}
				sc, ok := s.eval(trial)
				used++
				if !ok {
					return best
				}
				if sc < bestSSE {
					best = trial
					bestSSE = sc
					improved = true
				}
			}
		}
		if !improved {
			step /= 2
		}
	}
	return best
}

func (s *compSess) prune(kids []svg.Node) []svg.Node {
	if len(kids) < 2 || s.left <= 0 {
		return kids
	}
	cur, ok := s.eval(kids)
	if !ok {
		return kids
	}
	i := len(kids) - 1
	for i >= 0 && s.left > 0 {
		trial := append(append([]svg.Node(nil), kids[:i]...), kids[i+1:]...)
		sc, ok := s.eval(trial)
		if !ok {
			break
		}
		if sc <= cur {
			kids = trial
			cur = sc
		}
		i--
	}
	return kids
}

// ssePixels is the adapter accept metric. want.A==0 is don't-care.
// Per-channel |d|² uses uint32; the sum is returned as float64.
// Nil or size mismatch → +Inf. Inlined so search does not import loss.
func ssePixels(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var sum float64
	w := want.Rect.Dx()
	h := want.Rect.Dy()
	for y := 0; y < h; y++ {
		ws := y * want.Stride
		gs := y * got.Stride
		for x := 0; x < w; x++ {
			wi := ws + 4*x
			if want.Pix[wi+3] == 0 {
				continue
			}
			gi := gs + 4*x
			sum += float64(u8sq(got.Pix[gi], want.Pix[wi]) + u8sq(got.Pix[gi+1], want.Pix[wi+1]) + u8sq(got.Pix[gi+2], want.Pix[wi+2]))
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

func origin0(img *image.NRGBA) *image.NRGBA {
	if img.Rect.Min == (image.Point{}) {
		return img
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func capLongEdge(want *image.NRGBA, maxEdge int) *image.NRGBA {
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return want
	}
	maxd := w
	if h > maxd {
		maxd = h
	}
	if maxd <= maxEdge && b.Min == (image.Point{}) {
		return want
	}
	nw, nh := w, h
	if maxd > maxEdge {
		scale := float64(maxEdge) / float64(maxd)
		nw = int(math.Round(float64(w) * scale))
		nh = int(math.Round(float64(h) * scale))
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		srcY := b.Min.Y + y*h/nh
		if srcY >= b.Max.Y {
			srcY = b.Max.Y - 1
		}
		for x := 0; x < nw; x++ {
			srcX := b.Min.X + x*w/nw
			if srcX >= b.Max.X {
				srcX = b.Max.X - 1
			}
			out.SetNRGBA(x, y, want.NRGBAAt(srcX, srcY))
		}
	}
	return out
}

type compBlob struct {
	x0, y0, bw, bh int
	pix            []bool
	n              int
	fill           color.NRGBA
	colorN         int
}

func (b compBlob) minX() int { return b.x0 }
func (b compBlob) minY() int { return b.y0 }
func (b compBlob) maxX() int { return b.x0 + b.bw }
func (b compBlob) maxY() int { return b.y0 + b.bh }

func (b compBlob) at(x, y int) bool {
	lx, ly := x-b.x0, y-b.y0
	if lx < 0 || ly < 0 || lx >= b.bw || ly >= b.bh {
		return false
	}
	return b.pix[ly*b.bw+lx]
}

func colorBlobs(want *image.NRGBA, cmap palette.ColorMap, pal []color.NRGBA) []compBlob {
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
	var blobs []compBlob
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
			pts := []int{start}
			minX, minY, maxX, maxY := x, y, x+1, y+1
			hist := map[color.NRGBA]int{}
			for len(q) > 0 {
				p := q[0]
				q = q[1:]
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
					// reject wrap on horizontal steps
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
					pts = append(pts, n)
				}
			}
			if len(pts) < compSpeckle {
				continue
			}
			bw, bh := maxX-minX, maxY-minY
			pix := make([]bool, bw*bh)
			for _, p := range pts {
				lx, ly := p%w-minX, p/w-minY
				pix[ly*bw+lx] = true
			}
			fill := pal[lab]
			fillN := 0
			for c, n := range hist {
				if n > fillN || (n == fillN && lessNRGBA(c, fill)) {
					fill, fillN = c, n
				}
			}
			blobs = append(blobs, compBlob{
				x0: minX, y0: minY, bw: bw, bh: bh,
				pix: pix, n: len(pts), fill: fill, colorN: counts[lab],
			})
		}
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

func fillOf(c color.NRGBA) (color.NRGBA, float64, bool) {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}, float64(c.A) / 255, c.A != 255
}

func seedRect(b compBlob) svg.Node {
	r := svg.NewRect().WithX(float64(b.minX())).WithY(float64(b.minY())).
		WithWidth(float64(b.bw)).WithHeight(float64(b.bh))
	col, op, fade := fillOf(b.fill)
	r = r.WithFill(col)
	if fade {
		r = r.WithFillOpacity(op)
	}
	return r.Node()
}

func seedCircle(b compBlob) svg.Node {
	w := float64(b.bw)
	h := float64(b.bh)
	r := math.Min(w, h) / 2
	if r <= 0 {
		return svg.Node{}
	}
	cir := svg.NewCircle().
		WithCX(float64(b.minX()) + w/2).
		WithCY(float64(b.minY()) + h/2).
		WithR(r)
	col, op, fade := fillOf(b.fill)
	cir = cir.WithFill(col)
	if fade {
		cir = cir.WithFillOpacity(op)
	}
	return cir.Node()
}

func seedEllipse(b compBlob) svg.Node {
	w := float64(b.bw)
	h := float64(b.bh)
	rx, ry := w/2, h/2
	if rx <= 0 || ry <= 0 {
		return svg.Node{}
	}
	e := svg.NewEllipse().
		WithCX(float64(b.minX()) + rx).
		WithCY(float64(b.minY()) + ry).
		WithRX(rx).WithRY(ry)
	col, op, fade := fillOf(b.fill)
	e = e.WithFill(col)
	if fade {
		e = e.WithFillOpacity(op)
	}
	return e.Node()
}

func seedPolygon(b compBlob) svg.Node {
	pts := b.outline()
	if len(pts) < 3 {
		pts = b.bboxVerts()
	}
	if len(pts) < 3 {
		return svg.Node{}
	}
	p, err := svg.NewPolygon().WithPoints(pts)
	if err != nil {
		return svg.Node{}
	}
	col, op, fade := fillOf(b.fill)
	p = p.WithFill(col)
	if fade {
		p = p.WithFillOpacity(op)
	}
	return p.Node()
}

func (b compBlob) bboxVerts() [][2]float64 {
	if b.n == 0 {
		return nil
	}
	x0, y0 := float64(b.minX()), float64(b.minY())
	x1, y1 := float64(b.maxX()), float64(b.maxY())
	return [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

// 8-connected clockwise from east.
var n8 = [...][2]int{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1},
	{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

func (b compBlob) outline() [][2]float64 {
	sx, sy := -1, -1
	for y := b.minY(); y < b.maxY(); y++ {
		for x := b.minX(); x < b.maxX(); x++ {
			if b.at(x, y) {
				sx, sy = x, y
				break
			}
		}
		if sx >= 0 {
			break
		}
	}
	if sx < 0 {
		return nil
	}
	nx, ny, nd, ok := b.next(sx, sy, 4)
	if !ok {
		return b.bboxVerts()
	}
	pts := make([][2]int, 0, 32)
	pts = append(pts, [2]int{sx, sy})
	x0, y0 := nx, ny
	x, y, back := nx, ny, (nd+4)%8
	for {
		pts = append(pts, [2]int{x, y})
		nnx, nny, nnd, ok := b.next(x, y, back)
		if !ok {
			break
		}
		if x == sx && y == sy && nnx == x0 && nny == y0 {
			break
		}
		if len(pts) > b.n+16 {
			break
		}
		x, y, back = nnx, nny, (nnd+4)%8
	}
	return takeVerts(pts, compMaxPolyVerts)
}

func (b compBlob) next(x, y, back int) (nx, ny, dir int, ok bool) {
	for k := 1; k <= 8; k++ {
		d := (back + k) % 8
		px, py := x+n8[d][0], y+n8[d][1]
		if b.at(px, py) {
			return px, py, d, true
		}
	}
	return 0, 0, 0, false
}

func takeVerts(pts [][2]int, maxn int) [][2]float64 {
	if len(pts) == 0 {
		return nil
	}
	if len(pts) > maxn {
		sampled := make([][2]int, 0, maxn)
		n := len(pts)
		for i := 0; i < maxn; i++ {
			sampled = append(sampled, pts[i*n/maxn])
		}
		pts = sampled
	}
	out := make([][2]float64, 0, len(pts))
	for i, p := range pts {
		pt := [2]float64{float64(p[0]), float64(p[1])}
		if i > 0 && pt == out[len(out)-1] {
			continue
		}
		out = append(out, pt)
	}
	if len(out) >= 2 && out[0] == out[len(out)-1] {
		out = out[:len(out)-1]
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

func paramCount(n svg.Node) int {
	switch n.Kind() {
	case svg.KindRect, svg.KindEllipse:
		return 4
	case svg.KindCircle:
		return 3
	case svg.KindPolygon:
		p, _ := n.Polygon()
		return len(p.Points()) * 2
	default:
		return 0
	}
}

func nudgeNode(n svg.Node, which int, delta, W, H float64) (svg.Node, bool) {
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		x, y, w, h := r.X(), r.Y(), r.Width(), r.Height()
		switch which {
		case 0:
			x += delta
		case 1:
			y += delta
		case 2:
			w += delta
		case 3:
			h += delta
		default:
			return n, false
		}
		if w <= 0 || h <= 0 || x < 0 || y < 0 || x+w > W || y+h > H {
			return n, false
		}
		return r.WithX(x).WithY(y).WithWidth(w).WithHeight(h).Node(), true
	case svg.KindCircle:
		c, _ := n.Circle()
		cx, cy, r := c.CX(), c.CY(), c.R()
		switch which {
		case 0:
			cx += delta
		case 1:
			cy += delta
		case 2:
			r += delta
		default:
			return n, false
		}
		if r <= 0 || cx < 0 || cy < 0 || cx > W || cy > H || r > math.Max(W, H) {
			return n, false
		}
		return c.WithCX(cx).WithCY(cy).WithR(r).Node(), true
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		cx, cy, rx, ry := e.CX(), e.CY(), e.RX(), e.RY()
		switch which {
		case 0:
			cx += delta
		case 1:
			cy += delta
		case 2:
			rx += delta
		case 3:
			ry += delta
		default:
			return n, false
		}
		if rx <= 0 || ry <= 0 || cx < 0 || cy < 0 || cx > W || cy > H || rx > W || ry > H {
			return n, false
		}
		return e.WithCX(cx).WithCY(cy).WithRX(rx).WithRY(ry).Node(), true
	case svg.KindPolygon:
		p, _ := n.Polygon()
		pts := p.Points()
		vi, axis := which/2, which%2
		if vi < 0 || vi >= len(pts) {
			return n, false
		}
		pts[vi][axis] += delta
		if pts[vi][0] < 0 || pts[vi][1] < 0 || pts[vi][0] > W || pts[vi][1] > H {
			return n, false
		}
		np, err := p.WithPoints(pts)
		if err != nil {
			return n, false
		}
		return np.Node(), true
	default:
		return n, false
	}
}
