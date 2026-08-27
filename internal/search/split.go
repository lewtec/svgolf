package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Split is a recursive axis-aligned partition Search.
// Color is not a gene: fills come from palette.Auto on the target.
type Split struct {
	Colors int // 0 = auto, cap 8
}

var _ Search = Split{}

const (
	splitMaxRenders = 200
	splitMinLeaf    = 8
)

type plate struct {
	x, y, w, h int
	c          color.NRGBA
	err        int
	frozen     bool
}

func (s Split) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	doc, _, err := s.search(ctx, target)
	return doc, err
}

func (s Split) search(ctx context.Context, target *image.NRGBA) (svg.Document, int, error) {
	if err := ctx.Err(); err != nil {
		return svg.Document{}, 0, err
	}
	if target == nil {
		return svg.Document{}, 0, fmt.Errorf("search: nil pixmap")
	}
	b := target.Bounds()
	w, h := b.Dx(), b.Dy()
	empty := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	_, pal, err := palette.Auto(target, s.Colors)
	if err != nil {
		return empty, 0, err
	}
	if len(pal) == 0 {
		return empty, 0, nil
	}

	ox, oy := b.Min.X, b.Min.Y
	c0, e0 := bestFill(target, ox, oy, 0, 0, w, h, pal)
	leaves := []plate{{x: 0, y: 0, w: w, h: h, c: c0, err: e0}}
	doc := documentOf(w, h, leaves)
	score, err := ofDoc(doc, target)
	if err != nil {
		return doc, 1, err
	}
	renders := 1

	for renders < splitMaxRenders {
		if err := ctx.Err(); err != nil {
			return doc, renders, err
		}
		i := pickSplit(leaves)
		if i < 0 {
			break
		}
		a, b2, ok := splitPlate(target, ox, oy, leaves[i], pal)
		if !ok {
			leaves[i].frozen = true
			continue
		}
		cand := replacePlate(leaves, i, a, b2)
		candDoc := documentOf(w, h, cand)
		candScore, err := ofDoc(candDoc, target)
		renders++
		if err != nil {
			leaves[i].frozen = true
			continue
		}
		// PerCost = deviate/cost: extra Cost=1 rects lower Of even when
		// exact Pixels stay flat (median-cut averages often match 0 source
		// pixels). The Of gate still accepts those cuts; that is the v1 bias.
		if candScore < score {
			leaves = cand
			doc = candDoc
			score = candScore
			continue
		}
		leaves[i].frozen = true
	}
	return doc, renders, nil
}

func documentOf(w, h int, leaves []plate) svg.Document {
	order := append([]plate(nil), leaves...)
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

func pickSplit(leaves []plate) int {
	best := -1
	for i, p := range leaves {
		if p.frozen || p.err == 0 || (p.w <= splitMinLeaf && p.h <= splitMinLeaf) {
			continue
		}
		if best < 0 || p.err > leaves[best].err || (p.err == leaves[best].err && p.w*p.h > leaves[best].w*leaves[best].h) {
			best = i
		}
	}
	return best
}

func replacePlate(leaves []plate, i int, a, b plate) []plate {
	out := make([]plate, 0, len(leaves)+1)
	out = append(out, leaves[:i]...)
	out = append(out, a, b)
	out = append(out, leaves[i+1:]...)
	return out
}

func splitPlate(img *image.NRGBA, ox, oy int, p plate, pal []color.NRGBA) (plate, plate, bool) {
	type cut struct {
		a, b plate
	}
	bestN := p.err
	var best cut
	found := false
	consider := func(vert bool) {
		c, ok := bestCut(img, ox, oy, p, pal, vert)
		if !ok || c.a.err+c.b.err >= bestN {
			return
		}
		bestN = c.a.err + c.b.err
		best = c
		found = true
	}
	// Longest axis first; the other axis is the fallback if it cuts more Loss.
	if p.w >= p.h {
		consider(true)
		consider(false)
	} else {
		consider(false)
		consider(true)
	}
	if found {
		return best.a, best.b, true
	}
	// No cut lowered local snap loss: split the long axis so recursion can continue.
	vert := p.w >= p.h
	length := p.h
	if vert {
		length = p.w
	}
	if length <= splitMinLeaf {
		return plate{}, plate{}, false
	}
	mid := length / 2
	var a, b plate
	if vert {
		a = plate{x: p.x, y: p.y, w: mid, h: p.h}
		b = plate{x: p.x + mid, y: p.y, w: p.w - mid, h: p.h}
	} else {
		a = plate{x: p.x, y: p.y, w: p.w, h: mid}
		b = plate{x: p.x, y: p.y + mid, w: p.w, h: p.h - mid}
	}
	a.c, a.err = bestFill(img, ox, oy, a.x, a.y, a.w, a.h, pal)
	b.c, b.err = bestFill(img, ox, oy, b.x, b.y, b.w, b.h, pal)
	return a, b, true
}

func bestCut(img *image.NRGBA, ox, oy int, p plate, pal []color.NRGBA, vert bool) (struct{ a, b plate }, bool) {
	var none struct{ a, b plate }
	length := p.w
	if !vert {
		length = p.h
	}
	if length <= splitMinLeaf {
		return none, false
	}
	nPal := len(pal)
	hist := make([][]int, length)
	scored := make([]int, length)
	for i := range hist {
		hist[i] = make([]int, nPal)
	}
	if vert {
		for yy := 0; yy < p.h; yy++ {
			py := oy + p.y + yy
			for xx := 0; xx < p.w; xx++ {
				q := nrgbaAt(img, ox+p.x+xx, py)
				if q.A == 0 {
					continue
				}
				scored[xx]++
				hist[xx][snapPal(pal, q)]++
			}
		}
	} else {
		for yy := 0; yy < p.h; yy++ {
			py := oy + p.y + yy
			for xx := 0; xx < p.w; xx++ {
				q := nrgbaAt(img, ox+p.x+xx, py)
				if q.A == 0 {
					continue
				}
				scored[yy]++
				hist[yy][snapPal(pal, q)]++
			}
		}
	}

	pref := make([][]int, length+1)
	prefS := make([]int, length+1)
	pref[0] = make([]int, nPal)
	for i := 0; i < length; i++ {
		pref[i+1] = make([]int, nPal)
		prefS[i+1] = prefS[i] + scored[i]
		for k := 0; k < nPal; k++ {
			pref[i+1][k] = pref[i][k] + hist[i][k]
		}
	}
	totalS := prefS[length]

	bestErr := p.err
	bestAt := -1
	var leftC, rightC color.NRGBA
	var leftE, rightE int
	midDist := length
	for cut := 1; cut < length; cut++ {
		lc, le := argMaxFill(pal, pref[cut], prefS[cut])
		var rh [32]int
		rhist := rh[:nPal]
		if nPal > len(rh) {
			rhist = make([]int, nPal)
		}
		for k := 0; k < nPal; k++ {
			rhist[k] = pref[length][k] - pref[cut][k]
		}
		rc, re := argMaxFill(pal, rhist, totalS-prefS[cut])
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
	if bestAt < 0 || bestErr >= p.err {
		return none, false
	}
	var a, b plate
	if vert {
		a = plate{x: p.x, y: p.y, w: bestAt, h: p.h, c: leftC, err: leftE}
		b = plate{x: p.x + bestAt, y: p.y, w: p.w - bestAt, h: p.h, c: rightC, err: rightE}
	} else {
		a = plate{x: p.x, y: p.y, w: p.w, h: bestAt, c: leftC, err: leftE}
		b = plate{x: p.x, y: p.y + bestAt, w: p.w, h: p.h - bestAt, c: rightC, err: rightE}
	}
	return struct{ a, b plate }{a, b}, true
}

func argMaxFill(pal []color.NRGBA, hits []int, scored int) (color.NRGBA, int) {
	best := 0
	for i := 1; i < len(pal); i++ {
		if hits[i] > hits[best] {
			best = i
		}
	}
	return pal[best], scored - hits[best]
}

func bestFill(img *image.NRGBA, ox, oy, x, y, w, h int, pal []color.NRGBA) (color.NRGBA, int) {
	hits := make([]int, len(pal))
	scored := 0
	for yy := 0; yy < h; yy++ {
		py := oy + y + yy
		for xx := 0; xx < w; xx++ {
			q := nrgbaAt(img, ox+x+xx, py)
			if q.A == 0 {
				continue
			}
			scored++
			hits[snapPal(pal, q)]++
		}
	}
	return argMaxFill(pal, hits, scored)
}

// snapPal picks the nearest palette swatch in RGBA (ColorMap is RGB-only).
func snapPal(pal []color.NRGBA, q color.NRGBA) int {
	best, bestD := 0, dist2rgba(q, pal[0])
	for i := 1; i < len(pal); i++ {
		if d := dist2rgba(q, pal[i]); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func dist2rgba(a, b color.NRGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	da := int(a.A) - int(b.A)
	return dr*dr + dg*dg + db*db + da*da
}

func nrgbaAt(img *image.NRGBA, x, y int) color.NRGBA {
	i := img.PixOffset(x, y)
	s := img.Pix[i : i+4 : i+4]
	return color.NRGBA{R: s[0], G: s[1], B: s[2], A: s[3]}
}

// ofDoc is loss.Of without importing loss (loss tests import search).
func ofDoc(doc svg.Document, want *image.NRGBA) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return math.Inf(1), err
	}
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1), nil
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
	deviate := float64(n)
	complexity := svg.CostDocument(doc)
	if complexity <= 0 {
		if deviate == 0 {
			return 0, nil
		}
		return math.Inf(1), nil
	}
	return deviate / float64(complexity), nil
}
