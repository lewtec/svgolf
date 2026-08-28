package loss

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestPixelsDontCare(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	got := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	want.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	want.SetNRGBA(1, 0, color.NRGBA{}) // don't-care
	got.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	got.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	if n := Pixels(got, want); n != 1 {
		t.Fatalf("Pixels=%v want 1", n)
	}
}

func TestPixelsMatch(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})
	if n := Pixels(img, img); n != 0 {
		t.Fatalf("Pixels=%v want 0", n)
	}
}

func TestPixelsSizeMismatch(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	b := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if n := Pixels(a, b); !math.IsInf(n, 1) {
		t.Fatalf("Pixels=%v want +Inf", n)
	}
}
