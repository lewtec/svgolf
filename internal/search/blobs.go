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

// Blobs is a Search adapter: one primitive per 4-connected palette blob.
// Speckles smaller than max(32, 0.02% of canvas area) are dropped.
type Blobs struct {
	Colors  int // 0 = auto, cap 8
	Renders int // Render calls used by the last Search
}

var _ Search = (*Blobs)(nil)

const (
	blobRenderBudget = 200
	blobMaxKids      = 4096
	blobMaxPolyVerts = 8
	blobNudgePad     = 8
	blobPruneArea    = 1_200_000
)

func init() {
	Register("blobs", func() Search { return &Blobs{} })
}

func (b *Blobs) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if b == nil {
		return svg.Document{}, fmt.Errorf("search: nil Blobs")
	}
	b.Renders = 0
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
	cmap, pal, err := palette.Auto(want, b.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}
	blobs := colorBlobs(want, cmap, pal, speckleMin(w, h))
	if len(blobs) == 0 {
		return doc, nil
	}
	if len(blobs) > blobMaxKids {
		blobs = blobs[:blobMaxKids]
	}
	s := &blobSess{
		ctx:  ctx,
		want: want,
		got:  image.NewNRGBA(image.Rect(0, 0, w, h)),
		w:    float64(w),
		h:    float64(h),
		left: blobRenderBudget,
	}
	var kids []svg.Node
	cur := emptySSE(want)
	for _, bl := range blobs {
		if err := ctx.Err(); err != nil {
			b.Renders = s.used
			return doc.Append(kids...), err
		}
		if len(kids) >= blobMaxKids {
			break
		}
		node := fitBlob(want, bl, w, h)
		if node.Kind() == svg.KindInvalid {
			continue
		}
		// Accept iff scored SSE strictly drops. Local opaque paint, not Render:
		// a 13Mpx canvas cannot spend the 200-Render budget on full-frame evals.
		delta := paintDelta(s.got, want, node, false)
		if delta < 0 {
			paintDelta(s.got, want, node, true)
			cur += delta
			kids = append(kids, node)
		}
	}
	if w*h <= blobPruneArea {
		kids = s.prune(kids, math.Inf(1))
	}
	b.Renders = s.used
	return doc.Append(kids...), nil
}

// speckleMin is max(32, 0.02% of canvas area). A fixed 32-px cut on a
// multi-megapixel scene keeps thousands of kids and hits the Encode cap.
func speckleMin(w, h int) int {
	return max(32, w*h/5000)
}

type blobSess struct {
	ctx  context.Context
	want *image.NRGBA
	got  *image.NRGBA
	w, h float64
	left int
	used int
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

// paintDelta is opaque source-over SSE change on scored pixels. commit writes got.
func paintDelta(got, want *image.NRGBA, n svg.Node, commit bool) float64 {
	fill, ok := nodeFill(n)
	if !ok {
		return 0
	}
	W, H := want.Rect.Dx(), want.Rect.Dy()
	x0, y0, x1, y1 := nodeBBox(n, W, H)
	var delta float64
	for y := y0; y < y1; y++ {
		woff := y * want.Stride
		goff := y * got.Stride
		for x := x0; x < x1; x++ {
			wi := woff + 4*x
			if want.Pix[wi+3] == 0 {
				continue
			}
			if !covers(n, float64(x)+0.5, float64(y)+0.5) {
				continue
			}
			gi := goff + 4*x
			delta -= float64(u8sq(got.Pix[gi], want.Pix[wi]) + u8sq(got.Pix[gi+1], want.Pix[wi+1]) + u8sq(got.Pix[gi+2], want.Pix[wi+2]))
			delta += float64(u8sq(fill.R, want.Pix[wi]) + u8sq(fill.G, want.Pix[wi+1]) + u8sq(fill.B, want.Pix[wi+2]))
			if commit {
				got.Pix[gi] = fill.R
				got.Pix[gi+1] = fill.G
				got.Pix[gi+2] = fill.B
				got.Pix[gi+3] = 255
			}
		}
	}
	return delta
}

func (s *blobSess) eval(nodes []svg.Node) (float64, bool) {
	if s.ctx.Err() != nil || s.left <= 0 {
		return 0, false
	}
	doc := svg.NewDocument(s.w, s.h).WithViewBox(0, 0, s.w, s.h).Append(nodes...)
	got, err := render.Render(doc)
	s.left--
	s.used++
	if err != nil {
		return math.Inf(1), true
	}
	return ssePixels(got, s.want), true
}

func (s *blobSess) prune(kids []svg.Node, cur float64) []svg.Node {
	if len(kids) < 2 || s.left <= 0 {
		return kids
	}
	if math.IsInf(cur, 0) {
		sc, ok := s.eval(kids)
		if !ok {
			return kids
		}
		cur = sc
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
			continue
		}
		i--
	}
	return kids
}

// ssePixels is the adapter accept metric. want.A==0 is don't-care.
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

func fillOf(c color.NRGBA) (color.NRGBA, float64, bool) {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}, float64(c.A) / 255, c.A != 255
}

func applyFill(n svg.Node, c color.NRGBA) svg.Node {
	col, op, fade := fillOf(c)
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

// Render drops large caps of a circle/ellipse that straddles the canvas
// origin, so keep geometry inside [0,W]×[0,H].
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

func seedPolygon(b blob) svg.Node {
	pts := b.outline()
	if len(pts) < 3 {
		return svg.Node{}
	}
	p, err := svg.NewPolygon().WithPoints(pts)
	if err != nil {
		return svg.Node{}
	}
	return applyFill(p.Node(), b.fill)
}

func (b blob) outline() [][2]float64 {
	sx, sy := -1, -1
	for y := b.y0; y < b.y0+b.bh; y++ {
		for x := b.x0; x < b.x0+b.bw; x++ {
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
	nx, ny, nd, ok := b.next8(sx, sy, 4)
	if !ok {
		return nil
	}
	pts := make([][2]int, 0, 32)
	pts = append(pts, [2]int{sx, sy})
	x0, y0 := nx, ny
	x, y, back := nx, ny, (nd+4)%8
	for {
		pts = append(pts, [2]int{x, y})
		nnx, nny, nnd, ok := b.next8(x, y, back)
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
	return takeVerts(pts, blobMaxPolyVerts)
}

var n8 = [...][2]int{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1},
	{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

func (b blob) next8(x, y, back int) (nx, ny, dir int, ok bool) {
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

type scanPix struct {
	x, y       float64
	r, g, b, a uint8
	in         bool
}

func (b blob) scan(want *image.NRGBA, pad int) []scanPix {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	x0 := max(0, b.x0-pad)
	y0 := max(0, b.y0-pad)
	x1 := min(w, b.x0+b.bw+pad)
	y1 := min(h, b.y0+b.bh+pad)
	area := (x1 - x0) * (y1 - y0)
	step := 1
	if area > 250_000 {
		step = int(math.Sqrt(float64(area) / 250_000))
		if step < 2 {
			step = 2
		}
	}
	out := make([]scanPix, 0, (area+step*step-1)/(step*step))
	for y := y0; y < y1; y += step {
		off := y * want.Stride
		for x := x0; x < x1; x += step {
			i := off + 4*x
			out = append(out, scanPix{
				x: float64(x) + 0.5, y: float64(y) + 0.5,
				r: want.Pix[i], g: want.Pix[i+1], b: want.Pix[i+2], a: want.Pix[i+3],
				in: b.at(x, y),
			})
		}
	}
	return out
}

type fitScore struct {
	sse   float64
	miss  int
	cover int
}

func betterFit(a, b fitScore) bool {
	if a.miss != b.miss {
		return a.miss < b.miss
	}
	if a.sse != b.sse {
		return a.sse < b.sse
	}
	return a.cover < b.cover
}

func scoreNode(n svg.Node, px []scanPix, fill color.NRGBA) fitScore {
	if n.Kind() == svg.KindInvalid {
		return fitScore{sse: math.Inf(1)}
	}
	fr, fg, fb := fill.R, fill.G, fill.B
	var sse float64
	cover, miss := 0, 0
	for i := range px {
		p := &px[i]
		if covers(n, p.x, p.y) {
			cover++
			if p.a != 0 {
				sse += float64(u8sq(fr, p.r) + u8sq(fg, p.g) + u8sq(fb, p.b))
			}
			continue
		}
		if p.in {
			miss++
			if p.a != 0 {
				r, g, b := uint32(p.r), uint32(p.g), uint32(p.b)
				sse += float64(r*r + g*g + b*b)
			}
		}
	}
	return fitScore{sse: sse, miss: miss, cover: cover}
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

func blobRound(b blob) bool {
	if b.bw < 8 || b.bh < 8 {
		return false
	}
	aspect := float64(b.bw) / float64(b.bh)
	if aspect < 0.85 || aspect > 1.15 {
		return false
	}
	fill := float64(b.n) / float64(b.bw*b.bh)
	// Filled disk is ~π/4. Rings/letters sit lower; keep those on bbox rects.
	return fill >= 0.60 && fill <= 0.86
}

func fitBlob(want *image.NRGBA, b blob, cw, ch int) svg.Node {
	px := b.scan(want, blobNudgePad)
	W, H := float64(cw), float64(ch)
	round := blobRound(b)
	var best svg.Node
	if round {
		best = seedCircle(b, W, H)
	}
	if best.Kind() == svg.KindInvalid {
		best = seedRect(b)
	}
	bestSc := scoreNode(best, px, b.fill)
	best = nudgeLocal(best, px, b.fill, W, H, bestSc)
	bestSc = scoreNode(best, px, b.fill)

	try := []svg.Node{seedEllipse(b, W, H)}
	if !round {
		try = append([]svg.Node{seedCircle(b, W, H)}, try...)
	}
	for _, seed := range try {
		if seed.Kind() == svg.KindInvalid {
			continue
		}
		sc := scoreNode(seed, px, b.fill)
		n := nudgeLocal(seed, px, b.fill, W, H, sc)
		nsc := scoreNode(n, px, b.fill)
		if betterFit(nsc, bestSc) {
			best, bestSc = n, nsc
		}
	}
	if b.n <= 100_000 {
		poly := seedPolygon(b)
		if poly.Kind() != svg.KindInvalid {
			psc := scoreNode(poly, px, b.fill)
			if betterFit(psc, bestSc) {
				best = poly
			}
		}
	}
	return best
}

func nudgeLocal(n svg.Node, px []scanPix, fill color.NRGBA, W, H float64, sc fitScore) svg.Node {
	best, bestSc := n, sc
	for _, step := range []float64{2, 1} {
		improved := false
		for k := 0; k < 6; k++ {
			improved = false
			np := paramCount(best)
			for p := 0; p < np; p++ {
				for _, dir := range [2]float64{-step, step} {
					cand, ok := nudgeNode(best, p, dir, W, H)
					if !ok {
						continue
					}
					nsc := scoreNode(cand, px, fill)
					if betterFit(nsc, bestSc) {
						best, bestSc = cand, nsc
						improved = true
					}
				}
			}
			if !improved {
				break
			}
		}
	}
	return best
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
		if w <= 0 || h <= 0 {
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
		if r <= 0 || clampRad(r, cx, cy, W, H) != r {
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
		if rx <= 0 || ry <= 0 || clampAxis(rx, cx, W) != rx || clampAxis(ry, cy, H) != ry {
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
		np, err := p.WithPoints(pts)
		if err != nil {
			return n, false
		}
		return np.Node(), true
	default:
		return n, false
	}
}
