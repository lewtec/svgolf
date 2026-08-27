package loss

import (
	"image"
	"math"
)

// Pixels counts scored pixels where got != want. want.A == 0 is don't-care.
type Pixels struct{}

var _ Loss = Pixels{}

func (Pixels) Loss(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			if got.NRGBAAt(x, y) != q {
				n++
			}
		}
	}
	return float64(n)
}
