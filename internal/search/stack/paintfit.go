package stack

import (
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

// paintChoice is one fill a path can wear. linear false is the solid mean.
type paintChoice struct {
	solid  color.NRGBA
	grad   svg.LinearFill
	linear bool
}

// paintChoices is the solid mean and a two-stop gradient on each axis of the
// position-and-color covariance. A zero covariance is the solid only.
// Endpoint slides along the same color line are not choices: they paint the same pixels.
func paintChoices(island []pix, want *image.NRGBA) []paintChoice {
	solid := meanFill(want, island)
	out := []paintChoice{{solid: solid}}
	if want == nil || len(island) == 0 {
		return out
	}
	u, v, ok := colorAxes(island, want)
	if !ok {
		return out
	}
	if grad, ok := gradientAlong(island, want, u); ok {
		out = append(out, paintChoice{solid: solid, grad: grad, linear: true})
	}
	if v != u {
		if grad, ok := gradientAlong(island, want, v); ok {
			out = append(out, paintChoice{solid: solid, grad: grad, linear: true})
		}
	}
	return out
}

func colorAxes(island []pix, want *image.NRGBA) (u, v [2]float64, ok bool) {
	n := float64(len(island))
	var mx, my, mr, mg, mb float64
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		mx += float64(p.x) + 0.5
		my += float64(p.y) + 0.5
		mr += float64(c.R)
		mg += float64(c.G)
		mb += float64(c.B)
	}
	mx /= n
	my /= n
	mr /= n
	mg /= n
	mb /= n
	var c00, c01, c02, c10, c11, c12 float64
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		dx := float64(p.x) + 0.5 - mx
		dy := float64(p.y) + 0.5 - my
		dr := float64(c.R) - mr
		dg := float64(c.G) - mg
		db := float64(c.B) - mb
		c00 += dx * dr
		c01 += dx * dg
		c02 += dx * db
		c10 += dy * dr
		c11 += dy * dg
		c12 += dy * db
	}
	m00 := c00*c00 + c01*c01 + c02*c02
	m01 := c00*c10 + c01*c11 + c02*c12
	m11 := c10*c10 + c11*c11 + c12*c12
	if m00 == 0 && m01 == 0 && m11 == 0 {
		return [2]float64{}, [2]float64{}, false
	}
	tr := m00 + m11
	det := m00*m11 - m01*m01
	disc := tr*tr/4 - det
	if disc < 0 {
		disc = 0
	}
	root := math.Sqrt(disc)
	l1 := tr/2 + root
	l2 := tr/2 - root
	u = unitAxis(eigenVector(m00, m01, m11, l1))
	v = unitAxis(eigenVector(m00, m01, m11, l2))
	return u, v, true
}

func eigenVector(m00, m01, m11, lambda float64) [2]float64 {
	x, y := m01, lambda-m00
	if x == 0 && y == 0 {
		x, y = lambda-m11, m01
	}
	if x == 0 && y == 0 {
		return [2]float64{1, 0}
	}
	return [2]float64{x, y}
}

func unitAxis(v [2]float64) [2]float64 {
	h := math.Hypot(v[0], v[1])
	if h == 0 {
		return [2]float64{1, 0}
	}
	x, y := v[0]/h, v[1]/h
	if x < 0 || (x == 0 && y < 0) {
		x, y = -x, -y
	}
	return [2]float64{x, y}
}

func gradientAlong(island []pix, want *image.NRGBA, axis [2]float64) (svg.LinearFill, bool) {
	n := float64(len(island))
	var mx, my float64
	for _, p := range island {
		mx += float64(p.x) + 0.5
		my += float64(p.y) + 0.5
	}
	mx /= n
	my /= n
	parameter := func(p pix) float64 {
		return (float64(p.x)+0.5-mx)*axis[0] + (float64(p.y)+0.5-my)*axis[1]
	}
	var meanT, meanR, meanG, meanB float64
	tMin, tMax := parameter(island[0]), parameter(island[0])
	for _, p := range island {
		t := parameter(p)
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		meanT += t
		meanR += float64(c.R)
		meanG += float64(c.G)
		meanB += float64(c.B)
		if t < tMin {
			tMin = t
		}
		if t > tMax {
			tMax = t
		}
	}
	meanT /= n
	meanR /= n
	meanG /= n
	meanB /= n
	var varT, covR, covG, covB float64
	for _, p := range island {
		t := parameter(p) - meanT
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		varT += t * t
		covR += t * (float64(c.R) - meanR)
		covG += t * (float64(c.G) - meanG)
		covB += t * (float64(c.B) - meanB)
	}
	if varT == 0 {
		return svg.LinearFill{}, false
	}
	slopeR := covR / varT
	slopeG := covG / varT
	slopeB := covB / varT
	at := func(t float64) color.NRGBA {
		return color.NRGBA{
			R: clampU8(meanR + slopeR*(t-meanT)),
			G: clampU8(meanG + slopeG*(t-meanT)),
			B: clampU8(meanB + slopeB*(t-meanT)),
			A: 255,
		}
	}
	c0, c1 := at(tMin), at(tMax)
	if c0 == c1 {
		return svg.LinearFill{}, false
	}
	return svg.NewLinearFill(mx+axis[0]*tMin, my+axis[1]*tMin, mx+axis[0]*tMax, my+axis[1]*tMax, c0, c1), true
}

func clampU8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}
