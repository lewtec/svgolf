package dumb

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/search"
)

func TestDumbSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := search.Last((Dumb{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 1 {
		t.Fatalf("kids=%d", len(kids))
	}
	r, ok := kids[0].Rect()
	if !ok || r.Width() != 10 || r.Height() != 8 {
		t.Fatalf("rect %+v", r)
	}
}

func TestDumbTwoColorConcentric(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Dumb{Colors: 2}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 2 {
		t.Fatalf("kids=%d", n)
	}
	outer, _ := doc.Children()[0].Rect()
	inner, _ := doc.Children()[1].Rect()
	if inner.Width() != outer.Width()*0.75 {
		t.Fatalf("inner w=%v outer=%v", inner.Width(), outer.Width())
	}
}

func TestDumbAlphaBBox(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	img.SetNRGBA(2, 3, color.NRGBA{R: 1, A: 200})
	img.SetNRGBA(4, 6, color.NRGBA{R: 1, A: 200})
	doc, err := search.Last((Dumb{Colors: 1}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := doc.Children()[0].Rect()
	if r.X() != 2 || r.Y() != 3 || r.Width() != 3 || r.Height() != 4 {
		t.Fatalf("bbox rect x=%v y=%v w=%v h=%v", r.X(), r.Y(), r.Width(), r.Height())
	}
}

func TestDumbNilPixmap(t *testing.T) {
	_, err := search.Last((Dumb{}).Search(t.Context(), nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDumbOneEpoch(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})
	n := 0
	for _, err := range (Dumb{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 1 {
		t.Fatalf("epochs=%d want 1", n)
	}
}
