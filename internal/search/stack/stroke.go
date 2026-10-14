package stack

import (
	"context"
	"image/color"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Stroke paints one leftover with an open butt stroke when that stroke
// covers the island and no other pixel. A square stays a filled path.
// A stroke that spills is not proposed, so command cost cannot buy it.
type Stroke struct {
	world *world
	left  leftover
}

func (Stroke) ID() Op { return OpStroke }

func (st Stroke) Applies() bool {
	return st.world.big(st.left) && !st.left.paper && st.world.paths < maxPaths
}

func (st Stroke) Run(ctx context.Context) (formPick, error) {
	s := st.world
	g := st.left.fresh
	if len(g.work) == 0 {
		g.work = st.left.island
	}
	island := g.work
	if s == nil || len(island) < 2 || s.w <= 0 || s.h <= 0 {
		return nonePick(), nil
	}
	col := modeFill(s.want, island)
	if col == paper {
		return nonePick(), nil
	}
	best := nonePick()
	for _, path := range strokeCandidates(island, col) {
		if err := live(ctx); err != nil {
			return nonePick(), err
		}
		if !strokeCovers(s.w, s.h, path, island, col) {
			continue
		}
		g.fill = col
		g = s.seedGrow(g)
		pick, err := s.addLayer(ctx, path, g, OpStroke)
		if err != nil {
			return nonePick(), err
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

func strokeCandidates(island []pix, col color.NRGBA) []svg.Path {
	if path, ok := rectStroke(island, col); ok {
		return []svg.Path{path}
	}
	order, ok := chainOrder(island)
	if !ok {
		return nil
	}
	if dx, dy, straight := constantStep(order); straight {
		if path, ok := diagonalStroke(order, dx, dy, col); ok {
			return []svg.Path{path}
		}
	}
	if path, ok := curveStroke(order, col); ok {
		return []svg.Path{path}
	}
	return nil
}

// rectStroke is an axis-aligned filled box whose long side is longer
// than its short side. The centerline runs along the long side and the
// width is the short side, so a butt cap paints the box and nothing past it.
func rectStroke(island []pix, col color.NRGBA) (svg.Path, bool) {
	minX, minY, maxX, maxY, ok := filledBox(island)
	if !ok {
		return svg.Path{}, false
	}
	bw := maxX - minX + 1
	bh := maxY - minY + 1
	if bw == bh {
		return svg.Path{}, false
	}
	if bw > bh {
		y := float64(minY) + float64(bh)/2
		return buttLine(float64(minX), y, float64(maxX+1), y, float64(bh), col), true
	}
	x := float64(minX) + float64(bw)/2
	return buttLine(x, float64(minY), x, float64(maxY+1), float64(bw), col), true
}

func filledBox(island []pix) (minX, minY, maxX, maxY int, ok bool) {
	if len(island) == 0 {
		return 0, 0, 0, 0, false
	}
	minX, minY = island[0].x, island[0].y
	maxX, maxY = minX, minY
	seen := make(map[[2]int]struct{}, len(island))
	for _, p := range island {
		if p.x < minX {
			minX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y > maxY {
			maxY = p.y
		}
		seen[[2]int{p.x, p.y}] = struct{}{}
	}
	bw := maxX - minX + 1
	bh := maxY - minY + 1
	if bw <= 0 || bh <= 0 || len(seen) != bw*bh {
		return 0, 0, 0, 0, false
	}
	return minX, minY, maxX, maxY, true
}

// chainOrder is an 8-connected path: two ends, and every other pixel
// has two neighbors. A branch, a loop, or a blob is not a chain.
func chainOrder(island []pix) ([]pix, bool) {
	if len(island) < 2 {
		return nil, false
	}
	index := make(map[[2]int]int, len(island))
	for i, p := range island {
		key := [2]int{p.x, p.y}
		if _, dup := index[key]; dup {
			return nil, false
		}
		index[key] = i
	}
	dirs := [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	neigh := make([][]int, len(island))
	for i, p := range island {
		for _, d := range dirs {
			j, ok := index[[2]int{p.x + d[0], p.y + d[1]}]
			if !ok || j <= i {
				continue
			}
			neigh[i] = append(neigh[i], j)
			neigh[j] = append(neigh[j], i)
		}
	}
	ends := make([]int, 0, 2)
	for i := range island {
		switch len(neigh[i]) {
		case 1:
			ends = append(ends, i)
		case 2:
		default:
			return nil, false
		}
	}
	if len(ends) != 2 {
		return nil, false
	}
	order := make([]pix, 0, len(island))
	prev := -1
	cur := ends[0]
	for {
		order = append(order, island[cur])
		next := -1
		for _, j := range neigh[cur] {
			if j != prev {
				next = j
				break
			}
		}
		if next < 0 {
			break
		}
		prev, cur = cur, next
		if len(order) > len(island) {
			return nil, false
		}
	}
	if len(order) != len(island) {
		return nil, false
	}
	return order, true
}

func constantStep(order []pix) (dx, dy int, ok bool) {
	if len(order) < 2 {
		return 0, 0, false
	}
	dx = order[1].x - order[0].x
	dy = order[1].y - order[0].y
	if dx == 0 && dy == 0 {
		return 0, 0, false
	}
	for i := 2; i < len(order); i++ {
		if order[i].x-order[i-1].x != dx || order[i].y-order[i-1].y != dy {
			return 0, 0, false
		}
	}
	return dx, dy, true
}

// diagonalStroke runs from the back corner of the first pixel to the
// forward corner of the last, one pixel wide. An axis-aligned chain is
// a filled box and uses rectStroke instead.
func diagonalStroke(order []pix, dx, dy int, col color.NRGBA) (svg.Path, bool) {
	if dx == 0 || dy == 0 || absInt(dx) != 1 || absInt(dy) != 1 {
		return svg.Path{}, false
	}
	a := order[0]
	b := order[len(order)-1]
	x0, y0 := float64(a.x), float64(a.y)
	if dx < 0 {
		x0 = float64(a.x + 1)
	}
	if dy < 0 {
		y0 = float64(a.y + 1)
	}
	x1, y1 := float64(b.x), float64(b.y)
	if dx > 0 {
		x1 = float64(b.x + 1)
	}
	if dy > 0 {
		y1 = float64(b.y + 1)
	}
	return buttLine(x0, y0, x1, y1, 1, col), true
}

func curveStroke(order []pix, col color.NRGBA) (svg.Path, bool) {
	if len(order) < 3 {
		return svg.Path{}, false
	}
	samples := make([][2]float64, len(order))
	for i, p := range order {
		samples[i] = [2]float64{float64(p.x) + 0.5, float64(p.y) + 0.5}
	}
	c1, c2, line := fitCubic(samples)
	p0 := samples[0]
	p3 := samples[len(samples)-1]
	path := svg.NewPath().MoveTo(p0[0], p0[1])
	if line {
		path = path.LineTo(p3[0], p3[1])
	} else {
		path = path.CubicTo(c1[0], c1[1], c2[0], c2[1], p3[0], p3[1])
	}
	return path.WithFillNone().WithStroke(svg.NewStroke().WithColor(col).WithWidth(1).WithCap(svg.CapButt)), true
}

func buttLine(x0, y0, x1, y1, width float64, col color.NRGBA) svg.Path {
	return svg.NewPath().MoveTo(x0, y0).LineTo(x1, y1).WithFillNone().WithStroke(svg.NewStroke().WithColor(col).WithWidth(width).WithCap(svg.CapButt))
}

// strokeCovers reports that path on a white pane paints col on every
// island pixel and paper everywhere else.
func strokeCovers(w, h int, path svg.Path, island []pix, col color.NRGBA) bool {
	if w <= 0 || h <= 0 || len(island) == 0 {
		return false
	}
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	doc = doc.Append(whitePane(w, h).Node()).Append(path.Node())
	img, err := render.Scratch(doc)
	if err != nil {
		return false
	}
	defer render.Release(img)
	mark := make([]byte, w*h)
	for _, p := range island {
		if p.x < 0 || p.y < 0 || p.x >= w || p.y >= h {
			return false
		}
		mark[p.y*w+p.x] = 1
	}
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			got := img.NRGBAAt(x, y)
			if mark[row+x] == 1 {
				if got != col {
					return false
				}
				continue
			}
			if got != paper {
				return false
			}
		}
	}
	return true
}
