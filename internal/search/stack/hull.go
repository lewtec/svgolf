package stack

import (
	"math"
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

// quadRing is coverRing with four sides.
func quadRing(work []pix) [][2]float64 {
	return coverRing(work, 4)
}

// coverRing walks the residual outline, then collapses to sides.
// Convex hull filled notches; the outline does not.
func coverRing(work []pix, sides int) [][2]float64 {
	if len(work) == 0 {
		return nil
	}
	ring := outline(work)
	if len(ring) < 3 {
		return bbox(work)
	}
	return collapseToSides(ring, sides)
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

func collapseToSides(ring [][2]float64, sides int) [][2]float64 {
	if sides < 3 {
		sides = 3
	}
	if len(ring) <= sides {
		return ring
	}
	out := append([][2]float64{}, ring...)
	for len(out) > sides {
		n := len(out)
		drop := 0
		best := math.Inf(1)
		for i := 0; i < n; i++ {
			d := distLine(out[i], out[(i-1+n)%n], out[(i+1)%n])
			if d < best {
				best = d
				drop = i
			}
		}
		out = append(out[:drop], out[drop+1:]...)
	}
	return out
}
