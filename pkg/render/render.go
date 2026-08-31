package render

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

func Render(d svg.Document) (*image.NRGBA, error) {
	w, h := d.Width(), d.Height()
	if err := checkCanvas(w, h); err != nil {
		return nil, err
	}
	sx, sy, tx, ty, ok := viewBoxTransform(d)
	if !ok {
		return nil, fmt.Errorf("render: invalid viewBox")
	}
	pm := newPixmap(int(w), int(h))
	defer releasePixmap(pm)
	if err := paintNodes(pm, d.Children(), sx, sy, tx, ty); err != nil {
		return nil, err
	}
	return pm.toNRGBA(), nil
}

func checkCanvas(w, h float64) error {
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 || w > svg.MaxCanvas || h > svg.MaxCanvas || w != math.Trunc(w) || h != math.Trunc(h) {
		return fmt.Errorf("render: canvas %v×%v", w, h)
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func paintNodes(pm *pixmap, nodes []svg.Node, sx, sy, tx, ty float32) error {
	for _, n := range nodes {
		if err := paintNode(pm, n, sx, sy, tx, ty); err != nil {
			return err
		}
	}
	return nil
}

func paintNode(pm *pixmap, n svg.Node, sx, sy, tx, ty float32) error {
	switch n.Kind() {
	case svg.KindGroup:
		g, _ := n.Group()
		return paintNodes(pm, g.Children(), sx, sy, tx, ty)
	case svg.KindInvalid:
		return fmt.Errorf("render: invalid node")
	case svg.KindRect, svg.KindCircle, svg.KindEllipse, svg.KindPolygon, svg.KindPath:
		return paintPrimitive(pm, n, sx, sy, tx, ty)
	default:
		return fmt.Errorf("render: unknown node")
	}
}

func paintPrimitive(pm *pixmap, n svg.Node, sx, sy, tx, ty float32) error {
	p, ok := flattenNode(n)
	if !ok {
		return nil
	}
	p.transform(sx, sy, tx, ty)
	fill, fillOn, fillRule, stroke, strokeOn := paintOf(n)
	if lin, ok := linearOf(n); ok {
		fillPathGrad(pm, p, fillRule != svg.FillEvenOdd, lin, fill.a, sx, sy, tx, ty)
	} else if fillOn {
		fillPath(pm, p, fillRule != svg.FillEvenOdd, fill.col, fill.a)
	}
	if strokeOn && stroke.Width() > 0 {
		col := stroke.Color()
		a := uint8(stroke.Opacity()*255 + 0.5)
		pr := div255(uint32(col.R) * uint32(a))
		pg := div255(uint32(col.G) * uint32(a))
		pb := div255(uint32(col.B) * uint32(a))
		w := float32(stroke.Width())
		if cov, hair := treatAsHairline(w, sx, sy); hair {
			if cov != 1 {
				a = uint8((int32(a) * int32(cov*256)) >> 8)
				pr = premultiplyU8(col.R, a)
				pg = premultiplyU8(col.G, a)
				pb = premultiplyU8(col.B, a)
			}
			strokeHairline(pm, p, [4]uint8{pr, pg, pb, a})
		} else if rp, ok := strokeRectRing(p, w); ok {
			fillPath(pm, rp, true, col, a) // opposite-wound ring, nonzero
		} else {
			sp := strokeToPath(p, stroke)
			if !sp.empty && len(sp.segs) > 0 {
				fillPath(pm, sp, true, col, a)
			}
		}
	}
	return nil
}

type fillCol struct {
	col color.NRGBA
	a   uint8
}

func paintOf(n svg.Node) (fillCol, bool, svg.FillRule, svg.Stroke, bool) {
	switch n.Kind() {
	case svg.KindRect:
		r, _ := n.Rect()
		return takePaint(r.Fill, r.FillOpacity, r.FillRule, r.Stroke)
	case svg.KindCircle:
		c, _ := n.Circle()
		return takePaint(c.Fill, c.FillOpacity, c.FillRule, c.Stroke)
	case svg.KindEllipse:
		e, _ := n.Ellipse()
		return takePaint(e.Fill, e.FillOpacity, e.FillRule, e.Stroke)
	case svg.KindPolygon:
		p, _ := n.Polygon()
		return takePaint(p.Fill, p.FillOpacity, p.FillRule, p.Stroke)
	case svg.KindPath:
		p, _ := n.Path()
		return takePaint(p.Fill, p.FillOpacity, p.FillRule, p.Stroke)
	default:
		return fillCol{}, false, 0, svg.Stroke{}, false
	}
}

func takePaint(
	fillFn func() (color.NRGBA, bool),
	opFn func() float64,
	ruleFn func() svg.FillRule,
	strokeFn func() (svg.Stroke, bool),
) (fillCol, bool, svg.FillRule, svg.Stroke, bool) {
	col, on := fillFn()
	a := uint8(opFn()*255 + 0.5)
	st, son := strokeFn()
	return fillCol{col: col, a: a}, on, ruleFn(), st, son
}

func linearOf(n svg.Node) (svg.LinearFill, bool) {
	return n.LinearFill()
}

func fillPathGrad(pm *pixmap, p path, nonzero bool, g svg.LinearFill, a uint8, sx, sy, tx, ty float32) {
	bl := &gradientBlitter{
		pm: pm,
		x1: float32(g.X1())*sx + tx,
		y1: float32(g.Y1())*sy + ty,
		x2: float32(g.X2())*sx + tx,
		y2: float32(g.Y2())*sy + ty,
		c0: g.C0(),
		c1: g.C1(),
		a:  a,
	}
	dx := bl.x2 - bl.x1
	dy := bl.y2 - bl.y1
	len2 := dx*dx + dy*dy
	bl.dx, bl.dy = dx, dy
	if len2 > 0 {
		bl.inv = 1 / len2
	}
	fillPathAA(p, nonzero, uint32(pm.w), uint32(pm.h), bl)
}

func fillPath(pm *pixmap, p path, nonzero bool, col color.NRGBA, a uint8) {
	if a == 0 && col.A == 0 {
		// still may paint if col has A 255 and a is opacity
	}
	pr := div255(uint32(col.R) * uint32(a))
	pg := div255(uint32(col.G) * uint32(a))
	pb := div255(uint32(col.B) * uint32(a))
	bl := &solidBlitter{pm: pm, pr: pr, pg: pg, pb: pb, pa: a}
	fillPathAA(p, nonzero, uint32(pm.w), uint32(pm.h), bl)
}

func (p *pixmap) toNRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, p.w, p.h))
	for i := 0; i < p.w*p.h; i++ {
		pr, pg, pb, pa := p.pix[i*4], p.pix[i*4+1], p.pix[i*4+2], p.pix[i*4+3]
		off := i * 4
		img.Pix[off] = demultiplyU8(pr, pa)
		img.Pix[off+1] = demultiplyU8(pg, pa)
		img.Pix[off+2] = demultiplyU8(pb, pa)
		img.Pix[off+3] = pa
	}
	return img
}
