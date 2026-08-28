package loss

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestRMSEZero(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})
	if g := RMSE(img, img); g != 0 {
		t.Fatalf("RMSE=%v", g)
	}
}

func TestRMSEDontCare(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	got := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	want.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	got.SetNRGBA(0, 0, color.NRGBA{A: 255})
	got.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255}) // don't-care on want
	wantRMSE := 255 / math.Sqrt(3)
	if g := RMSE(got, want); math.Abs(g-wantRMSE) > 1e-6 {
		t.Fatalf("RMSE=%v want %v", g, wantRMSE)
	}
}
