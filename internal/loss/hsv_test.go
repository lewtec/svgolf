package loss

import (
	"image"
	"image/color"
	"testing"
)

func TestColorAtHSVMatchesColorAt(t *testing.T) {
	pairs := [][2]color.NRGBA{
		{{R: 255, A: 255}, {R: 255, A: 255}},
		{{R: 80, A: 255}, {R: 255, A: 255}},
		{{R: 255, A: 255}, {B: 255, A: 255}},
		{{A: 255}, {R: 255, G: 255, B: 255, A: 255}},
	}
	for _, p := range pairs {
		a, b := ColorAt(p[0], p[1]), ColorAtHSV(HSVOf(p[0]), HSVOf(p[1]))
		if a != b {
			t.Fatalf("ColorAt=%v ColorAtHSV=%v for %+v %+v", a, b, p[0], p[1])
		}
	}
}

func TestPlaneConvertsOnce(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	p := NewPlane(img)
	p.Ensure()
	if p.At(0, 0).H > 10 {
		t.Fatalf("red hsv=%+v", p.At(0, 0))
	}
	img.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	if p.At(0, 0).H > 10 {
		t.Fatal("plane converted again without Reset")
	}
	p.Reset(img)
	if p.At(0, 0).H < 200 {
		t.Fatalf("after Reset hsv=%+v want blue", p.At(0, 0))
	}
}
