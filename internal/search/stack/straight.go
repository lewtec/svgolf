package stack

import "github.com/lewtec/svgolf/pkg/svg"

// chainStraight reports that pts is one straight form.
// Exact colinear points are straight at any spacing.
// Integer points are straight when they are one 4-connected
// digital line (Reveilles): the cross span is narrower than
// |a|+|b| for the reduced chord, and the chain does not
// step backwards. That width is the grid, not a tolerance.
func chainStraight(pts [][2]float64) bool {
	m := len(pts)
	if m <= 2 {
		return true
	}
	x0, y0 := pts[0][0], pts[0][1]
	x1, y1 := pts[m-1][0], pts[m-1][1]
	if colinearPoints(pts) {
		return monotoneChord(pts)
	}
	if !allInteger(pts) {
		return false
	}
	dx := int(x1 - x0)
	dy := int(y1 - y0)
	g := gcd(absInt(dx), absInt(dy))
	if g == 0 {
		return false
	}
	a := dy / g
	b := dx / g
	thickness := absInt(a) + absInt(b)
	minV, maxV := 0, 0
	prevT := 0
	for i := 1; i < m; i++ {
		vx := int(pts[i][0] - x0)
		vy := int(pts[i][1] - y0)
		v := a*vx - b*vy
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		t := vx*dx + vy*dy
		if t < prevT {
			return false
		}
		prevT = t
	}
	return maxV-minV < thickness
}

func colinearPoints(pts [][2]float64) bool {
	a, b := pts[0], pts[len(pts)-1]
	dx, dy := b[0]-a[0], b[1]-a[1]
	for _, p := range pts[1 : len(pts)-1] {
		if (p[0]-a[0])*dy != (p[1]-a[1])*dx {
			return false
		}
	}
	return true
}

func monotoneChord(pts [][2]float64) bool {
	a, b := pts[0], pts[len(pts)-1]
	dx, dy := b[0]-a[0], b[1]-a[1]
	prev := 0.0
	for i := 1; i < len(pts); i++ {
		t := (pts[i][0]-a[0])*dx + (pts[i][1]-a[1])*dy
		if t < prev {
			return false
		}
		prev = t
	}
	return true
}

func allInteger(pts [][2]float64) bool {
	for _, p := range pts {
		if p[0] != float64(int(p[0])) || p[1] != float64(int(p[1])) {
			return false
		}
	}
	return true
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 0
	}
	return a
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func lineEdge(c svg.PathCmd) bool {
	return c.Kind == svg.CmdLine || c.Kind == svg.CmdClose
}

// straightVertex is one vertex that already lies on the line
// through its two neighbors. Both edges have to be lines; a
// cubic is the curve and must stay. A corner fails the test,
// so each accepted drop is one step along a straight run.
func (r pathRing) straightVertex(i int) bool {
	n := len(r.verts)
	if n < 4 || i < 0 || i >= n || len(r.edges) != n {
		return false
	}
	prev := (i - 1 + n) % n
	if !lineEdge(r.edges[prev]) || !lineEdge(r.edges[i]) {
		return false
	}
	next := (i + 1) % n
	return chainStraight([][2]float64{r.verts[prev], r.verts[i], r.verts[next]})
}
