package stack

import (
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

type ringSpan struct {
	from, to int
}

// monotoneSpans are maximal chains of line edges that share a sign in x and a
// sign in y. A zero component may join either sign. A run of one edge is omitted:
// replacing one line with one cubic lowers command weight without changing the silhouette.
func monotoneSpans(ring pathRing) []ringSpan {
	n := len(ring.verts)
	if n < 4 || len(ring.edges) != n {
		return nil
	}
	var out []ringSpan
	for start := 0; start < n; {
		if !lineEdge(ring.edges[start]) {
			start++
			continue
		}
		last := start
		for step := 1; step < n-2; step++ {
			edge := (start + step) % n
			end := (edge + 1) % n
			if end == start {
				break
			}
			if !lineEdge(ring.edges[edge]) {
				break
			}
			if !spanMonotone(ring.verts, start, end) {
				break
			}
			kept := n - step
			if kept < 3 {
				break
			}
			last = edge
		}
		end := (last + 1) % n
		if edgesInSpan(start, end, n) >= 2 {
			out = append(out, ringSpan{start, end})
			if end <= start {
				break
			}
			start = end
			continue
		}
		start++
	}
	return out
}

func lineEdge(cmd svg.PathCmd) bool {
	return cmd.Kind == svg.CmdLine || cmd.Kind == svg.CmdClose
}

func edgesInSpan(from, to, n int) int {
	if n == 0 {
		return 0
	}
	if from == to {
		return n
	}
	return (to - from + n) % n
}

func spanMonotone(verts [][2]float64, from, to int) bool {
	n := len(verts)
	posX, negX, posY, negY := false, false, false, false
	for e := from; e != to; e = (e + 1) % n {
		dx, dy := edgeDelta(verts, e)
		if dx > 0 {
			posX = true
		}
		if dx < 0 {
			negX = true
		}
		if dy > 0 {
			posY = true
		}
		if dy < 0 {
			negY = true
		}
		if posX && negX || posY && negY {
			return false
		}
	}
	return from != to
}

func edgeDelta(verts [][2]float64, edge int) (float64, float64) {
	n := len(verts)
	a := verts[edge]
	b := verts[(edge+1)%n]
	return b[0] - a[0], b[1] - a[1]
}

func spanPoints(verts [][2]float64, from, to int) [][2]float64 {
	n := len(verts)
	out := [][2]float64{verts[from]}
	for e := from; e != to; e = (e + 1) % n {
		out = append(out, verts[(e+1)%n])
		if len(out) > n+1 {
			break
		}
	}
	return out
}

func farthestOnSpan(verts [][2]float64, from, to int) (int, bool) {
	n := len(verts)
	a, b := verts[from], verts[to]
	bestI := -1
	var best float64
	for e := (from + 1) % n; e != to; e = (e + 1) % n {
		d := distLine(verts[e], a, b)
		if bestI < 0 || d > best {
			bestI = e
			best = d
		}
		if e == from {
			break
		}
	}
	return bestI, bestI >= 0
}

// fitCubic is the least-squares cubic through samples with fixed endpoints.
// Colinear samples return line true and the chord.
func fitCubic(samples [][2]float64) (c1, c2 [2]float64, line bool) {
	if len(samples) < 2 {
		return [2]float64{}, [2]float64{}, true
	}
	p0 := samples[0]
	p3 := samples[len(samples)-1]
	ts := chordT(samples)
	var a00, a01, a11 float64
	var bx0, bx1, by0, by1 float64
	for i, p := range samples {
		t := ts[i]
		u := 1 - t
		b1 := 3 * u * u * t
		b2 := 3 * u * t * t
		tx := p[0] - (u*u*u*p0[0] + t*t*t*p3[0])
		ty := p[1] - (u*u*u*p0[1] + t*t*t*p3[1])
		a00 += b1 * b1
		a01 += b1 * b2
		a11 += b2 * b2
		bx0 += b1 * tx
		bx1 += b2 * tx
		by0 += b1 * ty
		by1 += b2 * ty
	}
	det := a00*a11 - a01*a01
	if det == 0 {
		return [2]float64{}, [2]float64{}, true
	}
	c1 = [2]float64{(a11*bx0 - a01*bx1) / det, (a11*by0 - a01*by1) / det}
	c2 = [2]float64{(a00*bx1 - a01*bx0) / det, (a00*by1 - a01*by0) / det}
	return c1, c2, false
}

func chordT(samples [][2]float64) []float64 {
	ts := make([]float64, len(samples))
	var total float64
	seg := make([]float64, len(samples))
	for i := 1; i < len(samples); i++ {
		seg[i] = math.Hypot(samples[i][0]-samples[i-1][0], samples[i][1]-samples[i-1][1])
		total += seg[i]
	}
	if total == 0 {
		for i := range ts {
			ts[i] = float64(i) / float64(len(samples)-1)
		}
		return ts
	}
	for i := 1; i < len(samples); i++ {
		ts[i] = ts[i-1] + seg[i]/total
	}
	return ts
}

func cubicCommand(c1, c2, dest [2]float64) svg.PathCmd {
	return svg.PathCmd{Kind: svg.CmdCubic, X1: c1[0], Y1: c1[1], X2: c2[0], Y2: c2[1], X: dest[0], Y: dest[1]}
}

func lineCommand(dest [2]float64) svg.PathCmd {
	return svg.PathCmd{Kind: svg.CmdLine, X: dest[0], Y: dest[1]}
}

// replaceSpan rotates the ring so from is the first vertex and replaces the
// forward walk from->to with cmds. The rest of the ring stays.
func (r pathRing) replaceSpan(from, to int, cmds []svg.PathCmd) pathRing {
	n := len(r.verts)
	if n < 3 || len(r.edges) != n || len(cmds) == 0 || from < 0 || to < 0 || from >= n || to >= n {
		return pathRing{}
	}
	verts := [][2]float64{r.verts[from]}
	edges := append([]svg.PathCmd{}, cmds...)
	if from == to {
		return pathRing{}
	}
	for i := to; i != from; i = (i + 1) % n {
		verts = append(verts, r.verts[i])
		next := (i + 1) % n
		if next == from {
			edges = append(edges, svg.PathCmd{Kind: svg.CmdClose, X: r.verts[from][0], Y: r.verts[from][1]})
			break
		}
		edges = append(edges, r.edges[i])
	}
	if len(verts) < 3 || len(edges) != len(verts) {
		return pathRing{}
	}
	return pathRing{verts: verts, edges: edges}
}

func spanCommands(samples [][2]float64) []svg.PathCmd {
	if len(samples) < 2 {
		return nil
	}
	dest := samples[len(samples)-1]
	c1, c2, line := fitCubic(samples)
	if line {
		return []svg.PathCmd{lineCommand(dest)}
	}
	return []svg.PathCmd{cubicCommand(c1, c2, dest)}
}

func splitCommands(verts [][2]float64, from, far, to int) []svg.PathCmd {
	n := len(verts)
	if edgesInSpan(from, far, n) < 2 || edgesInSpan(far, to, n) < 2 {
		return nil
	}
	left := spanCommands(spanPoints(verts, from, far))
	right := spanCommands(spanPoints(verts, far, to))
	if len(left) != 1 || len(right) != 1 {
		return nil
	}
	return append(left, right...)
}

func cubicSamples(cmd svg.PathCmd, from [2]float64) [][2]float64 {
	if cmd.Kind != svg.CmdCubic {
		return [][2]float64{from, {cmd.X, cmd.Y}}
	}
	p0 := from
	p1 := [2]float64{cmd.X1, cmd.Y1}
	p2 := [2]float64{cmd.X2, cmd.Y2}
	p3 := [2]float64{cmd.X, cmd.Y}
	out := make([][2]float64, 0, 9)
	for i := 0; i <= 8; i++ {
		out = append(out, cubicAt(p0, p1, p2, p3, float64(i)/8))
	}
	return out
}

func ringLoops(ring pathRing) bool {
	if len(ring.verts) < 3 {
		return true
	}
	var pts [][2]float64
	n := len(ring.verts)
	for i, edge := range ring.edges {
		samples := cubicSamples(edge, ring.verts[i])
		if i == n-1 && len(samples) > 0 {
			samples = samples[:len(samples)-1]
		}
		if len(pts) > 0 && len(samples) > 0 {
			samples = samples[1:]
		}
		pts = append(pts, samples...)
	}
	return ringCrosses(pts)
}
