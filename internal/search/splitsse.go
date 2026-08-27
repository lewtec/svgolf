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

// SplitSSE is a recursive axis-aligned partition Search.
// Color is not a gene: fills come from palette.Auto on the target.
// A cut is kept only when rendered SSE strictly drops; Cost is not a score.
type SplitSSE struct {
	Colors int // 0 = auto, cap 8
}

var _ Search = SplitSSE{}

const (
	sseMaxRenders = 200
	sseMinLeaf    = 16
	sseMinDrop    = 256 // one 16-level RGB step on one pixel
	sseMaxEdge    = 4096
)

type ssePlate struct {
	x, y, w, h int
	c          color.NRGBA
	err        int64
	frozen     bool
}

type sseMom struct {
	n, r, g, b, r2, g2, b2 int64
}

func (s SplitSSE) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	doc, _, err := s.search(ctx, target)
	return doc, err
}

func (s SplitSSE) search(ctx context.Context, target *image.NRGBA) (svg.Document, int, error) {
	if err := ctx.Err(); err != nil {
		return svg.Document{}, 0, err
	}
	if target == nil {
		return svg.Document{}, 0, fmt.Errorf("search: nil pixmap")
	}
	want := sseFit(target)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	empty := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return empty, 0, nil
	}
	_, pal, err := palette.Auto(want, s.Colors)
	if err != nil {
		return empty, 0, err
	}
	if len(pal) == 0 {
		return empty, 0, nil
	}

	c0, e0 := sseBestFill(want, 0, 0, w, h, pal)
	leaves := []ssePlate{{x: 0, y: 0, w: w, h: h, c: c0, err: e0}}
	doc := sseDocument(w, h, leaves)
	score, err := sseOfDoc(doc, want)
	if err != nil {
		return doc, 1, err
	}
	renders := 1

	for renders < sseMaxRenders {
		if err := ctx.Err(); err != nil {
			return doc, renders, err
		}
		i := ssePick(leaves)
		if i < 0 {
			break
		}
		a, b2, ok := sseCut(want, leaves[i], pal)
		if !ok {
			leaves[i].frozen = true
			continue
		}
		// Local residual already dropped; Render is the official gate.
		cand := sseReplace(leaves, i, a, b2)
		candDoc := sseDocument(w, h, cand)
		candScore, err := sseOfDoc(candDoc, want)
		renders++
		if err != nil {
			leaves[i].frozen = true
			continue
		}
		if candScore < score && score-candScore >= float64(sseMinDrop) {
			leaves = cand
			doc = candDoc
			score = candScore
			continue
		}
		// Flat SSE: reject. Extra Cost=1 tiles must not buy a score.
		leaves[i].frozen = true
	}
	return doc, renders, nil
}

func sseDocument(w, h int, leaves []ssePlate) svg.Document {
	order := append([]ssePlate(nil), leaves...)
	sort.Slice(order, func(i, j int) bool {
		ai, aj := order[i].w*order[i].h, order[j].w*order[j].h
		if ai != aj {
			return ai > aj
		}
		if order[i].y != order[j].y {
			return order[i].y < order[j].y
		}
		return order[i].x < order[j].x
	})
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	for _, p := range order {
		r := svg.NewRect().
			WithX(float64(p.x)).WithY(float64(p.y)).
			WithWidth(float64(p.w)).WithHeight(float64(p.h)).
			WithFill(color.NRGBA{R: p.c.R, G: p.c.G, B: p.c.B, A: 255})
		if p.c.A != 255 {
			r = r.WithFillOpacity(float64(p.c.A) / 255)
		}
		doc = doc.Append(r.Node())
	}
	return doc
}

func ssePick(leaves []ssePlate) int {
	best := -1
	for i, p := range leaves {
		if p.frozen || p.err == 0 {
			continue
		}
		if p.w < 2*sseMinLeaf && p.h < 2*sseMinLeaf {
			continue
		}
		if best < 0 || p.err > leaves[best].err || (p.err == leaves[best].err && p.w*p.h > leaves[best].w*leaves[best].h) {
			best = i
		}
	}
	return best
}

func sseReplace(leaves []ssePlate, i int, a, b ssePlate) []ssePlate {
	out := make([]ssePlate, 0, len(leaves)+1)
	out = append(out, leaves[:i]...)
	out = append(out, a, b)
	out = append(out, leaves[i+1:]...)
	return out
}

func sseCut(img *image.NRGBA, p ssePlate, pal []color.NRGBA) (ssePlate, ssePlate, bool) {
	try := []bool{p.w >= p.h, p.w < p.h}
	for _, vert := range try {
		a, b, ok := sseBestCut(img, p, pal, vert)
		if ok {
			return a, b, true
		}
	}
	return ssePlate{}, ssePlate{}, false
}

func sseBestCut(img *image.NRGBA, p ssePlate, pal []color.NRGBA, vert bool) (ssePlate, ssePlate, bool) {
	length := p.h
	if vert {
		length = p.w
	}
	if length < 2*sseMinLeaf {
		return ssePlate{}, ssePlate{}, false
	}
	slices := make([]sseMom, length)
	if vert {
		for yy := 0; yy < p.h; yy++ {
			py := p.y + yy
			for xx := 0; xx < p.w; xx++ {
				q := sseAt(img, p.x+xx, py)
				if q.A == 0 {
					continue
				}
				slices[xx].add(q)
			}
		}
	} else {
		for yy := 0; yy < p.h; yy++ {
			py := p.y + yy
			row := &slices[yy]
			for xx := 0; xx < p.w; xx++ {
				q := sseAt(img, p.x+xx, py)
				if q.A == 0 {
					continue
				}
				row.add(q)
			}
		}
	}

	pref := make([]sseMom, length+1)
	for i := 0; i < length; i++ {
		pref[i+1] = pref[i].plus(slices[i])
	}
	total := pref[length]

	bestErr := p.err
	bestAt := -1
	midDist := length
	var leftC, rightC color.NRGBA
	var leftE, rightE int64
	// Residual-weighted: min left+right SSE. Tie-break toward midpoint.
	for cut := sseMinLeaf; cut <= length-sseMinLeaf; cut++ {
		lc, le := sseFillMom(pal, pref[cut])
		rc, re := sseFillMom(pal, total.minus(pref[cut]))
		te := le + re
		bal := cut - length/2
		if bal < 0 {
			bal = -bal
		}
		if te < bestErr || (te == bestErr && bestAt >= 0 && bal < midDist) {
			bestErr = te
			bestAt = cut
			midDist = bal
			leftC, rightC = lc, rc
			leftE, rightE = le, re
		}
	}
	if bestAt < 0 || p.err-bestErr < sseMinDrop {
		return ssePlate{}, ssePlate{}, false
	}
	var a, b ssePlate
	if vert {
		a = ssePlate{x: p.x, y: p.y, w: bestAt, h: p.h, c: leftC, err: leftE}
		b = ssePlate{x: p.x + bestAt, y: p.y, w: p.w - bestAt, h: p.h, c: rightC, err: rightE}
	} else {
		a = ssePlate{x: p.x, y: p.y, w: p.w, h: bestAt, c: leftC, err: leftE}
		b = ssePlate{x: p.x, y: p.y + bestAt, w: p.w, h: p.h - bestAt, c: rightC, err: rightE}
	}
	return a, b, true
}

func sseBestFill(img *image.NRGBA, x, y, w, h int, pal []color.NRGBA) (color.NRGBA, int64) {
	var m sseMom
	for yy := 0; yy < h; yy++ {
		py := y + yy
		for xx := 0; xx < w; xx++ {
			q := sseAt(img, x+xx, py)
			if q.A == 0 {
				continue
			}
			m.add(q)
		}
	}
	return sseFillMom(pal, m)
}

func sseFillMom(pal []color.NRGBA, m sseMom) (color.NRGBA, int64) {
	if len(pal) == 0 {
		return color.NRGBA{}, 0
	}
	if m.n == 0 {
		return pal[0], 0
	}
	best, bestE := pal[0], m.sse(pal[0])
	for _, c := range pal[1:] {
		if e := m.sse(c); e < bestE {
			best, bestE = c, e
		}
	}
	return best, bestE
}

func (m *sseMom) add(q color.NRGBA) {
	r, g, b := int64(q.R), int64(q.G), int64(q.B)
	m.n++
	m.r += r
	m.g += g
	m.b += b
	m.r2 += r * r
	m.g2 += g * g
	m.b2 += b * b
}

func (m sseMom) plus(o sseMom) sseMom {
	return sseMom{
		n: m.n + o.n, r: m.r + o.r, g: m.g + o.g, b: m.b + o.b,
		r2: m.r2 + o.r2, g2: m.g2 + o.g2, b2: m.b2 + o.b2,
	}
}

func (m sseMom) minus(o sseMom) sseMom {
	return sseMom{
		n: m.n - o.n, r: m.r - o.r, g: m.g - o.g, b: m.b - o.b,
		r2: m.r2 - o.r2, g2: m.g2 - o.g2, b2: m.b2 - o.b2,
	}
}

func (m sseMom) sse(c color.NRGBA) int64 {
	if m.n == 0 {
		return 0
	}
	r, g, b := int64(c.R), int64(c.G), int64(c.B)
	return m.r2 + m.g2 + m.b2 - 2*(r*m.r+g*m.g+b*m.b) + m.n*(r*r+g*g+b*b)
}

func sseAt(img *image.NRGBA, x, y int) color.NRGBA {
	i := img.PixOffset(x, y)
	s := img.Pix[i : i+4 : i+4]
	return color.NRGBA{R: s[0], G: s[1], B: s[2], A: s[3]}
}

// sseOfDoc is rendered RGB SSE over want.A!=0. search cannot import loss.
func sseOfDoc(doc svg.Document, want *image.NRGBA) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return math.Inf(1), err
	}
	return sseNRGBA(got, want), nil
}

func sseNRGBA(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var s int64
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			p := got.NRGBAAt(x, y)
			dr := int(p.R) - int(q.R)
			dg := int(p.G) - int(q.G)
			db := int(p.B) - int(q.B)
			s += int64(dr*dr + dg*dg + db*db)
		}
	}
	return float64(s)
}

func sseFit(img *image.NRGBA) *image.NRGBA {
	img = sseOrigin0(img)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= sseMaxEdge && h <= sseMaxEdge {
		return img
	}
	var nw, nh int
	if w >= h {
		nw = sseMaxEdge
		nh = h * sseMaxEdge / w
	} else {
		nh = sseMaxEdge
		nw = w * sseMaxEdge / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	return sseResizeNN(img, nw, nh)
}

func sseOrigin0(img *image.NRGBA) *image.NRGBA {
	if img.Rect.Min == (image.Point{}) {
		return img
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func sseResizeNN(src *image.NRGBA, nw, nh int) *image.NRGBA {
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
