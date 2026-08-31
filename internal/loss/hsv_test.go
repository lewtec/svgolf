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

func TestPlaneEnsureRectLeavesRestUnset(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(3, 1, color.NRGBA{B: 255, A: 255})
	p := NewPlane(img)
	p.EnsureRect(image.Rect(0, 0, 1, 1))
	if p.At(0, 0).H > 10 {
		t.Fatalf("red hsv=%+v", p.At(0, 0))
	}
	if p.pix[3+1*4].A != 0 {
		t.Fatal("EnsureRect converted the whole plane")
	}
}

func TestPlaneResetReusesBuffer(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	p := NewPlane(img)
	p.Ensure()
	buf := p.pix
	p.Reset(img)
	p.EnsureRect(image.Rect(0, 0, 2, 2))
	if cap(p.pix) != cap(buf) {
		t.Fatalf("EnsureRect cap=%d want %d", cap(p.pix), cap(buf))
	}
	if p.pix[3+1*8].A != 0 {
		t.Fatal("reused EnsureRect did not clear the rest")
	}
	p.Reset(img)
	p.Ensure()
	if cap(p.pix) != cap(buf) {
		t.Fatalf("Ensure cap=%d want %d", cap(p.pix), cap(buf))
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
