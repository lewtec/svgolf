package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
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

func TestStackTwoColorRMSEFalls(t *testing.T) {
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
	var last float64
	n := 0
	for doc, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		got, err := render.Render(doc)
		if err != nil {
			t.Fatal(err)
		}
		r := loss.Hue(got, img)
		if n > 0 && r >= last {
			t.Fatalf("epoch %d hue %v not < %v", n, r, last)
		}
		last = r
		n++
	}
	if n < 2 {
		t.Fatalf("epochs=%d want >=2", n)
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

func TestHullSquare(t *testing.T) {
	h := convexHull([][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}})
	if len(h) != 4 {
		t.Fatalf("hull=%v", h)
	}
}
