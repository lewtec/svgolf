package stack

import (
	"image"
	"sort"
)

func islandPoints(island []pix) [][2]float64 {
	pts := make([][2]float64, len(island))
	for i, p := range island {
		pts[i] = [2]float64{float64(p.x) + 0.5, float64(p.y) + 0.5}
	}
	return pts
}

func islandCorners(island []pix) [][2]float64 {
	set := pixSet(island)
	pts := make([][2]float64, 0, 16)
	for _, p := range island {
		fx, fy := float64(p.x), float64(p.y)
		if !set[pix{p.x - 1, p.y}] || !set[pix{p.x, p.y - 1}] {
			pts = append(pts, [2]float64{fx, fy})
		}
		if !set[pix{p.x + 1, p.y}] || !set[pix{p.x, p.y - 1}] {
			pts = append(pts, [2]float64{fx + 1, fy})
		}
		if !set[pix{p.x + 1, p.y}] || !set[pix{p.x, p.y + 1}] {
			pts = append(pts, [2]float64{fx + 1, fy + 1})
		}
		if !set[pix{p.x - 1, p.y}] || !set[pix{p.x, p.y + 1}] {
			pts = append(pts, [2]float64{fx, fy + 1})
		}
	}
	return pts
}

func convexHull(pts [][2]float64) [][2]float64 {
	if len(pts) < 3 {
		return pts
	}
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	uniq := pts[:1]
	for _, p := range pts[1:] {
		last := uniq[len(uniq)-1]
		if p[0] == last[0] && p[1] == last[1] {
			continue
		}
		uniq = append(uniq, p)
	}
	pts = uniq
	if len(pts) < 3 {
		return pts
	}
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	lower := make([][2]float64, 0, len(pts))
	for _, p := range pts {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	upper := make([][2]float64, 0, len(pts))
	for i := len(pts) - 1; i >= 0; i-- {
		p := pts[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

func hullRing(work []pix) [][2]float64 {
	if len(work) == 0 {
		return nil
	}
	hull := convexHull(islandCorners(work))
	if len(hull) < 3 {
		return bbox(work)
	}
	return uncross(hull)
}

// coverRing is the leftover outline with only exact colinear
// vertices removed. Simplify drops more vertices later.
func coverRing(work []pix) [][2]float64 {
	if len(work) == 0 {
		return nil
	}
	ring := outline(work)
	if len(ring) < 3 {
		return bbox(work)
	}
	return uncross(collapseColinear(ring))
}

func collapseColinear(ring [][2]float64) [][2]float64 {
	if len(ring) < 4 {
		return ring
	}
	out := append([][2]float64{}, ring...)
	for len(out) > 3 {
		n := len(out)
		drop := -1
		for i := 0; i < n; i++ {
			a, b, c := out[(i-1+n)%n], out[i], out[(i+1)%n]
			if (b[0]-a[0])*(c[1]-a[1]) == (b[1]-a[1])*(c[0]-a[0]) {
				drop = i
				break
			}
		}
		if drop < 0 {
			break
		}
		out = append(out[:drop], out[drop+1:]...)
	}
	return out
}

func pointInRing(ring [][2]float64, x, y float64) bool {
	in := false
	n := len(ring)
	for i := 0; i < n; i++ {
		a, b := ring[i], ring[(i+1)%n]
		if (a[1] > y) != (b[1] > y) {
			t := (y - a[1]) / (b[1] - a[1])
			if x < a[0]+t*(b[0]-a[0]) {
				in = !in
			}
		}
	}
	return in
}

func pointOnSeg(p, a, b [2]float64) bool {
	if (b[0]-a[0])*(p[1]-a[1]) != (b[1]-a[1])*(p[0]-a[0]) {
		return false
	}
	minX, maxX := a[0], b[0]
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := a[1], b[1]
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	return p[0] >= minX && p[0] <= maxX && p[1] >= minY && p[1] <= maxY
}

// ringsOverlap is true when two simple rings share area or an edge.
func ringsOverlap(a, b [][2]float64) bool {
	if len(a) < 3 || len(b) < 3 {
		return false
	}
	hit := func(p [2]float64, ring [][2]float64) bool {
		if pointInRing(ring, p[0], p[1]) {
			return true
		}
		n := len(ring)
		for i := 0; i < n; i++ {
			if pointOnSeg(p, ring[i], ring[(i+1)%n]) {
				return true
			}
		}
		return false
	}
	for _, p := range a {
		if hit(p, b) {
			return true
		}
	}
	for _, p := range b {
		if hit(p, a) {
			return true
		}
	}
	na, nb := len(a), len(b)
	for i := 0; i < na; i++ {
		for j := 0; j < nb; j++ {
			if edgesCross(a[i], a[(i+1)%na], b[j], b[(j+1)%nb]) {
				return true
			}
		}
	}
	return false
}

func edgesCross(a, b, c, d [2]float64) bool {
	cross := func(p, q, r [2]float64) float64 {
		return (q[0]-p[0])*(r[1]-p[1]) - (q[1]-p[1])*(r[0]-p[0])
	}
	d1, d2 := cross(a, b, c), cross(a, b, d)
	d3, d4 := cross(c, d, a), cross(c, d, b)
	return d1*d2 < 0 && d3*d4 < 0
}

func ringCrosses(ring [][2]float64) bool {
	n := len(ring)
	if n < 4 {
		return false
	}
	for i := 0; i < n; i++ {
		a, b := ring[i], ring[(i+1)%n]
		for j := i + 1; j < n; j++ {
			if (i+1)%n == j || (j+1)%n == i {
				continue
			}
			if edgesCross(a, b, ring[j], ring[(j+1)%n]) {
				return true
			}
		}
	}
	return false
}

// uncross keeps a simple ring. Fan-order around the centroid
// repairs a bowtie; if that still crosses, the ring is dropped.
func uncross(ring [][2]float64) [][2]float64 {
	if len(ring) < 3 || !ringCrosses(ring) {
		return ring
	}
	out := fanOrder(ring)
	if len(out) < 3 || ringCrosses(out) {
		return nil
	}
	return out
}

func leftoverCenter(island []pix) [2]float64 {
	if len(island) == 0 {
		return [2]float64{}
	}
	var sx, sy float64
	for _, p := range island {
		sx += float64(p.x) + 0.5
		sy += float64(p.y) + 0.5
	}
	n := float64(len(island))
	return [2]float64{sx / n, sy / n}
}

// leftoverIsHole is true when removing leftover from owned leaves
// leftover as an enclosed void. An outer overshoot is not a hole.
func ringSubtract(keep, cut [][2]float64, bounds image.Rectangle) []pix {
	if len(keep) < 3 || bounds.Empty() {
		return nil
	}
	var out []pix
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if pointInRing(keep, fx, fy) && (len(cut) < 3 || !pointInRing(cut, fx, fy)) {
				out = append(out, pix{x, y})
			}
		}
	}
	return out
}

func leftoverIsHole(owned, leftover []pix) bool {
	drop := pixSet(leftover)
	var rem []pix
	for _, p := range owned {
		if !drop[p] {
			rem = append(rem, p)
		}
	}
	if len(rem) == 0 {
		return false
	}
	want := pixSet(leftover)
	for _, h := range voids(rem) {
		for _, p := range h {
			if want[p] {
				return true
			}
		}
	}
	return false
}

// shrinkOuter pulls one covering edge onto the leftover outline
// so a hull overshoot becomes a dent, not an evenodd white hole.
func shrinkOuter(outer [][2]float64, leftover []pix) [][2]float64 {
	bite := coverRing(leftover)
	if len(outer) < 3 || len(bite) < 2 {
		return nil
	}
	ctr := leftoverCenter(leftover)
	n := len(outer)
	ei, bestD := 0, -1.0
	for e := 0; e < n; e++ {
		a, c := outer[e], outer[(e+1)%n]
		mid := [2]float64{(a[0] + c[0]) / 2, (a[1] + c[1]) / 2}
		d := (mid[0]-ctr[0])*(mid[0]-ctr[0]) + (mid[1]-ctr[1])*(mid[1]-ctr[1])
		if bestD < 0 || d < bestD {
			ei, bestD = e, d
		}
	}
	chain := longerBite(bite, outer[ei], outer[(ei+1)%n])
	if len(chain) < 1 {
		return nil
	}
	out := append([][2]float64{}, outer[:ei+1]...)
	out = append(out, chain...)
	out = append(out, outer[ei+1:]...)
	return uncross(collapseColinear(out))
}

func longerBite(bite [][2]float64, a, b [2]float64) [][2]float64 {
	n := len(bite)
	ia, ib := 0, 0
	da, db := -1.0, -1.0
	for i, p := range bite {
		d := (p[0]-a[0])*(p[0]-a[0]) + (p[1]-a[1])*(p[1]-a[1])
		if da < 0 || d < da {
			da, ia = d, i
		}
		d = (p[0]-b[0])*(p[0]-b[0]) + (p[1]-b[1])*(p[1]-b[1])
		if db < 0 || d < db {
			db, ib = d, i
		}
	}
	if ia == ib {
		return nil
	}
	walk := func(step int) [][2]float64 {
		var out [][2]float64
		for i := ia; i != ib; i = (i + step + n) % n {
			if i != ia {
				out = append(out, bite[i])
			}
		}
		return out
	}
	w1, w2 := walk(1), walk(-1)
	if biteLen(w2) > biteLen(w1) {
		return w2
	}
	return w1
}

func biteLen(pts [][2]float64) float64 {
	var sum float64
	for i := 1; i < len(pts); i++ {
		dx := pts[i][0] - pts[i-1][0]
		dy := pts[i][1] - pts[i-1][1]
		sum += dx*dx + dy*dy
	}
	return sum
}

func nearest(ring [][2]float64, q [2]float64) [2]float64 {
	if len(ring) == 0 {
		return q
	}
	best := ring[0]
	bestD := (best[0]-q[0])*(best[0]-q[0]) + (best[1]-q[1])*(best[1]-q[1])
	for _, p := range ring[1:] {
		d := (p[0]-q[0])*(p[0]-q[0]) + (p[1]-q[1])*(p[1]-q[1])
		if d < bestD {
			best, bestD = p, d
		}
	}
	return best
}
