package stack

import (
	"image"
	"image/color"
	"testing"
)

func TestScoreHoles(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	want.SetNRGBA(0, 0, navy)
	want.SetNRGBA(1, 0, navy)
	want.SetNRGBA(0, 1, navy)
	// (1,1) is a hole
	empty := image.NewNRGBA(want.Rect)
	allNavy := image.NewNRGBA(want.Rect)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			allNavy.SetNRGBA(x, y, navy)
		}
	}
	tight := image.NewNRGBA(want.Rect)
	tight.SetNRGBA(0, 0, navy)
	tight.SetNRGBA(1, 0, navy)
	tight.SetNRGBA(0, 1, navy)
	if !(Score(allNavy, want, 0) < Score(empty, want, 0)) {
		t.Fatalf("plate error should beat empty: plate=%v empty=%v", Score(allNavy, want, 0), Score(empty, want, 0))
	}
	if !(Score(tight, want, 1) < Score(allNavy, want, 1)) {
		t.Fatalf("hole empty should beat filled hole: tight=%v plate=%v", Score(tight, want, 1), Score(allNavy, want, 1))
	}
}

func TestScoreBlackOnHoleCosts(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	want.SetNRGBA(0, 0, navy)
	want.SetNRGBA(1, 0, navy)
	want.SetNRGBA(0, 1, navy)
	tight := image.NewNRGBA(want.Rect)
	tight.SetNRGBA(0, 0, navy)
	tight.SetNRGBA(1, 0, navy)
	tight.SetNRGBA(0, 1, navy)
	black := image.NewNRGBA(want.Rect)
	copy(black.Pix, tight.Pix)
	black.SetNRGBA(1, 1, color.NRGBA{A: 255})
	if !(Score(tight, want, 1) < Score(black, want, 1)) {
		t.Fatalf("empty hole should beat black fill: tight=%v black=%v", Score(tight, want, 1), Score(black, want, 1))
	}
}

func TestScoreRectMatchesScore(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	got := image.NewNRGBA(want.Rect)
	want.SetNRGBA(1, 1, color.NRGBA{R: 255, A: 255})
	if ScoreRect(got, want, want.Rect) != Score(got, want, 0) {
		t.Fatalf("rect=%v full=%v", ScoreRect(got, want, want.Rect), Score(got, want, 0))
	}
}

func TestScoreChargesPaths(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if Score(img, img, 2)-Score(img, img, 1) != pathCost {
		t.Fatalf("path tax missing")
	}
}

func TestScoreSmallMarkPaysOnLargeCanvas(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	got := image.NewNRGBA(want.Rect)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			want.SetNRGBA(x, y, navy)
			got.SetNRGBA(x, y, navy)
		}
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			want.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	fixed := image.NewNRGBA(want.Rect)
	copy(fixed.Pix, got.Pix)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			fixed.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	if !(Score(fixed, want, 2) < Score(got, want, 1)) {
		t.Fatalf("10x10 mark should pay for a path: after=%v before=%v", Score(fixed, want, 2), Score(got, want, 1))
	}
}
