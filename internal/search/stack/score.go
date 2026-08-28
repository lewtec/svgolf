package stack

import (
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/internal/loss"
)

// pathCost is the error-sum one extra path must buy (minIsland pixels at 180°).
const pathCost = 180 * minIsland

// Score is the sum of per-pixel error plus pathCost·parts. Opaque pixels use
// HueAt. A hole (want.A==0) is HueAt(got, black). Mean would hide letters on
// a large canvas; sum does not.
func Score(got, want *image.NRGBA, parts int) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var sum float64
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			sum += errAt(got.NRGBAAt(x, y), want.NRGBAAt(x, y))
		}
	}
	if parts < 0 {
		parts = 0
	}
	return sum + pathCost*float64(parts)
}

func errAt(g, q color.NRGBA) float64 {
	if q.A == 0 {
		if g.A == 0 {
			return 0
		}
		return loss.HueAt(g, color.NRGBA{A: 255})
	}
	return loss.HueAt(g, q)
}
