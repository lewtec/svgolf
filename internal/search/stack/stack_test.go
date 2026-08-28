package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestStackSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("paths=%d want 1", n)
	}
	if _, ok := doc.Children()[0].Path(); !ok {
		t.Fatal("not a path")
	}
}

func TestStackTwoColorGetsBoth(t *testing.T) {
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
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want >=2", n)
	}
}

func TestStackMarkAfterPlate(t *testing.T) {
	// navy field + black block: global hue must not block the mark
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := navy
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want >=2 (plate + mark)", n)
	}
	empty := image.NewNRGBA(img.Rect)
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if Score(got, img, len(doc.Children())) >= Score(empty, img, 0) {
		t.Fatalf("final score not better than empty")
	}
}

func TestStackKeepsGoingAfterReject(t *testing.T) {
	// three disjoint blobs: a rejected hull must not end the run
	img := image.NewNRGBA(image.Rect(0, 0, 24, 8))
	paint := func(x0, x1 int, c color.NRGBA) {
		for y := 1; y < 7; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	paint(1, 7, color.NRGBA{R: 255, A: 255})
	paint(9, 15, color.NRGBA{G: 255, A: 255})
	paint(17, 23, color.NRGBA{B: 255, A: 255})
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 3 {
		t.Fatalf("paths=%d want >=3", n)
	}
}

func TestStackDoesNotKeepFilledHoles(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 12; y < 20; y++ {
		for x := 12; x < 20; x++ {
			img.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	c := got.NRGBAAt(16, 16)
	if c.A != 0 && c.B > 40 {
		t.Fatalf("hole still navy %+v paths=%d", c, len(doc.Children()))
	}
}

func TestStackTinyMark(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 18; y < 22; y++ {
		for x := 18; x < 22; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want plate + 16px mark", n)
	}
}

func TestStackNilPixmap(t *testing.T) {
	_, err := search.Last((Stack{}).Search(t.Context(), nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStackFirstFormIsBBox(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for doc, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		p, ok := doc.Children()[0].Path()
		if !ok {
			t.Fatal("not a path")
		}
		n := 0
		for _, c := range p.Commands() {
			if c.Kind != svg.CmdClose {
				n++
			}
		}
		if n != 4 {
			t.Fatalf("first form points=%d want 4 (bbox)", n)
		}
		return
	}
	t.Fatal("no epoch")
}

func TestRDPCollinear(t *testing.T) {
	got := rdp([][2]float64{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, 0.5)
	if len(got) != 2 {
		t.Fatalf("rdp=%v", got)
	}
}

func TestFilledEllipseUsesCubics(t *testing.T) {
	p := filledEllipse(10, 10, 8, 8, color.NRGBA{R: 255, A: 255})
	n := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("cubics=%d want 4", n)
	}
}

func TestHullSquare(t *testing.T) {
	h := convexHull([][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}})
	if len(h) != 4 {
		t.Fatalf("hull=%v", h)
	}
}
