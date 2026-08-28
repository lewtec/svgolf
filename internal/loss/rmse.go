package loss

import (
	"image"
	"math"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
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

// Lambda is the default Fit complexity weight: one extra part must cut RMSE by ~2.5.
const Lambda = 0.01

// Eps is the default "fits" RMSE threshold (about 3% of 255).
const Eps = 8

// Fit is RMSE/255 + λ·k. Lower is better. k = primitive count.
func Fit(got, want *image.NRGBA, k int) float64 {
	r := RMSE(got, want)
	if math.IsInf(r, 0) || math.IsNaN(r) {
		return r
	}
	if k < 0 {
		k = 0
	}
	return r/255 + Lambda*float64(k)
}

// EpsFit: if RMSE > Eps, rank by fidelity (1+RMSE/255); else rank by k.
func EpsFit(got, want *image.NRGBA, k int) float64 {
	r := RMSE(got, want)
	if math.IsInf(r, 0) || math.IsNaN(r) {
		return r
	}
	if k < 0 {
		k = 0
	}
	if r > Eps {
		return 1 + r/255
	}
	return float64(k)
}

// OfFit renders and returns Fit(got, want, k). k is Search-owned.
func OfFit(doc svg.Document, want *image.NRGBA, k int) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return math.Inf(1), err
	}
	return Fit(got, want, k), nil
}

// OfEps renders and returns EpsFit(got, want, k). k is Search-owned.
func OfEps(doc svg.Document, want *image.NRGBA, k int) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return math.Inf(1), err
	}
	return EpsFit(got, want, k), nil
}
