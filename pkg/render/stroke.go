package render

import (
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

func strokeToPath(src path, st svg.Stroke) path {
	w := float32(st.Width())
	if w < 0 {
		return path{}
	}
	if w == 0 {
		w = 1
	}
	half := w / 2
	pts := flattenToPoly(src)
	if len(pts) < 2 {
		return path{}
	}
	closed := pts[0][0] == pts[len(pts)-1][0] && pts[0][1] == pts[len(pts)-1][1]
	if closed && len(pts) >= 3 {
		pts = pts[:len(pts)-1]
	}
	left, right := offsetPoly(pts, half, closed, st)
	var out path
	out.empty = true
	if len(left) == 0 {
		return out
	}
	out.moveTo(left[0][0], left[0][1])
	for _, p := range left[1:] {
		out.lineTo(p[0], p[1])
	}
	if !closed {
		// cap at end: connect to right reversed
		for i := len(right) - 1; i >= 0; i-- {
			out.lineTo(right[i][0], right[i][1])
		}
	} else {
		out.close()
		out.moveTo(right[len(right)-1][0], right[len(right)-1][1])
		for i := len(right) - 2; i >= 0; i-- {
			out.lineTo(right[i][0], right[i][1])
		}
	}
	out.close()
	return out
}

func flattenToPoly(p path) [][2]float32 {
	var pts [][2]float32
	var mx, my, cx, cy float32
	have := false
	for _, s := range p.segs {
		switch s.kind {
		case segMove:
			mx, my, cx, cy = s.x, s.y, s.x, s.y
			pts = append(pts, [2]float32{s.x, s.y})
			have = true
		case segLine:
			pts = append(pts, [2]float32{s.x, s.y})
			cx, cy = s.x, s.y
		case segCubic:
			pts = append(pts, cubicToLines(cx, cy, s.x1, s.y1, s.x2, s.y2, s.x, s.y)...)
			cx, cy = s.x, s.y
		case segClose:
			if have {
				pts = append(pts, [2]float32{mx, my})
				cx, cy = mx, my
			}
		}
	}
	return pts
}

func cubicToLines(x0, y0, x1, y1, x2, y2, x3, y3 float32) [][2]float32 {
	var out [][2]float32
	var rec func(ax, ay, bx, by, cx, cy, dx, dy float32, depth int)
	rec = func(ax, ay, bx, by, cx, cy, dx, dy float32, depth int) {
		dx0, dy0 := dx-ax, dy-ay
		d1 := abs32((bx-ax)*dy0 - (by-ay)*dx0)
		d2 := abs32((cx-ax)*dy0 - (cy-ay)*dx0)
		if depth > 8 || d1+d2 < 0.25*0.25*(dx0*dx0+dy0*dy0+1) {
			out = append(out, [2]float32{dx, dy})
			return
		}
		abx, aby := (ax+bx)/2, (ay+by)/2
		bcx, bcy := (bx+cx)/2, (by+cy)/2
		cdx, cdy := (cx+dx)/2, (cy+dy)/2
		abcx, abcy := (abx+bcx)/2, (aby+bcy)/2
		bcdx, bcdy := (bcx+cdx)/2, (bcy+cdy)/2
		mx, my := (abcx+bcdx)/2, (abcy+bcdy)/2
		rec(ax, ay, abx, aby, abcx, abcy, mx, my, depth+1)
		rec(mx, my, bcdx, bcdy, cdx, cdy, dx, dy, depth+1)
	}
	rec(x0, y0, x1, y1, x2, y2, x3, y3, 0)
	return out
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func offsetPoly(pts [][2]float32, half float32, closed bool, st svg.Stroke) (left, right [][2]float32) {
	n := len(pts)
	if n < 2 {
		return nil, nil
	}
	norm := func(i0, i1 int) (float32, float32) {
		dx := pts[i1][0] - pts[i0][0]
		dy := pts[i1][1] - pts[i0][1]
		l := float32(math.Hypot(float64(dx), float64(dy)))
		if l == 0 {
			return 0, 0
		}
		return -dy / l * half, dx / l * half
	}
	left = make([][2]float32, 0, n+2)
	right = make([][2]float32, 0, n+2)
	segN := n
	if !closed {
		segN = n - 1
	}
	for i := 0; i < n; i++ {
		var nx0, ny0, nx1, ny1 float32
		if closed {
			i0 := (i - 1 + n) % n
			i1 := i
			i2 := (i + 1) % n
			nx0, ny0 = norm(i0, i1)
			nx1, ny1 = norm(i1, i2)
		} else {
			if i == 0 {
				nx1, ny1 = norm(0, 1)
				nx0, ny0 = nx1, ny1
			} else if i == n-1 {
				nx0, ny0 = norm(n-2, n-1)
				nx1, ny1 = nx0, ny0
			} else {
				nx0, ny0 = norm(i-1, i)
				nx1, ny1 = norm(i, i+1)
			}
		}
		px, py := pts[i][0], pts[i][1]
		ix, iy, ok := offsetIntersect(px, py, nx0, ny0, nx1, ny1)
		ml := float32(st.MiterLimit()) * half
		useMiter := ok && (ix-px)*(ix-px)+(iy-py)*(iy-py) <= ml*ml+0.01
		if st.Join() == svg.JoinMiter && useMiter {
			left = append(left, [2]float32{ix, iy})
			rx, ry, _ := offsetIntersect(px, py, -nx0, -ny0, -nx1, -ny1)
			right = append(right, [2]float32{rx, ry})
		} else if st.Join() == svg.JoinRound && i > 0 && i < n-1 || (closed && st.Join() == svg.JoinRound) {
			left = append(left, [2]float32{px + nx0, py + ny0})
			left = append(left, arcPoints(px, py, px+nx0, py+ny0, px+nx1, py+ny1)...)
			right = append(right, [2]float32{px - nx0, py - ny0})
			right = append(right, arcPoints(px, py, px-nx0, py-ny0, px-nx1, py-ny1)...)
		} else {
			left = append(left, [2]float32{px + nx0, py + ny0})
			if nx0 != nx1 || ny0 != ny1 {
				left = append(left, [2]float32{px + nx1, py + ny1})
			}
			right = append(right, [2]float32{px - nx0, py - ny0})
			if nx0 != nx1 || ny0 != ny1 {
				right = append(right, [2]float32{px - nx1, py - ny1})
			}
		}
		_ = segN
	}
	if !closed {
		// butt / square / round caps
		applyCap := func(atStart bool) {
			var i0, i1 int
			if atStart {
				i0, i1 = 0, 1
			} else {
				i0, i1 = n-1, n-2
			}
			dx := pts[i0][0] - pts[i1][0]
			dy := pts[i0][1] - pts[i1][1]
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l == 0 {
				return
			}
			ux, uy := dx/l, dy/l
			switch st.Cap() {
			case svg.CapSquare:
				offx, offy := ux*half, uy*half
				if atStart {
					left[0][0] += offx
					left[0][1] += offy
					right[0][0] += offx
					right[0][1] += offy
				} else {
					left[len(left)-1][0] += offx
					left[len(left)-1][1] += offy
					right[len(right)-1][0] += offx
					right[len(right)-1][1] += offy
				}
			case svg.CapRound:
				// round cap is handled by connecting left/right with an arc at outline build
			}
		}
		applyCap(true)
		applyCap(false)
	}
	return left, right
}

func offsetIntersect(px, py, nx0, ny0, nx1, ny1 float32) (float32, float32, bool) {
	// intersection of line through (px+nx0,py+ny0) dir (-ny0, nx0)
	// and line through (px+nx1,py+ny1) dir (-ny1, nx1)
	return lineIntersect(px+nx0, py+ny0, -ny0, nx0, px+nx1, py+ny1, -ny1, nx1)
}

func lineIntersect(ax, ay, adx, ady, bx, by, bdx, bdy float32) (float32, float32, bool) {
	den := adx*bdy - ady*bdx
	if abs32(den) < 1e-8 {
		return 0, 0, false
	}
	t := ((bx-ax)*bdy - (by-ay)*bdx) / den
	return ax + t*adx, ay + t*ady, true
}

func ptsPrev(px, py, nx0, ny0 float32, pts [][2]float32, i int) (float32, float32) {
	return px, py
}

func intersect(x1, y1, x2, y2, x3, y3, x4, y4 float32) (float32, float32, bool) {
	return lineIntersect(x1, y1, x2-x1, y2-y1, x3, y3, x4-x3, y4-y3)
}

func arcPoints(cx, cy, x0, y0, x1, y1 float32) [][2]float32 {
	a0 := math.Atan2(float64(y0-cy), float64(x0-cx))
	a1 := math.Atan2(float64(y1-cy), float64(x1-cx))
	da := a1 - a0
	for da <= -math.Pi {
		da += 2 * math.Pi
	}
	for da > math.Pi {
		da -= 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(da) / (math.Pi / 6)))
	if n < 1 {
		n = 1
	}
	r := math.Hypot(float64(x0-cx), float64(y0-cy))
	out := make([][2]float32, 0, n)
	for i := 1; i <= n; i++ {
		a := a0 + da*float64(i)/float64(n)
		out = append(out, [2]float32{cx + float32(r*math.Cos(a)), cy + float32(r*math.Sin(a))})
	}
	return out
}

func treatAsHairline(width, sx, sy float32) (float32, bool) {
	if width == 0 {
		return 1, true
	}
	lx := abs32(width * sx)
	ly := abs32(width * sy)
	if lx <= 1 && ly <= 1 {
		return (lx + ly) / 2, true
	}
	return 0, false
}
