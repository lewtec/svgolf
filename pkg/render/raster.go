package render

import (
	"math"
)

const (
	supersampleShift = 2
	aaScale          = 1 << supersampleShift
	aaMask           = aaScale - 1
)

type lineEdge struct {
	prev, next       uint32
	hasPrev, hasNext bool
	x, dx            int32
	firstY, lastY    int32
	winding          int8
	cub              *cubicState
}

func leftShift(value, shift int32) int32 {
	return int32(uint32(value) << uint32(shift))
}

func leftShift64(value int64, shift int32) int64 {
	return int64(uint64(value) << uint32(shift))
}

func fdot6Round(n int32) int32 { return (n + 32) >> 6 }

func fdot6ToFdot16(n int32) int32 { return leftShift(n, 10) }

func fdot16Mul(a, b int32) int32 {
	return int32((int64(a) * int64(b)) >> 16)
}

func fdot16RoundToI32(x int32) int32 { return (x + (1 << 15)) >> 16 }

func fdot6Div(a, b int32) int32 {
	if a == int32(int16(a)) {
		return leftShift(a, 16) / b
	}
	v := leftShift64(int64(a), 16) / int64(b)
	if v < math.MinInt32 {
		return math.MinInt32
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

func computeDY(top, y0 int32) int32 {
	return leftShift(top, 6) + 32 - y0
}

func newLineEdge(x0, y0, x1, y1 float32, shift int32) (lineEdge, bool) {
	scale := float32(int32(1) << uint(shift+6))
	ix0 := int32(x0 * scale)
	iy0 := int32(y0 * scale)
	ix1 := int32(x1 * scale)
	iy1 := int32(y1 * scale)
	winding := int8(1)
	if iy0 > iy1 {
		ix0, ix1 = ix1, ix0
		iy0, iy1 = iy1, iy0
		winding = -1
	}
	top := fdot6Round(iy0)
	bottom := fdot6Round(iy1)
	if top == bottom {
		return lineEdge{}, false
	}
	slope := fdot6Div(ix1-ix0, iy1-iy0)
	dy := computeDY(top, iy0)
	return lineEdge{
		x:       fdot6ToFdot16(ix0 + fdot16Mul(slope, dy)),
		dx:      slope,
		firstY:  top,
		lastY:   bottom - 1,
		winding: winding,
	}, true
}

func buildLineEdges(p path, shift int32) []lineEdge {
	var edges []lineEdge
	var mx, my, cx, cy float32
	have := false
	flushLine := func(x, y float32) {
		if !have {
			return
		}
		if e, ok := newLineEdge(cx, cy, x, y, shift); ok {
			edges = append(edges, e)
		}
		cx, cy = x, y
	}
	flushCubic := func(s pathSeg) {
		if !have {
			return
		}
		for _, cub := range [][4][2]float32{{{cx, cy}, {s.x1, s.y1}, {s.x2, s.y2}, {s.x, s.y}}} {
			le, cs, ok := newCubicEdge(cub[0][0], cub[0][1], cub[1][0], cub[1][1], cub[2][0], cub[2][1], cub[3][0], cub[3][1], shift)
			if !ok {
				pushCubicLines(&edges, cub, shift, 0)
				continue
			}
			csCopy := cs
			le.cub = &csCopy
			edges = append(edges, le)
		}
		cx, cy = s.x, s.y
	}
	for _, s := range p.segs {
		switch s.kind {
		case segMove:
			if have {
				flushLine(mx, my) // tiny-skia fill closes the previous contour
			}
			mx, my, cx, cy = s.x, s.y, s.x, s.y
			have = true
		case segLine:
			flushLine(s.x, s.y)
		case segCubic:
			flushCubic(s)
		case segClose:
			if have {
				flushLine(mx, my)
			}
		}
	}
	if have {
		flushLine(mx, my)
	}
	return edges
}

func pushCubicLines(edges *[]lineEdge, cub [4][2]float32, shift int32, depth int) {
	if depth < 4 {
		a, b := splitCubic(cub, 0.5)
		if _, _, ok := newCubicEdge(a[0][0], a[0][1], a[1][0], a[1][1], a[2][0], a[2][1], a[3][0], a[3][1], shift); !ok {
			pushCubicLines(edges, a, shift, depth+1)
		} else {
			le, cs, _ := newCubicEdge(a[0][0], a[0][1], a[1][0], a[1][1], a[2][0], a[2][1], a[3][0], a[3][1], shift)
			csCopy := cs
			le.cub = &csCopy
			*edges = append(*edges, le)
		}
		if _, _, ok := newCubicEdge(b[0][0], b[0][1], b[1][0], b[1][1], b[2][0], b[2][1], b[3][0], b[3][1], shift); !ok {
			pushCubicLines(edges, b, shift, depth+1)
		} else {
			le, cs, _ := newCubicEdge(b[0][0], b[0][1], b[1][0], b[1][1], b[2][0], b[2][1], b[3][0], b[3][1], shift)
			csCopy := cs
			le.cub = &csCopy
			*edges = append(*edges, le)
		}
		return
	}
	if e, ok := newLineEdge(cub[0][0], cub[0][1], cub[3][0], cub[3][1], shift); ok {
		*edges = append(*edges, e)
	}
}

type alphaRuns struct {
	runs  []uint16 // 0 = end
	alpha []uint8
}

func newAlphaRuns(width uint32) alphaRuns {
	a := alphaRuns{
		runs:  make([]uint16, width+1),
		alpha: make([]uint8, width+1),
	}
	a.reset(width)
	return a
}

func (a *alphaRuns) reset(width uint32) {
	a.runs[0] = uint16(width)
	a.runs[width] = 0
	a.alpha[0] = 0
}

func (a *alphaRuns) empty() bool {
	if a.runs[0] == 0 {
		return true
	}
	return a.alpha[0] == 0 && a.runs[int(a.runs[0])] == 0
}

func catchOverflow(alpha uint16) uint8 {
	return uint8(alpha - (alpha >> 8))
}

func breakRun(runs []uint16, alpha []uint8, x, count int) {
	origX := x
	ri, ai := 0, 0
	for x > 0 {
		n := int(runs[ri])
		if x < n {
			alpha[ai+x] = alpha[ai]
			runs[ri] = uint16(x)
			runs[ri+x] = uint16(n - x)
			break
		}
		ri += n
		ai += n
		x -= n
	}
	ri, ai = origX, origX
	x = count
	for {
		n := int(runs[ri])
		if x < n {
			alpha[ai+x] = alpha[ai]
			runs[ri] = uint16(x)
			runs[ri+x] = uint16(n - x)
			break
		}
		x -= n
		if x == 0 {
			break
		}
		ri += n
		ai += n
	}
}

func (a *alphaRuns) add(x uint32, startAlpha uint8, middleCount int, stopAlpha, maxValue uint8, offsetX int) int {
	xi := int(x)
	runsOff, alphaOff := offsetX, offsetX
	lastAlpha := offsetX
	xi -= offsetX
	if startAlpha != 0 {
		breakRun(a.runs[runsOff:], a.alpha[alphaOff:], xi, 1)
		tmp := uint16(a.alpha[alphaOff+xi]) + uint16(startAlpha)
		a.alpha[alphaOff+xi] = uint8(tmp - (tmp >> 8))
		runsOff += xi + 1
		alphaOff += xi + 1
		xi = 0
	}
	if middleCount != 0 {
		breakRun(a.runs[runsOff:], a.alpha[alphaOff:], xi, middleCount)
		alphaOff += xi
		runsOff += xi
		xi = 0
		for {
			a.alpha[alphaOff] = catchOverflow(uint16(a.alpha[alphaOff]) + uint16(maxValue))
			n := int(a.runs[runsOff])
			alphaOff += n
			runsOff += n
			middleCount -= n
			if middleCount == 0 {
				break
			}
		}
		lastAlpha = alphaOff
	}
	if stopAlpha != 0 {
		breakRun(a.runs[runsOff:], a.alpha[alphaOff:], xi, 1)
		alphaOff += xi
		a.alpha[alphaOff] += stopAlpha
		lastAlpha = alphaOff
	}
	return lastAlpha
}

type pixmap struct {
	w, h int
	pix  []uint8 // premul RGBA
}

func newPixmap(w, h int) *pixmap {
	return &pixmap{w: w, h: h, pix: make([]uint8, w*h*4)}
}

func (p *pixmap) blend(x, y int, sr, sg, sb, sa uint8) {
	if x < 0 || y < 0 || x >= p.w || y >= p.h {
		return
	}
	i := (y*p.w + x) * 4
	r, g, b, a := sourceOver(p.pix[i], p.pix[i+1], p.pix[i+2], p.pix[i+3], sr, sg, sb, sa)
	p.pix[i], p.pix[i+1], p.pix[i+2], p.pix[i+3] = r, g, b, a
}

type solidBlitter struct {
	pm             *pixmap
	pr, pg, pb, pa uint8 // premul source
}

func (b *solidBlitter) blitH(x, y, width uint32) {
	for i := uint32(0); i < width; i++ {
		b.pm.blend(int(x+i), int(y), b.pr, b.pg, b.pb, b.pa)
	}
}

func (b *solidBlitter) blitAntiH(x, y uint32, alpha []uint8, runs []uint16) {
	i := 0
	px := x
	for {
		n := runs[i]
		if n == 0 {
			return
		}
		a := alpha[i]
		if a != 0 {
			sr, sg, sb, sa := scalePremul(b.pr, b.pg, b.pb, b.pa, a)
			for k := uint16(0); k < n; k++ {
				b.pm.blend(int(px+uint32(k)), int(y), sr, sg, sb, sa)
			}
		}
		px += uint32(n)
		i += int(n)
	}
}

type superBlitter struct {
	real      *solidBlitter
	currIY    int32
	width     uint32
	left      uint32
	superLeft uint32
	currY     int32
	top       int32
	runs      alphaRuns
	offsetX   int
}

func newSuperBlitter(boundsL, boundsT, boundsR, boundsB int32, clipW, clipH uint32, real *solidBlitter) *superBlitter {
	// intersect bounds with clip [0,clipW)×[0,clipH)
	l := boundsL
	t := boundsT
	r := boundsR
	b := boundsB
	if l < 0 {
		l = 0
	}
	if t < 0 {
		t = 0
	}
	if r > int32(clipW) {
		r = int32(clipW)
	}
	if b > int32(clipH) {
		b = int32(clipH)
	}
	if l >= r || t >= b {
		return nil
	}
	w := uint32(r - l)
	return &superBlitter{
		real:      real,
		currIY:    t - 1,
		width:     w,
		left:      uint32(l),
		superLeft: uint32(l) << supersampleShift,
		currY:     (t << supersampleShift) - 1,
		top:       t,
		runs:      newAlphaRuns(w),
	}
}

func coverageToPartialAlpha(aa uint32) uint8 {
	aa <<= 8 - 2*supersampleShift
	return uint8(aa)
}

func (s *superBlitter) flush() {
	if s.currIY >= s.top {
		if !s.runs.empty() {
			s.real.blitAntiH(s.left, uint32(s.currIY), s.runs.alpha, s.runs.runs)
			s.runs.reset(s.width)
			s.offsetX = 0
		}
		s.currIY = s.top - 1
	}
}

func (s *superBlitter) blitH(x, y, width uint32) {
	iy := int32(y >> supersampleShift)
	if x < s.superLeft {
		width = x + width
		x = 0
	} else {
		x -= s.superLeft
	}
	if s.currY != int32(y) {
		s.offsetX = 0
		s.currY = int32(y)
	}
	if iy != s.currIY {
		s.flush()
		s.currIY = iy
	}
	start := x
	stop := x + width
	fb := start & aaMask
	fe := stop & aaMask
	n := int32(stop>>supersampleShift) - int32(start>>supersampleShift) - 1
	if n < 0 {
		fb = fe - fb
		n = 0
		fe = 0
	} else if fb == 0 {
		n++
	} else {
		fb = aaScale - fb
	}
	maxValue := uint8((1 << (8 - supersampleShift)) - (((y & aaMask) + 1) >> supersampleShift))
	s.offsetX = s.runs.add(x>>supersampleShift, coverageToPartialAlpha(fb), int(n), coverageToPartialAlpha(fe), maxValue, s.offsetX)
}

func fillPathAA(p path, nonzero bool, clipW, clipH uint32, blit *solidBlitter) {
	if p.empty || len(p.segs) == 0 {
		return
	}
	bl := int32(math.Floor(float64(p.minX)))
	bt := int32(math.Floor(float64(p.minY)))
	br := int32(math.Ceil(float64(p.maxX)))
	bb := int32(math.Ceil(float64(p.maxY)))
	sb := newSuperBlitter(bl, bt, br, bb, clipW, clipH, blit)
	if sb == nil {
		return
	}
	// Always walk the full edge list. blend() drops out-of-canvas pixels.
	// Clipping startY to the clip box dropped winding for shapes that straddle 0
	// (e.g. circle at origin).
	contained := true
	edges := buildLineEdges(p, supersampleShift)
	if len(edges) < 2 {
		sb.flush()
		return
	}
	walkEdges(nonzero, bt, bb, clipW, clipH, contained, edges, sb)
	sb.flush()
}

func walkEdges(nonzero bool, startY, stopY int32, clipW, clipH uint32, contained bool, src []lineEdge, sb *superBlitter) {
	shift := int32(supersampleShift)
	edges := make([]lineEdge, 0, len(src)+2)
	edges = append(edges, src...)
	for i := range edges {
		edges[i].hasPrev = true
		edges[i].prev = uint32(i)
		edges[i].hasNext = true
		edges[i].next = uint32(i + 2)
	}
	// sort by firstY, then x
	for i := 0; i < len(edges); i++ {
		for j := i + 1; j < len(edges); j++ {
			ai, aj := edges[i].firstY, edges[j].firstY
			if aj < ai || (aj == ai && edges[j].x < edges[i].x) {
				edges[i], edges[j] = edges[j], edges[i]
			}
		}
	}
	// re-link after sort
	for i := range edges {
		edges[i].hasPrev = true
		edges[i].prev = uint32(i)
		edges[i].hasNext = true
		edges[i].next = uint32(i + 2)
	}
	head := lineEdge{hasNext: true, next: 1, x: math.MinInt32, firstY: math.MinInt32}
	edges = append([]lineEdge{head}, edges...)
	tail := lineEdge{hasPrev: true, prev: uint32(len(edges) - 1), firstY: math.MaxInt32}
	edges = append(edges, tail)

	startY <<= shift
	stopY <<= shift
	if startY < 0 {
		startY = 0
	}
	bottom := int32(clipH) << shift
	if stopY > bottom {
		stopY = bottom
	}
	if stopY <= startY {
		return
	}
	windingMask := int32(-1)
	if !nonzero {
		windingMask = 1
	}
	rightClip := clipW << uint32(shift)
	currY := uint32(startY)
	stop := uint32(stopY)
	for {
		w := int32(0)
		left := int32(0)
		prevX := edges[0].x
		currIdx := int(edges[0].next)
		for edges[currIdx].firstY <= int32(currY) {
			if edges[currIdx].lastY < int32(currY) {
				nextIdx := int(edges[currIdx].next)
				removeEdge(currIdx, edges)
				currIdx = nextIdx
				continue
			}
			x := fdot16RoundToI32(edges[currIdx].x)
			if w&windingMask == 0 {
				left = x
			}
			w += int32(edges[currIdx].winding)
			if w&windingMask == 0 {
				blitSpan(sb, left, x, currY, int32(rightClip))
			}
			nextIdx := int(edges[currIdx].next)
			if edges[currIdx].lastY == int32(currY) {
				if edges[currIdx].cub != nil && edges[currIdx].cub.count < 0 && updateCubic(&edges[currIdx], edges[currIdx].cub) {
					newX := edges[currIdx].x
					if newX < prevX {
						backwardInsert(currIdx, edges)
					} else {
						prevX = newX
					}
				} else {
					removeEdge(currIdx, edges)
				}
			} else {
				newX := edges[currIdx].x + edges[currIdx].dx
				edges[currIdx].x = newX
				if newX < prevX {
					backwardInsert(currIdx, edges)
				} else {
					prevX = newX
				}
			}
			currIdx = nextIdx
		}
		if w&windingMask != 0 {
			blitSpan(sb, left, int32(rightClip), currY, int32(rightClip))
		}
		currY++
		if currY >= stop {
			break
		}
		insertNewEdges(currIdx, int32(currY), edges)
	}
}

func blitSpan(sb *superBlitter, l, r int32, y uint32, clipR int32) {
	if l < 0 {
		l = 0
	}
	if r > clipR {
		r = clipR
	}
	if r > l {
		sb.blitH(uint32(l), y, uint32(r-l))
	}
}

func removeEdge(curr int, edges []lineEdge) {
	prev := edges[curr].prev
	next := edges[curr].next
	edges[prev].next = next
	edges[next].prev = prev
}

func insertEdgeAfter(curr, after int, edges []lineEdge) {
	edges[curr].prev = uint32(after)
	edges[curr].next = edges[after].next
	edges[curr].hasPrev, edges[curr].hasNext = true, true
	an := int(edges[after].next)
	edges[an].prev = uint32(curr)
	edges[after].next = uint32(curr)
}

func backwardInsert(curr int, edges []lineEdge) {
	x := edges[curr].x
	prev := int(edges[curr].prev)
	for prev != 0 {
		if edges[prev].x > x {
			prev = int(edges[prev].prev)
			continue
		}
		break
	}
	next := int(edges[prev].next)
	if next != curr {
		removeEdge(curr, edges)
		insertEdgeAfter(curr, prev, edges)
	}
}

func backwardInsertStart(prev int, x int32, edges []lineEdge) int {
	for edges[prev].hasPrev {
		prev = int(edges[prev].prev)
		if edges[prev].x <= x {
			break
		}
	}
	return prev
}

func insertNewEdges(newIdx int, currY int32, edges []lineEdge) {
	if edges[newIdx].firstY != currY {
		return
	}
	prev := int(edges[newIdx].prev)
	if edges[prev].x <= edges[newIdx].x {
		return
	}
	start := backwardInsertStart(prev, edges[newIdx].x, edges)
	for {
		next := int(edges[newIdx].next)
		keep := false
		for {
			after := int(edges[start].next)
			if after == newIdx {
				keep = true
				break
			}
			if edges[after].x >= edges[newIdx].x {
				break
			}
			start = after
		}
		if !keep {
			removeEdge(newIdx, edges)
			insertEdgeAfter(newIdx, start, edges)
		}
		start = newIdx
		newIdx = next
		if edges[newIdx].firstY != currY {
			break
		}
	}
}
