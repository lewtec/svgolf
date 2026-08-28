package palette

import (
	"image"
	"image/color"
	"testing"
)

func TestAutoUniqueUnderCap(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(2, 0, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(3, 0, color.NRGBA{A: 0})
	_, pal, err := Auto(img, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pal) != 2 {
		t.Fatalf("pal=%v", pal)
	}
	if pal[0] != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("most used = %+v", pal[0])
	}
}

func TestMapPreservesAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, A: 255})
	m, _, err := Auto(img, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := m.Map(color.NRGBA{R: 10, G: 0, B: 0, A: 128})
	if got.A != 128 {
		t.Fatalf("A=%d", got.A)
	}
}

func TestAutoDeterministic(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: uint8(x + y), A: 255})
		}
	}
	_, a, err := Auto(img, 8)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := Auto(img, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("pal[%d] %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestAutoNonZeroMin(t *testing.T) {
	img := image.NewNRGBA(image.Rect(3, 5, 5, 6))
	img.SetNRGBA(3, 5, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(4, 5, color.NRGBA{G: 255, A: 255})
	_, pal, err := Auto(img, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(pal) != 1 || pal[0].G != 255 {
		t.Fatalf("%v", pal)
	}
}
