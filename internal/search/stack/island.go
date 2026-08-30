package stack

import (
	"image"
	"image/color"

	"github.com/lewtec/svgolf/internal/loss"
)

type pix struct{ x, y int }

func coarse(c color.NRGBA) int {
	if c.A == 0 {
		return -1
	}
	h, s, v := loss.HSV(c)
	vb := int(v * 4)
	if vb > 3 {
		vb = 3
	}
	if s < 0.08 {
		return vb
	}
	return 4 + vb*12 + int(h/30)%12
}

func paperLeftover(col color.NRGBA) bool {
	return loss.ColorAt(col, paper) <= minErr
}

func ownsAny(owner []uint16, island []pix, w int, id uint16) bool {
	for _, p := range island {
		if owner[p.y*w+p.x] == id {
			return true
		}
	}
	return false
}

func claim(owner []uint16, island []pix, w int, id uint16) {
	for _, p := range island {
		i := p.y*w + p.x
		if owner[i] == 0 || owner[i] <= id {
			owner[i] = id
		}
	}
}

func clearOwner(owner []uint16, id uint16) {
	for i, v := range owner {
		if v == id {
			owner[i] = 0
		}
	}
}

type scratch struct {
	mark, seen []byte
	family     []int
	buckets    [][]pix
	work       []pix
}

func (s *scratch) ensure(n int) {
	if cap(s.mark) < n {
		s.mark = make([]byte, n)
		s.seen = make([]byte, n)
		s.family = make([]int, n)
		return
	}
	s.mark = s.mark[:n]
	s.seen = s.seen[:n]
	s.family = s.family[:n]
	clear(s.mark)
}

func ownedUnion(owner []uint16, island []pix, w, h int, id uint16, seen []byte) []pix {
	if len(seen) < w*h {
		seen = make([]byte, w*h)
	}
	var st, out []pix
	for _, p := range island {
		i := p.y*w + p.x
		if seen[i] != 0 {
			continue
		}
		seen[i] = 1
		st = append(st, p)
	}
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(st) > 0 {
		p := st[len(st)-1]
		st = st[:len(st)-1]
		out = append(out, p)
		for _, d := range dirs {
			q := pix{p.x + d.x, p.y + d.y}
			if q.x < 0 || q.y < 0 || q.x >= w || q.y >= h {
				continue
			}
			i := q.y*w + q.x
			if seen[i] != 0 {
				continue
			}
			if owner[i] != id {
				continue
			}
			seen[i] = 1
			st = append(st, q)
		}
	}
	for _, p := range out {
		seen[p.y*w+p.x] = 0
	}
	return out
}

func ownedMinus(owner []uint16, drop []pix, w int, id uint16, seen []byte) []pix {
	if len(seen) < len(owner) {
		seen = make([]byte, len(owner))
	}
	for _, p := range drop {
		seen[p.y*w+p.x] = 2
	}
	var out []pix
	for i, v := range owner {
		if v != id || seen[i] == 2 {
			continue
		}
		out = append(out, pix{i % w, i / w})
	}
	for _, p := range drop {
		seen[p.y*w+p.x] = 0
	}
	return out
}

func residual(got, want *image.NRGBA, skip []byte, x, y, w int) bool {
	if skip != nil && skip[y*w+x] != 0 {
		return false
	}
	q := want.NRGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
	g := got.NRGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
	return colorErr(g, q) > minErr
}

func residualHSV(got, want *loss.Plane, skip []byte, x, y, w int) bool {
	if skip != nil && skip[y*w+x] != 0 {
		return false
	}
	return colorErrHSV(got.At(x, y), want.At(x, y)) > minErr
}

// hottestIsland is the leftover blob Score would miss the most:
// same-coarse 4-connected leftover, ranked by ΣerrAt. Pixel count
// preferred a huge mild rim over a small full miss. A later spike
// or gap detector would feed the same ranking, not a second loop.
func hottestIsland(got, want *image.NRGBA, skip []byte, sc *scratch, gotP, wantP *loss.Plane) (color.NRGBA, []pix) {
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	if sc == nil {
		sc = &scratch{}
	}
	sc.ensure(w * h)
	mark, family := sc.mark, sc.family
	if gotP == nil {
		gotP = loss.NewPlane(got)
	}
	if wantP == nil {
		wantP = loss.NewPlane(want)
	}
	gotP.Ensure()
	wantP.Ensure()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !residualHSV(gotP, wantP, skip, x, y, w) {
				continue
			}
			mark[y*w+x] = 1
			family[y*w+x] = coarse(want.NRGBAAt(b.Min.X+x, b.Min.Y+y))
		}
	}
	despeckle(mark, w, h)
	best := []pix{}
	var bestErr float64
	var cur []pix
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 1 {
				continue
			}
			cur = cur[:0]
			bin := family[y*w+x]
			var errSum float64
			pending := []pix{{x, y}}
			mark[y*w+x] = 2
			for len(pending) > 0 {
				p := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				cur = append(cur, p)
				errSum += errAtHSV(gotP.At(p.x, p.y), wantP.At(p.x, p.y))
				for _, d := range dirs {
					nx, ny := p.x+d.x, p.y+d.y
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if mark[ny*w+nx] != 1 || family[ny*w+nx] != bin {
						continue
					}
					mark[ny*w+nx] = 2
					pending = append(pending, pix{nx, ny})
				}
			}
			if len(cur) < minIsland {
				continue
			}
			if errSum > bestErr {
				bestErr = errSum
				best = append(best[:0], cur...)
			}
		}
	}
	return meanFill(want, best), best
}

func despeckle(mark []byte, w, h int) {
	drop := make([]int, 0, 32)
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 1 {
				continue
			}
			n := 0
			for _, d := range dirs {
				nx, ny := x+d.x, y+d.y
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				if mark[ny*w+nx] == 1 {
					n++
				}
			}
			if n < 2 {
				drop = append(drop, y*w+x)
			}
		}
	}
	for _, i := range drop {
		mark[i] = 0
	}
}

func meanFill(want *image.NRGBA, island []pix) color.NRGBA {
	if len(island) == 0 {
		return color.NRGBA{}
	}
	var sr, sg, sb, sa int
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		sr += int(c.R)
		sg += int(c.G)
		sb += int(c.B)
		sa += int(c.A)
	}
	n := len(island)
	if sa/n < 128 {
		return paper
	}
	return color.NRGBA{R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n), A: 255}
}

func bbox(island []pix) [][2]float64 {
	minX, minY := island[0].x, island[0].y
	maxX, maxY := minX+1, minY+1
	for _, p := range island {
		if p.x < minX {
			minX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.x+1 > maxX {
			maxX = p.x + 1
		}
		if p.y+1 > maxY {
			maxY = p.y + 1
		}
	}
	return [][2]float64{
		{float64(minX), float64(minY)},
		{float64(maxX), float64(minY)},
		{float64(maxX), float64(maxY)},
		{float64(minX), float64(maxY)},
	}
}

// voids are enclosed non-island pockets (4-connected), not the exterior.
func voids(island []pix) [][]pix {
	if len(island) == 0 {
		return nil
	}
	minX, minY := island[0].x, island[0].y
	maxX, maxY := minX, minY
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
	}
	minX--
	minY--
	maxX += 2
	maxY += 2
	w, h := maxX-minX, maxY-minY
	mark := make([]byte, w*h)
	for _, p := range island {
		mark[(p.y-minY)*w+(p.x-minX)] = 1
	}
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	stack := []pix{{0, 0}}
	mark[0] = 2
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, d := range dirs {
			nx, ny := p.x+d.x, p.y+d.y
			if nx < 0 || ny < 0 || nx >= w || ny >= h || mark[ny*w+nx] != 0 {
				continue
			}
			mark[ny*w+nx] = 2
			stack = append(stack, pix{nx, ny})
		}
	}
	var holes [][]pix
	var cur []pix
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 0 {
				continue
			}
			cur = cur[:0]
			stack = []pix{{x, y}}
			mark[y*w+x] = 3
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cur = append(cur, pix{p.x + minX, p.y + minY})
				for _, d := range dirs {
					nx, ny := p.x+d.x, p.y+d.y
					if nx < 0 || ny < 0 || nx >= w || ny >= h || mark[ny*w+nx] != 0 {
						continue
					}
					mark[ny*w+nx] = 3
					stack = append(stack, pix{nx, ny})
				}
			}
			if len(cur) >= minIsland && !thinIsland(cur) {
				holes = append(holes, append([]pix{}, cur...))
			}
		}
	}
	return holes
}
