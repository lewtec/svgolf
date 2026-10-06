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

// ScoreOn is Score. A device evaluator reads the pixmaps. Otherwise the
// planes are converted and summed in float64.
func ScoreOn(got, want *loss.Plane) float64 {
	if got == nil || want == nil || got.Image() == nil || want.Image() == nil || !got.Image().Rect.Eq(want.Image().Rect) {
		return math.Inf(1)
	}
	if sum, ok := ndScoreImages(got.Image(), want.Image()); ok {
		return sum
	}
	got.Ensure()
	want.Ensure()
	return scorePixels(got.Slice(), want.Slice())
}

// scratchErr holds per-pixel error for one in-flight parallel sum.
// The following sequential add keeps Score identical to scoreScalar.
var scratchErr []float64

func scorePixels(got, want []loss.Pix) float64 {
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	parallel, claim := loss.EnterCores(n)
	if !parallel {
		sum := scoreScalar(got[:n], want[:n])
		claim.Release()
		return sum
	}
	defer claim.Release()
	return parallelSum(n, func(lo, hi int, dst []float64) {
		for i := lo; i < hi; i++ {
			dst[i] = errAtHSV(got[i], want[i])
		}
	})
}

func scoreScalar(got, want []loss.Pix) float64 {
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += errAtHSV(got[i], want[i])
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
	if sum, ok := ndScoreImageRect(got.Image(), want.Image(), r); ok {
		return sum
	}
	want.Ensure()
	r = r.Intersect(want.Image().Rect)
	if r.Empty() {
		return 0
	}
	got.EnsureRect(r)
	b := want.Image().Rect
	gp, wp := got.Slice(), want.Slice()
	if r.Eq(b) {
		return scorePixels(gp, wp)
	}
	return scoreRectPixels(gp, wp, b.Dx(), r, b.Min)
}

func scoreRectPixels(got, want []loss.Pix, width int, r image.Rectangle, origin image.Point) float64 {
	n := r.Dx() * r.Dy()
	parallel, claim := loss.EnterCores(n)
	if !parallel {
		sum := scoreScalarRect(got, want, width, r, origin)
		claim.Release()
		return sum
	}
	defer claim.Release()
	dx := r.Dx()
	return parallelSum(n, func(lo, hi int, dst []float64) {
		for i := lo; i < hi; i++ {
			y := r.Min.Y + i/dx
			x := i - (y-r.Min.Y)*dx
			row := (y-origin.Y)*width + (r.Min.X - origin.X)
			dst[i] = errAtHSV(got[row+x], want[row+x])
		}
	})
}

func parallelSum(n int, fill func(lo, hi int, dst []float64)) float64 {
	if cap(scratchErr) < n {
		scratchErr = make([]float64, n)
	}
	dst := scratchErr[:n]
	loss.SplitRange(n, func(lo, hi int) { fill(lo, hi, dst) })
	var sum float64
	for i := 0; i < n; i++ {
		sum += dst[i]
	}
	return sum
}

func scoreScalarRect(got, want []loss.Pix, width int, r image.Rectangle, origin image.Point) float64 {
	var sum float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := (y-origin.Y)*width + (r.Min.X - origin.X)
		for x := 0; x < r.Dx(); x++ {
			sum += errAtHSV(got[row+x], want[row+x])
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
