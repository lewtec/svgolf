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
	heat, island := DebugFrames(got, want, nil)
	if heat == nil || island == nil {
		t.Fatal("nil frames")
	}
	if island.NRGBAAt(8, 8).R < 200 {
		t.Fatalf("island pixel %+v want highlight", island.NRGBAAt(8, 8))
	}
	if heat.NRGBAAt(8, 8).R < 100 {
		t.Fatalf("heat miss %+v", heat.NRGBAAt(8, 8))
	}
	if heat.NRGBAAt(0, 0).R > 40 {
		t.Fatalf("heat paper %+v want dark", heat.NRGBAAt(0, 0))
	}
}

func TestDebugFramesUsesProvidedBlob(t *testing.T) {
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
	small := []pix{{1, 1}, {2, 1}, {1, 2}, {2, 2}}
	_, isle := DebugFrames(got, want, small)
	if isle.NRGBAAt(2, 2).R < 200 {
		t.Fatal("provided blob not marked")
	}
	if isle.NRGBAAt(12, 12).R > 80 {
		t.Fatal("hottest() used instead of provided leftover")
	}
}
