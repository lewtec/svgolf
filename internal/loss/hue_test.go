package loss

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestHueAtWrap(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	almostRed := color.NRGBA{R: 255, B: 20, A: 255}
	d := HueAt(red, almostRed)
	if d > 10 {
		t.Fatalf("wrap hue=%v", d)
	}
	green := color.NRGBA{G: 255, A: 255}
	if g := HueAt(red, green); math.Abs(g-120) > 1 {
		t.Fatalf("red-green=%v want 120", g)
	}
}

func TestHueAtGray(t *testing.T) {
	black := color.NRGBA{A: 255}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if d := HueAt(black, white); math.Abs(d-180) > 1 {
		t.Fatalf("black-white=%v want 180", d)
	}
	if d := HueAt(black, black); d != 0 {
		t.Fatalf("black-black=%v", d)
	}
}

func TestHueMean(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	got := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	want.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	got.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	want.SetNRGBA(1, 0, color.NRGBA{}) // don't-care
	got.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	if d := Hue(got, want); d != 0 {
		t.Fatalf("Hue=%v", d)
	}
}
