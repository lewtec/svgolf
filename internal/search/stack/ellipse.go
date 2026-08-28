package stack

import (
	"image/color"

	"github.com/lewtec/svgolf/pkg/svg"
)

// kappa is the cubic handle length for a quarter-circle (4-spline ellipse).
const kappa = 0.5522847498307934

func fitEllipse(island []pix) (cx, cy, rx, ry float64, ok bool) {
	if len(island) < 8 {
		return 0, 0, 0, 0, false
	}
	bb := bbox(island)
	cx = (bb[0][0] + bb[1][0]) / 2
	cy = (bb[0][1] + bb[2][1]) / 2
	rx = (bb[1][0] - bb[0][0]) / 2
	ry = (bb[2][1] - bb[0][1]) / 2
	if rx < 1 || ry < 1 {
		return 0, 0, 0, 0, false
	}
	return cx, cy, rx, ry, true
}

func filledEllipse(cx, cy, rx, ry float64, col color.NRGBA) svg.Path {
	kx, ky := kappa*rx, kappa*ry
	p := svg.NewPath().MoveTo(cx+rx, cy)
	p = p.CubicTo(cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry)
	p = p.CubicTo(cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy)
	p = p.CubicTo(cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry)
	p = p.CubicTo(cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy)
	p = p.Close().WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
	if col.A != 255 {
		p = p.WithFillOpacity(float64(col.A) / 255)
	}
	return p
}

func pathLen(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	return len(p.Commands())
}
