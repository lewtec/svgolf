package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Mask is a Search adapter: one primitive per palette color.
// Most-used color is back. Each color starts as a bbox rect and may escalate
// to circle, ellipse, then polygon when the local score improves.
type Mask struct {
	Colors  int // 0 = auto, cap 8
	Renders int // Render calls used by the last Search
}

var _ Search = (*Mask)(nil)

const (
	maskRenderBudget = 200
	maskRenderCap    = 4096

	rankRect     = 1
	rankCircle   = 1
	rankEllipse  = 4
	rankPoly     = 16
	maxPolyVerts = 8
)

// maskScore is the adapter decision metric: pixel_deviate / sum(rank).
// Ranks are local (1/1/4/16), not svg.Cost. Empty tree: 0 if deviate==0 else +Inf.
func maskScore(deviate float64, rank int) float64 {
	if math.IsInf(deviate, 0) || math.IsNaN(deviate) {
		return deviate
	}
	if rank <= 0 {
		if deviate == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return deviate / float64(rank)
}

func (m *Mask) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if m == nil {
		return svg.Document{}, fmt.Errorf("search: nil Mask")
	}
	m.Renders = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	b := target.Bounds()
	origW, origH := b.Dx(), b.Dy()
	doc := svg.NewDocument(float64(origW), float64(origH)).WithViewBox(0, 0, float64(origW), float64(origH))
	want, sx, sy := scoringTarget(target)
	sw, sh := want.Rect.Dx(), want.Rect.Dy()
	docW, docH := float64(sw), float64(sh)

	cmap, pal, err := palette.Auto(want, m.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}

	s := &maskSess{
		ctx:  ctx,
		want: want,
		w:    docW,
		h:    docH,
		left: maskRenderBudget,
	}
	var kids []svg.Node
	for _, c := range pal {
		if err := ctx.Err(); err != nil {
			return finishMask(kids, origW, origH, sx, sy), err
		}
		mk, fill := buildMask(want, cmap, c)
		if mk.n == 0 {
			continue
		}
		best := seedRect(mk, fill)
		kids = append(kids, best)
		bestScore, bestDev, ok := s.eval(kids)
		if !ok {
			break
		}
		for _, cand := range []svg.Node{
			seedCircle(mk, fill),
			seedEllipse(mk, fill),
			seedPolygon(mk, fill),
		} {
			if cand.Kind() == svg.KindInvalid {
				continue
			}
			if err := ctx.Err(); err != nil {
				break
			}
			trial := append(append([]svg.Node(nil), kids[:len(kids)-1]...), cand)
			sc, dev, ok := s.eval(trial)
			if !ok {
				break
			}
			// Score is deviate/rank, so a worse (or equal) fit can "win" on rank
			// inflation. Escalate only when the family is a better fit too.
			if sc < bestScore && dev < bestDev {
				kids[len(kids)-1] = cand
				bestScore = sc
				bestDev = dev
			}
		}
		kids = s.converge(kids)
	}
	m.Renders = s.used
	return finishMask(kids, origW, origH, sx, sy), nil
}

func finishMask(kids []svg.Node, origW, origH int, sx, sy float64) svg.Document {
	out := svg.NewDocument(float64(origW), float64(origH)).WithViewBox(0, 0, float64(origW), float64(origH))
	if sx == 1 && sy == 1 {
		return out.Append(kids...)
	}
	scaled := make([]svg.Node, len(kids))
	for i, n := range kids {
		scaled[i] = scaleNode(n, sx, sy)
	}
	return out.Append(scaled...)
}

func scoringTarget(want *image.NRGBA) (scored *image.NRGBA, sx, sy float64) {
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return want, 1, 1
	}
	maxd := w
	if h > maxd {
		maxd = h
	}
	if maxd <= maskRenderCap && b.Min == (image.Point{}) {
		return want, 1, 1
	}
	nw, nh := w, h
	if maxd > maskRenderCap {
		scale := float64(maskRenderCap) / float64(maxd)
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
	return out, float64(w) / float64(nw), float64(h) / float64(nh)
}

type maskSess struct {
	ctx  context.Context
	want *image.NRGBA
	w, h float64
	left int
	used int
}

func (s *maskSess) eval(nodes []svg.Node) (score, dev float64, ok bool) {
	if s.ctx.Err() != nil || s.left <= 0 {
		return 0, 0, false
	}
	doc := svg.NewDocument(s.w, s.h).WithViewBox(0, 0, s.w, s.h).Append(nodes...)
	got, err := render.Render(doc)
	s.left--
	s.used++
	if err != nil {
		return math.Inf(1), math.Inf(1), true
	}
	dev = pixelDeviate(got, s.want)
	return maskScore(dev, rankSum(nodes)), dev, true
}

// pixelDeviate matches loss.Pixels (want.A==0 is don't-care). Inlined so
// search does not import loss — eval_test.go already imports search.
func pixelDeviate(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			if got.NRGBAAt(x, y) != q {
				n++
			}
		}
	}
	return float64(n)
}

func (s *maskSess) converge(nodes []svg.Node) []svg.Node {
	if len(nodes) == 0 {
		return nodes
	}
	best := nodes
	bestScore, _, ok := s.eval(best)
	if !ok {
		return nodes
	}
	step := 8.0
	if m := math.Min(s.w, s.h); m < 16 {
		step = math.Max(1, m/4)
	}
	i := len(best) - 1
	for step >= 0.5 {
		if s.ctx.Err() != nil || s.left <= 0 {
			break
		}
		improved := false
		n := paramCount(best[i])
		for p := 0; p < n; p++ {
			for _, dir := range [2]float64{-step, step} {
				cand, ok := nudge(best[i], p, dir, s.w, s.h)
				if !ok {
					continue
				}
				trial := append(append([]svg.Node(nil), best[:i]...), cand)
				sc, _, ok := s.eval(trial)
				if !ok {
					return best
				}
				if sc < bestScore {
					best = trial
					bestScore = sc
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

func rankOf(n svg.Node) int {
	switch n.Kind() {
	case svg.KindCircle:
		return rankCircle
	case svg.KindEllipse:
		return rankEllipse
	case svg.KindPolygon:
		return rankPoly
	case svg.KindRect:
		return rankRect
	default:
		return 0
	}
}

func rankSum(nodes []svg.Node) int {
	sum := 0
	for _, n := range nodes {
		sum += rankOf(n)
	}
	return sum
}

type bitMask struct {
	w, h                   int
	pix                    []bool
	n                      int
	minX, minY, maxX, maxY int
}

func buildMask(want *image.NRGBA, cmap palette.ColorMap, swatch color.NRGBA) (bitMask, color.NRGBA) {
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	m := bitMask{w: w, h: h, pix: make([]bool, w*h), minX: w, minY: h}
	hist := map[color.NRGBA]int{}
	fill := color.NRGBA{R: swatch.R, G: swatch.G, B: swatch.B, A: swatch.A}
	fillN := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := want.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A == 0 {
				continue
			}
			mp := cmap.Map(c)
			if mp.R != swatch.R || mp.G != swatch.G || mp.B != swatch.B {
				continue
			}
			m.pix[y*w+x] = true
			m.n++
			hist[c]++
			if n := hist[c]; n > fillN {
				fill, fillN = c, n
			}
			if x < m.minX {
				m.minX = x
			}
			if y < m.minY {
				m.minY = y
			}
			if x+1 > m.maxX {
				m.maxX = x + 1
			}
			if y+1 > m.maxY {
				m.maxY = y + 1
			}
		}
	}
	return m, fill
}

func (m bitMask) at(x, y int) bool {
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return false
	}
	return m.pix[y*m.w+x]
}

func fillOf(c color.NRGBA) (color.NRGBA, float64, bool) {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}, float64(c.A) / 255, c.A != 255
}

func seedRect(m bitMask, c color.NRGBA) svg.Node {
	r := svg.NewRect().WithX(float64(m.minX)).WithY(float64(m.minY)).
		WithWidth(float64(m.maxX - m.minX)).WithHeight(float64(m.maxY - m.minY))
	col, op, fade := fillOf(c)
	r = r.WithFill(col)
	if fade {
		r = r.WithFillOpacity(op)
	}
	return r.Node()
}

func seedCircle(m bitMask, c color.NRGBA) svg.Node {
	w := float64(m.maxX - m.minX)
	h := float64(m.maxY - m.minY)
	r := math.Min(w, h) / 2
	if r <= 0 {
		return svg.Node{}
	}
	cir := svg.NewCircle().
		WithCX(float64(m.minX) + w/2).
		WithCY(float64(m.minY) + h/2).
		WithR(r)
	col, op, fade := fillOf(c)
	cir = cir.WithFill(col)
	if fade {
		cir = cir.WithFillOpacity(op)
	}
	return cir.Node()
}

func seedEllipse(m bitMask, c color.NRGBA) svg.Node {
	w := float64(m.maxX - m.minX)
	h := float64(m.maxY - m.minY)
	rx, ry := w/2, h/2
	if rx <= 0 || ry <= 0 {
		return svg.Node{}
	}
	e := svg.NewEllipse().
		WithCX(float64(m.minX) + rx).
		WithCY(float64(m.minY) + ry).
		WithRX(rx).WithRY(ry)
	col, op, fade := fillOf(c)
	e = e.WithFill(col)
	if fade {
		e = e.WithFillOpacity(op)
	}
	return e.Node()
}

func seedPolygon(m bitMask, c color.NRGBA) svg.Node {
	pts := m.outline()
	if len(pts) < 3 {
		pts = m.bboxVerts()
	}
	if len(pts) < 3 {
		return svg.Node{}
	}
	p, err := svg.NewPolygon().WithPoints(pts)
	if err != nil {
		return svg.Node{}
	}
	col, op, fade := fillOf(c)
	p = p.WithFill(col)
	if fade {
		p = p.WithFillOpacity(op)
	}
	return p.Node()
}

func (m bitMask) bboxVerts() [][2]float64 {
	if m.n == 0 {
		return nil
	}
	x0, y0 := float64(m.minX), float64(m.minY)
	x1, y1 := float64(m.maxX), float64(m.maxY)
	return [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

// 8-connected clockwise from east.
var n8 = [...][2]int{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1},
	{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

func (m bitMask) outline() [][2]float64 {
	sx, sy := -1, -1
	for y := m.minY; y < m.maxY; y++ {
		for x := m.minX; x < m.maxX; x++ {
			if m.pix[y*m.w+x] {
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
	nx, ny, nd, ok := m.next(sx, sy, 4)
	if !ok {
		return m.bboxVerts()
	}
	pts := make([][2]int, 0, 32)
	pts = append(pts, [2]int{sx, sy})
	x0, y0 := nx, ny
	x, y, back := nx, ny, (nd+4)%8
	for {
		pts = append(pts, [2]int{x, y})
		nnx, nny, nnd, ok := m.next(x, y, back)
		if !ok {
			break
		}
		if x == sx && y == sy && nnx == x0 && nny == y0 {
			break
		}
		if len(pts) > m.n+16 {
			break
		}
		x, y, back = nnx, nny, (nnd+4)%8
	}
	return takeVerts(pts, maxPolyVerts)
}

func (m bitMask) next(x, y, back int) (nx, ny, dir int, ok bool) {
	for k := 1; k <= 8; k++ {
		d := (back + k) % 8
		px, py := x+n8[d][0], y+n8[d][1]
		if m.at(px, py) {
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

func nudge(n svg.Node, which int, delta, W, H float64) (svg.Node, bool) {
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

func scaleNode(n svg.Node, sx, sy float64) svg.Node {
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		return r.WithX(r.X() * sx).WithY(r.Y() * sy).WithWidth(r.Width() * sx).WithHeight(r.Height() * sy).Node()
	case svg.KindCircle:
		c, _ := n.Circle()
		// Uniform r: average axis so a downsampled circle stays a circle.
		s := (sx + sy) / 2
		return c.WithCX(c.CX() * sx).WithCY(c.CY() * sy).WithR(c.R() * s).Node()
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		return e.WithCX(e.CX() * sx).WithCY(e.CY() * sy).WithRX(e.RX() * sx).WithRY(e.RY() * sy).Node()
	case svg.KindPolygon:
		p, _ := n.Polygon()
		pts := p.Points()
		for i := range pts {
			pts[i][0] *= sx
			pts[i][1] *= sy
		}
		np, err := p.WithPoints(pts)
		if err != nil {
			return n
		}
		return np.Node()
	default:
		return n
	}
}
