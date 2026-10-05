package stack

import (
	"context"
	"image"
	"sync"
	"sync/atomic"

	_ "github.com/lewtec/lewkit/x/driver/ndeval"
	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/lewtec/svgolf/internal/loss"
)

// gpuEval is set when ndarray.Open returns a device evaluator.
// The CPU tape interprets every pixel and loses to the Go loop, so a
// dry machine leaves this nil and scorePixels stays in float64.
var (
	gpuOnce sync.Once
	gpuEval ndarray.Evaluator
	gpuOff  atomic.Bool
	gpuMu   sync.Mutex
	gpuTape *errTape
)

func gpuEvaluator() ndarray.Evaluator {
	gpuOnce.Do(func() {
		ev, err := ndarray.Open(context.Background())
		if err != nil || ev == nil || ev == ndarray.CPU {
			return
		}
		if named, ok := ev.(interface{ Name() string }); ok && named.Name() == "cpu" {
			return
		}
		gpuEval = ev
	})
	if gpuOff.Load() {
		return nil
	}
	return gpuEval
}

// errTape is the squared HSV error of two pixels as one ndarray kernel.
// Leaves are float32 planes; Eval writes dst.
type errTape struct {
	cap                        int
	gotH, gotS, gotV, gotA     *ndarray.Tensor[float32]
	wantH, wantS, wantV, wantA *ndarray.Tensor[float32]
	out                        *ndarray.Tensor[float32]
	dst                        []float32
}

const saturationFloor float32 = 0.08

func pow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func gpuTapeFor(n int) (*errTape, error) {
	if gpuTape != nil && gpuTape.cap >= n {
		if err := gpuTape.out.Resize(ndarray.Shape{n}); err != nil {
			return nil, err
		}
		return gpuTape, nil
	}
	tape, err := newErrTape(pow2(n))
	if err != nil {
		return nil, err
	}
	if err := tape.out.Resize(ndarray.Shape{n}); err != nil {
		return nil, err
	}
	gpuTape = tape
	return tape, nil
}

func newErrTape(n int) (*errTape, error) {
	shape := ndarray.Shape{n}
	plane := func() (*ndarray.Tensor[float32], error) {
		return ndarray.New(make([]float32, n), shape)
	}
	var err error
	tape := &errTape{cap: n, dst: make([]float32, n)}
	if tape.gotH, err = plane(); err != nil {
		return nil, err
	}
	if tape.gotS, err = plane(); err != nil {
		return nil, err
	}
	if tape.gotV, err = plane(); err != nil {
		return nil, err
	}
	if tape.gotA, err = plane(); err != nil {
		return nil, err
	}
	if tape.wantH, err = plane(); err != nil {
		return nil, err
	}
	if tape.wantS, err = plane(); err != nil {
		return nil, err
	}
	if tape.wantV, err = plane(); err != nil {
		return nil, err
	}
	if tape.wantA, err = plane(); err != nil {
		return nil, err
	}
	tape.out = squaredHSVError(tape)
	if tape.out == nil {
		return nil, ndarray.ErrOp
	}
	return tape, nil
}

func squaredHSVError(tape *errTape) *ndarray.Tensor[float32] {
	dist := hsvDistance(tape.gotH, tape.gotS, tape.gotV, tape.wantH, tape.wantS, tape.wantV)
	paper := hsvDistance(tape.gotH, tape.gotS, tape.gotV,
		ndarray.Const(float32(paperHSV.H)),
		ndarray.Const(float32(paperHSV.S)),
		ndarray.Const(float32(paperHSV.V)),
	)
	zero := ndarray.Const(float32(0))
	miss := ndarray.Const(float32(180))
	err := tape.wantA.Equal(zero).Where(paper, dist)
	err = tape.gotA.Equal(zero).Where(miss, err)
	return err.Mul(err)
}

func hsvDistance(h1, s1, v1, h2, s2, v2 *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	scale := ndarray.Const(float32(180))
	dv := abs32(v1.Add(v2.Neg())).Mul(scale)
	ds := abs32(s1.Add(s2.Neg())).Mul(scale)
	colored := hueDistance(h1, h2).Max(ds).Max(dv)
	low := ndarray.Const(saturationFloor)
	low1 := s1.CmpLt(low)
	low2 := s2.CmpLt(low)
	either := low1.Or(low2).Where(scale, colored)
	return low1.And(low2).Where(dv, either)
}

func hueDistance(a, b *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	d := abs32(a.Add(b.Neg()))
	turn := ndarray.Const(float32(180))
	full := ndarray.Const(float32(360))
	return d.CmpLt(turn).Where(d, full.Add(d.Neg()))
}

func abs32(v *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	return v.Max(v.Neg())
}

func (tape *errTape) load(got, want []loss.Pix, n int) {
	gh, gs, gv, ga := tape.gotH.Buffer(), tape.gotS.Buffer(), tape.gotV.Buffer(), tape.gotA.Buffer()
	wh, ws, wv, wa := tape.wantH.Buffer(), tape.wantS.Buffer(), tape.wantV.Buffer(), tape.wantA.Buffer()
	for i := range n {
		g, q := got[i], want[i]
		gh[i], gs[i], gv[i], ga[i] = float32(g.H), float32(g.S), float32(g.V), float32(g.A)
		wh[i], ws[i], wv[i], wa[i] = float32(q.H), float32(q.S), float32(q.V), float32(q.A)
	}
}

func (tape *errTape) loadRect(got, want []loss.Pix, width int, r image.Rectangle, origin image.Point) {
	gh, gs, gv, ga := tape.gotH.Buffer(), tape.gotS.Buffer(), tape.gotV.Buffer(), tape.gotA.Buffer()
	wh, ws, wv, wa := tape.wantH.Buffer(), tape.wantS.Buffer(), tape.wantV.Buffer(), tape.wantA.Buffer()
	i := 0
	dx := r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := (y-origin.Y)*width + (r.Min.X - origin.X)
		for x := range dx {
			g, q := got[row+x], want[row+x]
			gh[i], gs[i], gv[i], ga[i] = float32(g.H), float32(g.S), float32(g.V), float32(g.A)
			wh[i], ws[i], wv[i], wa[i] = float32(q.H), float32(q.S), float32(q.V), float32(q.A)
			i++
		}
	}
}

func (tape *errTape) eval(ev ndarray.Evaluator, n int) (float64, error) {
	if err := tape.out.Eval(context.Background(), ev, tape.dst[:n]); err != nil {
		return 0, err
	}
	var sum float64
	for _, v := range tape.dst[:n] {
		sum += float64(v)
	}
	return sum, nil
}

func ndScore(got, want []loss.Pix) (float64, bool) {
	ev := gpuEvaluator()
	if ev == nil {
		return 0, false
	}
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	if n == 0 {
		return 0, true
	}
	gpuMu.Lock()
	defer gpuMu.Unlock()
	tape, err := gpuTapeFor(n)
	if err != nil {
		gpuOff.Store(true)
		return 0, false
	}
	tape.load(got[:n], want[:n], n)
	sum, err := tape.eval(ev, n)
	if err != nil {
		gpuOff.Store(true)
		return 0, false
	}
	return sum, true
}

func ndScoreRect(got, want []loss.Pix, width int, r image.Rectangle, origin image.Point) (float64, bool) {
	ev := gpuEvaluator()
	if ev == nil || r.Empty() {
		return 0, ev != nil && r.Empty()
	}
	n := r.Dx() * r.Dy()
	gpuMu.Lock()
	defer gpuMu.Unlock()
	tape, err := gpuTapeFor(n)
	if err != nil {
		gpuOff.Store(true)
		return 0, false
	}
	tape.loadRect(got, want, width, r, origin)
	sum, err := tape.eval(ev, n)
	if err != nil {
		gpuOff.Store(true)
		return 0, false
	}
	return sum, true
}
