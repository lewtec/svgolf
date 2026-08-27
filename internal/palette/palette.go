package palette

import (
	"image"
	"image/color"
	"sort"
)

type ColorMap interface {
	Map(c color.NRGBA) color.NRGBA
}

type snapMap struct {
	pal []color.NRGBA
}

func (m snapMap) Map(c color.NRGBA) color.NRGBA {
	if c.A == 0 {
		return color.NRGBA{}
	}
	best := m.pal[0]
	bestD := dist2(c, best)
	for _, p := range m.pal[1:] {
		if d := dist2(c, p); d < bestD || (d == bestD && lessRGB(p, best)) {
			best, bestD = p, d
		}
	}
	best.A = c.A
	return best
}

func dist2(a, b color.NRGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	return dr*dr + dg*dg + db*db
}

func lessRGB(a, b color.NRGBA) bool {
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

type histEntry struct {
	c color.NRGBA
	n int
}

func Auto(img image.Image, n int) (ColorMap, []color.NRGBA, error) {
	if n <= 0 {
		n = 8
	}
	b := img.Bounds()
	hist := map[color.NRGBA]int{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			c := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
			if c.A == 0 {
				continue
			}
			hist[c]++
		}
	}
	entries := make([]histEntry, 0, len(hist))
	for c, cnt := range hist {
		entries = append(entries, histEntry{c, cnt})
	}
	var pal []color.NRGBA
	if len(entries) <= n {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].n != entries[j].n {
				return entries[i].n > entries[j].n
			}
			return lessRGB(entries[i].c, entries[j].c)
		})
		for _, e := range entries {
			pal = append(pal, e.c)
		}
	} else {
		pal = medianCut(entries, n)
	}
	return snapMap{pal: pal}, pal, nil
}

func medianCut(entries []histEntry, n int) []color.NRGBA {
	type box struct {
		items []histEntry
	}
	boxes := []box{{items: append([]histEntry(nil), entries...)}}
	for len(boxes) < n {
		bi := 0
		bestRange := -1
		axis := 0
		for i, bx := range boxes {
			if len(bx.items) < 2 {
				continue
			}
			minR, maxR := 255, 0
			minG, maxG := 255, 0
			minB, maxB := 255, 0
			for _, e := range bx.items {
				if int(e.c.R) < minR {
					minR = int(e.c.R)
				}
				if int(e.c.R) > maxR {
					maxR = int(e.c.R)
				}
				if int(e.c.G) < minG {
					minG = int(e.c.G)
				}
				if int(e.c.G) > maxG {
					maxG = int(e.c.G)
				}
				if int(e.c.B) < minB {
					minB = int(e.c.B)
				}
				if int(e.c.B) > maxB {
					maxB = int(e.c.B)
				}
			}
			ranges := []int{maxR - minR, maxG - minG, maxB - minB}
			ax := 0
			for k := 1; k < 3; k++ {
				if ranges[k] > ranges[ax] {
					ax = k
				}
			}
			if ranges[ax] > bestRange {
				bestRange = ranges[ax]
				bi = i
				axis = ax
			}
		}
		if bestRange <= 0 {
			break
		}
		items := boxes[bi].items
		sort.Slice(items, func(i, j int) bool {
			switch axis {
			case 0:
				return items[i].c.R < items[j].c.R
			case 1:
				return items[i].c.G < items[j].c.G
			default:
				return items[i].c.B < items[j].c.B
			}
		})
		total := 0
		for _, e := range items {
			total += e.n
		}
		acc := 0
		split := 1
		for i, e := range items {
			acc += e.n
			if acc >= total/2 {
				split = i + 1
				if split >= len(items) {
					split = len(items) - 1
				}
				if split < 1 {
					split = 1
				}
				break
			}
		}
		boxes[bi] = box{items: items[:split]}
		boxes = append(boxes, box{items: items[split:]})
	}
	type swatch struct {
		c color.NRGBA
		n int
	}
	out := make([]swatch, 0, len(boxes))
	for _, bx := range boxes {
		if len(bx.items) == 0 {
			continue
		}
		var sr, sg, sb, tot int
		var a uint8
		for _, e := range bx.items {
			sr += int(e.c.R) * e.n
			sg += int(e.c.G) * e.n
			sb += int(e.c.B) * e.n
			tot += e.n
			a = e.c.A
		}
		out = append(out, swatch{
			c: color.NRGBA{
				R: uint8(float64(sr)/float64(tot) + 0.5),
				G: uint8(float64(sg)/float64(tot) + 0.5),
				B: uint8(float64(sb)/float64(tot) + 0.5),
				A: a,
			},
			n: tot,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return lessRGB(out[i].c, out[j].c)
	})
	result := make([]color.NRGBA, len(out))
	for i, p := range out {
		result[i] = p.c
	}
	return result
}
