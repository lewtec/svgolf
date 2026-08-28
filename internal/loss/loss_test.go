package loss

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestPixelsDontCare(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	got := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	want.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	want.SetNRGBA(1, 0, color.NRGBA{}) // don't-care
	got.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	got.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	if n := (Pixels{}).Loss(got, want); n != 1 {
		t.Fatalf("Loss=%v want 1", n)
	}
}

func TestPixelsMatch(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})
	if n := (Pixels{}).Loss(img, img); n != 0 {
		t.Fatalf("Loss=%v want 0", n)
	}
}

func TestPixelsSizeMismatch(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	b := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if n := (Pixels{}).Loss(a, b); !math.IsInf(n, 1) {
		t.Fatalf("Loss=%v want +Inf", n)
	}
}

func TestPerCost(t *testing.T) {
	if got := PerCost(10, 2); got != 5 {
		t.Fatalf("PerCost(10,2)=%v want 5", got)
	}
	if got := PerCost(0, 0); got != 0 {
		t.Fatalf("PerCost(0,0)=%v want 0", got)
	}
	if got := PerCost(3, 0); !math.IsInf(got, 1) {
		t.Fatalf("PerCost(3,0)=%v want +Inf", got)
	}
}

func TestOfEmptyMatch(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	doc := svg.NewDocument(4, 4)
	s, err := Of(doc, want, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s != 0 {
		t.Fatalf("Of empty on transparent=%v want 0", s)
	}
}

func TestOfSolidPlate(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			want.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc := svg.NewDocument(4, 4).Append(
		svg.NewRect().WithWidth(4).WithHeight(4).WithFill(color.NRGBA{R: 255, A: 255}).Node(),
	)
	s, err := Of(doc, want, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s != 0 {
		t.Fatalf("Of plate=%v want 0", s)
	}
}
