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

// Reduce seeds one bbox rect per 4-connected palette blob, then deletes or
// merges while rendered SSE holds. Color is not a gene.
type Reduce struct {
	Colors  int // 0 = auto, cap 8
	Renders int // Render calls used by the last Search
	Seeded  int // primitives after seed, before reduce
}

var _ Search = (*Reduce)(nil)

func init() {
	Register("reduce", func() Search { return &Reduce{} })
}

const (
	reduceRenderBudget = 200
	reduceSSEEps       = 256  // accept a rise of at most one 16-level RGB step
	reduceSpeckleDen   = 256  // drop blobs with area < colorN/256
	reduceMaxKids      = 4096 // Encode child cap
)

func (r *Reduce) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if r == nil {
		return svg.Document{}, fmt.Errorf("search: nil Reduce")
	}
	r.Renders = 0
	r.Seeded = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	want := FitCanvas(FromImage(target), MaxCanvas)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	empty := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return empty, nil
	}
	cmap, pal, err := palette.Auto(want, r.Colors)
	if err != nil {
		return empty, err
	}
	if len(pal) == 0 {
		return empty, nil
	}
	prims := seedBlobs(want, cmap, pal)
	if len(prims) > reduceMaxKids {
		prims = prims[:reduceMaxKids]
	}
	r.Seeded = len(prims)
	if len(prims) == 0 {
		return empty, nil
	}
	s := &redSess{ctx: ctx, want: want, w: w, h: h, left: reduceRenderBudget}
	prims = s.reduce(prims)
	r.Renders = s.used
	sortPainter(prims)
	return docFrom(w, h, prims), nil
}

type redPrim struct {
	x, y, w, h int
	n          int // blob pixels; 0 if unknown
	fill       color.NRGBA
}

func (p redPrim) area() int { return p.w * p.h }

func (p redPrim) slop() int {
	a := p.area()
	if p.n <= 0 || p.n >= a {
		return 0
	}
	return a - p.n
}

type primKey struct {
	x, y, w, h int
	r, g, b, a uint8
}

func keyOf(p redPrim) primKey {
	return primKey{p.x, p.y, p.w, p.h, p.fill.R, p.fill.G, p.fill.B, p.fill.A}
}

type pairKey struct{ a, b primKey }

func makePairKey(a, b redPrim) pairKey {
	ka, kb := keyOf(a), keyOf(b)
	if ka.y > kb.y || (ka.y == kb.y && ka.x > kb.x) {
		ka, kb = kb, ka
	}
	return pairKey{ka, kb}
}

func (p redPrim) node() svg.Node {
	r := svg.NewRect().
		WithX(float64(p.x)).WithY(float64(p.y)).
		WithWidth(float64(p.w)).WithHeight(float64(p.h)).
		WithFill(color.NRGBA{R: p.fill.R, G: p.fill.G, B: p.fill.B, A: 255})
	if p.fill.A != 255 {
		r = r.WithFillOpacity(float64(p.fill.A) / 255)
	}
	return r.Node()
}

type redSess struct {
	ctx         context.Context
	want        *image.NRGBA
	w, h        int
	left        int
	used        int
	failedDel   map[primKey]bool
	failedMerge map[pairKey]bool
}

func (s *redSess) reduce(prims []redPrim) []redPrim {
	if s.failedDel == nil {
		s.failedDel = map[primKey]bool{}
	}
	if s.failedMerge == nil {
		s.failedMerge = map[pairKey]bool{}
	}
	sortPainter(prims)
	cur, ok := s.eval(prims)
	if !ok {
		return prims
	}
	for s.left > 0 && s.ctx.Err() == nil {
		next, sc, ok := s.tryEdit(prims, cur)
		if !ok {
			break
		}
		prims, cur = next, sc
	}
	return prims
}

// reducePrims is the loop used by Search and by tests that inject extra rects.
func reducePrims(ctx context.Context, want *image.NRGBA, prims []redPrim, budget int) ([]redPrim, int) {
	if want == nil || len(prims) == 0 {
		return prims, 0
	}
	b := want.Bounds()
	s := &redSess{ctx: ctx, want: want, w: b.Dx(), h: b.Dy(), left: budget}
	out := s.reduce(append([]redPrim(nil), prims...))
	sortPainter(out)
	return out, s.used
}

func (s *redSess) tryEdit(prims []redPrim, cur float64) ([]redPrim, float64, bool) {
	if next, sc, ok := s.tryMerges(prims, cur, true); ok {
		return next, sc, true
	}
	if next, sc, ok := s.tryDeletes(prims, cur); ok {
		return next, sc, true
	}
	return s.tryMerges(prims, cur, false)
}

func (s *redSess) tryMerges(prims []redPrim, cur float64, exact bool) ([]redPrim, float64, bool) {
	type pair struct {
		i, j, waste int
	}
	var pairs []pair
	for i := 0; i < len(prims); i++ {
		for j := i + 1; j < len(prims); j++ {
			a, b := prims[i], prims[j]
			if a.fill != b.fill || !rectsTouch(a, b) {
				continue
			}
			if s.failedMerge[makePairKey(a, b)] {
				continue
			}
			w := unionWaste(a, b)
			if exact && w != 0 {
				continue
			}
			if !exact && w == 0 {
				continue
			}
			pairs = append(pairs, pair{i: i, j: j, waste: w})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].waste != pairs[j].waste {
			return pairs[i].waste < pairs[j].waste
		}
		if pairs[i].i != pairs[j].i {
			return pairs[i].i < pairs[j].i
		}
		return pairs[i].j < pairs[j].j
	})
	for _, p := range pairs {
		if s.left <= 0 || s.ctx.Err() != nil {
			return nil, 0, false
		}
		a, b := prims[p.i], prims[p.j]
		u := unionOf(a, b)
		u.n = a.n + b.n
		trial := make([]redPrim, 0, len(prims)-1)
		for k, q := range prims {
			if k == p.i || k == p.j {
				continue
			}
			trial = append(trial, q)
		}
		trial = append(trial, u)
		sortPainter(trial)
		sc, ok := s.eval(trial)
		if !ok {
			return nil, 0, false
		}
		if acceptSSE(sc, cur) {
			return trial, sc, true
		}
		s.failedMerge[makePairKey(a, b)] = true
	}
	return nil, 0, false
}

func (s *redSess) tryDeletes(prims []redPrim, cur float64) ([]redPrim, float64, bool) {
	if len(prims) < 2 {
		return nil, 0, false
	}
	order := make([]int, 0, len(prims))
	for i, p := range prims {
		if !s.failedDel[keyOf(p)] {
			order = append(order, i)
		}
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := prims[order[i]], prims[order[j]]
		if sa, sb := a.slop(), b.slop(); sa != sb {
			return sa > sb
		}
		if a.area() != b.area() {
			return a.area() < b.area()
		}
		if a.y != b.y {
			return a.y < b.y
		}
		return a.x < b.x
	})
	for _, i := range order {
		if s.left <= 0 || s.ctx.Err() != nil {
			return nil, 0, false
		}
		trial := append(append([]redPrim(nil), prims[:i]...), prims[i+1:]...)
		sortPainter(trial)
		sc, ok := s.eval(trial)
		if !ok {
			return nil, 0, false
		}
		if acceptSSE(sc, cur) {
			return trial, sc, true
		}
		s.failedDel[keyOf(prims[i])] = true
	}
	return nil, 0, false
}

func (s *redSess) eval(prims []redPrim) (float64, bool) {
	if s.ctx.Err() != nil || s.left <= 0 {
		return 0, false
	}
	got, err := render.Render(docFrom(s.w, s.h, prims))
	s.left--
	s.used++
	if err != nil {
		return math.Inf(1), true
	}
	return ssePixels(got, s.want), true
}

func acceptSSE(next, cur float64) bool {
	if math.IsInf(next, 1) {
		return false
	}
	return next <= cur+float64(reduceSSEEps)
}

func docFrom(w, h int, prims []redPrim) svg.Document {
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	for _, p := range prims {
		doc = doc.Append(p.node())
	}
	return doc
}

func sortPainter(p []redPrim) {
	sort.Slice(p, func(i, j int) bool {
		ai, aj := p[i].area(), p[j].area()
		if ai != aj {
			return ai > aj
		}
		if p[i].y != p[j].y {
			return p[i].y < p[j].y
		}
		return p[i].x < p[j].x
	})
}

func rectsTouch(a, b redPrim) bool {
	return a.x <= b.x+b.w && b.x <= a.x+a.w && a.y <= b.y+b.h && b.y <= a.y+a.h
}

func unionOf(a, b redPrim) redPrim {
	x0 := min(a.x, b.x)
	y0 := min(a.y, b.y)
	x1 := max(a.x+a.w, b.x+b.w)
	y1 := max(a.y+a.h, b.y+b.h)
	return redPrim{x: x0, y: y0, w: x1 - x0, h: y1 - y0, fill: a.fill}
}

func overlapArea(a, b redPrim) int {
	x0 := max(a.x, b.x)
	y0 := max(a.y, b.y)
	x1 := min(a.x+a.w, b.x+b.w)
	y1 := min(a.y+a.h, b.y+b.h)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	return (x1 - x0) * (y1 - y0)
}

func unionWaste(a, b redPrim) int {
	return unionOf(a, b).area() - a.area() - b.area() + overlapArea(a, b)
}

func seedBlobs(want *image.NRGBA, cmap palette.ColorMap, pal []color.NRGBA) []redPrim {
	blobs := colorBlobs(want, cmap, pal)
	out := make([]redPrim, 0, len(blobs))
	for _, b := range blobs {
		if b.n*reduceSpeckleDen < b.colorN {
			continue
		}
		out = append(out, redPrim{x: b.x0, y: b.y0, w: b.bw, h: b.bh, n: b.n, fill: b.fill})
	}
	sortPainter(out)
	return out
}

type redBlob struct {
	x0, y0, bw, bh int
	n, colorN      int
	fill           color.NRGBA
}

func colorBlobs(want *image.NRGBA, cmap palette.ColorMap, pal []color.NRGBA) []redBlob {
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
	var blobs []redBlob
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
			n := 0
			minX, minY, maxX, maxY := x, y, x+1, y+1
			hist := map[color.NRGBA]int{}
			for head := 0; head < len(q); head++ {
				p := q[head]
				n++
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
					nb := p + d
					if d == 1 && px+1 >= w {
						continue
					}
					if d == -1 && px <= 0 {
						continue
					}
					if nb < 0 || nb >= w*h || visited[nb] || idx[nb] != lab {
						continue
					}
					visited[nb] = true
					q = append(q, nb)
				}
			}
			fill := pal[lab]
			fillN := 0
			for c, cnt := range hist {
				if cnt > fillN || (cnt == fillN && lessNRGBA(c, fill)) {
					fill, fillN = c, cnt
				}
			}
			blobs = append(blobs, redBlob{
				x0: minX, y0: minY, bw: maxX - minX, bh: maxY - minY,
				n: n, colorN: counts[lab], fill: fill,
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
