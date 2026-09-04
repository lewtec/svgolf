package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestScoreTransparentIsMiss(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	clear := color.NRGBA{}
	if colorErr(clear, navy) != 180 {
		t.Fatalf("clear on navy=%v", colorErr(clear, navy))
	}
	if colorErr(clear, clear) != 180 {
		t.Fatalf("clear on hole=%v want 180", colorErr(clear, clear))
	}
}

func TestScoreHoleWantsPaper(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	hole := color.NRGBA{}
	if colorErr(paper, hole) != 0 {
		t.Fatalf("paper on hole=%v", colorErr(paper, hole))
	}
	if colorErr(navy, hole) < 90 {
		t.Fatalf("navy on hole=%v want a miss", colorErr(navy, hole))
	}
}

func TestScoreHoles(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	want.SetNRGBA(0, 0, navy)
	want.SetNRGBA(1, 0, navy)
	want.SetNRGBA(0, 1, navy)
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
	tight.SetNRGBA(1, 1, paper)
	if !(Score(allNavy, want) < Score(empty, want)) {
		t.Fatalf("plate error should beat empty: plate=%v empty=%v", Score(allNavy, want), Score(empty, want))
	}
	if !(Score(tight, want) < Score(allNavy, want)) {
		t.Fatalf("paper hole should beat filled hole: tight=%v plate=%v", Score(tight, want), Score(allNavy, want))
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
	tight.SetNRGBA(1, 1, paper)
	black := image.NewNRGBA(want.Rect)
	copy(black.Pix, tight.Pix)
	black.SetNRGBA(1, 1, color.NRGBA{A: 255})
	if !(Score(tight, want) < Score(black, want)) {
		t.Fatalf("paper hole should beat black fill: tight=%v black=%v", Score(tight, want), Score(black, want))
	}
}

func TestScoreReusesPlanes(t *testing.T) {
	got := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	want := image.NewNRGBA(got.Rect)
	_ = Score(got, want)
	allocs := testing.AllocsPerRun(50, func() {
		_ = Score(got, want)
	})
	if allocs != 0 {
		t.Fatalf("Score allocs=%v want 0 after the pair is warm", allocs)
	}
}

func TestScoreRectMatchesScore(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	got := image.NewNRGBA(want.Rect)
	want.SetNRGBA(1, 1, color.NRGBA{R: 255, A: 255})
	if ScoreRect(got, want, want.Rect) != Score(got, want) {
		t.Fatalf("rect=%v full=%v", ScoreRect(got, want, want.Rect), Score(got, want))
	}
}

func TestLineEdgeCostsMoreThanCubic(t *testing.T) {
	lines := svg.NewPath().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10).Close()
	curve := svg.NewPath().MoveTo(0, 0).CubicTo(3, 0, 7, 0, 10, 0).CubicTo(10, 3, 10, 7, 10, 10).Close()
	if pathCommandWeight(lines.Node()) <= pathCommandWeight(curve.Node()) {
		t.Fatalf("lines=%d cubics=%d; straight edges should cost more", pathCommandWeight(lines.Node()), pathCommandWeight(curve.Node()))
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
	if !(Score(fixed, want) < Score(got, want)) {
		t.Fatalf("10x10 mark should be visible in the sum: after=%v before=%v", Score(fixed, want), Score(got, want))
	}
}
