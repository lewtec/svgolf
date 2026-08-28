package svg

import "image/color"

type Rect struct {
	x, y, width, height float64
	rx, ry              float64
	rxSet, rySet        bool
	paint               paint
}

func NewRect() Rect { return Rect{} }

func (r Rect) WithX(v float64) Rect      { r.x = v; return r }
func (r Rect) WithY(v float64) Rect      { r.y = v; return r }
func (r Rect) WithWidth(v float64) Rect  { r.width = v; return r }
func (r Rect) WithHeight(v float64) Rect { r.height = v; return r }

func (r Rect) WithRX(v float64) Rect {
	r.rx = v
	r.rxSet = true
	return r
}

func (r Rect) WithRY(v float64) Rect {
	r.ry = v
	r.rySet = true
	return r
}

func (r Rect) X() float64      { return r.x }
func (r Rect) Y() float64      { return r.y }
func (r Rect) Width() float64  { return r.width }
func (r Rect) Height() float64 { return r.height }
func (r Rect) RX() float64     { return r.rx }
func (r Rect) RY() float64     { return r.ry }

func (r Rect) radiiSet() (rxSet, rySet bool) { return r.rxSet, r.rySet }

// ClampedRadii is paint-time rx/ry, each capped at half the stored width/height.
func (r Rect) ClampedRadii() (rx, ry float64) {
	rx, ry = r.rx, r.ry
	hw := r.width / 2
	hh := r.height / 2
	if hw >= 0 && rx > hw {
		rx = hw
	}
	if hh >= 0 && ry > hh {
		ry = hh
	}
	return rx, ry
}

func (r Rect) Node() Node {
	return Node{kind: KindRect, rect: r}
}

func (r Rect) WithFill(col color.NRGBA) Rect {
	r.paint = r.paint.withFill(col)
	return r
}

func (r Rect) WithFillOpacity(a float64) Rect {
	r.paint = r.paint.withFillOpacity(a)
	return r
}

func (r Rect) WithFillNone() Rect {
	r.paint = r.paint.withFillNone()
	return r
}

func (r Rect) WithFillRule(r2 FillRule) Rect {
	r.paint = r.paint.withFillRule(r2)
	return r
}

func (r Rect) WithStroke(s Stroke) Rect {
	r.paint = r.paint.withStroke(s)
	return r
}

func (r Rect) WithoutStroke() Rect {
	r.paint = r.paint.withoutStroke()
	return r
}

func (r Rect) Fill() (color.NRGBA, bool) { return r.paint.fill() }
func (r Rect) FillOpacity() float64      { return r.paint.fillOpacity() }
func (r Rect) FillRule() FillRule        { return r.paint.fillRule }
func (r Rect) Stroke() (Stroke, bool)    { return r.paint.stroke() }
