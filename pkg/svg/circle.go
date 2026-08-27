package svg

import "image/color"

type Circle struct {
	cx, cy, r float64
	paint     paint
}

func NewCircle() Circle { return Circle{} }

func (c Circle) WithCX(v float64) Circle { c.cx = v; return c }
func (c Circle) WithCY(v float64) Circle { c.cy = v; return c }
func (c Circle) WithR(v float64) Circle  { c.r = v; return c }

func (c Circle) CX() float64 { return c.cx }
func (c Circle) CY() float64 { return c.cy }
func (c Circle) R() float64  { return c.r }

func (c Circle) Node() Node {
	return Node{kind: KindCircle, circle: c}
}

func (c Circle) WithFill(col color.NRGBA) Circle {
	c.paint = c.paint.withFill(col)
	return c
}

func (c Circle) WithFillOpacity(a float64) Circle {
	c.paint = c.paint.withFillOpacity(a)
	return c
}

func (c Circle) WithFillNone() Circle {
	c.paint = c.paint.withFillNone()
	return c
}

func (c Circle) WithFillRule(r FillRule) Circle {
	c.paint = c.paint.withFillRule(r)
	return c
}

func (c Circle) WithStroke(s Stroke) Circle {
	c.paint = c.paint.withStroke(s)
	return c
}

func (c Circle) WithoutStroke() Circle {
	c.paint = c.paint.withoutStroke()
	return c
}

func (c Circle) Fill() (color.NRGBA, bool) { return c.paint.fill() }
func (c Circle) FillOpacity() float64      { return c.paint.fillOpacity() }
func (c Circle) FillRule() FillRule        { return c.paint.fillRule }
func (c Circle) Stroke() (Stroke, bool)    { return c.paint.stroke() }
