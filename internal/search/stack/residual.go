package stack

import (
	"context"
	"image"

	"github.com/lewtec/lewkit/x/ndarray"
)

// saturationFloor is the grey split inside loss.ColorAt.
// Pixels under it compare value; a grey pixel against a saturated pixel is the full miss.
const saturationFloor float32 = 0.08

// residualSum is the squared hue-saturation-value error of rect, dispatched
// through an ndarray evaluator. The CPU tape and a GPU evaluator run the same graph.
func residualSum(ctx context.Context, evaluator ndarray.Evaluator, got, want *image.NRGBA, rect image.Rectangle) (float64, error) {
	if evaluator == nil {
		return 0, ndarray.ErrOp
	}
	if got == nil || want == nil {
		return 0, nil
	}
	rect = rect.Intersect(got.Bounds()).Intersect(want.Bounds())
	if rect.Empty() {
		return 0, nil
	}
	width, height := rect.Dx(), rect.Dy()
	gr, gg, gb, ga := plane(got, rect)
	wr, wg, wb, wa := plane(want, rect)
	shape := ndarray.Shape{height, width}
	gotR, err := ndarray.New(gr, shape)
	if err != nil {
		return 0, err
	}
	gotG, err := ndarray.New(gg, shape)
	if err != nil {
		return 0, err
	}
	gotB, err := ndarray.New(gb, shape)
	if err != nil {
		return 0, err
	}
	gotA, err := ndarray.New(ga, shape)
	if err != nil {
		return 0, err
	}
	wantR, err := ndarray.New(wr, shape)
	if err != nil {
		return 0, err
	}
	wantG, err := ndarray.New(wg, shape)
	if err != nil {
		return 0, err
	}
	wantB, err := ndarray.New(wb, shape)
	if err != nil {
		return 0, err
	}
	wantA, err := ndarray.New(wa, shape)
	if err != nil {
		return 0, err
	}
	errPix := squaredColorError(gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
	out := make([]float32, width*height)
	if err := errPix.Eval(ctx, evaluator, out); err != nil {
		return 0, err
	}
	var sum float64
	for _, v := range out {
		sum += float64(v)
	}
	return sum, nil
}

func plane(img *image.NRGBA, rect image.Rectangle) (r, g, b, a []float32) {
	n := rect.Dx() * rect.Dy()
	r = make([]float32, n)
	g = make([]float32, n)
	b = make([]float32, n)
	a = make([]float32, n)
	i := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			r[i] = float32(c.R) / 255
			g[i] = float32(c.G) / 255
			b[i] = float32(c.B) / 255
			a[i] = float32(c.A)
			i++
		}
	}
	return r, g, b, a
}

func squaredColorError(gr, gg, gb, ga, wr, wg, wb, wa *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	zero := ndarray.Const(float32(0))
	full := ndarray.Const(float32(180))
	gotGone := ga.Equal(zero)
	wantGone := wa.Equal(zero)
	paperR := ndarray.Const(float32(paper.R) / 255)
	paperG := ndarray.Const(float32(paper.G) / 255)
	paperB := ndarray.Const(float32(paper.B) / 255)
	againstWant := colorDistance(gr, gg, gb, wr, wg, wb)
	againstPaper := colorDistance(gr, gg, gb, paperR, paperG, paperB)
	dist := wantGone.Where(againstPaper, againstWant)
	dist = gotGone.Where(full, dist)
	return dist.Mul(dist)
}

func colorDistance(ar, ag, ab, br, bg, bb *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	ah, as, av := hsvTensor(ar, ag, ab)
	bh, bs, bv := hsvTensor(br, bg, bb)
	dv := abs32(av.Add(bv.Neg())).Mul(ndarray.Const(float32(180)))
	ds := abs32(as.Add(bs.Neg())).Mul(ndarray.Const(float32(180)))
	hd := hueDistance(ah, bh)
	floor := ndarray.Const(saturationFloor)
	grayA := as.CmpLt(floor)
	grayB := bs.CmpLt(floor)
	both := grayA.Mul(grayB)
	either := grayA.Or(grayB)
	mixed := hd.Max(ds).Max(dv)
	full := ndarray.Const(float32(180))
	dist := either.Where(full, mixed)
	return both.Where(dv, dist)
}

func hsvTensor(r, g, b *ndarray.Tensor[float32]) (h, s, v *ndarray.Tensor[float32]) {
	zero := ndarray.Const(float32(0))
	one := ndarray.Const(float32(1))
	mx := r.Max(g).Max(b)
	mn := r.Neg().Max(g.Neg()).Max(b.Neg()).Neg()
	delta := mx.Add(mn.Neg())
	mxSafe := mx.Equal(zero).Where(one, mx)
	sat := delta.Div(mxSafe)
	s = mx.Equal(zero).Where(zero, sat)
	v = mx
	deltaSafe := delta.Equal(zero).Where(one, delta)
	inv := deltaSafe.Reciprocal()
	sixty := ndarray.Const(float32(60))
	hr := g.Add(b.Neg()).Mul(inv).Mul(sixty)
	hr = hr.CmpLt(zero).Where(hr.Add(ndarray.Const(float32(360))), hr)
	hg := b.Add(r.Neg()).Mul(inv).Mul(sixty).Add(ndarray.Const(float32(120)))
	hb := r.Add(g.Neg()).Mul(inv).Mul(sixty).Add(ndarray.Const(float32(240)))
	hue := r.Equal(mx).Where(hr, g.Equal(mx).Where(hg, hb))
	h = delta.Equal(zero).Where(zero, hue)
	return h, s, v
}

func hueDistance(a, b *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	d := abs32(a.Add(b.Neg()))
	turn := ndarray.Const(float32(180))
	return d.CmpLt(turn).Where(d, ndarray.Const(float32(360)).Add(d.Neg()))
}

func abs32(x *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	return x.Max(x.Neg())
}

// deviceAgrees reports whether the ndarray residual can change a lexicographic
// comparison against the renderer score. The gap grows with the number of
// float32 terms. A caller confirms with Score when the gap still reaches the parent.
func deviceAgrees(device, exact float64, pixels int) bool {
	gap := summationGap(pixels)
	if device > exact {
		return device-exact <= gap
	}
	return exact-device <= gap
}

func summationGap(pixels int) float64 {
	if pixels < 2 {
		return 0
	}
	const unitRoundoff = 1.0 / (1 << 23)
	const maxTerm = 180 * 180
	n := float64(pixels)
	return (n - 1) * unitRoundoff * n * maxTerm
}

// confirmResidual renders nothing. It checks one rectangle against loss.ScoreRect
// so a drifting graph fails before a search trusts it.
func confirmResidual(ctx context.Context, evaluator ndarray.Evaluator, got, want *image.NRGBA, rect image.Rectangle) (float64, float64, error) {
	device, err := residualSum(ctx, evaluator, got, want, rect)
	if err != nil {
		return 0, 0, err
	}
	exact := ScoreRect(got, want, rect)
	return device, exact, nil
}
