package stack

import (
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/lewtec/svgolf/internal/loss"
)

// paper is the empty pane. Source holes (want.A==0) must look like paper.
// got.A==0 is always a full miss — no transparent holes.
var paper = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

var paperHSV = loss.HSVOf(paper)

// scorePair is a dedicated got/want Plane so Score cannot take the
// last worker planes and deadlock Acquire.
var (
	scoreMu   sync.Mutex
	scoreGot  *loss.Plane
	scoreWant *loss.Plane
)

func scorePair() (*loss.Plane, *loss.Plane) {
	if scoreGot == nil {
		scoreGot = &loss.Plane{}
		scoreWant = &loss.Plane{}
	}
	return scoreGot, scoreWant
}

// Score is the sum of per-pixel HSV error. Opaque pixels use ColorAt².
// A hole (want.A==0) must match paper. Transparent got is 180².
// Mean would hide letters on a large canvas; sum does not.
func Score(got, want *image.NRGBA) float64 {
	scoreMu.Lock()
	defer scoreMu.Unlock()
	gp, wp := scorePair()
	gp.Reset(got)
	wp.Reset(want)
	return ScoreOn(gp, wp)
}

// ScoreOn is Score on HSV planes (want converted once, got after Render).
func ScoreOn(got, want *loss.Plane) float64 {
	if got == nil || want == nil || got.Image() == nil || want.Image() == nil || !got.Image().Rect.Eq(want.Image().Rect) {
		return math.Inf(1)
	}
	got.Ensure()
	want.Ensure()
	gp, wp := got.Slice(), want.Slice()
	n := len(gp)
	if len(wp) < n {
		n = len(wp)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += errAtHSV(gp[i], wp[i])
	}
	return sum
}

// ScoreRect is the errAt sum on r. r is clipped to want.
func ScoreRect(got, want *image.NRGBA, r image.Rectangle) float64 {
	scoreMu.Lock()
	defer scoreMu.Unlock()
	gp, wp := scorePair()
	gp.Reset(got)
	wp.Reset(want)
	return ScoreRectOn(gp, wp, r)
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
	b := want.Image().Rect
	gp, wp := got.Slice(), want.Slice()
	w := b.Dx()
	var sum float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := (y-b.Min.Y)*w + (r.Min.X - b.Min.X)
		for x := 0; x < r.Dx(); x++ {
			sum += errAtHSV(gp[row+x], wp[row+x])
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
