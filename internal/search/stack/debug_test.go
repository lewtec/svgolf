package stack

import (
	"image"
	"image/color"
	"testing"
)

func TestDebugFramesMarksHottestIsland(t *testing.T) {
	got := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	want := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			got.SetNRGBA(x, y, paper)
			want.SetNRGBA(x, y, paper)
		}
	}
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			want.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	heat, island := DebugFrames(got, want, nil, nil)
	if heat == nil || island == nil {
		t.Fatal("nil frames")
	}
	if c := island.NRGBAAt(8, 8); c.R < 200 || c.G < 200 || c.B < 200 {
		t.Fatalf("island pixel %+v want white mask", c)
	}
	if c := island.NRGBAAt(0, 0); c.R > 40 || c.G > 40 || c.B > 40 {
		t.Fatalf("outside mask %+v want black", c)
	}
	if heat.NRGBAAt(8, 8).R < 100 {
		t.Fatalf("heat miss %+v", heat.NRGBAAt(8, 8))
	}
	if heat.NRGBAAt(0, 0).R > 40 {
		t.Fatalf("heat paper %+v want dark", heat.NRGBAAt(0, 0))
	}
}

func TestDebugFramesMarksEveryDifference(t *testing.T) {
	got := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	want := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			got.SetNRGBA(x, y, paper)
			want.SetNRGBA(x, y, paper)
		}
	}
	for y := 1; y < 5; y++ {
		for x := 1; x < 5; x++ {
			want.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for y := 10; y < 15; y++ {
		for x := 10; x < 15; x++ {
			want.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	_, isle := DebugFrames(got, want, nil, nil)
	if c := isle.NRGBAAt(2, 2); c.R < 200 || c.G < 200 || c.B < 200 {
		t.Fatalf("red miss %+v want white", c)
	}
	if c := isle.NRGBAAt(12, 12); c.R < 200 || c.G < 200 || c.B < 200 {
		t.Fatalf("blue miss %+v want white", c)
	}
	if c := isle.NRGBAAt(0, 0); c.R > 40 || c.G > 40 || c.B > 40 {
		t.Fatalf("match %+v want black", c)
	}
}

func TestDebugFramesPaintsFittedTriangle(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	got := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got.SetNRGBA(x, y, paper)
			want.SetNRGBA(x, y, paper)
		}
	}
	for _, p := range []pix{{1, 1}, {2, 1}, {3, 1}, {1, 2}, {2, 2}, {3, 2}} {
		want.SetNRGBA(p.x, p.y, color.NRGBA{B: 255, A: 255})
	}
	fitted := []pix{{2, 1}, {2, 2}}
	_, isle := DebugFrames(got, want, nil, fitted)
	if c := isle.NRGBAAt(1, 1); c.R < 200 || c.G < 200 {
		t.Fatalf("mask %+v want white", c)
	}
	if c := isle.NRGBAAt(2, 1); c.R < 200 || c.G > 120 {
		t.Fatalf("fitted %+v want orange", c)
	}
	if c := isle.NRGBAAt(0, 0); c.R > 40 {
		t.Fatalf("outside %+v want black", c)
	}
}
