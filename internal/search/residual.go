package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Residual is a fogleman/primitive-style Search that centers candidates on
// high-SSE residual pixels (top error quartile). Color is the mean of covered
// scored pixels, snapped to palette.Auto. Accept only a strict SSE drop.
type Residual struct {
	Colors  int // 0 = auto, cap 8
	Renders int // set by the last Search
	SSE     float64
}

var _ Search = (*Residual)(nil)

func init() {
	Register("residual", func() Search { return &Residual{} })
}

const (
	resMaxRenders = 200
	resMaxShapes  = 50
	resCandidates = 32
	resFailMut    = 32
	resMaxSeeds   = 4096
	resMaxSpan    = 256
	resSeed       = 1
	resMaxPixSSE  = 3 * 255 * 255
)

func (r *Residual) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if r == nil {
		return svg.Document{}, fmt.Errorf("search: nil Residual")
	}
	r.Renders = 0
	r.SSE = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}

	want := FitCanvas(target, MaxCanvas)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return doc, nil
	}

	cmap, pal, err := palette.Auto(opaqueOnly(want), r.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		cmap, pal, err = palette.Auto(want, r.Colors)
		if err != nil {
			return doc, err
		}
	}
	if len(pal) == 0 {
		return doc, nil
	}

	s := &resSess{
		ctx:  ctx,
		r:    r,
		want: want,
		got:  image.NewNRGBA(image.Rect(0, 0, w, h)),
		w:    w,
		h:    h,
		cmap: cmap,
		rng:  rand.New(rand.NewSource(resSeed)),
		hist: make([]uint32, resMaxPixSSE+1),
	}
	s.sse = sseNRGBA(s.got, want)
	s.refreshSeeds()
	if s.sse == 0 {
		r.SSE = s.sse
		return doc, nil
	}

	doc, err = s.seedPlate(doc, pal[0])
	if err != nil {
		return doc, err
	}
	r.SSE = s.sse
	if s.sse == 0 || r.Renders >= resMaxRenders {
		return doc, nil
	}

	rejects := 0
	for len(doc.Children()) < resMaxShapes && r.Renders < resMaxRenders {
		if err := ctx.Err(); err != nil {
			r.SSE = s.sse
			return doc, err
		}
		if s.sse == 0 {
			break
		}
		best, ok := s.bestCandidate()
		if !ok {
			break
		}
		best = s.climb(best)
		next, ok, err := s.accept(doc, best)
		if err != nil {
			r.SSE = s.sse
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
	r.SSE = s.sse
	return doc, nil
}

type resSess struct {
	ctx   context.Context
	r     *Residual
	want  *image.NRGBA
	got   *image.NRGBA
	w, h  int
	cmap  palette.ColorMap
	rng   *rand.Rand
	sse   float64
	seeds []resPix
	hist  []uint32
}

type resPix struct {
	x, y, e int
}

type resKind int

const (
	resRect resKind = iota
	resCircle
	resEllipse
	resPoly
)

type resShape struct {
	k           resKind
	x, y        float64
	rw, rh      float64
	cx, cy      float64
	rad, rx, ry float64
	pts         [6][2]float64
	n           int
	fill        color.NRGBA
	scanSSE     float64
}

func (s *resSess) render(doc svg.Document) (*image.NRGBA, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if s.r.Renders >= resMaxRenders {
		return nil, nil
	}
	img, err := render.Render(doc)
	s.r.Renders++
	return img, err
}

func (s *resSess) seedPlate(doc svg.Document, fill color.NRGBA) (svg.Document, error) {
	sh := resShape{k: resRect, x: 0, y: 0, rw: float64(s.w), rh: float64(s.h), fill: fill}
	trial := doc.Append(sh.node())
	got, err := s.render(trial)
	if err != nil || got == nil {
		return doc, err
	}
	e := sseNRGBA(got, s.want)
	if !(e < s.sse) {
		return doc, nil
	}
	s.got = got
	s.sse = e
	s.refreshSeeds()
	return trial, nil
}

func (s *resSess) accept(doc svg.Document, sh resShape) (svg.Document, bool, error) {
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
	s.refreshSeeds()
	return trial, true, nil
}

func (s *resSess) pixErr(x, y int) int {
	q := s.want.NRGBAAt(x, y)
	if q.A == 0 {
		return 0
	}
	g := s.got.NRGBAAt(x, y)
	e := rgb2(g, q)
	if e == 0 {
		return 0
	}
	snap := s.cmap.Map(color.NRGBA{R: q.R, G: q.G, B: q.B, A: 255})
	if rgb2(snap, q) >= e {
		return 0
	}
	return e
}

func (s *resSess) refreshSeeds() {
	s.seeds = s.seeds[:0]
	if s.w <= 0 || s.h <= 0 {
		return
	}
	for i := range s.hist {
		s.hist[i] = 0
	}
	var scored uint64
	bestE := 0
	bestX, bestY := 0, 0
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			e := s.pixErr(x, y)
			if e == 0 {
				continue
			}
			s.hist[e]++
			scored++
			if e > bestE {
				bestE, bestX, bestY = e, x, y
			}
		}
	}
	if scored == 0 {
		return
	}
	need := scored / 4
	if need < 1 {
		need = 1
	}
	var acc uint64
	thresh := 1
	for e := resMaxPixSSE; e >= 1; e-- {
		acc += uint64(s.hist[e])
		if acc >= need {
			thresh = e
			break
		}
	}
	var above uint64
	for e := thresh; e <= resMaxPixSSE; e++ {
		above += uint64(s.hist[e])
	}
	step := 1
	if above > resMaxSeeds {
		step = int(above / resMaxSeeds)
		if step < 1 {
			step = 1
		}
	}
	off := 0
	if step > 1 {
		off = s.rng.Intn(step)
	}
	seen := 0
	haveBest := false
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			e := s.pixErr(x, y)
			if e < thresh {
				continue
			}
			if seen%step == off {
				s.seeds = append(s.seeds, resPix{x, y, e})
				if x == bestX && y == bestY {
					haveBest = true
				}
			}
			seen++
		}
	}
	if !haveBest && bestE > 0 {
		s.seeds = append(s.seeds, resPix{bestX, bestY, bestE})
	}
}

func (s *resSess) pickSeed() (int, int) {
	if len(s.seeds) == 0 {
		return s.rng.Intn(s.w), s.rng.Intn(s.h)
	}
	p := s.seeds[s.rng.Intn(len(s.seeds))]
	return p.x, p.y
}

func (s *resSess) hottest() (int, int, bool) {
	if len(s.seeds) == 0 {
		return 0, 0, false
	}
	best := s.seeds[0]
	for _, p := range s.seeds[1:] {
		if p.e > best.e {
			best = p
		}
	}
	return best.x, best.y, true
}

func (s *resSess) bestCandidate() (resShape, bool) {
	var best resShape
	found := false
	consider := func(c resShape) {
		sc, fill, ok := s.scan(c)
		if !ok || sc >= s.sse {
			return
		}
		c.fill = fill
		c.scanSSE = sc
		if !found || betterShape(c, best) {
			best, found = c, true
		}
	}
	if hx, hy, ok := s.hottest(); ok {
		for _, span := range []float64{1, 2, 4, 8} {
			consider((resShape{k: resRect, x: float64(hx) - span/2, y: float64(hy) - span/2, rw: span, rh: span}).clamp(s.w, s.h))
		}
	}
	for i := 0; i < resCandidates; i++ {
		if err := s.ctx.Err(); err != nil {
			break
		}
		consider(s.randomShape())
	}
	return best, found
}

func betterShape(a, b resShape) bool {
	if a.scanSSE != b.scanSSE {
		return a.scanSSE < b.scanSSE
	}
	return a.verts() < b.verts()
}

func (c resShape) verts() int {
	switch c.k {
	case resRect, resCircle:
		return 1
	case resEllipse:
		return 2
	case resPoly:
		if c.n < 3 {
			return 4
		}
		return c.n
	default:
		return 8
	}
}

func (s *resSess) climb(c resShape) resShape {
	failed := 0
	steps := 0
	for failed < resFailMut && steps < resFailMut*2 {
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

func (s *resSess) randomShape() resShape {
	sx, sy := s.pickSeed()
	switch resKind(s.rng.Intn(4)) {
	case resRect:
		return s.randRect(sx, sy)
	case resCircle:
		return s.randCircle(sx, sy)
	case resEllipse:
		return s.randEllipse(sx, sy)
	default:
		return s.randPoly(sx, sy)
	}
}

func (s *resSess) randExtent() float64 {
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
		return 1 + s.rng.Float64()*math.Min(float64(resMaxSpan), m)
	}
}

func (s *resSess) randRect(sx, sy int) resShape {
	rw, rh := s.randExtent(), s.randExtent()
	sh := resShape{k: resRect, x: float64(sx) - rw/2, y: float64(sy) - rh/2, rw: rw, rh: rh}
	return sh.clamp(s.w, s.h)
}

func (s *resSess) randCircle(sx, sy int) resShape {
	sh := resShape{k: resCircle, cx: float64(sx), cy: float64(sy), rad: s.randExtent() / 2}
	return sh.clamp(s.w, s.h)
}

func (s *resSess) randEllipse(sx, sy int) resShape {
	sh := resShape{k: resEllipse, cx: float64(sx), cy: float64(sy), rx: s.randExtent() / 2, ry: s.randExtent() / 2}
	return sh.clamp(s.w, s.h)
}

func (s *resSess) randPoly(sx, sy int) resShape {
	n := 4
	switch s.rng.Intn(6) {
	case 3, 4:
		n = 5
	case 5:
		n = 6
	}
	span := s.randExtent()
	r := span / 2
	if r < 1 {
		r = 1
	}
	rot := s.rng.Float64() * 2 * math.Pi
	sh := resShape{k: resPoly, n: n}
	for i := 0; i < n; i++ {
		a := rot + float64(i)*2*math.Pi/float64(n)
		rr := r * (0.7 + 0.6*s.rng.Float64())
		sh.pts[i][0] = float64(sx) + math.Cos(a)*rr
		sh.pts[i][1] = float64(sy) + math.Sin(a)*rr
	}
	return sh.clamp(s.w, s.h)
}

func (s *resSess) mutate(c resShape) resShape {
	mag := []float64{16, 8, 4, 2, 1}[s.rng.Intn(5)]
	if s.rng.Intn(2) == 0 {
		mag = -mag
	}
	mag *= 0.5 + s.rng.Float64()
	switch c.k {
	case resRect:
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
	case resCircle:
		switch s.rng.Intn(3) {
		case 0:
			c.cx += mag
		case 1:
			c.cy += mag
		default:
			c.rad += mag
		}
	case resEllipse:
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
	case resPoly:
		i := s.rng.Intn(c.n)
		if s.rng.Intn(2) == 0 {
			c.pts[i][0] += mag
		} else {
			c.pts[i][1] += mag
		}
	}
	return c.clamp(s.w, s.h)
}

func (c resShape) clamp(W, H int) resShape {
	wf, hf := float64(W), float64(H)
	maxW, maxH := math.Min(resMaxSpan, wf), math.Min(resMaxSpan, hf)
	switch c.k {
	case resRect:
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
	case resCircle:
		if c.rad < 0.5 {
			c.rad = 0.5
		}
		if c.rad > maxW/2 {
			c.rad = maxW / 2
		}
		c.cx = clampF(c.cx, 0, wf)
		c.cy = clampF(c.cy, 0, hf)
	case resEllipse:
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
	case resPoly:
		if c.n < 3 {
			c.n = 3
		}
		if c.n > 6 {
			c.n = 6
		}
		for i := 0; i < c.n; i++ {
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

func (s *resSess) scan(c resShape) (float64, color.NRGBA, bool) {
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

func (c resShape) bbox(W, H int) (x0, y0, x1, y1 int) {
	var minX, minY, maxX, maxY float64
	switch c.k {
	case resRect:
		minX, minY, maxX, maxY = c.x, c.y, c.x+c.rw, c.y+c.rh
	case resCircle:
		minX, minY, maxX, maxY = c.cx-c.rad, c.cy-c.rad, c.cx+c.rad, c.cy+c.rad
	case resEllipse:
		minX, minY, maxX, maxY = c.cx-c.rx, c.cy-c.ry, c.cx+c.rx, c.cy+c.ry
	case resPoly:
		minX, minY, maxX, maxY = c.pts[0][0], c.pts[0][1], c.pts[0][0], c.pts[0][1]
		for i := 1; i < c.n; i++ {
			p := c.pts[i]
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

func (c resShape) covers(px, py int) bool {
	x := float64(px) + 0.5
	y := float64(py) + 0.5
	switch c.k {
	case resRect:
		return x >= c.x && x < c.x+c.rw && y >= c.y && y < c.y+c.rh
	case resCircle:
		dx, dy := x-c.cx, y-c.cy
		return dx*dx+dy*dy <= c.rad*c.rad
	case resEllipse:
		if c.rx <= 0 || c.ry <= 0 {
			return false
		}
		dx, dy := (x-c.cx)/c.rx, (y-c.cy)/c.ry
		return dx*dx+dy*dy <= 1
	case resPoly:
		return pointInPoly(x, y, c.pts[:c.n])
	default:
		return false
	}
}

func pointInPoly(x, y float64, pts [][2]float64) bool {
	n := len(pts)
	if n < 3 {
		return false
	}
	wn := 0
	for i := 0; i < n; i++ {
		x1, y1 := pts[i][0], pts[i][1]
		x2, y2 := pts[(i+1)%n][0], pts[(i+1)%n][1]
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

func (c resShape) node() svg.Node {
	col := color.NRGBA{R: c.fill.R, G: c.fill.G, B: c.fill.B, A: 255}
	fade := c.fill.A != 255
	op := float64(c.fill.A) / 255
	switch c.k {
	case resRect:
		r := svg.NewRect().WithX(c.x).WithY(c.y).WithWidth(c.rw).WithHeight(c.rh).WithFill(col)
		if fade {
			r = r.WithFillOpacity(op)
		}
		return r.Node()
	case resCircle:
		cir := svg.NewCircle().WithCX(c.cx).WithCY(c.cy).WithR(c.rad).WithFill(col)
		if fade {
			cir = cir.WithFillOpacity(op)
		}
		return cir.Node()
	case resEllipse:
		e := svg.NewEllipse().WithCX(c.cx).WithCY(c.cy).WithRX(c.rx).WithRY(c.ry).WithFill(col)
		if fade {
			e = e.WithFillOpacity(op)
		}
		return e.Node()
	case resPoly:
		p, err := svg.NewPolygon().WithPoints(c.pts[:c.n])
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

func opaqueOnly(src *image.NRGBA) *image.NRGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A != 255 {
				continue
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}
