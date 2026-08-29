package svg

import "image/color"

type Ellipse struct {
	cx, cy, rx, ry float64
	paint          paint
}

func NewEllipse() Ellipse { return Ellipse{} }

func (e Ellipse) WithCX(v float64) Ellipse { e.cx = v; return e }
func (e Ellipse) WithCY(v float64) Ellipse { e.cy = v; return e }
func (e Ellipse) WithRX(v float64) Ellipse { e.rx = v; return e }
func (e Ellipse) WithRY(v float64) Ellipse { e.ry = v; return e }

func (e Ellipse) CX() float64 { return e.cx }
func (e Ellipse) CY() float64 { return e.cy }
func (e Ellipse) RX() float64 { return e.rx }
func (e Ellipse) RY() float64 { return e.ry }

func (e Ellipse) Node() Node {
	return Node{kind: KindEllipse, ellipse: e}
}

func (e Ellipse) WithFill(col color.NRGBA) Ellipse {
	e.paint = e.paint.withFill(col)
	return e
}

func (e Ellipse) WithLinearFill(g LinearFill) Ellipse {
	e.paint = e.paint.withLinear(g)
	return e
}

func (e Ellipse) WithFillOpacity(a float64) Ellipse {
	e.paint = e.paint.withFillOpacity(a)
	return e
}

func (e Ellipse) WithFillNone() Ellipse {
	e.paint = e.paint.withFillNone()
	return e
}

func (e Ellipse) WithFillRule(r FillRule) Ellipse {
	e.paint = e.paint.withFillRule(r)
	return e
}

func (e Ellipse) WithStroke(s Stroke) Ellipse {
	e.paint = e.paint.withStroke(s)
	return e
}

func (e Ellipse) WithoutStroke() Ellipse {
	e.paint = e.paint.withoutStroke()
	return e
}

func (e Ellipse) Fill() (color.NRGBA, bool)      { return e.paint.fill() }
func (e Ellipse) LinearFill() (LinearFill, bool) { return e.paint.linearFill() }
func (e Ellipse) FillOpacity() float64           { return e.paint.fillOpacity() }
func (e Ellipse) FillRule() FillRule             { return e.paint.fillRule }
func (e Ellipse) Stroke() (Stroke, bool)         { return e.paint.stroke() }
