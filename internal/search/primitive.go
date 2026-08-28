package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Primitive is a fogleman/primitive-style Search: sample random shapes, keep
// the best SSE, then hill-climb geometry. Color is the mean of covered scored
// pixels snapped to palette.Auto — not a gene. One shape at a time.
type Primitive struct {
	Colors  int // 0 = auto, cap 8
	Renders int // set by the last Search
}

var _ Search = (*Primitive)(nil)

const (
	primMaxRenders = 200
	primMaxShapes  = 50
	primCandidates = 32
	primFailMut    = 32
	primRenderCap  = 4096
	primHot        = 2048
	primMaxSpan    = 256
	primSeed       = 1
)

func (p *Primitive) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if p == nil {
		return svg.Document{}, fmt.Errorf("search: nil Primitive")
	}
	p.Renders = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}

	want := capWant(origin0(target))
	w, h := want.Rect.Dx(), want.Rect.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return doc, nil
	}

	cmap, pal, err := palette.Auto(want, p.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}

	s := &primSess{
		ctx:  ctx,
		p:    p,
		want: want,
		got:  image.NewNRGBA(image.Rect(0, 0, w, h)),
		w:    w,
		h:    h,
		cmap: cmap,
		rng:  rand.New(rand.NewSource(primSeed)),
	}
	s.sse = sseNRGBA(s.got, want)
	s.refreshHot()
	if s.sse == 0 {
		return doc, nil
	}

	doc, err = s.seedCover(doc, pal[0])
	if err != nil {
		return doc, err
	}
	if s.sse == 0 || p.Renders >= primMaxRenders {
		return doc, nil
	}

	rejects := 0
	for len(doc.Children()) < primMaxShapes && p.Renders < primMaxRenders {
		if err := ctx.Err(); err != nil {
			return doc, err
		}
		if s.sse == 0 {
			return doc, nil
		}
		best, ok := s.bestCandidate()
		if !ok {
			break
		}
		best = s.climb(best)
		next, ok, err := s.accept(doc, best)
		if err != nil {
			return doc, err
		}
		if !ok {
			rejects++
			if rejects >= 3 {
				break
			}
			continue
		}
		doc = next
		rejects = 0
	}
	return doc, nil
}

type primSess struct {
	ctx  context.Context
	p    *Primitive
	want *image.NRGBA
	got  *image.NRGBA
	w, h int
	cmap palette.ColorMap
	rng  *rand.Rand
	sse  float64
	hot  []hotPix
}

type hotPix struct {
	x, y, e int
}

type primKind int

const (
	primRect primKind = iota
	primCircle
	primEllipse
	primPoly
)

type primShape struct {
	k         primKind
	x, y      float64 // rect origin
	rw, rh    float64 // rect size
	cx, cy    float64
	r, rx, ry float64
	pts       [4][2]float64
	fill      color.NRGBA
	scanSSE   float64
}

func (s *primSess) render(doc svg.Document) (*image.NRGBA, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if s.p.Renders >= primMaxRenders {
		return nil, nil
	}
	img, err := render.Render(doc)
	s.p.Renders++
	return img, err
}

func (s *primSess) seedCover(doc svg.Document, fill color.NRGBA) (svg.Document, error) {
	// Prefer a covering circle/ellipse (logo marks with transparent corners)
	// before a full-canvas plate; accept still requires strictly lower SSE.
	var seeds []primShape
	if bb, ok := opaqueBBox(s.want); ok {
		x, y := float64(bb.minX), float64(bb.minY)
		rw, rh := float64(bb.maxX-bb.minX), float64(bb.maxY-bb.minY)
		cx, cy := x+rw/2, y+rh/2
		rx, ry := rw/2, rh/2
		if rx < 0.5 {
			rx = 0.5
		}
		if ry < 0.5 {
			ry = 0.5
		}
		seeds = append(seeds,
			primShape{k: primCircle, cx: cx, cy: cy, r: math.Max(rx, ry) + 2, fill: fill},
			primShape{k: primEllipse, cx: cx, cy: cy, rx: rx + 2, ry: ry + 2, fill: fill},
			primShape{k: primRect, x: x, y: y, rw: rw, rh: rh, fill: fill},
		)
	}
	seeds = append(seeds, primShape{k: primRect, x: 0, y: 0, rw: float64(s.w), rh: float64(s.h), fill: fill})
	bestDoc := doc
	bestSSE := s.sse
	var bestGot *image.NRGBA
	found := false
	for _, sh := range seeds {
		n := sh.node()
		if n.Kind() == svg.KindInvalid {
			continue
		}
		trial := doc.Append(n)
		got, err := s.render(trial)
		if err != nil {
			return doc, err
		}
		if got == nil {
			break
		}
		e := sseNRGBA(got, s.want)
		if e < bestSSE {
			bestSSE, bestDoc, bestGot, found = e, trial, got, true
			if e == 0 {
				break
			}
		}
	}
	if found {
		s.got = bestGot
		s.sse = bestSSE
		s.refreshHot()
	}
	return bestDoc, nil
}

func opaqueBBox(img *image.NRGBA) (struct{ minX, minY, maxX, maxY int }, bool) {
	var bb struct{ minX, minY, maxX, maxY int }
	first := true
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A < 128 {
				continue
			}
			ox, oy := x-b.Min.X, y-b.Min.Y
			if first {
				bb.minX, bb.minY, bb.maxX, bb.maxY = ox, oy, ox+1, oy+1
				first = false
				continue
			}
			if ox < bb.minX {
				bb.minX = ox
			}
			if oy < bb.minY {
				bb.minY = oy
			}
			if ox+1 > bb.maxX {
				bb.maxX = ox + 1
			}
			if oy+1 > bb.maxY {
				bb.maxY = oy + 1
			}
		}
	}
	return bb, !first
}

func (s *primSess) accept(doc svg.Document, sh primShape) (svg.Document, bool, error) {
	n := sh.node()
	if n.Kind() == svg.KindInvalid {
		return doc, false, nil
	}
	trial := doc.Append(n)
	got, err := s.render(trial)
	if err != nil || got == nil {
		return doc, false, err
	}
	e := sseNRGBA(got, s.want)
	if !(e < s.sse) {
		return doc, false, nil
	}
	s.got = got
	s.sse = e
	s.refreshHot()
	return trial, true, nil
}

func (s *primSess) refreshHot() {
	s.hot = s.hot[:0]
	if s.w <= 0 || s.h <= 0 {
		return
	}
	area := s.w * s.h
	step := 1
	if area > primHot {
		step = int(math.Sqrt(float64(area) / float64(primHot)))
		if step < 1 {
			step = 1
		}
	}
	offx, offy := s.rng.Intn(step), s.rng.Intn(step)
	for y := offy; y < s.h && len(s.hot) < primHot*2; y += step {
		for x := offx; x < s.w && len(s.hot) < primHot*2; x += step {
			q := s.want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			e := rgb2(s.got.NRGBAAt(x, y), q)
			if e == 0 {
				continue
			}
			s.hot = append(s.hot, hotPix{x, y, e})
		}
	}
}

func (s *primSess) pickSeed() (int, int) {
	if len(s.hot) == 0 {
		return s.rng.Intn(s.w), s.rng.Intn(s.h)
	}
	best := s.hot[s.rng.Intn(len(s.hot))]
	for i := 0; i < 7; i++ {
		p := s.hot[s.rng.Intn(len(s.hot))]
		if p.e > best.e {
			best = p
		}
	}
	return best.x, best.y
}

func (s *primSess) bestCandidate() (primShape, bool) {
	var best primShape
	found := false
	consider := func(c primShape) {
		sc, fill, ok := s.scan(c)
		if !ok || sc >= s.sse {
			return
		}
		c.fill = fill
		c.scanSSE = sc
		if !found || sc < best.scanSSE {
			best, found = c, true
		}
	}
	// Tight rect on the hottest residual pixel so sparse marks are not missed.
	if hx, hy, ok := s.hottest(); ok {
		consider((primShape{k: primRect, x: float64(hx) - 4, y: float64(hy) - 4, rw: 8, rh: 8}).clamp(s.w, s.h))
	}
	for i := 0; i < primCandidates; i++ {
		if err := s.ctx.Err(); err != nil {
			break
		}
		consider(s.randomShape())
	}
	return best, found
}

func (s *primSess) hottest() (int, int, bool) {
	if len(s.hot) == 0 {
		return 0, 0, false
	}
	best := s.hot[0]
	for _, p := range s.hot[1:] {
		if p.e > best.e {
			best = p
		}
	}
	return best.x, best.y, true
}

func (s *primSess) climb(c primShape) primShape {
	failed := 0
	steps := 0
	for failed < primFailMut && steps < primFailMut*2 {
		if err := s.ctx.Err(); err != nil {
			break
		}
		steps++
		mut := s.mutate(c)
		sc, fill, ok := s.scan(mut)
		if !ok || sc >= c.scanSSE {
			failed++
			continue
		}
		mut.fill = fill
		mut.scanSSE = sc
		c = mut
		failed = 0
	}
	return c
}

func (s *primSess) randomShape() primShape {
	sx, sy := s.pickSeed()
	switch primKind(s.rng.Intn(4)) {
	case primRect:
		return s.randRect(sx, sy)
	case primCircle:
		return s.randCircle(sx, sy)
	case primEllipse:
		return s.randEllipse(sx, sy)
	default:
		return s.randPoly(sx, sy)
	}
}

func (s *primSess) randExtent() float64 {
	m := float64(s.w)
	if float64(s.h) < m {
		m = float64(s.h)
	}
	switch s.rng.Intn(3) {
	case 0:
		return 1 + s.rng.Float64()*math.Min(16, m)
	case 1:
		return 1 + s.rng.Float64()*math.Min(64, m)
	default:
		return 1 + s.rng.Float64()*math.Min(128, m)
	}
}

func (s *primSess) randRect(sx, sy int) primShape {
	rw, rh := s.randExtent(), s.randExtent()
	sh := primShape{k: primRect, x: float64(sx) - s.rng.Float64()*rw, y: float64(sy) - s.rng.Float64()*rh, rw: rw, rh: rh}
	return sh.clamp(s.w, s.h)
}

func (s *primSess) randCircle(sx, sy int) primShape {
	sh := primShape{k: primCircle, cx: float64(sx), cy: float64(sy), r: s.randExtent() / 2}
	return sh.clamp(s.w, s.h)
}

func (s *primSess) randEllipse(sx, sy int) primShape {
	sh := primShape{k: primEllipse, cx: float64(sx), cy: float64(sy), rx: s.randExtent() / 2, ry: s.randExtent() / 2}
	return sh.clamp(s.w, s.h)
}

func (s *primSess) randPoly(sx, sy int) primShape {
	span := s.randExtent()
	var pts [4][2]float64
	for i := range pts {
		pts[i][0] = float64(sx) + s.rng.Float64()*span - span/2
		pts[i][1] = float64(sy) + s.rng.Float64()*span - span/2
	}
	return (primShape{k: primPoly, pts: pts}).clamp(s.w, s.h)
}

func (s *primSess) mutate(c primShape) primShape {
	mag := []float64{16, 8, 4, 2, 1}[s.rng.Intn(5)]
	if s.rng.Intn(2) == 0 {
		mag = -mag
	}
	mag *= 0.5 + s.rng.Float64()
	switch c.k {
	case primRect:
		switch s.rng.Intn(4) {
		case 0:
			c.x += mag
		case 1:
			c.y += mag
		case 2:
			c.rw += mag
		default:
			c.rh += mag
		}
	case primCircle:
		switch s.rng.Intn(3) {
		case 0:
			c.cx += mag
		case 1:
			c.cy += mag
		default:
			c.r += mag
		}
	case primEllipse:
		switch s.rng.Intn(4) {
		case 0:
			c.cx += mag
		case 1:
			c.cy += mag
		case 2:
			c.rx += mag
		default:
			c.ry += mag
		}
	case primPoly:
		i := s.rng.Intn(4)
		if s.rng.Intn(2) == 0 {
			c.pts[i][0] += mag
		} else {
			c.pts[i][1] += mag
		}
	}
	return c.clamp(s.w, s.h)
}

func (c primShape) clamp(W, H int) primShape {
	wf, hf := float64(W), float64(H)
	maxW, maxH := math.Min(primMaxSpan, wf), math.Min(primMaxSpan, hf)
	switch c.k {
	case primRect:
		if c.rw < 1 {
			c.rw = 1
		}
		if c.rh < 1 {
			c.rh = 1
		}
		if c.rw > maxW {
			c.rw = maxW
		}
		if c.rh > maxH {
			c.rh = maxH
		}
		if c.x < 0 {
			c.x = 0
		}
		if c.y < 0 {
			c.y = 0
		}
		if c.x+c.rw > wf {
			c.x = wf - c.rw
			if c.x < 0 {
				c.x, c.rw = 0, wf
			}
		}
		if c.y+c.rh > hf {
			c.y = hf - c.rh
			if c.y < 0 {
				c.y, c.rh = 0, hf
			}
		}
	case primCircle:
		if c.r < 0.5 {
			c.r = 0.5
		}
		if c.r > maxW/2 {
			c.r = maxW / 2
		}
		c.cx = clampF(c.cx, 0, wf)
		c.cy = clampF(c.cy, 0, hf)
	case primEllipse:
		if c.rx < 0.5 {
			c.rx = 0.5
		}
		if c.ry < 0.5 {
			c.ry = 0.5
		}
		if c.rx > maxW/2 {
			c.rx = maxW / 2
		}
		if c.ry > maxH/2 {
			c.ry = maxH / 2
		}
		c.cx = clampF(c.cx, 0, wf)
		c.cy = clampF(c.cy, 0, hf)
	case primPoly:
		for i := range c.pts {
			c.pts[i][0] = clampF(c.pts[i][0], 0, wf)
			c.pts[i][1] = clampF(c.pts[i][1], 0, hf)
		}
	}
	return c
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (s *primSess) scan(c primShape) (float64, color.NRGBA, bool) {
	x0, y0, x1, y1 := c.bbox(s.w, s.h)
	var sr, sg, sb, n int
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if !c.covers(x, y) {
				continue
			}
			q := s.want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			sr += int(q.R)
			sg += int(q.G)
			sb += int(q.B)
			n++
		}
	}
	if n == 0 {
		return 0, color.NRGBA{}, false
	}
	fill := s.cmap.Map(color.NRGBA{
		R: uint8(float64(sr)/float64(n) + 0.5),
		G: uint8(float64(sg)/float64(n) + 0.5),
		B: uint8(float64(sb)/float64(n) + 0.5),
		A: 255,
	})
	delta := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if !c.covers(x, y) {
				continue
			}
			q := s.want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			delta += rgb2(fill, q) - rgb2(s.got.NRGBAAt(x, y), q)
		}
	}
	return s.sse + float64(delta), fill, true
}

func (c primShape) bbox(W, H int) (x0, y0, x1, y1 int) {
	var minX, minY, maxX, maxY float64
	switch c.k {
	case primRect:
		minX, minY, maxX, maxY = c.x, c.y, c.x+c.rw, c.y+c.rh
	case primCircle:
		minX, minY, maxX, maxY = c.cx-c.r, c.cy-c.r, c.cx+c.r, c.cy+c.r
	case primEllipse:
		minX, minY, maxX, maxY = c.cx-c.rx, c.cy-c.ry, c.cx+c.rx, c.cy+c.ry
	case primPoly:
		minX, minY, maxX, maxY = c.pts[0][0], c.pts[0][1], c.pts[0][0], c.pts[0][1]
		for _, p := range c.pts[1:] {
			if p[0] < minX {
				minX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
	}
	x0 = clampInt(int(math.Floor(minX)), 0, W)
	y0 = clampInt(int(math.Floor(minY)), 0, H)
	x1 = clampInt(int(math.Ceil(maxX)), 0, W)
	y1 = clampInt(int(math.Ceil(maxY)), 0, H)
	return
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (c primShape) covers(px, py int) bool {
	x := float64(px) + 0.5
	y := float64(py) + 0.5
	switch c.k {
	case primRect:
		return x >= c.x && x < c.x+c.rw && y >= c.y && y < c.y+c.rh
	case primCircle:
		dx, dy := x-c.cx, y-c.cy
		return dx*dx+dy*dy <= c.r*c.r
	case primEllipse:
		if c.rx <= 0 || c.ry <= 0 {
			return false
		}
		dx, dy := (x-c.cx)/c.rx, (y-c.cy)/c.ry
		return dx*dx+dy*dy <= 1
	case primPoly:
		return pointInPoly(x, y, c.pts)
	default:
		return false
	}
}

func pointInPoly(x, y float64, pts [4][2]float64) bool {
	wn := 0
	for i := 0; i < 4; i++ {
		x1, y1 := pts[i][0], pts[i][1]
		x2, y2 := pts[(i+1)%4][0], pts[(i+1)%4][1]
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

func (c primShape) node() svg.Node {
	col := color.NRGBA{R: c.fill.R, G: c.fill.G, B: c.fill.B, A: 255}
	fade := c.fill.A != 255
	op := float64(c.fill.A) / 255
	switch c.k {
	case primRect:
		r := svg.NewRect().WithX(c.x).WithY(c.y).WithWidth(c.rw).WithHeight(c.rh).WithFill(col)
		if fade {
			r = r.WithFillOpacity(op)
		}
		return r.Node()
	case primCircle:
		cir := svg.NewCircle().WithCX(c.cx).WithCY(c.cy).WithR(c.r).WithFill(col)
		if fade {
			cir = cir.WithFillOpacity(op)
		}
		return cir.Node()
	case primEllipse:
		e := svg.NewEllipse().WithCX(c.cx).WithCY(c.cy).WithRX(c.rx).WithRY(c.ry).WithFill(col)
		if fade {
			e = e.WithFillOpacity(op)
		}
		return e.Node()
	case primPoly:
		p, err := svg.NewPolygon().WithPoints(c.pts[:])
		if err != nil {
			return svg.Node{}
		}
		p = p.WithFill(col)
		if fade {
			p = p.WithFillOpacity(op)
		}
		return p.Node()
	default:
		return svg.Node{}
	}
}

// sseNRGBA is sum (dR²+dG²+dB²) over want.A != 0. Inlined so search
// does not import loss (eval tests already import search).
func sseNRGBA(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var s uint64
	w := want.Rect.Dx()
	h := want.Rect.Dy()
	for y := 0; y < h; y++ {
		wi := want.PixOffset(want.Rect.Min.X, want.Rect.Min.Y+y)
		gi := got.PixOffset(got.Rect.Min.X, got.Rect.Min.Y+y)
		for x := 0; x < w; x++ {
			if want.Pix[wi+3] == 0 {
				wi += 4
				gi += 4
				continue
			}
			dr := int(got.Pix[gi]) - int(want.Pix[wi])
			dg := int(got.Pix[gi+1]) - int(want.Pix[wi+1])
			db := int(got.Pix[gi+2]) - int(want.Pix[wi+2])
			s += uint64(dr*dr + dg*dg + db*db)
			wi += 4
			gi += 4
		}
	}
	return float64(s)
}

func rgb2(a, b color.NRGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	return dr*dr + dg*dg + db*db
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

// capWant downsamples so neither edge exceeds the Encode/Render cap.
func capWant(want *image.NRGBA) *image.NRGBA {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	if w <= primRenderCap && h <= primRenderCap {
		return want
	}
	sw, sh := w, h
	if w >= h {
		sw = primRenderCap
		sh = h * primRenderCap / w
	} else {
		sh = primRenderCap
		sw = w * primRenderCap / h
	}
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	return resizeNN(want, sw, sh)
}

func resizeNN(src *image.NRGBA, nw, nh int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for y := 0; y < nh; y++ {
		sy := src.Rect.Min.Y + y*sh/nh
		for x := 0; x < nw; x++ {
			sx := src.Rect.Min.X + x*sw/nw
			dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
		}
	}
	return dst
}
