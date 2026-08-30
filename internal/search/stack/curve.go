package stack

import (
	"image/color"
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

func filledFit(island []pix, ring [][2]float64, col color.NRGBA) svg.Path {
	set := pixSet(island)
	ring = flowRing(set, ring)
	return appendFit(svg.NewPath(), set, ring).WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
}

// flowRing moves each vertex toward the neighbor midpoint when that fits the island better.
func pixSet(island []pix) map[pix]bool {
	m := make(map[pix]bool, len(island))
	for _, p := range island {
		m[p] = true
	}
	return m
}

func flowRing(set map[pix]bool, ring [][2]float64) [][2]float64 {
	n := len(ring)
	if n < 3 {
		return ring
	}
	out := append([][2]float64{}, ring...)
	for i := 0; i < n; i++ {
		a, b, c := out[(i-1+n)%n], out[i], out[(i+1)%n]
		mid := [2]float64{(a[0] + c[0]) / 2, (a[1] + c[1]) / 2}
		mix := [2]float64{(b[0] + mid[0]) / 2, (b[1] + mid[1]) / 2}
		best, bestE := b, edgeScore(set, a, b, c, false)
		for _, cand := range [][2]float64{mix, mid} {
			if e := edgeScore(set, a, cand, c, false); e < bestE {
				best, bestE = cand, e
			}
		}
		out[i] = best
	}
	return out
}

// appendFit walks the ring. At each vertex, keep two lines or an interpolating
// cubic pair, whichever scores better on the island in that corner's bbox.
func appendFit(p svg.Path, set map[pix]bool, ring [][2]float64) svg.Path {
	n := len(ring)
	if n < 3 {
		return p
	}
	cmds := p.Commands()
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdMove, X: ring[0][0], Y: ring[0][1]})
	for i := 0; i < n-1; {
		a := ring[i]
		b := ring[i+1]
		c := ring[(i+2)%n]
		if (i+1 < n-1 || i+2 == n) && edgeScore(set, a, b, c, true) < edgeScore(set, a, b, c, false) {
			c1, c2 := crTo(a, a, b, c)
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdCubic, X1: c1[0], Y1: c1[1], X2: c2[0], Y2: c2[1], X: b[0], Y: b[1]})
			d1, d2 := crTo(a, b, c, c)
			cmds = append(cmds, svg.PathCmd{Kind: svg.CmdCubic, X1: d1[0], Y1: d1[1], X2: d2[0], Y2: d2[1], X: c[0], Y: c[1]})
			i += 2
			continue
		}
		cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: b[0], Y: b[1]})
		i++
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	p, _ = p.WithCommands(cmds)
	return p
}

// crTo is the Catmull-Rom cubic from p1 to p2 given neighbors p0, p3.
func crTo(p0, p1, p2, p3 [2]float64) (c1, c2 [2]float64) {
	return [2]float64{p1[0] + (p2[0]-p0[0])/6, p1[1] + (p2[1]-p0[1])/6},
		[2]float64{p2[0] - (p3[0]-p1[0])/6, p2[1] - (p3[1]-p1[1])/6}
}

func edgeScore(set map[pix]bool, a, b, c [2]float64, curve bool) float64 {
	var pts [][2]float64
	if curve {
		c1, c2 := crTo(a, a, b, c)
		d1, d2 := crTo(a, b, c, c)
		for i := 1; i <= 4; i++ {
			t := float64(i) / 5
			pts = append(pts, cubicAt(a, c1, c2, b, t), cubicAt(b, d1, d2, c, t))
		}
	} else {
		for i := 1; i <= 4; i++ {
			t := float64(i) / 5
			pts = append(pts, lerp(a, b, t), lerp(b, c, t))
		}
	}
	var e float64
	for _, q := range pts {
		e += edgeErr(set, q)
	}
	return e
}

func edgeErr(set map[pix]bool, q [2]float64) float64 {
	ix, iy := int(math.Floor(q[0])), int(math.Floor(q[1]))
	p := pix{ix, iy}
	n4 := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	if !set[p] {
		for _, d := range n4 {
			if set[pix{ix + d.x, iy + d.y}] {
				return 0.5
			}
		}
		return 2
	}
	for _, d := range n4 {
		if !set[pix{ix + d.x, iy + d.y}] {
			return 0
		}
	}
	return 1
}

func lerp(a, b [2]float64, t float64) [2]float64 {
	return [2]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
}

func cubicAt(p0, p1, p2, p3 [2]float64, t float64) [2]float64 {
	u := 1 - t
	return [2]float64{
		u*u*u*p0[0] + 3*u*u*t*p1[0] + 3*u*t*t*p2[0] + t*t*t*p3[0],
		u*u*u*p0[1] + 3*u*u*t*p1[1] + 3*u*t*t*p2[1] + t*t*t*p3[1],
	}
}
