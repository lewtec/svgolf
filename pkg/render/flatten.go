package render

import (
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

type pathSegKind uint8

const (
	segMove pathSegKind = iota
	segLine
	segCubic
	segClose
)

type pathSeg struct {
	kind           pathSegKind
	x, y           float32
	x1, y1, x2, y2 float32
}

type path struct {
	segs                   []pathSeg
	minX, minY, maxX, maxY float32
	empty                  bool
}

func (p *path) note(x, y float32) {
	if p.empty {
		p.minX, p.maxX, p.minY, p.maxY = x, x, y, y
		p.empty = false
		return
	}
	if x < p.minX {
		p.minX = x
	}
	if x > p.maxX {
		p.maxX = x
	}
	if y < p.minY {
		p.minY = y
	}
	if y > p.maxY {
		p.maxY = y
	}
}

func (p *path) moveTo(x, y float32) {
	p.note(x, y)
	p.segs = append(p.segs, pathSeg{kind: segMove, x: x, y: y})
}

func (p *path) lineTo(x, y float32) {
	p.note(x, y)
	p.segs = append(p.segs, pathSeg{kind: segLine, x: x, y: y})
}

func (p *path) cubicTo(x1, y1, x2, y2, x, y float32) {
	p.note(x1, y1)
	p.note(x2, y2)
	p.note(x, y)
	p.segs = append(p.segs, pathSeg{kind: segCubic, x: x, y: y, x1: x1, y1: y1, x2: x2, y2: y2})
}

func (p *path) close() {
	p.segs = append(p.segs, pathSeg{kind: segClose})
}

func (p *path) lastPoint() (float32, float32, bool) {
	for i := len(p.segs) - 1; i >= 0; i-- {
		s := p.segs[i]
		if s.kind != segClose {
			return s.x, s.y, true
		}
	}
	return 0, 0, false
}

func (p *path) transform(sx, sy, tx, ty float32) {
	for i := range p.segs {
		s := &p.segs[i]
		if s.kind == segClose {
			continue
		}
		s.x = s.x*sx + tx
		s.y = s.y*sy + ty
		if s.kind == segCubic {
			s.x1 = s.x1*sx + tx
			s.y1 = s.y1*sy + ty
			s.x2 = s.x2*sx + tx
			s.y2 = s.y2*sy + ty
		}
	}
	if !p.empty {
		// recompute bounds
		p.empty = true
		for _, s := range p.segs {
			if s.kind == segClose {
				continue
			}
			p.note(s.x, s.y)
			if s.kind == segCubic {
				p.note(s.x1, s.y1)
				p.note(s.x2, s.y2)
			}
		}
	}
}

func flattenNode(n svg.Node) (path, bool) {
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		return flattenRect(r)
	case svg.KindCircle:
		c, _ := n.Circle()
		return flattenEllipse(float32(c.CX()), float32(c.CY()), float32(c.R()), float32(c.R()))
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		return flattenEllipse(float32(e.CX()), float32(e.CY()), float32(e.RX()), float32(e.RY()))
	case svg.KindPolygon:
		p, _ := n.Polygon()
		return flattenPolygon(p)
	case svg.KindPath:
		p, _ := n.Path()
		return flattenSVGPath(p)
	default:
		return path{}, false
	}
}

func flattenRect(r svg.Rect) (path, bool) {
	w := float32(r.Width())
	h := float32(r.Height())
	if w <= 0 || h <= 0 {
		return path{}, false
	}
	rx, ry := r.RX(), r.RY()
	hw, hh := r.Width()/2, r.Height()/2
	if hw >= 0 && rx > hw {
		rx = hw
	}
	if hh >= 0 && ry > hh {
		ry = hh
	}
	x, y := float32(r.X()), float32(r.Y())
	if rx == 0 && ry == 0 {
		var p path
		p.empty = true
		p.moveTo(x, y)
		p.lineTo(x+w, y)
		p.lineTo(x+w, y+h)
		p.lineTo(x, y+h)
		p.close()
		return p, true
	}
	frx, fry := float32(rx), float32(ry)
	var p path
	p.empty = true
	p.moveTo(x+frx, y)
	p.lineTo(x+w-frx, y)
	p.arcTo(frx, fry, 0, false, true, x+w, y+fry)
	p.lineTo(x+w, y+h-fry)
	p.arcTo(frx, fry, 0, false, true, x+w-frx, y+h)
	p.lineTo(x+frx, y+h)
	p.arcTo(frx, fry, 0, false, true, x, y+h-fry)
	p.lineTo(x, y+fry)
	p.arcTo(frx, fry, 0, false, true, x+frx, y)
	p.close()
	return p, true
}

func flattenEllipse(cx, cy, rx, ry float32) (path, bool) {
	if rx <= 0 || ry <= 0 {
		return path{}, false
	}
	var p path
	p.empty = true
	p.moveTo(cx+rx, cy)
	p.arcTo(rx, ry, 0, false, true, cx, cy+ry)
	p.arcTo(rx, ry, 0, false, true, cx-rx, cy)
	p.arcTo(rx, ry, 0, false, true, cx, cy-ry)
	p.arcTo(rx, ry, 0, false, true, cx+rx, cy)
	p.close()
	return p, true
}

func flattenPolygon(poly svg.Polygon) (path, bool) {
	pts := poly.Points()
	if len(pts) < 2 {
		return path{}, false
	}
	var p path
	p.empty = true
	p.moveTo(float32(pts[0][0]), float32(pts[0][1]))
	for _, pt := range pts[1:] {
		p.lineTo(float32(pt[0]), float32(pt[1]))
	}
	p.close()
	return p, true
}

func flattenSVGPath(sp svg.Path) (path, bool) {
	cmds := sp.Commands()
	if len(cmds) == 0 {
		return path{}, false
	}
	var p path
	p.empty = true
	for _, c := range cmds {
		switch c.Kind {
		case svg.CmdMove:
			p.moveTo(float32(c.X), float32(c.Y))
		case svg.CmdLine:
			p.lineTo(float32(c.X), float32(c.Y))
		case svg.CmdCubic:
			p.cubicTo(float32(c.X1), float32(c.Y1), float32(c.X2), float32(c.Y2), float32(c.X), float32(c.Y))
		case svg.CmdClose:
			p.close()
		}
	}
	if p.empty && len(p.segs) == 0 {
		return path{}, false
	}
	return p, true
}

// usvg PathBuilderExt::arc_to → kurbo SvgArc → Arc → cubics (tolerance 0.1)
func (p *path) arcTo(rx, ry, xRotDeg float32, large, sweep bool, x, y float32) {
	px, py, ok := p.lastPoint()
	if !ok {
		return
	}
	if arc, ok := svgArcToArc(float64(px), float64(py), float64(x), float64(y), float64(rx), float64(ry), float64(xRotDeg)*math.Pi/180, large, sweep); ok {
		arc.toCubics(0.1, func(x1, y1, x2, y2, x3, y3 float64) {
			p.cubicTo(float32(x1), float32(y1), float32(x2), float32(y2), float32(x3), float32(y3))
		})
		return
	}
	p.lineTo(x, y)
}

type ellArc struct {
	cx, cy, rx, ry float64
	start, sweep   float64
	xrot           float64
}

func svgArcToArc(x0, y0, x1, y1, rx, ry, xrot float64, large, sweep bool) (ellArc, bool) {
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx <= 1e-5 || ry <= 1e-5 || (x0 == x1 && y0 == y1) {
		return ellArc{}, false
	}
	sinPhi, cosPhi := math.Sincos(math.Mod(xrot, 2*math.Pi))
	hdx := (x0 - x1) * 0.5
	hdy := (y0 - y1) * 0.5
	px := cosPhi*hdx + sinPhi*hdy
	py := -sinPhi*hdx + cosPhi*hdy
	rf := px*px/(rx*rx) + py*py/(ry*ry)
	if rf > 1 {
		s := math.Sqrt(rf)
		rx *= s
		ry *= s
	}
	rxpy := rx * py
	rypx := ry * px
	sum := rxpy*rxpy + rypx*rypx
	if sum == 0 {
		return ellArc{}, false
	}
	sign := 1.0
	if large == sweep {
		sign = -1
	}
	rxry := rx * ry
	coe := sign * math.Sqrt(math.Abs((rxry*rxry-sum)/sum))
	tcx := coe * rxpy / ry
	tcy := -coe * rypx / rx
	cx := cosPhi*tcx - sinPhi*tcy + (x0+x1)*0.5
	cy := sinPhi*tcx + cosPhi*tcy + (y0+y1)*0.5
	startV := [2]float64{(px - tcx) / rx, (py - tcy) / ry}
	endV := [2]float64{(-px - tcx) / rx, (-py - tcy) / ry}
	start := math.Atan2(startV[1], startV[0])
	sweepA := math.Mod(math.Atan2(endV[1], endV[0])-start, 2*math.Pi)
	if sweep && sweepA < 0 {
		sweepA += 2 * math.Pi
	} else if !sweep && sweepA > 0 {
		sweepA -= 2 * math.Pi
	}
	return ellArc{cx, cy, rx, ry, start, sweepA, xrot}, true
}

func (a ellArc) toCubics(tol float64, emit func(x1, y1, x2, y2, x3, y3 float64)) {
	sign := 1.0
	if a.sweep < 0 {
		sign = -1
	}
	scaled := math.Max(a.rx, a.ry) / tol
	nErr := math.Max(math.Pow(1.1163*scaled, 1.0/6.0), 3.999999)
	n := math.Ceil(nErr * math.Abs(a.sweep) / (2 * math.Pi))
	step := a.sweep / n
	ni := int(n)
	arm := (4.0 / 3.0) * math.Tan(math.Abs(0.25*step)) * sign
	ang0 := a.start
	p0x, p0y := sampleEllipse(a.rx, a.ry, a.xrot, ang0)
	for i := 0; i < ni; i++ {
		ang1 := ang0 + step
		q1x, q1y := sampleEllipse(a.rx, a.ry, a.xrot, ang0+math.Pi/2)
		p1x, p1y := p0x+arm*q1x, p0y+arm*q1y
		p3x, p3y := sampleEllipse(a.rx, a.ry, a.xrot, ang1)
		q2x, q2y := sampleEllipse(a.rx, a.ry, a.xrot, ang1+math.Pi/2)
		p2x, p2y := p3x-arm*q2x, p3y-arm*q2y
		emit(a.cx+p1x, a.cy+p1y, a.cx+p2x, a.cy+p2y, a.cx+p3x, a.cy+p3y)
		ang0, p0x, p0y = ang1, p3x, p3y
	}
}

func sampleEllipse(rx, ry, rot, ang float64) (float64, float64) {
	s, c := math.Sincos(ang)
	u, v := rx*c, ry*s
	rs, rc := math.Sincos(rot)
	return u*rc - v*rs, u*rs + v*rc
}

func viewBoxTransform(d interface {
	Width() float64
	Height() float64
	ViewBox() svg.ViewBox
}) (sx, sy, tx, ty float32, ok bool) {
	vb := d.ViewBox()
	if !vb.Set() {
		return 1, 1, 0, 0, true
	}
	cw, ch := float32(d.Width()), float32(d.Height())
	vw, vh := float32(vb.Width()), float32(vb.Height())
	if vw == 0 || vh == 0 {
		return 0, 0, 0, 0, false
	}
	s := cw / vw
	if ch/vh < s {
		s = ch / vh
	}
	sx, sy = s, s
	tx = (cw-vw*s)/2 - float32(vb.MinX())*s
	ty = (ch-vh*s)/2 - float32(vb.MinY())*s
	return sx, sy, tx, ty, true
}
