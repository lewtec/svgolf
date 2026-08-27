package svg

import (
	"errors"
	"fmt"
	"image/color"
	"slices"
)

const (
	minPolygonPoints = 3
	maxPolygonPoints = 1024
)

// ErrPolygonPoints is returned when a polygon does not have 3–1024 vertices.
var ErrPolygonPoints = errors.New("polygon: need 3 to 1024 points")

type Polygon struct {
	points [][2]float64
	paint  paint
}

func NewPolygon() Polygon { return Polygon{} }

func (p Polygon) WithPoints(pts [][2]float64) (Polygon, error) {
	if n := len(pts); n < minPolygonPoints || n > maxPolygonPoints {
		return p, fmt.Errorf("%w, got %d", ErrPolygonPoints, n)
	}
	p.points = slices.Clone(pts)
	return p, nil
}

func (p Polygon) Points() [][2]float64 {
	return slices.Clone(p.points)
}

func (p Polygon) Node() Node {
	return Node{kind: KindPolygon, polygon: p}
}

func (p Polygon) WithFill(col color.NRGBA) Polygon {
	p.paint = p.paint.withFill(col)
	return p
}

func (p Polygon) WithFillOpacity(a float64) Polygon {
	p.paint = p.paint.withFillOpacity(a)
	return p
}

func (p Polygon) WithFillNone() Polygon {
	p.paint = p.paint.withFillNone()
	return p
}

func (p Polygon) WithFillRule(r FillRule) Polygon {
	p.paint = p.paint.withFillRule(r)
	return p
}

func (p Polygon) WithStroke(s Stroke) Polygon {
	p.paint = p.paint.withStroke(s)
	return p
}

func (p Polygon) WithoutStroke() Polygon {
	p.paint = p.paint.withoutStroke()
	return p
}

func (p Polygon) Fill() (color.NRGBA, bool) { return p.paint.fill() }
func (p Polygon) FillOpacity() float64      { return p.paint.fillOpacity() }
func (p Polygon) FillRule() FillRule        { return p.paint.fillRule }
func (p Polygon) Stroke() (Stroke, bool)    { return p.paint.stroke() }
