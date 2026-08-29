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

func layerCovers(island []pix, got *image.NRGBA, fill color.NRGBA, owner []uint16, w, idx int) bool {
	id := uint16(idx + 1)
	for _, p := range island {
		if owner[p.y*w+p.x] == id {
			return true
		}
		g := got.NRGBAAt(got.Rect.Min.X+p.x, got.Rect.Min.Y+p.y)
		if loss.ColorAt(g, fill) < recolorAt {
			return true
		}
	}
	return false
}

func sameObject(fill, col color.NRGBA) bool {
	return coarse(fill) == coarse(col) && loss.ColorAt(fill, col) < recolorAt
}

func paperLeftover(col color.NRGBA) bool {
	return loss.ColorAt(col, paper) <= minErr
}

// topPainter is the highest path whose fill matches what is showing
// on the leftover. Owner-at-place-time tags the plate under an inner
// mark, so a hole in the mark would shrink the plate.
func topPainter(island []pix, got *image.NRGBA, fills []color.NRGBA) (int, bool) {
	if len(fills) == 0 || got == nil {
		return 0, false
	}
	hist := make([]int, len(fills))
	for _, p := range island {
		g := got.NRGBAAt(got.Rect.Min.X+p.x, got.Rect.Min.Y+p.y)
		for i := len(fills) - 1; i >= 0; i-- {
			if loss.ColorAt(g, fills[i]) < recolorAt {
				hist[i]++
				break
			}
		}
	}
	best, n := 0, 0
	for i, c := range hist {
		if c > n {
			best, n = i, c
		}
	}
	if n*2 <= len(island) {
		return 0, false
	}
	return best, true
}

func majorityOwner(owner []uint16, island []pix, w int) (int, bool) {
	hist := map[uint16]int{}
	for _, p := range island {
		id := owner[p.y*w+p.x]
		if id != 0 {
			hist[id]++
		}
	}
	var best uint16
	n := 0
	for id, c := range hist {
		if c > n {
			best, n = id, c
		}
	}
	if best == 0 || n*2 <= len(island) {
		return 0, false
	}
	return int(best - 1), true
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

func ownedUnion(owner []uint16, island []pix, w, h int, id uint16) []pix {
	seed := make(map[pix]bool, len(island))
	seen := make([]byte, w*h)
	var st, out []pix
	for _, p := range island {
		seed[p] = true
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
			if owner[i] != id && !seed[q] {
				continue
			}
			seen[i] = 1
			st = append(st, q)
		}
	}
	return out
}

func ownedMinus(owner []uint16, drop []pix, w int, id uint16) []pix {
	gone := make(map[pix]bool, len(drop))
	for _, p := range drop {
		gone[p] = true
	}
	var out []pix
	for i, v := range owner {
		if v != id {
			continue
		}
		p := pix{i % w, i / w}
		if gone[p] {
			continue
		}
		out = append(out, p)
	}
	return out
}

func residual(got, want *image.NRGBA, skip []byte, x, y, w int) bool {
	if skip[y*w+x] != 0 {
		return false
	}
	q := want.NRGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
	g := got.NRGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
	return colorErr(g, q) > minErr
}

func largestIsland(got, want *image.NRGBA, skip []byte) (color.NRGBA, []pix) {
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	hist := map[int]int{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !residual(got, want, skip, x, y, w) {
				continue
			}
			q := want.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			hist[coarse(q)]++
		}
	}
	top, topN := -1, 0
	for k, n := range hist {
		if n > topN {
			top, topN = k, n
		}
	}
	if topN == 0 {
		return color.NRGBA{}, nil
	}
	mark := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !residual(got, want, skip, x, y, w) {
				continue
			}
			q := want.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if coarse(q) != top {
				continue
			}
			mark[y*w+x] = 1
		}
	}
	despeckle(mark, w, h)
	best := []pix{}
	var cur []pix
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 1 {
				continue
			}
			cur = cur[:0]
			stack := []pix{{x, y}}
			mark[y*w+x] = 2
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cur = append(cur, p)
				for _, d := range dirs {
					nx, ny := p.x+d.x, p.y+d.y
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if mark[ny*w+nx] != 1 {
						continue
					}
					mark[ny*w+nx] = 2
					stack = append(stack, pix{nx, ny})
				}
			}
			if len(cur) > len(best) {
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
