package stack

import (
	"math"
	"sort"

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
	if !finitePoint(c1) || !finitePoint(c2) {
		return [2]float64{}, [2]float64{}, true
	}
	return c1, c2, false
}

func finitePoint(point [2]float64) bool {
	return !math.IsNaN(point[0]) && !math.IsNaN(point[1]) && !math.IsInf(point[0], 0) && !math.IsInf(point[1], 0)
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
	for i := 0; i < len(cmds)-1; i++ {
		verts = append(verts, [2]float64{cmds[i].X, cmds[i].Y})
	}
	if from == to {
		return pathRing{}
	}
	for i := to; i != from; i = (i + 1) % n {
		verts = append(verts, r.verts[i])
		next := (i + 1) % n
		if next == from {
			edges = append(edges, r.edges[i])
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

// fitPiece is one stretch of a monotone run. A cubic is kept only when
// the curve stays closer to the stair than the stair stays to the chord.
// Anything else is split, or left as its original lines when it cannot split.
type fitPiece struct {
	samples  [][2]float64
	commands []svg.PathCmd
}

func fitPieces(samples [][2]float64) []fitPiece {
	if len(samples) < 2 {
		return nil
	}
	if commands, ok := winningCubic(samples); ok {
		return []fitPiece{{samples: append([][2]float64{}, samples...), commands: commands}}
	}
	far := farthestSample(samples)
	if len(samples) < 5 || far < 2 || far > len(samples)-3 {
		return []fitPiece{{samples: append([][2]float64{}, samples...), commands: lineChain(samples)}}
	}
	return append(fitPieces(samples[:far+1]), fitPieces(samples[far:])...)
}

// refineFit splits the cubic that leaves its stair farthest. The two
// sides are fitted again, so a half that still loses to its chord is
// not emitted as that cubic.
func refineFit(pieces []fitPiece) ([]fitPiece, bool) {
	worst := -1
	worstBulge := -1.0
	for i, piece := range pieces {
		if len(piece.commands) != 1 || piece.commands[0].Kind != svg.CmdCubic || len(piece.samples) < 5 {
			continue
		}
		bulge := pieceBulge(piece)
		if bulge > worstBulge {
			worst = i
			worstBulge = bulge
		}
	}
	if worst < 0 {
		return nil, false
	}
	samples := pieces[worst].samples
	far := farthestSample(samples)
	if far < 2 || far > len(samples)-3 {
		return nil, false
	}
	left := fitPieces(samples[:far+1])
	right := fitPieces(samples[far:])
	next := make([]fitPiece, 0, len(pieces)+len(left)+len(right))
	next = append(next, pieces[:worst]...)
	next = append(next, left...)
	next = append(next, right...)
	next = append(next, pieces[worst+1:]...)
	if samePieceCommands(next, pieces) {
		return nil, false
	}
	return next, true
}

func pieceCommands(pieces []fitPiece) []svg.PathCmd {
	var commands []svg.PathCmd
	for _, piece := range pieces {
		commands = append(commands, piece.commands...)
	}
	return commands
}

func winningCubic(samples [][2]float64) ([]svg.PathCmd, bool) {
	if len(samples) < 3 {
		return nil, false
	}
	c1, c2, line := fitCubic(samples)
	if line {
		return nil, false
	}
	if stairBulge(samples, c1, c2) > chordGap(samples) {
		return nil, false
	}
	return []svg.PathCmd{cubicCommand(c1, c2, samples[len(samples)-1])}, true
}

func pieceBulge(piece fitPiece) float64 {
	command := piece.commands[0]
	return stairBulge(piece.samples, [2]float64{command.X1, command.Y1}, [2]float64{command.X2, command.Y2})
}

func farthestSample(samples [][2]float64) int {
	if len(samples) < 3 {
		return 0
	}
	a, b := samples[0], samples[len(samples)-1]
	bestI := 1
	best := -1.0
	for i := 1; i < len(samples)-1; i++ {
		distance := distLine(samples[i], a, b)
		if distance > best {
			best = distance
			bestI = i
		}
	}
	return bestI
}

func chordGap(samples [][2]float64) float64 {
	if len(samples) < 3 {
		return 0
	}
	a, b := samples[0], samples[len(samples)-1]
	gap := 0.0
	for _, point := range samples[1 : len(samples)-1] {
		if distance := segmentDistance(point, a, b); distance > gap {
			gap = distance
		}
	}
	return gap
}

func stairBulge(samples [][2]float64, c1, c2 [2]float64) float64 {
	if len(samples) < 2 {
		return 0
	}
	steps := (len(samples) - 1) * 4
	if steps < 16 {
		steps = 16
	}
	curve := make([][2]float64, steps+1)
	p0 := samples[0]
	p3 := samples[len(samples)-1]
	for i := 0; i <= steps; i++ {
		curve[i] = cubicAt(p0, c1, c2, p3, float64(i)/float64(steps))
	}
	bulge := 0.0
	for _, point := range curve {
		if distance := polylineDistance(point, samples); distance > bulge {
			bulge = distance
		}
	}
	for _, point := range samples {
		if distance := polylineDistance(point, curve); distance > bulge {
			bulge = distance
		}
	}
	return bulge
}

func lineChain(samples [][2]float64) []svg.PathCmd {
	commands := make([]svg.PathCmd, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		commands = append(commands, lineCommand(samples[i]))
	}
	return commands
}

func sameLineChain(commands []svg.PathCmd, samples [][2]float64) bool {
	if len(commands) != len(samples)-1 {
		return false
	}
	for i, command := range commands {
		if command.Kind != svg.CmdLine || command.X != samples[i+1][0] || command.Y != samples[i+1][1] {
			return false
		}
	}
	return true
}

func samePieceCommands(left, right []fitPiece) bool {
	return sameCommands(pieceCommands(left), pieceCommands(right))
}

func sameCommands(left, right []svg.PathCmd) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func segmentDistance(point, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	length := dx*dx + dy*dy
	if length == 0 {
		return math.Hypot(point[0]-a[0], point[1]-a[1])
	}
	t := ((point[0]-a[0])*dx + (point[1]-a[1])*dy) / length
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(point[0]-(a[0]+t*dx), point[1]-(a[1]+t*dy))
}

func polylineDistance(point [2]float64, polyline [][2]float64) float64 {
	if len(polyline) == 0 {
		return 0
	}
	best := math.Hypot(point[0]-polyline[0][0], point[1]-polyline[0][1])
	for i := 1; i < len(polyline); i++ {
		if distance := segmentDistance(point, polyline[i-1], polyline[i]); distance < best {
			best = distance
		}
	}
	return best
}

func spansByChord(ring pathRing, spans []ringSpan) []ringSpan {
	ordered := append([]ringSpan{}, spans...)
	n := len(ring.verts)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := chordGap(spanPoints(ring.verts, ordered[i].from, ordered[i].to))
		right := chordGap(spanPoints(ring.verts, ordered[j].from, ordered[j].to))
		if left != right {
			return left > right
		}
		return edgesInSpan(ordered[i].from, ordered[i].to, n) > edgesInSpan(ordered[j].from, ordered[j].to, n)
	})
	return ordered
}

func pullCubicJoint(p0, p1, p2, p3, pull [2]float64) (left, right svg.PathCmd, ok bool) {
	a, b := splitBezier(p0, p1, p2, p3)
	mid := a[3]
	if mid == pull {
		return svg.PathCmd{}, svg.PathCmd{}, false
	}
	moved := [2]float64{(mid[0] + pull[0]) / 2, (mid[1] + pull[1]) / 2}
	deltaX, deltaY := moved[0]-mid[0], moved[1]-mid[1]
	innerLeft := [2]float64{a[2][0] + deltaX, a[2][1] + deltaY}
	innerRight := [2]float64{b[1][0] + deltaX, b[1][1] + deltaY}
	return cubicCommand(a[1], innerLeft, moved), cubicCommand(innerRight, b[2], p3), true
}

func splitBezier(p0, p1, p2, p3 [2]float64) (left, right [4][2]float64) {
	midpoint := func(a, b [2]float64) [2]float64 {
		return [2]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
	}
	p01 := midpoint(p0, p1)
	p12 := midpoint(p1, p2)
	p23 := midpoint(p2, p3)
	p012 := midpoint(p01, p12)
	p123 := midpoint(p12, p23)
	joint := midpoint(p012, p123)
	return [4][2]float64{p0, p01, p012, joint}, [4][2]float64{joint, p123, p23, p3}
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
