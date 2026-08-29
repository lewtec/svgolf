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
	if s < 0.08 {
		if v < 0.5 {
			return 0
		}
		return 1
	}
	return 2 + int(h/30)%12
}

func residual(got, want *image.NRGBA, skip []byte, x, y, w int) bool {
	if skip[y*w+x] != 0 {
		return false
	}
	q := want.NRGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
	g := got.NRGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
	if q.A == 0 {
		return g.A != 0
	}
	return errAt(g, q) > 1
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
		return color.NRGBA{A: 255}
	}
	return color.NRGBA{R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n), A: uint8(sa / n)}
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

func transparentIsland(want *image.NRGBA, island []pix) bool {
	if len(island) == 0 {
		return false
	}
	var sa int
	for _, p := range island {
		sa += int(want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y).A)
	}
	return sa/len(island) < 128
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
			if len(cur) >= 4 {
				holes = append(holes, append([]pix{}, cur...))
			}
		}
	}
	return holes
}

func corners(island []pix) [][2]float64 {
	out := make([][2]float64, 0, len(island)*4)
	for _, p := range island {
		x, y := float64(p.x), float64(p.y)
		out = append(out,
			[2]float64{x, y},
			[2]float64{x + 1, y},
			[2]float64{x + 1, y + 1},
			[2]float64{x, y + 1},
		)
	}
	return out
}
