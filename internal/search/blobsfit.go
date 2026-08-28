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

// BlobsFit is connected-component blobs scored on loss.Fit, not raw SSE.
// Speckles smaller than max(32, 0.02% of canvas area) are dropped.
type BlobsFit struct {
	Colors  int // 0 = auto, cap 8
	Renders int // Render calls used by the last Search
}

var _ Search = (*BlobsFit)(nil)

const (
	bfRenderBudget = 200
	bfMaxKids      = 4096
	bfPruneArea    = 1_200_000
)

func init() {
	Register("blobsfit", func() Search { return &BlobsFit{} })
}

func (b *BlobsFit) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if b == nil {
		return svg.Document{}, fmt.Errorf("search: nil BlobsFit")
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
	blobs := bfColorBlobs(want, cmap, pal, bfSpeckleMin(w, h))
	if len(blobs) == 0 {
		return doc, nil
	}
	if len(blobs) > bfMaxKids {
		blobs = blobs[:bfMaxKids]
	}
	s := newBFSess(ctx, want, w, h)
	var kids []svg.Node
	cur := s.fitOf(s.sse, 0)
	for _, bl := range blobs {
		if err := ctx.Err(); err != nil {
			b.Renders = s.used
			return doc.Append(kids...), err
		}
		if len(kids) >= bfMaxKids {
			break
		}
		node := s.fitBlob(bl)
		if node.Kind() == svg.KindInvalid {
			continue
		}
		// Accept iff Fit strictly drops (RMSE/255 + λk). Same-k family
		// choice is RMSE; adding a part must cut RMSE by more than λ·255.
		next := s.trialFit(node, len(kids)+1)
		if next < cur {
			s.sse += paintDelta(s.got, want, node, true)
			kids = append(kids, node)
			cur = next
		}
	}
	kids = s.prune(kids)
	b.Renders = s.used
	return doc.Append(kids...), nil
}

// bfSpeckleMin is max(32, 0.02% of canvas area).
func bfSpeckleMin(w, h int) int {
	return max(32, w*h/5000)
}

type bfSess struct {
	ctx  context.Context
	want *image.NRGBA
	got  *image.NRGBA
	w, h float64
	n    int
	sse  float64
	left int
	used int
}

func newBFSess(ctx context.Context, want *image.NRGBA, w, h int) *bfSess {
	return &bfSess{
		ctx:  ctx,
		want: want,
		got:  image.NewNRGBA(image.Rect(0, 0, w, h)),
		w:    float64(w),
		h:    float64(h),
		n:    bfScoredN(want),
		sse:  emptySSE(want),
		left: bfRenderBudget,
	}
}

func bfScoredN(want *image.NRGBA) int {
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		off := y * want.Stride
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			if want.Pix[off+4*x+3] != 0 {
				n++
			}
		}
	}
	return n
}

// fitOf is loss.Fit from incremental SSE: RMSE/255 + λ·k.
func (s *bfSess) fitOf(sse float64, k int) float64 {
	if s.n <= 0 {
		if k < 0 {
			k = 0
		}
		return loss.Lambda * float64(k)
	}
	rmse := math.Sqrt(sse / (3 * float64(s.n)))
	if math.IsInf(rmse, 0) || math.IsNaN(rmse) {
		return rmse
	}
	if k < 0 {
		k = 0
	}
	return rmse/255 + loss.Lambda*float64(k)
}

func (s *bfSess) trialFit(n svg.Node, k int) float64 {
	if n.Kind() == svg.KindInvalid {
		return math.Inf(1)
	}
	return s.fitOf(s.sse+paintDelta(s.got, s.want, n, false), k)
}

func (s *bfSess) eval(nodes []svg.Node) (float64, bool) {
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
	return loss.Fit(got, s.want, svg.PartsDocument(doc)), true
}

func (s *bfSess) paintFit(nodes []svg.Node) float64 {
	got := image.NewNRGBA(s.want.Rect)
	sse := emptySSE(s.want)
	for _, n := range nodes {
		sse += paintDelta(got, s.want, n, true)
	}
	k := 0
	for _, n := range nodes {
		k += svg.Parts(n)
	}
	return loss.Fit(got, s.want, k)
}

// prune deletes a part when Fit decreases (RMSE rise < λ·255 per deleted part).
func (s *bfSess) prune(kids []svg.Node) []svg.Node {
	if len(kids) < 2 {
		return kids
	}
	useRender := s.w*s.h <= bfPruneArea && s.left > 0
	var cur float64
	if useRender {
		sc, ok := s.eval(kids)
		if !ok {
			return kids
		}
		cur = sc
	} else {
		cur = s.paintFit(kids)
	}
	i := len(kids) - 1
	for i >= 0 && (s.left > 0 || !useRender) {
		if s.ctx.Err() != nil {
			break
		}
		trial := append(append([]svg.Node(nil), kids[:i]...), kids[i+1:]...)
		var sc float64
		if useRender {
			var ok bool
			sc, ok = s.eval(trial)
			if !ok {
				break
			}
		} else {
			sc = s.paintFit(trial)
		}
		if sc < cur {
			kids = trial
			cur = sc
			if i >= len(kids) {
				i = len(kids) - 1
			}
			continue
		}
		i--
	}
	return kids
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

type bfBlob struct {
	x0, y0, bw, bh int
	n              int
	fill           color.NRGBA
	colorN         int
}

func bfColorBlobs(want *image.NRGBA, cmap palette.ColorMap, pal []color.NRGBA, speckle int) []bfBlob {
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
	var blobs []bfBlob
	var biggest bfBlob
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
			bw, bh := maxX-minX, maxY-minY
			fill := pal[lab]
			fillN := 0
			for c, n := range hist {
				if n > fillN || (n == fillN && lessNRGBA(c, fill)) {
					fill, fillN = c, n
				}
			}
			bl := bfBlob{
				x0: minX, y0: minY, bw: bw, bh: bh,
				n: len(q), fill: fill, colorN: counts[lab],
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
		blobs = []bfBlob{biggest}
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
	default:
		return n
	}
}

func seedRect(b bfBlob) svg.Node {
	r := svg.NewRect().WithX(float64(b.x0)).WithY(float64(b.y0)).
		WithWidth(float64(b.bw)).WithHeight(float64(b.bh))
	return applyFill(r.Node(), b.fill)
}

func seedCircle(b bfBlob, W, H float64) svg.Node {
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

func seedEllipse(b bfBlob, W, H float64) svg.Node {
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
	default:
		return false
	}
}

func (s *bfSess) fitBlob(b bfBlob) svg.Node {
	// Same k for every family: ranking is RMSE after swapping this primitive.
	k := 1
	climb := b.bw*b.bh <= 250_000
	best := seedRect(b)
	bestSc := s.trialFit(best, k)
	if climb {
		best = s.nudge(best, k, bestSc)
		bestSc = s.trialFit(best, k)
	}

	for _, seed := range []svg.Node{seedCircle(b, s.w, s.h), seedEllipse(b, s.w, s.h)} {
		if seed.Kind() == svg.KindInvalid {
			continue
		}
		sc := s.trialFit(seed, k)
		n := seed
		if climb {
			n = s.nudge(seed, k, sc)
			sc = s.trialFit(n, k)
		}
		if sc < bestSc {
			best, bestSc = n, sc
		}
	}
	return best
}

func (s *bfSess) nudge(n svg.Node, k int, sc float64) svg.Node {
	best, bestSc := n, sc
	for _, step := range []float64{2, 1} {
		for i := 0; i < 6; i++ {
			improved := false
			np := paramCount(best)
			for p := 0; p < np; p++ {
				for _, dir := range [2]float64{-step, step} {
					cand, ok := nudgeNode(best, p, dir, s.w, s.h)
					if !ok {
						continue
					}
					nsc := s.trialFit(cand, k)
					if nsc < bestSc {
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
	default:
		return n, false
	}
}
