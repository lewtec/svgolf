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
// ColorAt (hue, sat, value). A hole (want.A==0) costs 180 if painted —
// black is not a hole. Missing paint on an opaque pixel also costs 180.
// Mean would hide letters on a large canvas; sum does not.
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

// ScoreRect is the errAt sum on r (no path tax). r is clipped to want.
func ScoreRect(got, want *image.NRGBA, r image.Rectangle) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	r = r.Intersect(want.Rect)
	if r.Empty() {
		return 0
	}
	var sum float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sum += errAt(got.NRGBAAt(x, y), want.NRGBAAt(x, y))
		}
	}
	return sum
}

func errAt(g, q color.NRGBA) float64 {
	if q.A == 0 {
		if g.A == 0 {
			return 0
		}
		return 180
	}
	if g.A == 0 {
		return 180
	}
	return loss.ColorAt(g, q)
}
