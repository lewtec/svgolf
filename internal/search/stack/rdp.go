package stack

import "math"

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
