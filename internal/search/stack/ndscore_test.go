package stack

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/lewtec/svgolf/internal/loss"
)

func TestErrTapeMatchesScalar(t *testing.T) {
	palette := []color.NRGBA{
		{},
		{A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 255, A: 255},
		{G: 255, A: 255},
		{B: 255, A: 255},
		{R: 12, G: 52, B: 88, A: 255},
		{R: 250, G: 245, B: 240, A: 255},
		{R: 20, G: 20, B: 19, A: 255},
		{R: 255, G: 0, B: 128, A: 255},
	}
	var got, want []loss.Pix
	for _, g := range palette {
		for _, q := range palette {
			got = append(got, loss.HSVOf(g))
			want = append(want, loss.HSVOf(q))
		}
	}
	tape, err := newErrTape(len(got))
	if err != nil {
		t.Fatal(err)
	}
	tape.load(got, want, len(got))
	if _, err := tape.eval(ndarray.CPU, len(got)); err != nil {
		t.Fatal(err)
	}
	for i := range got {
		scalar := errAtHSV(got[i], want[i])
		nd := float64(tape.dst[i])
		if scalar == 0 {
			if nd != 0 {
				t.Fatalf("pixel %d: scalar 0 tape %v", i, nd)
			}
			continue
		}
		rel := math.Abs(nd-scalar) / scalar
		if rel > 1e-4 && math.Abs(nd-scalar) > 1e-2 {
			t.Fatalf("pixel %d: scalar %v tape %v", i, scalar, nd)
		}
	}
}

func TestErrTapeRectMatchesScalar(t *testing.T) {
	const width = 8
	got := make([]loss.Pix, width*6)
	want := make([]loss.Pix, len(got))
	for i := range got {
		got[i] = loss.HSVOf(color.NRGBA{R: uint8(i * 3), G: 40, B: 10, A: 255})
		a := uint8(255)
		if i%5 == 0 {
			a = 0
		}
		want[i] = loss.HSVOf(color.NRGBA{R: 12, G: 52, B: 88, A: a})
	}
	r := image.Rect(1, 1, 7, 5)
	origin := image.Pt(0, 0)
	n := r.Dx() * r.Dy()
	tape, err := newErrTape(n)
	if err != nil {
		t.Fatal(err)
	}
	tape.loadRect(got, want, width, r, origin)
	sum, err := tape.eval(ndarray.CPU, n)
	if err != nil {
		t.Fatal(err)
	}
	scalar := scoreScalarRect(got, want, width, r, origin)
	if scalar == 0 {
		if sum != 0 {
			t.Fatalf("scalar 0 tape %v", sum)
		}
		return
	}
	rel := math.Abs(sum-scalar) / scalar
	if rel > 1e-4 && math.Abs(sum-scalar) > 1e-2 {
		t.Fatalf("scalar %v tape %v", scalar, sum)
	}
}
