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

// outline is the residual border at pixel corners, in walk order.
// Centers sit inside the leftover and leave a half-pixel rim.
func outline(island []pix) [][2]float64 {
	if len(island) == 0 {
		return nil
	}
	set := pixSet(island)
	type vert struct{ x, y int }
	next := make(map[vert]vert, len(island)*2)
	for _, p := range island {
		if !set[pix{p.x, p.y - 1}] {
			next[vert{p.x, p.y}] = vert{p.x + 1, p.y}
		}
		if !set[pix{p.x + 1, p.y}] {
			next[vert{p.x + 1, p.y}] = vert{p.x + 1, p.y + 1}
		}
		if !set[pix{p.x, p.y + 1}] {
			next[vert{p.x + 1, p.y + 1}] = vert{p.x, p.y + 1}
		}
		if !set[pix{p.x - 1, p.y}] {
			next[vert{p.x, p.y + 1}] = vert{p.x, p.y}
		}
	}
	if len(next) < 3 {
		return nil
	}
	start := vert{island[0].x, island[0].y}
	for v := range next {
		if v.y < start.y || (v.y == start.y && v.x < start.x) {
			start = v
		}
	}
	cur := start
	ring := make([][2]float64, 0, len(next))
	for {
		ring = append(ring, [2]float64{float64(cur.x), float64(cur.y)})
		nxt, ok := next[cur]
		if !ok {
			break
		}
		delete(next, cur)
		cur = nxt
		if cur == start {
			break
		}
		if len(ring) > len(island)*4 {
			break
		}
	}
	return ring
}
