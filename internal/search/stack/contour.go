package stack

var n8 = []pix{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}

func contour(island []pix) [][2]float64 {
	if len(island) == 0 {
		return nil
	}
	in := make(map[pix]bool, len(island))
	start := island[0]
	for _, p := range island {
		in[p] = true
		if p.y < start.y || (p.y == start.y && p.x < start.x) {
			start = p
		}
	}
	cur := start
	come := 4 // arrived from the west
	ring := make([]pix, 0, len(island))
	for {
		ring = append(ring, cur)
		found := -1
		var nxt pix
		for k := 1; k <= 8; k++ {
			d := (come + k) % 8
			q := pix{cur.x + n8[d].x, cur.y + n8[d].y}
			if in[q] {
				found = d
				nxt = q
				break
			}
		}
		if found < 0 {
			break
		}
		come = (found + 4) % 8
		cur = nxt
		if cur == start {
			break
		}
		if len(ring) > len(island)*2 {
			break
		}
	}
	out := make([][2]float64, len(ring))
	for i, p := range ring {
		out[i] = [2]float64{float64(p.x) + 0.5, float64(p.y) + 0.5}
	}
	return out
}
