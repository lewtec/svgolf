package render

import (
	"math"
	"math/bits"
)

const maxCoeffShift = 6

type cubicState struct {
	count, shift, dshift     int32
	cx, cy, cdx, cdy         int32
	cddx, cddy, cdddx, cdddy int32
	lastX, lastY             int32
	winding                  int8
}

func (e *lineEdge) update(x0, y0, x1, y1 int32) bool {
	y0 >>= 10
	y1 >>= 10
	if y1 < y0 {
		y1 = y0
	}
	top := fdot6Round(y0)
	bot := fdot6Round(y1)
	if top == bot {
		return false
	}
	x0 >>= 10
	x1 >>= 10
	slope := fdot6Div(x1-x0, y1-y0)
	dy := computeDY(top, y0)
	e.x = fdot6ToFdot16(x0 + fdot16Mul(slope, dy))
	e.dx = slope
	e.firstY = top
	e.lastY = bot - 1
	return true
}

func fdot6ToFixedDiv2(v int32) int32 { return leftShift(v, 16-6-1) }

func fdot6UpShift(x, up int32) int32 { return leftShift(x, up) }

func cheapDistance(dx, dy int32) int32 {
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx + (dy >> 1)
	}
	return dy + (dx >> 1)
}

func cubicDeltaFromLine(a, b, c, d int32) int32 {
	one := ((a*8 - b*15 + 6*c + d) * 19) >> 9
	two := ((a + 6*b - c*15 + d*8) * 19) >> 9
	if one < 0 {
		one = -one
	}
	if two < 0 {
		two = -two
	}
	if one > two {
		return one
	}
	return two
}

func diffToShift(dx, dy, shiftAA int32) int32 {
	dist := cheapDistance(dx, dy)
	dist = (dist + (1 << (2 + shiftAA))) >> (3 + shiftAA)
	return (32 - int32(bits.LeadingZeros32(uint32(dist)))) >> 1
}

func newCubicEdge(p0x, p0y, p1x, p1y, p2x, p2y, p3x, p3y float32, shift int32) (lineEdge, cubicState, bool) {
	scale := float32(int32(1) << uint(shift+6))
	x0, y0 := int32(p0x*scale), int32(p0y*scale)
	x1, y1 := int32(p1x*scale), int32(p1y*scale)
	x2, y2 := int32(p2x*scale), int32(p2y*scale)
	x3, y3 := int32(p3x*scale), int32(p3y*scale)
	winding := int8(1)
	if y0 > y3 {
		x0, x3 = x3, x0
		x1, x2 = x2, x1
		y0, y3 = y3, y0
		y1, y2 = y2, y1
		winding = -1
	}
	top := fdot6Round(y0)
	bot := fdot6Round(y3)
	if top == bot {
		return lineEdge{}, cubicState{}, false
	}
	dx := cubicDeltaFromLine(x0, x1, x2, x3)
	dy := cubicDeltaFromLine(y0, y1, y2, y3)
	shift = diffToShift(dx, dy, 2) + 1
	if shift < 1 {
		shift = 1
	}
	if shift > maxCoeffShift {
		shift = maxCoeffShift
	}
	up := int32(6)
	down := shift + up - 10
	if down < 0 {
		down = 0
		up = 10 - shift
	}
	cs := cubicState{
		count:   leftShift(-1, shift),
		shift:   shift,
		dshift:  down,
		winding: winding,
	}
	b := fdot6UpShift(3*(x1-x0), up)
	c := fdot6UpShift(3*(x0-x1-x1+x2), up)
	d := fdot6UpShift(x3+3*(x1-x2)-x0, up)
	cs.cx = fdot6ToFdot16(x0)
	cs.cdx = b + (c >> shift) + (d >> (2 * shift))
	cs.cddx = 2*c + ((3 * d) >> (shift - 1))
	cs.cdddx = (3 * d) >> (shift - 1)
	b = fdot6UpShift(3*(y1-y0), up)
	c = fdot6UpShift(3*(y0-y1-y1+y2), up)
	d = fdot6UpShift(y3+3*(y1-y2)-y0, up)
	cs.cy = fdot6ToFdot16(y0)
	cs.cdy = b + (c >> shift) + (d >> (2 * shift))
	cs.cddy = 2*c + ((3 * d) >> (shift - 1))
	cs.cdddy = (3 * d) >> (shift - 1)
	cs.lastX = fdot6ToFdot16(x3)
	cs.lastY = fdot6ToFdot16(y3)
	var le lineEdge
	le.winding = winding
	if !updateCubic(&le, &cs) {
		return lineEdge{}, cubicState{}, false
	}
	return le, cs, true
}

func updateCubic(le *lineEdge, cs *cubicState) bool {
	count := cs.count
	oldx, oldy := cs.cx, cs.cy
	var newx, newy int32
	ok := false
	for {
		count++
		if count < 0 {
			newx = oldx + (cs.cdx >> cs.dshift)
			cs.cdx += cs.cddx >> cs.shift
			cs.cddx += cs.cdddx
			newy = oldy + (cs.cdy >> cs.dshift)
			cs.cdy += cs.cddy >> cs.shift
			cs.cddy += cs.cdddy
		} else {
			newx, newy = cs.lastX, cs.lastY
		}
		if newy < oldy {
			newy = oldy
		}
		ok = le.update(oldx, oldy, newx, newy)
		oldx, oldy = newx, newy
		if count == 0 || ok {
			break
		}
	}
	cs.cx, cs.cy = newx, newy
	cs.count = count
	le.winding = cs.winding
	return ok
}

func chopCubicAtY(p [4][2]float32) [][4][2]float32 {
	y0, y1, y2, y3 := float64(p[0][1]), float64(p[1][1]), float64(p[2][1]), float64(p[3][1])
	a := y1 - y0
	b := y2 - y1
	c := y3 - y2
	// (a-2b+c)t^2 + 2(b-a)t + a = 0
	A := a - 2*b + c
	B := 2 * (b - a)
	C := a
	var ts []float64
	if math.Abs(A) < 1e-12 {
		if math.Abs(B) > 1e-12 {
			t := -C / B
			if t > 0 && t < 1 {
				ts = append(ts, t)
			}
		}
	} else {
		disc := B*B - 4*A*C
		if disc >= 0 {
			s := math.Sqrt(disc)
			t0 := (-B + s) / (2 * A)
			t1 := (-B - s) / (2 * A)
			for _, t := range []float64{t0, t1} {
				if t > 1e-6 && t < 1-1e-6 {
					ts = append(ts, t)
				}
			}
		}
	}
	if len(ts) == 0 {
		return [][4][2]float32{p}
	}
	if len(ts) == 2 && ts[1] < ts[0] {
		ts[0], ts[1] = ts[1], ts[0]
	}
	out := [][4][2]float32{p}
	for _, t := range ts {
		var next [][4][2]float32
		for _, cub := range out {
			a, b := splitCubic(cub, float32(t))
			next = append(next, a, b)
		}
		out = next
	}
	return out
}

func splitCubic(p [4][2]float32, t float32) ([4][2]float32, [4][2]float32) {
	lerp := func(a, b [2]float32) [2]float32 {
		return [2]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
	}
	ab := lerp(p[0], p[1])
	bc := lerp(p[1], p[2])
	cd := lerp(p[2], p[3])
	abc := lerp(ab, bc)
	bcd := lerp(bc, cd)
	abcd := lerp(abc, bcd)
	return [4][2]float32{p[0], ab, abc, abcd}, [4][2]float32{abcd, bcd, cd, p[3]}
}
