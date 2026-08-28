package loss

import (
	"image"
	"math"
)

// RMSE is the root-mean-square RGB error on scored pixels (want.A != 0).
// Range 0..255. Size mismatch or nil → +Inf. No scored pixels → 0.
func RMSE(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var sse float64
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			g := got.NRGBAAt(x, y)
			dr := float64(int(g.R) - int(q.R))
			dg := float64(int(g.G) - int(q.G))
			db := float64(int(g.B) - int(q.B))
			sse += dr*dr + dg*dg + db*db
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sse / (3 * float64(n)))
}
