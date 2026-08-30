package stack

import (
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/internal/loss"
)

// pathCost is the error-sum one extra path must buy. Two minIsland
// full misses: stacking a plate was cheaper than refitting the one
// already there (gradient / cubics).
const pathCost = 180 * 180 * minIsland * 2

// cmdCost is one extra path command. A straight edge costs two
// (see pathCommandWeight) so a spline that replaces many lines wins.
const cmdCost = 180 * 180

// paper is the empty pane. Source holes (want.A==0) must look like paper.
// got.A==0 is always a full miss — no transparent holes.
var paper = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

var paperHSV = loss.HSVOf(paper)

// Score is the sum of per-pixel error plus pathCost·parts. Opaque pixels
// use ColorAt². A hole (want.A==0) must match paper. Transparent got is
// 180². Mean would hide letters on a large canvas; sum does not.
func Score(got, want *image.NRGBA, parts int) float64 {
	return ScoreOn(loss.NewPlane(got), loss.NewPlane(want), parts)
}

// ScoreOn is Score on HSV planes (want converted once, got after Render).
func ScoreOn(got, want *loss.Plane, parts int) float64 {
	if got == nil || want == nil || got.Image() == nil || want.Image() == nil || !got.Image().Rect.Eq(want.Image().Rect) {
		return math.Inf(1)
	}
	got.Ensure()
	want.Ensure()
	b := want.Image().Rect
	var sum float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			sum += errAtHSV(got.At(x, y), want.At(x, y))
		}
	}
	if parts < 0 {
		parts = 0
	}
	return sum + pathCost*float64(parts)
}

// ScoreRect is the errAt sum on r (no path tax). r is clipped to want.
func ScoreRect(got, want *image.NRGBA, r image.Rectangle) float64 {
	return ScoreRectOn(loss.NewPlane(got), loss.NewPlane(want), r)
}

// ScoreRectOn is ScoreRect on HSV planes.
func ScoreRectOn(got, want *loss.Plane, r image.Rectangle) float64 {
	if got == nil || want == nil || got.Image() == nil || want.Image() == nil || !got.Image().Rect.Eq(want.Image().Rect) {
		return math.Inf(1)
	}
	want.Ensure()
	r = r.Intersect(want.Image().Rect)
	if r.Empty() {
		return 0
	}
	got.EnsureRect(r)
	var sum float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sum += errAtHSV(got.At(x, y), want.At(x, y))
		}
	}
	return sum
}

func errAt(g, q color.NRGBA) float64 {
	return errAtHSV(loss.HSVOf(g), loss.HSVOf(q))
}

func errAtHSV(g, q loss.Pix) float64 {
	e := colorErrHSV(g, q)
	return e * e
}

func colorErr(g, q color.NRGBA) float64 {
	return colorErrHSV(loss.HSVOf(g), loss.HSVOf(q))
}

func colorErrHSV(g, q loss.Pix) float64 {
	if g.A == 0 {
		return 180
	}
	if q.A == 0 {
		return loss.ColorAtHSV(g, paperHSV)
	}
	return loss.ColorAtHSV(g, q)
}
