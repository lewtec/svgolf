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
	if !(Score(allNavy, want, 1) < Score(empty, want, 0)) {
		t.Fatalf("plate should beat empty: plate=%v empty=%v", Score(allNavy, want, 1), Score(empty, want, 0))
	}
	if !(Score(tight, want, 1) < Score(allNavy, want, 1)) {
		t.Fatalf("hole empty should beat filled hole: tight=%v plate=%v", Score(tight, want, 1), Score(allNavy, want, 1))
	}
}

func TestScoreChargesPaths(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if Score(img, img, 2)-Score(img, img, 1) != pathCost {
		t.Fatalf("path tax missing")
	}
}
