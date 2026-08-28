package loss

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
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
	// one scored pixel, ΔR=255 → RMSE = 255/sqrt(3)
	wantRMSE := 255 / math.Sqrt(3)
	if g := RMSE(got, want); math.Abs(g-wantRMSE) > 1e-6 {
		t.Fatalf("RMSE=%v want %v", g, wantRMSE)
	}
}

func TestFitPaysForParts(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})
	a := Fit(img, img, 1)
	b := Fit(img, img, 2)
	if !(a < b) {
		t.Fatalf("Fit k=1 %v k=2 %v", a, b)
	}
}

func TestEpsFitPrefersFewerWhenUnder(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})
	if g := EpsFit(img, img, 3); g != 3 {
		t.Fatalf("EpsFit=%v want 3", g)
	}
	bad := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	bad.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if g := EpsFit(bad, img, 1); g <= 1 {
		t.Fatalf("EpsFit mismatch should be >1, got %v", g)
	}
}

func TestOfFitEmpty(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	s, err := OfFit(svg.NewDocument(4, 4), want, 0)
	if err != nil || s != 0 {
		t.Fatalf("OfFit=%v err=%v", s, err)
	}
}
