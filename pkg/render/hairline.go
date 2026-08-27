package render

// tiny-skia 0.12 src/scan/hairline_aa.rs (line segments).

func fdot6FromF32(n float32) int32 { return int32(n * 64) }
func fdot6Floor(n int32) int32     { return n >> 6 }
func fdot6Ceil(n int32) int32      { return (n + 63) >> 6 }
func fdot6FromI32(n int32) int32   { return n << 6 }

func fdot16FastDiv(a, b int32) int32 {
	return leftShift(a, 16) / b
}

func fdot6SmallScale(value uint8, dot6 int32) uint8 {
	return uint8((int32(value) * dot6) >> 6)
}

func contribution64(ordinate int32) int32 {
	return ((ordinate - 1) & 63) + 1
}

func i32ToAlpha(a int32) uint8 { return uint8(a & 0xFF) }

func anyBadInts(a, b, c, d int32) bool {
	bad := func(x int32) int32 { return x & -x }
	return (bad(a)|bad(b)|bad(c)|bad(d))>>31 != 0
}

func strokeHairline(pm *pixmap, src path, colPremul [4]uint8) {
	bl := &solidBlitter{pm: pm, pr: colPremul[0], pg: colPremul[1], pb: colPremul[2], pa: colPremul[3]}
	pts := flattenToPoly(src)
	if len(pts) < 2 {
		return
	}
	for i := 0; i < len(pts)-1; i++ {
		x0 := fdot6FromF32(pts[i][0])
		y0 := fdot6FromF32(pts[i][1])
		x1 := fdot6FromF32(pts[i+1][0])
		y1 := fdot6FromF32(pts[i+1][1])
		doAntiHairline(x0, y0, x1, y1, uint32(pm.w), uint32(pm.h), bl)
	}
}

func doAntiHairline(x0, y0, x1, y1 int32, clipW, clipH uint32, bl *solidBlitter) {
	if anyBadInts(x0, y0, x1, y1) {
		return
	}
	lim := fdot6FromI32(511)
	if absI32(x1-x0) > lim || absI32(y1-y0) > lim {
		hx := (x0 >> 1) + (x1 >> 1)
		hy := (y0 >> 1) + (y1 >> 1)
		doAntiHairline(x0, y0, hx, hy, clipW, clipH, bl)
		doAntiHairline(hx, hy, x1, y1, clipW, clipH, bl)
		return
	}

	var (
		scaleStart, scaleStop int32
		istart, istop         int32
		fstart, slope         int32
		horiz                 bool
		hline                 bool
	)
	if absI32(x1-x0) > absI32(y1-y0) {
		horiz = true
		if x0 > x1 {
			x0, x1 = x1, x0
			y0, y1 = y1, y0
		}
		istart = fdot6Floor(x0)
		istop = fdot6Ceil(x1)
		fstart = fdot6ToFdot16(y0)
		if y0 == y1 {
			slope = 0
			hline = true
		} else {
			slope = fdot16FastDiv(y1-y0, x1-x0)
			fstart += (slope*(32-(x0&63)) + 32) >> 6
		}
		if istop <= istart {
			return
		}
		if istop-istart == 1 {
			scaleStart = x1 - x0
			scaleStop = 0
		} else {
			scaleStart = 64 - (x0 & 63)
			scaleStop = x1 & 63
		}
	} else {
		if y0 > y1 {
			x0, x1 = x1, x0
			y0, y1 = y1, y0
		}
		istart = fdot6Floor(y0)
		istop = fdot6Ceil(y1)
		fstart = fdot6ToFdot16(x0)
		if x0 == x1 {
			if y0 == y1 {
				return
			}
			slope = 0
			hline = true // reuse as vline
		} else {
			slope = fdot16FastDiv(x1-x0, y1-y0)
			fstart += (slope*(32-(y0&63)) + 32) >> 6
		}
		if istop <= istart {
			return
		}
		if istop-istart == 1 {
			scaleStart = y1 - y0
			scaleStop = 0
		} else {
			scaleStart = 64 - (y0 & 63)
			scaleStop = y1 & 63
		}
	}

	if istart < 0 {
		return
	}
	start := uint32(istart)
	stop := uint32(istop)
	if horiz {
		fstart = hairDraw(bl, true, hline && y0 == y1, start, fstart, slope, scaleStart)
		start++
		full := int32(stop) - int32(start)
		if scaleStop > 0 {
			full--
		}
		if full > 0 {
			fstart = hairDrawLine(bl, true, hline && y0 == y1, start, start+uint32(full), fstart, slope)
		}
		if scaleStop > 0 {
			hairDraw(bl, true, hline && y0 == y1, stop-1, fstart, slope, scaleStop)
		}
		_ = clipW
		_ = clipH
		return
	}
	// vertical-ish
	vline := x0 == x1
	fstart = hairDraw(bl, false, vline, start, fstart, slope, scaleStart)
	start++
	full := int32(stop) - int32(start)
	if scaleStop > 0 {
		full--
	}
	if full > 0 {
		fstart = hairDrawLine(bl, false, vline, start, start+uint32(full), fstart, slope)
	}
	if scaleStop > 0 {
		hairDraw(bl, false, vline, stop-1, fstart, slope, scaleStop)
	}
}

func hairDraw(bl *solidBlitter, horiz, exact bool, pos uint32, f, slope, mod64 int32) int32 {
	f += 1 << 15
	if f < 0 {
		f = 0
	}
	if horiz {
		if exact {
			y := uint32(f >> 16)
			a := i32ToAlpha(f >> 8)
			if ma := fdot6SmallScale(a, mod64); ma != 0 {
				bl.callHLine(pos, y, 1, ma)
			}
			if ma := fdot6SmallScale(255-a, mod64); ma != 0 && y > 0 {
				bl.callHLine(pos, y-1, 1, ma)
			}
			return f - (1 << 15)
		}
		lowerY := uint32(f >> 16)
		a := i32ToAlpha(f >> 8)
		a0 := fdot6SmallScale(255-a, mod64)
		a1 := fdot6SmallScale(a, mod64)
		ly := lowerY
		if ly > 0 {
			ly--
		}
		bl.blitAntiV2(pos, ly, a0, a1)
		return f + slope - (1 << 15)
	}
	if exact {
		x := uint32(f >> 16)
		a := i32ToAlpha(f >> 8)
		if ma := fdot6SmallScale(a, mod64); ma != 0 {
			bl.blitV(x, pos, 1, ma)
		}
		if ma := fdot6SmallScale(255-a, mod64); ma != 0 && x > 0 {
			bl.blitV(x-1, pos, 1, ma)
		}
		return f - (1 << 15)
	}
	x := uint32(f >> 16)
	a := i32ToAlpha(f >> 8)
	xx := x
	if xx > 0 {
		xx--
	}
	bl.blitAntiH2(xx, pos, fdot6SmallScale(255-a, mod64), fdot6SmallScale(a, mod64))
	return f + slope - (1 << 15)
}

func hairDrawLine(bl *solidBlitter, horiz, exact bool, pos, stop uint32, f, slope int32) int32 {
	if pos >= stop {
		return f
	}
	f += 1 << 15
	if horiz {
		if exact {
			if f < 0 {
				f = 0
			}
			y := uint32(f >> 16)
			a := i32ToAlpha(f >> 8)
			if a != 0 {
				bl.callHLine(pos, y, stop-pos, a)
			}
			if a = 255 - a; a != 0 && y > 0 {
				bl.callHLine(pos, y-1, stop-pos, a)
			}
			return f - (1 << 15)
		}
		for pos < stop {
			if f < 0 {
				f = 0
			}
			lowerY := uint32(f >> 16)
			a := i32ToAlpha(f >> 8)
			ly := lowerY
			if ly > 0 {
				ly--
			}
			bl.blitAntiV2(pos, ly, 255-a, a)
			f += slope
			pos++
		}
		return f - (1 << 15)
	}
	if exact {
		if f < 0 {
			f = 0
		}
		x := uint32(f >> 16)
		a := i32ToAlpha(f >> 8)
		h := stop - pos
		if a != 0 {
			bl.blitV(x, pos, h, a)
		}
		if a = 255 - a; a != 0 && x > 0 {
			bl.blitV(x-1, pos, h, a)
		}
		return f - (1 << 15)
	}
	for pos < stop {
		if f < 0 {
			f = 0
		}
		x := uint32(f >> 16)
		a := i32ToAlpha(f >> 8)
		xx := x
		if xx > 0 {
			xx--
		}
		bl.blitAntiH2(xx, pos, 255-a, a)
		f += slope
		pos++
	}
	return f - (1 << 15)
}

func absI32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (b *solidBlitter) callHLine(x, y, count uint32, alpha uint8) {
	if count == 0 {
		return
	}
	sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, alpha)
	for i := uint32(0); i < count; i++ {
		b.pm.blend(int(x+i), int(y), sr, sg, sb, sa)
	}
}

func (b *solidBlitter) blitV(x, y, height uint32, alpha uint8) {
	sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, alpha)
	for i := uint32(0); i < height; i++ {
		b.pm.blend(int(x), int(y+i), sr, sg, sb, sa)
	}
}

func (b *solidBlitter) blitAntiH2(x, y uint32, a0, a1 uint8) {
	if a0 != 0 {
		sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, a0)
		b.pm.blend(int(x), int(y), sr, sg, sb, sa)
	}
	if a1 != 0 {
		sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, a1)
		b.pm.blend(int(x)+1, int(y), sr, sg, sb, sa)
	}
}

func (b *solidBlitter) blitAntiV2(x, y uint32, a0, a1 uint8) {
	if a0 != 0 {
		sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, a0)
		b.pm.blend(int(x), int(y), sr, sg, sb, sa)
	}
	if a1 != 0 {
		sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, a1)
		b.pm.blend(int(x), int(y)+1, sr, sg, sb, sa)
	}
}
