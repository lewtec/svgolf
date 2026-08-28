package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Simplify traces each color island as a path, then drops points while
// the island stays covered. Cubics replace long runs when they stay close.
type Simplify struct {
	Colors int // 0 = auto, cap 8
}

var _ Search = Simplify{}

const (
	simpCover    = 0.97
	simpMaxKids  = 24
	simpCycleλ   = 0.04 // extra closed path must cut RMSE by ~10
	simpMergeD2  = 48 * 48
	simpMinColor = 0.02 // extra palette slots need ≥2% of scored pixels
)

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
	want := FitCanvas(FromImage(target), MaxCanvas)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return doc, nil
	}
	_, pal, err := palette.Auto(want, s.Colors)
	if err != nil {
		return doc, err
	}
	pal = simpMergePal(pal)
	if len(pal) == 0 {
		return doc, nil
	}
	blobs := simpBlobs(want, pal, simpSpeckle(w, h))
	groups := simpColorGroups(blobs)
	got := image.NewNRGBA(image.Rect(0, 0, w, h))
	nScore := simpScoredN(want)
	sse := simpEmptySSE(want)
	cur := simpFit(sse, nScore, 0)
	var kids []svg.Node
	for _, g := range groups {
		if err := ctx.Err(); err != nil {
			return doc.Append(kids...), err
		}
		if len(kids) >= simpMaxKids {
			break
		}
		if len(kids) > 0 && nScore > 0 && float64(g.area) < simpMinColor*float64(nScore) {
			continue
		}
		var nodes []svg.Node
		var used []simpBlob
		for _, b := range g.blobs {
			if len(kids)+len(nodes) >= simpMaxKids {
				break
			}
			n, ok := simpPath(b, w, h)
			if !ok {
				continue
			}
			nodes = append(nodes, n)
			used = append(used, b)
		}
		if len(nodes) == 0 {
			continue
		}
		// Dominant color always stays. Extra swatches must pay in Fit.
		if len(kids) > 0 {
			delta := 0.0
			for _, b := range used {
				delta += simpPaint(got, want, b, false)
			}
			next := simpFit(sse+delta, nScore, len(kids)+len(nodes))
			if next >= cur {
				continue
			}
		}
		for _, b := range used {
			sse += simpPaint(got, want, b, true)
		}
		kids = append(kids, nodes...)
		cur = simpFit(sse, nScore, len(kids))
	}
	return doc.Append(kids...), nil
}

func simpSpeckle(w, h int) int {
	return max(4, w*h/20000)
}

type simpBlob struct {
	col  color.NRGBA
	pix  []image.Point
	area int
}

func simpMergePal(pal []color.NRGBA) []color.NRGBA {
	var kept []color.NRGBA
	for _, c := range pal {
		c.A = 255
		dup := false
		for _, k := range kept {
			if simpDist2(c, k) <= simpMergeD2 {
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, c)
		}
	}
	return kept
}

func simpSnap(pal []color.NRGBA, c color.NRGBA) color.NRGBA {
	if len(pal) == 0 || c.A == 0 {
		return color.NRGBA{}
	}
	best, bestD := pal[0], simpDist2(c, pal[0])
	for _, p := range pal[1:] {
		if d := simpDist2(c, p); d < bestD {
			best, bestD = p, d
		}
	}
	best.A = 255
	return best
}

func simpDist2(a, b color.NRGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	return dr*dr + dg*dg + db*db
}

func simpBlobs(want *image.NRGBA, pal []color.NRGBA, speckle int) []simpBlob {
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
			snap[y*w+x] = simpSnap(pal, c)
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
			if len(pix) < speckle {
				continue
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
		return simpCycleλ * float64(max(k, 0))
	}
	rmse := math.Sqrt(sse / (3 * float64(n)))
	if math.IsInf(rmse, 0) || math.IsNaN(rmse) {
		return rmse
	}
	return rmse/255 + simpCycleλ*float64(max(k, 0))
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
	core := simpErode(b.pix, w, h)
	if er := simpErode(core, w, h); len(er) > 0 {
		core = er
	}
	if len(core) == 0 {
		core = b.pix
	}
	core = simpSample(core, 1500)
	best := simpConverge(loops, core)
	if len(best) == 0 {
		return svg.Node{}, false
	}
	cmds := simpEmit(best)
	if len(cmds) == 0 {
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

func simpConverge(loops [][][2]float64, pix []image.Point) [][][2]float64 {
	// 1px stairs sit 0.707 from the diagonal; eps>=1 kills them.
	// Stay under ~3 so open bays (lewtec traces) are not filled in.
	eps := []float64{1, 1.5, 2, 2.5, 3}
	best := make([][][2]float64, 0, len(loops))
	for _, lp := range loops {
		got := simpRDPClosed(lp, 1)
		if len(got) < 3 {
			got = lp
		}
		best = append(best, got)
	}
	bestN := simpCount(best)
	for _, e := range eps[1:] {
		trial := make([][][2]float64, 0, len(loops))
		ok := true
		for _, lp := range loops {
			got := simpRDPClosed(lp, e)
			if len(got) < 3 {
				ok = false
				break
			}
			trial = append(trial, got)
		}
		if !ok {
			continue
		}
		trial = simpDrop(trial, pix)
		if !simpCovers(trial, pix) {
			continue
		}
		if n := simpCount(trial); n <= bestN {
			best, bestN = trial, n
		}
	}
	return best
}

func simpCount(loops [][][2]float64) int {
	n := 0
	for _, lp := range loops {
		n += len(lp)
	}
	return n
}

func simpDrop(loops [][][2]float64, pix []image.Point) [][][2]float64 {
	cur := copyLoops(loops)
	changed := true
	for changed {
		changed = false
		for li := range cur {
			if len(cur[li]) <= 3 || len(cur[li]) > 48 {
				continue
			}
			for i := 0; i < len(cur[li]); i++ {
				trial := copyLoops(cur)
				trial[li] = append(append([][2]float64(nil), cur[li][:i]...), cur[li][i+1:]...)
				if len(trial[li]) < 3 || !simpCovers(trial, pix) {
					continue
				}
				cur = trial
				changed = true
				break
			}
			if changed {
				break
			}
		}
	}
	return cur
}

func copyLoops(in [][][2]float64) [][][2]float64 {
	out := make([][][2]float64, len(in))
	for i, lp := range in {
		out[i] = append([][2]float64(nil), lp...)
	}
	return out
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
	return float64(hit) >= simpCover*float64(len(pix))
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

func simpSmooth(lp [][2]float64) []svg.PathCmd {
	n := len(lp)
	if n < 5 {
		return simpLines(lp)
	}
	corner := make([]bool, n)
	nCorner := 0
	for i := 0; i < n; i++ {
		if turnDeg(lp[(i-1+n)%n], lp[i], lp[(i+1)%n]) >= 40 {
			corner[i] = true
			nCorner++
		}
	}
	if nCorner == n {
		return simpLines(lp)
	}
	cmds := []svg.PathCmd{{Kind: svg.CmdMove, X: lp[0][0], Y: lp[0][1]}}
	for i := 0; i < n; i++ {
		a := lp[i]
		b := lp[(i+1)%n]
		if corner[i] || corner[(i+1)%n] || collinear3(lp[(i-1+n)%n], a, b) {
			if i+1 == n {
				break
			}
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: b[0], Y: b[1]})
			continue
		}
		prev := lp[(i-1+n)%n]
		next := lp[(i+2)%n]
		c1x := roundHalf(a[0] + (b[0]-prev[0])/6)
		c1y := roundHalf(a[1] + (b[1]-prev[1])/6)
		c2x := roundHalf(b[0] - (next[0]-a[0])/6)
		c2y := roundHalf(b[1] - (next[1]-a[1])/6)
		cmds = append(cmds, svg.PathCmd{
			Kind: svg.CmdCubic,
			X1:   c1x, Y1: c1y,
			X2: c2x, Y2: c2y,
			X: b[0], Y: b[1],
		})
		if i+1 == n {
			break
		}
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	return cmds
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
