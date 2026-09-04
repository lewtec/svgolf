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

func TestAcquireReusesBuffer(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := 0; i < planeWorkers(); i++ {
		p := Acquire(img)
		p.Ensure()
		Release(p)
	}
	allocs := testing.AllocsPerRun(50, func() {
		p := Acquire(img)
		p.Ensure()
		Release(p)
	})
	if allocs != 0 {
		t.Fatalf("Acquire+Ensure allocs=%v want 0 after the pool is warm", allocs)
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

func TestPlaneConvertMatchesNRGBAAt(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(2, 1, color.NRGBA{B: 200, G: 10, A: 255})
	p := NewPlane(img)
	p.Ensure()
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			want := HSVOf(img.NRGBAAt(x, y))
			got := p.At(x, y)
			if got != want {
				t.Fatalf("(%d,%d) %+v want %+v", x, y, got, want)
			}
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
