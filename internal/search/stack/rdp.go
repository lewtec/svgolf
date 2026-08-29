package stack

import (
	"math"
	"sort"
)

// smooth averages each vertex with its neighbors. Pixel stairs collapse
// toward the true edge so a later RDP can keep a short polygon.
func smooth(ring [][2]float64, passes int) [][2]float64 {
	if len(ring) < 3 || passes <= 0 {
		return ring
	}
	cur := append([][2]float64{}, ring...)
	for i := 0; i < passes; i++ {
		next := make([][2]float64, len(cur))
		n := len(cur)
		for j := 0; j < n; j++ {
			a, b, c := cur[(j-1+n)%n], cur[j], cur[(j+1)%n]
			next[j] = [2]float64{(a[0] + 2*b[0] + c[0]) / 4, (a[1] + 2*b[1] + c[1]) / 4}
		}
		cur = next
	}
	return cur
}

func fitPoly(ring [][2]float64, eps float64) [][2]float64 {
	if len(ring) < 3 {
		return ring
	}
	out := rdpClosed(smooth(ring, 2), eps)
	if len(out) < 3 {
		return ring
	}
	return out
}

// fanOrder rewrites a ring by angle around its centroid so edges cannot cross.
func fanOrder(ring [][2]float64) [][2]float64 {
	if len(ring) < 3 {
		return ring
	}
	var cx, cy float64
	for _, p := range ring {
		cx += p[0]
		cy += p[1]
	}
	n := float64(len(ring))
	cx /= n
	cy /= n
	type item struct {
		p    [2]float64
		ang  float64
		dist float64
	}
	pts := make([]item, 0, len(ring))
	seen := map[[2]float64]bool{}
	for _, p := range ring {
		if seen[p] {
			continue
		}
		seen[p] = true
		dx, dy := p[0]-cx, p[1]-cy
		pts = append(pts, item{p, math.Atan2(dy, dx), dx*dx + dy*dy})
	}
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].ang != pts[j].ang {
			return pts[i].ang < pts[j].ang
		}
		return pts[i].dist < pts[j].dist
	})
	if len(pts) < 3 {
		return ring
	}
	out := make([][2]float64, len(pts))
	for i := range pts {
		out[i] = pts[i].p
	}
	return out
}

func rdpClosed(pts [][2]float64, eps float64) [][2]float64 {
	n := len(pts)
	if n < 3 {
		return pts
	}
	ai, bi := 0, 1
	maxD := -1.0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			d := math.Hypot(pts[j][0]-pts[i][0], pts[j][1]-pts[i][1])
			if d > maxD {
				ai, bi, maxD = i, j, d
			}
		}
	}
	chain := func(from, to int) [][2]float64 {
		var s [][2]float64
		for i := from; ; i = (i + 1) % n {
			s = append(s, pts[i])
			if i == to {
				break
			}
		}
		return rdp(s, eps)
	}
	left := chain(ai, bi)
	right := chain(bi, ai)
	if len(right) > 1 {
		right = right[1:]
	}
	if len(right) > 0 {
		right = right[:len(right)-1]
	}
	out := append(left, right...)
	if len(out) < 3 {
		return pts
	}
	return out
}

func rdp(pts [][2]float64, eps float64) [][2]float64 {
	if len(pts) < 3 {
		return pts
	}
	var maxD float64
	idx := 0
	a, b := pts[0], pts[len(pts)-1]
	for i := 1; i < len(pts)-1; i++ {
		d := distLine(pts[i], a, b)
		if d > maxD {
			maxD = d
			idx = i
		}
	}
	if maxD <= eps {
		return [][2]float64{a, b}
	}
	left := rdp(pts[:idx+1], eps)
	right := rdp(pts[idx:], eps)
	return append(left[:len(left)-1], right...)
}

func distLine(p, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	if dx == 0 && dy == 0 {
		return math.Hypot(p[0]-a[0], p[1]-a[1])
	}
	return math.Abs(dy*p[0]-dx*p[1]+b[0]*a[1]-b[1]*a[0]) / math.Hypot(dx, dy)
}
