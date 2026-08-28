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
	"github.com/lewtec/svgolf/pkg/svg"
)

// Simplify traces each color island as a path, then drops points while
// the island stays covered. Cubics replace long runs when they stay close.
type Simplify struct {
	Colors int // 0 = auto, cap 8
}

var _ Search = Simplify{}

const simpCands = 128 // compute bound: only the largest islands can ever pay Fit

type simpItem struct {
	b  simpBlob
	n  svg.Node
	on bool
}

func init() {
	Register("simplify", func() Search { return Simplify{} })
}

func (s Simplify) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	want := FromImage(target)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return doc, nil
	}
	nPal := s.Colors
	if nPal <= 0 {
		nPal = 8
	}
	cmap, pal, err := palette.Auto(want, nPal)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}
	blobs := simpBlobs(want, func(c color.NRGBA) color.NRGBA {
		m := cmap.Map(c)
		m.A = 255
		return m
	})
	if len(blobs) > simpCands {
		blobs = blobs[:simpCands]
	}
	for i := range blobs {
		blobs[i].col = simpMean(want, blobs[i].pix)
	}
	items := make([]simpItem, 0, len(blobs))
	for _, b := range blobs {
		if err := ctx.Err(); err != nil {
			return doc, err
		}
		n, ok := simpPath(b, w, h)
		if !ok {
			continue
		}
		items = append(items, simpItem{b: b, n: n})
	}
	nScore := simpScoredN(want)
	got := image.NewNRGBA(image.Rect(0, 0, w, h))
	sse := simpEmptySSE(want)
	// Layer: largest island is the back plate. Each next island
	// that still cuts residual SSE paints on top.
	for i := range items {
		if err := ctx.Err(); err != nil {
			break
		}
		d := simpPaint(got, want, items[i].b, false)
		if d >= 0 {
			continue
		}
		sse += simpPaint(got, want, items[i].b, true)
		items[i].on = true
	}
	// Converge: peel a layer if Fit drops (coverage is already there).
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		k := 0
		for _, it := range items {
			if it.on {
				k++
			}
		}
		cur := simpFit(sse, nScore, k)
		improved := false
		for i := len(items) - 1; i >= 0; i-- {
			if !items[i].on {
				continue
			}
			got2 := image.NewNRGBA(image.Rect(0, 0, w, h))
			sse2 := simpEmptySSE(want)
			k2 := 0
			for j := range items {
				if !items[j].on || j == i {
					continue
				}
				sse2 += simpPaint(got2, want, items[j].b, true)
				k2++
			}
			if simpFit(sse2, nScore, k2) < cur {
				items[i].on = false
				got, sse = got2, sse2
				improved = true
				break
			}
		}
		if !improved {
			break
		}
	}
	type kid struct {
		n    svg.Node
		area int
	}
	var order []kid
	for _, it := range items {
		if it.on {
			order = append(order, kid{it.n, it.b.area})
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].area > order[j].area })
	var kids []svg.Node
	for _, k := range order {
		kids = append(kids, k.n)
	}
	return doc.Append(kids...), nil
}

type simpBlob struct {
	col  color.NRGBA
	pix  []image.Point
	area int
}

func simpBlobs(want *image.NRGBA, snapFn func(color.NRGBA) color.NRGBA) []simpBlob {
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	snap := make([]color.NRGBA, w*h)
	seen := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := want.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A == 0 {
				continue
			}
			snap[y*w+x] = snapFn(c)
		}
	}
	var out []simpBlob
	dirs := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			c := snap[i]
			if c.A == 0 || seen[i] {
				continue
			}
			seen[i] = true
			q := []image.Point{{x, y}}
			pix := []image.Point{{x, y}}
			for len(q) > 0 {
				p := q[0]
				q = q[1:]
				for _, d := range dirs {
					nx, ny := p.X+d[0], p.Y+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					j := ny*w + nx
					if seen[j] || snap[j] != c {
						continue
					}
					seen[j] = true
					q = append(q, image.Point{nx, ny})
					pix = append(pix, image.Point{nx, ny})
				}
			}
			out = append(out, simpBlob{col: c, pix: pix, area: len(pix)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].area != out[j].area {
			return out[i].area > out[j].area
		}
		return lessNRGBA(out[i].col, out[j].col)
	})
	return out
}

type simpGroup struct {
	col   color.NRGBA
	blobs []simpBlob
	area  int
}

func simpColorGroups(blobs []simpBlob) []simpGroup {
	idx := map[color.NRGBA]int{}
	var gs []simpGroup
	for _, b := range blobs {
		i, ok := idx[b.col]
		if !ok {
			i = len(gs)
			idx[b.col] = i
			gs = append(gs, simpGroup{col: b.col})
		}
		gs[i].blobs = append(gs[i].blobs, b)
		gs[i].area += b.area
	}
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].area != gs[j].area {
			return gs[i].area > gs[j].area
		}
		return lessNRGBA(gs[i].col, gs[j].col)
	})
	return gs
}

func simpScoredN(want *image.NRGBA) int {
	n := 0
	b := want.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if want.NRGBAAt(x, y).A != 0 {
				n++
			}
		}
	}
	return n
}

func simpEmptySSE(want *image.NRGBA) float64 {
	var sse float64
	b := want.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			sse += float64(uint32(q.R)*uint32(q.R) + uint32(q.G)*uint32(q.G) + uint32(q.B)*uint32(q.B))
		}
	}
	return sse
}

func simpFit(sse float64, n, k int) float64 {
	if n <= 0 {
		return loss.Lambda * float64(max(k, 0))
	}
	rmse := math.Sqrt(sse / (3 * float64(n)))
	if math.IsInf(rmse, 0) || math.IsNaN(rmse) {
		return rmse
	}
	return rmse/255 + loss.Lambda*float64(max(k, 0))
}

func simpPaint(got, want *image.NRGBA, b simpBlob, commit bool) float64 {
	var d float64
	for _, p := range b.pix {
		q := want.NRGBAAt(p.X, p.Y)
		if q.A == 0 {
			continue
		}
		g := got.NRGBAAt(p.X, p.Y)
		old := simpChan2(g.R, q.R) + simpChan2(g.G, q.G) + simpChan2(g.B, q.B)
		neu := simpChan2(b.col.R, q.R) + simpChan2(b.col.G, q.G) + simpChan2(b.col.B, q.B)
		d += float64(neu) - float64(old)
		if commit {
			got.SetNRGBA(p.X, p.Y, color.NRGBA{R: b.col.R, G: b.col.G, B: b.col.B, A: 255})
		}
	}
	return d
}

func simpChan2(a, b uint8) uint32 {
	d := int(a) - int(b)
	return uint32(d * d)
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

func simpPath(b simpBlob, w, h int) (svg.Node, bool) {
	loops := simpTrace(b, w, h)
	if len(loops) == 0 {
		return svg.Node{}, false
	}
	m := newSimpMask(b, w, h)
	best := simpConverge(loops, m)
	if len(best) == 0 {
		return svg.Node{}, false
	}
	cmds := simpEmit(best)
	for n := 2.0; len(cmds) > 4096 && n <= 64; n *= 2 {
		for i := range best {
			best[i] = simpRDPClosed(best[i], n)
		}
		cmds = simpEmit(best)
	}
	if len(cmds) == 0 || len(cmds) > 4096 {
		return svg.Node{}, false
	}
	p, err := svg.NewPath().WithCommands(cmds)
	if err != nil {
		return svg.Node{}, false
	}
	if len(best) > 1 {
		p = p.WithFillRule(svg.FillEvenOdd)
	}
	p = p.WithFill(color.NRGBA{R: b.col.R, G: b.col.G, B: b.col.B, A: 255})
	if b.col.A != 255 {
		p = p.WithFillOpacity(float64(b.col.A) / 255)
	}
	return p.Node(), true
}

type ipt struct{ x, y int }

func simpStep(b simpBlob) int {
	minX, minY, maxX, maxY := b.pix[0].X, b.pix[0].Y, b.pix[0].X, b.pix[0].Y
	for _, p := range b.pix[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	box := (maxX - minX + 1) * (maxY - minY + 1)
	if box > 0 && b.area*10 >= box*3 {
		return 1
	}
	switch {
	case b.area > 200000:
		return 8
	case b.area > 40000:
		return 4
	case b.area > 10000:
		return 2
	default:
		return 1
	}
}

func simpTrace(b simpBlob, w, h int) [][][2]float64 {
	if len(b.pix) == 0 {
		return nil
	}
	step := simpStep(b)
	tw := (w + step - 1) / step
	th := (h + step - 1) / step
	mask := make([]bool, tw*th)
	for _, p := range b.pix {
		mask[(p.Y/step)*tw+(p.X/step)] = true
	}
	in := func(x, y int) bool {
		if x < 0 || y < 0 || x >= tw || y >= th {
			return false
		}
		return mask[y*tw+x]
	}
	type ekey struct{ x0, y0, x1, y1 int }
	outg := map[ipt][]ipt{}
	var keys []ipt
	add := func(x0, y0, x1, y1 int) {
		a, b := ipt{x0, y0}, ipt{x1, y1}
		if len(outg[a]) == 0 {
			keys = append(keys, a)
		}
		outg[a] = append(outg[a], b)
	}
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			if !in(x, y) {
				continue
			}
			if !in(x, y-1) {
				add(x, y, x+1, y)
			}
			if !in(x+1, y) {
				add(x+1, y, x+1, y+1)
			}
			if !in(x, y+1) {
				add(x+1, y+1, x, y+1)
			}
			if !in(x-1, y) {
				add(x, y+1, x, y)
			}
		}
	}
	used := map[ekey]bool{}
	take := func(from ipt) (ipt, bool) {
		for _, to := range outg[from] {
			k := ekey{from.x, from.y, to.x, to.y}
			if !used[k] {
				used[k] = true
				return to, true
			}
		}
		return ipt{}, false
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].y != keys[j].y {
			return keys[i].y < keys[j].y
		}
		return keys[i].x < keys[j].x
	})
	var loops [][][2]float64
	for _, start := range keys {
		to, ok := take(start)
		if !ok {
			continue
		}
		ring := []ipt{start}
		cur := to
		for cur != start {
			ring = append(ring, cur)
			nxt, ok := take(cur)
			if !ok {
				break
			}
			cur = nxt
		}
		if len(ring) < 3 {
			continue
		}
		if a := simpArea(ring); a < 16 {
			continue
		}
		ring = simpCollapse(ring)
		if len(ring) < 3 {
			continue
		}
		lp := make([][2]float64, len(ring))
		sf := float64(step)
		for i, p := range ring {
			lp[i] = [2]float64{float64(p.x) * sf, float64(p.y) * sf}
		}
		loops = append(loops, lp)
	}
	if len(loops) > 8 {
		sort.Slice(loops, func(i, j int) bool { return simpFArea(loops[i]) > simpFArea(loops[j]) })
		loops = loops[:8]
	}
	return loops
}

func simpArea(pts []ipt) int {
	s := 0
	n := len(pts)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		s += pts[i].x*pts[j].y - pts[j].x*pts[i].y
	}
	if s < 0 {
		s = -s
	}
	return s / 2
}

func simpFArea(lp [][2]float64) float64 {
	s := 0.0
	n := len(lp)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		s += lp[i][0]*lp[j][1] - lp[j][0]*lp[i][1]
	}
	if s < 0 {
		s = -s
	}
	return s / 2
}

func simpCollapse(pts []ipt) []ipt {
	n := len(pts)
	if n < 3 {
		return pts
	}
	out := make([]ipt, 0, n)
	for i := 0; i < n; i++ {
		a, b, c := pts[(i-1+n)%n], pts[i], pts[(i+1)%n]
		if (b.x-a.x)*(c.y-b.y) == (b.y-a.y)*(c.x-b.x) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func simpSample(pix []image.Point, max int) []image.Point {
	if max <= 0 || len(pix) <= max {
		return pix
	}
	out := make([]image.Point, max)
	for i := 0; i < max; i++ {
		out[i] = pix[i*len(pix)/max]
	}
	return out
}

func simpErode(pix []image.Point, w, h int) []image.Point {
	if len(pix) == 0 {
		return nil
	}
	mask := make([]bool, w*h)
	for _, p := range pix {
		mask[p.Y*w+p.X] = true
	}
	in := func(x, y int) bool {
		if x < 0 || y < 0 || x >= w || y >= h {
			return false
		}
		return mask[y*w+x]
	}
	var out []image.Point
	for _, p := range pix {
		if in(p.X-1, p.Y) && in(p.X+1, p.Y) && in(p.X, p.Y-1) && in(p.X, p.Y+1) {
			out = append(out, p)
		}
	}
	return out
}

type simpMask struct {
	in, core, hull []bool
	w, h           int
}

func newSimpMask(b simpBlob, w, h int) *simpMask {
	m := &simpMask{in: make([]bool, w*h), core: make([]bool, w*h), hull: make([]bool, w*h), w: w, h: h}
	for _, p := range b.pix {
		m.in[p.Y*w+p.X] = true
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				x, y := p.X+dx, p.Y+dy
				if x >= 0 && y >= 0 && x < w && y < h {
					m.hull[y*w+x] = true
				}
			}
		}
	}
	core := simpErode(b.pix, w, h)
	if len(core) == 0 {
		core = b.pix
	}
	for _, p := range core {
		m.core[p.Y*w+p.X] = true
	}
	return m
}

func simpOutside(b simpBlob, w, h, max int) []image.Point {
	in := make([]bool, w*h)
	minX, minY, maxX, maxY := w, h, 0, 0
	for _, p := range b.pix {
		in[p.Y*w+p.X] = true
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	var out []image.Point
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !in[y*w+x] {
				out = append(out, image.Point{x, y})
			}
		}
	}
	return simpSample(out, max)
}

func simpConverge(loops [][][2]float64, m *simpMask) [][][2]float64 {
	cur := copyLoops(loops)
	for {
		n0 := simpCount(cur)
		for i := range cur {
			cur[i] = simpCollapseF(cur[i])
		}
		cur = simpDrop(cur, m)
		if simpCount(cur) >= n0 {
			break
		}
	}
	return cur
}

func simpCollapseF(pts [][2]float64) [][2]float64 {
	n := len(pts)
	if n < 3 {
		return pts
	}
	out := make([][2]float64, 0, n)
	for i := 0; i < n; i++ {
		a, b, c := pts[(i-1+n)%n], pts[i], pts[(i+1)%n]
		if (b[0]-a[0])*(c[1]-b[1]) == (b[1]-a[1])*(c[0]-b[0]) {
			continue
		}
		out = append(out, b)
	}
	if len(out) < 3 {
		return pts
	}
	return out
}

// simpSpanRDP splits each loop at sharp corners and RDP's each arc alone,
// so a long circle can collapse without filling open bays.
func simpSpanRDP(loops [][][2]float64, pix, outp []image.Point) [][][2]float64 {
	eps := []float64{2, 3, 4, 6, 8}
	out := copyLoops(loops)
	for li := range out {
		for _, e := range eps {
			trial := copyLoops(out)
			got := simpRDPBySpan(trial[li], e)
			if len(got) < 3 {
				continue
			}
			trial[li] = got
			if simpCovers(trial, pix) && !simpSpills(trial, outp) {
				out = trial
			}
		}
	}
	return out
}

func simpRDPBySpan(lp [][2]float64, eps float64) [][2]float64 {
	n := len(lp)
	if n < 4 {
		return lp
	}
	corner := simpCorners(lp)
	var cuts []int
	for i := 0; i < n; i++ {
		if corner[i] {
			cuts = append(cuts, i)
		}
	}
	if len(cuts) < 2 {
		return simpRDPClosed(lp, eps)
	}
	var out [][2]float64
	for i := 0; i < len(cuts); i++ {
		a := cuts[i]
		b := cuts[(i+1)%len(cuts)]
		var span [][2]float64
		if b > a {
			span = append([][2]float64(nil), lp[a:b+1]...)
		} else {
			span = append([][2]float64(nil), lp[a:]...)
			span = append(span, lp[:b+1]...)
		}
		span = simpRDPOpen(span, eps)
		if i > 0 && len(out) > 0 && len(span) > 0 {
			span = span[1:]
		}
		out = append(out, span...)
	}
	if len(out) > 1 && out[0] == out[len(out)-1] {
		out = out[:len(out)-1]
	}
	if len(out) < 3 {
		return lp
	}
	return out
}

func simpCorners(lp [][2]float64) []bool {
	n := len(lp)
	out := make([]bool, n)
	for i := 0; i < n; i++ {
		if turnDeg(lp[(i-1+n)%n], lp[i], lp[(i+1)%n]) >= 40 {
			out[i] = true
		}
	}
	return out
}

func simpCount(loops [][][2]float64) int {
	n := 0
	for _, lp := range loops {
		n += len(lp)
	}
	return n
}

func simpDrop(loops [][][2]float64, m *simpMask) [][][2]float64 {
	cur := copyLoops(loops)
	for li := range cur {
		for len(cur[li]) > 4 {
			i := simpDropAt(cur[li], m)
			if i < 0 {
				break
			}
			cur[li] = append(cur[li][:i], cur[li][i+1:]...)
		}
	}
	return cur
}

func simpDropAt(lp [][2]float64, m *simpMask) int {
	n := len(lp)
	if n <= 4 {
		return -1
	}
	ccw := simpFArea(lp) > 0
	best, bestA := -1, math.MaxFloat64
	for i := 0; i < n; i++ {
		a, b, c := lp[(i-1+n)%n], lp[i], lp[(i+1)%n]
		area := math.Abs((b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0]))
		if area >= bestA {
			continue
		}
		if !m.earOK(a, b, c, ccw) {
			continue
		}
		best, bestA = i, area
	}
	return best
}

func (m *simpMask) earOK(a, b, c [2]float64, ccw bool) bool {
	cross := (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
	convex := cross > 0
	if !ccw {
		convex = cross < 0
	}
	minX := int(math.Floor(math.Min(a[0], math.Min(b[0], c[0]))))
	maxX := int(math.Ceil(math.Max(a[0], math.Max(b[0], c[0]))))
	minY := int(math.Floor(math.Min(a[1], math.Min(b[1], c[1]))))
	maxY := int(math.Ceil(math.Max(a[1], math.Max(b[1], c[1]))))
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX >= m.w {
		maxX = m.w - 1
	}
	if maxY >= m.h {
		maxY = m.h - 1
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !inTri(float64(x)+0.5, float64(y)+0.5, a, b, c) {
				continue
			}
			i := y*m.w + x
			if convex && m.core[i] {
				return false
			}
			if !convex && !m.hull[i] {
				return false
			}
		}
	}
	return true
}

func inTri(x, y float64, a, b, c [2]float64) bool {
	d1 := (x-a[0])*(b[1]-a[1]) - (y-a[1])*(b[0]-a[0])
	d2 := (x-b[0])*(c[1]-b[1]) - (y-b[1])*(c[0]-b[0])
	d3 := (x-c[0])*(a[1]-c[1]) - (y-c[1])*(c[0]-c[0])
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func copyLoops(in [][][2]float64) [][][2]float64 {
	out := make([][][2]float64, len(in))
	for i, lp := range in {
		out[i] = append([][2]float64(nil), lp...)
	}
	return out
}

func simpSpills(loops [][][2]float64, outp []image.Point) bool {
	if len(outp) == 0 {
		return false
	}
	n := 0
	for _, p := range outp {
		if simpPIP(float64(p.X)+0.5, float64(p.Y)+0.5, loops) {
			n++
		}
	}
	return n > 0
}

func simpCovers(loops [][][2]float64, pix []image.Point) bool {
	if len(pix) == 0 {
		return true
	}
	hit := 0
	for _, p := range pix {
		if simpPIP(float64(p.X)+0.5, float64(p.Y)+0.5, loops) {
			hit++
		}
	}
	return hit == len(pix)
}

func simpPIP(x, y float64, loops [][][2]float64) bool {
	inside := false
	for _, lp := range loops {
		n := len(lp)
		for i, j := 0, n-1; i < n; j, i = i, i+1 {
			yi, yj := lp[i][1], lp[j][1]
			if (yi > y) == (yj > y) {
				continue
			}
			xi, xj := lp[i][0], lp[j][0]
			if x < (xj-xi)*(y-yi)/(yj-yi)+xi {
				inside = !inside
			}
		}
	}
	return inside
}

func simpRDPClosed(pts [][2]float64, eps float64) [][2]float64 {
	n := len(pts)
	if n <= 3 {
		return pts
	}
	iMax, dMax := 1, 0.0
	for i := 1; i < n; i++ {
		d := hypot2(pts[i][0]-pts[0][0], pts[i][1]-pts[0][1])
		if d > dMax {
			iMax, dMax = i, d
		}
	}
	left := simpRDPOpen(pts[:iMax+1], eps)
	right := make([][2]float64, 0, n-iMax+1)
	right = append(right, pts[iMax:]...)
	right = append(right, pts[0])
	right = simpRDPOpen(right, eps)
	out := make([][2]float64, 0, len(left)+len(right)-2)
	out = append(out, left[:len(left)-1]...)
	out = append(out, right[:len(right)-1]...)
	if len(out) < 3 {
		return pts
	}
	return out
}

func simpRDPOpen(pts [][2]float64, eps float64) [][2]float64 {
	if len(pts) <= 2 {
		return pts
	}
	eps2 := eps * eps
	idx, dMax := 0, 0.0
	a, b := pts[0], pts[len(pts)-1]
	for i := 1; i < len(pts)-1; i++ {
		d := distSeg2(pts[i], a, b)
		if d > dMax {
			idx, dMax = i, d
		}
	}
	if dMax <= eps2 {
		return [][2]float64{a, b}
	}
	left := simpRDPOpen(pts[:idx+1], eps)
	right := simpRDPOpen(pts[idx:], eps)
	return append(left[:len(left)-1], right...)
}

func distSeg2(p, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return hypot2(p[0]-a[0], p[1]-a[1])
	}
	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return hypot2(p[0]-(a[0]+t*dx), p[1]-(a[1]+t*dy))
}

func hypot2(x, y float64) float64 { return x*x + y*y }

func roundHalf(v float64) float64 { return math.Round(v*2) / 2 }

func simpEmit(loops [][][2]float64) []svg.PathCmd {
	var cmds []svg.PathCmd
	for _, lp := range loops {
		if len(lp) < 3 {
			continue
		}
		cmds = append(cmds, simpSmooth(lp)...)
	}
	return cmds
}

func simpMean(want *image.NRGBA, pix []image.Point) color.NRGBA {
	var r, g, b, n int64
	for _, p := range pix {
		c := want.NRGBAAt(p.X, p.Y)
		if c.A == 0 {
			continue
		}
		r += int64(c.R)
		g += int64(c.G)
		b += int64(c.B)
		n++
	}
	if n == 0 {
		return color.NRGBA{A: 255}
	}
	return color.NRGBA{
		R: uint8((r + n/2) / n),
		G: uint8((g + n/2) / n),
		B: uint8((b + n/2) / n),
		A: 255,
	}
}

func simpSmooth(lp [][2]float64) []svg.PathCmd {
	n := len(lp)
	if n < 5 {
		return simpLines(lp)
	}
	corner := simpCorners(lp)
	nCorner := 0
	for _, c := range corner {
		if c {
			nCorner++
		}
	}
	if nCorner == n {
		return simpLines(lp)
	}
	if nCorner == 0 && n >= 8 {
		return simpFourCubics(lp)
	}
	cmds := []svg.PathCmd{{Kind: svg.CmdMove, X: lp[0][0], Y: lp[0][1]}}
	for i := 0; i < n; {
		j := i + 1
		if j >= n {
			break
		}
		if corner[i] || corner[j] || collinear3(lp[(i-1+n)%n], lp[i], lp[j]) {
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: lp[j][0], Y: lp[j][1]})
			i = j
			continue
		}
		for j+1 < n && !corner[j] && !corner[j+1] {
			j++
		}
		merged := false
		for t := j; t >= i+3; t-- {
			run := lp[i : t+1]
			ok, c1, c2 := fitCubic(run, fitCubicEps(run))
			if !ok {
				continue
			}
			end := run[len(run)-1]
			cmds = append(cmds, svg.PathCmd{
				Kind: svg.CmdCubic,
				X1:   c1[0], Y1: c1[1], X2: c2[0], Y2: c2[1],
				X: end[0], Y: end[1],
			})
			i = t
			merged = true
			break
		}
		if merged {
			continue
		}
		a, b := lp[i], lp[(i+1)%n]
		prev, next := lp[(i-1+n)%n], lp[(i+2)%n]
		cmds = append(cmds, svg.PathCmd{
			Kind: svg.CmdCubic,
			X1:   roundHalf(a[0] + (b[0]-prev[0])/6),
			Y1:   roundHalf(a[1] + (b[1]-prev[1])/6),
			X2:   roundHalf(b[0] - (next[0]-a[0])/6),
			Y2:   roundHalf(b[1] - (next[1]-a[1])/6),
			X:    b[0], Y: b[1],
		})
		i++
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	return cmds
}

func simpFourCubics(lp [][2]float64) []svg.PathCmd {
	n := len(lp)
	cmds := []svg.PathCmd{{Kind: svg.CmdMove, X: lp[0][0], Y: lp[0][1]}}
	for q := 0; q < 4; q++ {
		a := q * n / 4
		b := (q + 1) * n / 4
		if q == 3 {
			b = n
		}
		run := append([][2]float64(nil), lp[a:min(b+1, n)]...)
		if q == 3 {
			run = append(run, lp[0])
		}
		if len(run) < 4 {
			end := run[len(run)-1]
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: end[0], Y: end[1]})
			continue
		}
		ok, c1, c2 := fitCubic(run, fitCubicEps(run))
		end := run[len(run)-1]
		if !ok {
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: end[0], Y: end[1]})
			continue
		}
		cmds = append(cmds, svg.PathCmd{
			Kind: svg.CmdCubic,
			X1:   c1[0], Y1: c1[1], X2: c2[0], Y2: c2[1],
			X: end[0], Y: end[1],
		})
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	return cmds
}

func simpSpanSmooth(run [][2]float64) bool {
	if len(run) < 3 {
		return false
	}
	for i := 1; i < len(run)-1; i++ {
		if turnDeg(run[i-1], run[i], run[i+1]) >= 35 {
			return false
		}
	}
	return true
}

func fitCubicEps(pts [][2]float64) float64 {
	if len(pts) < 2 {
		return 4
	}
	a, b := pts[0], pts[len(pts)-1]
	return 4 + math.Hypot(b[0]-a[0], b[1]-a[1])/25
}

func fitCubic(pts [][2]float64, eps float64) (bool, [2]float64, [2]float64) {
	if len(pts) < 4 {
		return false, [2]float64{}, [2]float64{}
	}
	c1, c2 := fitCubicAlways(pts)
	p0, p3 := pts[0], pts[len(pts)-1]
	eps2 := eps * eps
	n := len(pts)
	for k := 1; k < n-1; k++ {
		t := float64(k) / float64(n-1)
		u := 1 - t
		x := u*u*u*p0[0] + 3*u*u*t*c1[0] + 3*u*t*t*c2[0] + t*t*t*p3[0]
		y := u*u*u*p0[1] + 3*u*u*t*c1[1] + 3*u*t*t*c2[1] + t*t*t*p3[1]
		if hypot2(x-pts[k][0], y-pts[k][1]) > eps2 {
			return false, [2]float64{}, [2]float64{}
		}
	}
	return true, c1, c2
}

func fitCubicAlways(pts [][2]float64) ([2]float64, [2]float64) {
	n := len(pts)
	if n < 3 {
		return pts[0], pts[len(pts)-1]
	}
	p0, p3 := pts[0], pts[n-1]
	var aa, ab, bb, rax, rbx, ray, rby float64
	for k := 1; k < n-1; k++ {
		t := float64(k) / float64(n-1)
		u := 1 - t
		A := 3 * u * u * t
		B := 3 * u * t * t
		rx := pts[k][0] - u*u*u*p0[0] - t*t*t*p3[0]
		ry := pts[k][1] - u*u*u*p0[1] - t*t*t*p3[1]
		aa += A * A
		ab += A * B
		bb += B * B
		rax += A * rx
		rbx += B * rx
		ray += A * ry
		rby += B * ry
	}
	det := aa*bb - ab*ab
	if math.Abs(det) < 1e-12 {
		return p0, p3
	}
	c1 := [2]float64{roundHalf((bb*rax - ab*rbx) / det), roundHalf((bb*ray - ab*rby) / det)}
	c2 := [2]float64{roundHalf((aa*rbx - ab*rax) / det), roundHalf((aa*rby - ab*ray) / det)}
	return c1, c2
}

func turnDeg(a, b, c [2]float64) float64 {
	v1x, v1y := a[0]-b[0], a[1]-b[1]
	v2x, v2y := c[0]-b[0], c[1]-b[1]
	n1 := math.Hypot(v1x, v1y)
	n2 := math.Hypot(v2x, v2y)
	if n1 < 1e-9 || n2 < 1e-9 {
		return 0
	}
	cos := (v1x*v2x + v1y*v2y) / (n1 * n2)
	if cos < -1 {
		cos = -1
	} else if cos > 1 {
		cos = 1
	}
	// 180 = straight, 0 = U-turn. Corner = deviation from straight.
	return 180 - math.Acos(cos)*180/math.Pi
}

func collinear3(a, b, c [2]float64) bool {
	return math.Abs((b[0]-a[0])*(c[1]-b[1])-(b[1]-a[1])*(c[0]-b[0])) < 1e-6
}

func simpLines(lp [][2]float64) []svg.PathCmd {
	cmds := make([]svg.PathCmd, 0, len(lp)+1)
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdMove, X: lp[0][0], Y: lp[0][1]})
	for _, p := range lp[1:] {
		cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: p[0], Y: p[1]})
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	return cmds
}
