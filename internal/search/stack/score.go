package stack

import (
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/internal/loss"
)

// pathCost is how many mean-error degrees one extra path must buy.
const pathCost = 0.5

// Score is mean pixel error plus pathCost·parts. Opaque pixels use HueAt.
// A hole (want.A==0) is HueAt(got, black): empty or black is 0, a saturated
// fill is 180. An extra path must cut mean error by more than pathCost.
func Score(got, want *image.NRGBA, parts int) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var sum float64
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			sum += errAt(got.NRGBAAt(x, y), want.NRGBAAt(x, y))
			n++
		}
	}
	if n == 0 {
		return pathCost * float64(max(0, parts))
	}
	if parts < 0 {
		parts = 0
	}
	return sum/float64(n) + pathCost*float64(parts)
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
