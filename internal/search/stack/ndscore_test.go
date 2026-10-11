package stack

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/stretchr/testify/require"
)

func TestErrTapeMatchesScalar(t *testing.T) {
	palette := []color.NRGBA{
		{},
		{A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 255, A: 255},
		{G: 255, A: 255},
		{B: 255, A: 255},
		{R: 12, G: 52, B: 88, A: 255},
		{R: 250, G: 245, B: 240, A: 255},
		{R: 20, G: 20, B: 19, A: 255},
		{R: 255, G: 0, B: 128, A: 255},
	}
	n := len(palette) * len(palette)
	got := image.NewNRGBA(image.Rect(0, 0, n, 1))
	want := image.NewNRGBA(image.Rect(0, 0, n, 1))
	i := 0
	for _, g := range palette {
		for _, q := range palette {
			got.SetNRGBA(i, 0, g)
			want.SetNRGBA(i, 0, q)
			i++
		}
	}
	tape, err := newErrTape(n)
	require.NoError(t, err)
	tape.load(got, want, got.Rect)
	_, err = tape.eval(t.Context(), ndarray.CPU, n)
	require.NoError(t, err)
	for i := range n {
		scalar := errAt(got.NRGBAAt(i, 0), want.NRGBAAt(i, 0))
		nd := float64(tape.dst[i])
		delta := 0.0
		if scalar != 0 {
			delta = math.Max(1e-4*scalar, 1e-2)
		}
		require.InDelta(t, scalar, nd, delta, "pixel %d got %v want %v", i, got.NRGBAAt(i, 0), want.NRGBAAt(i, 0))
	}
}

func TestErrTapeRectMatchesScalar(t *testing.T) {
	const width, height = 8, 6
	got := image.NewNRGBA(image.Rect(0, 0, width, height))
	want := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			i := y*width + x
			got.SetNRGBA(x, y, color.NRGBA{R: uint8(i * 3), G: 40, B: 10, A: 255})
			a := uint8(255)
			if i%5 == 0 {
				a = 0
			}
			want.SetNRGBA(x, y, color.NRGBA{R: 12, G: 52, B: 88, A: a})
		}
	}
	r := image.Rect(1, 1, 7, 5)
	n := r.Dx() * r.Dy()
	tape, err := newErrTape(n)
	require.NoError(t, err)
	tape.load(got, want, r)
	sum, err := tape.eval(t.Context(), ndarray.CPU, n)
	require.NoError(t, err)
	var scalar float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			scalar += errAt(got.NRGBAAt(x, y), want.NRGBAAt(x, y))
		}
	}
	delta := 0.0
	if scalar != 0 {
		delta = math.Max(1e-4*scalar, 1e-2)
	}
	require.InDelta(t, scalar, sum, delta)
}

func TestResidualMarkMatchesBand(t *testing.T) {
	colors := []color.NRGBA{
		{R: 255, G: 255, B: 255, A: 255},
		{R: 255, A: 255},
		{B: 255, A: 255},
		{},
		{R: 12, G: 52, B: 88, A: 255},
	}
	n := len(colors)
	got := image.NewNRGBA(image.Rect(0, 0, n, 1))
	want := image.NewNRGBA(got.Rect)
	for i, c := range colors {
		got.SetNRGBA(i, 0, c)
		want.SetNRGBA(i, 0, colors[(i+1)%n])
	}
	tape, err := newErrTape(n)
	require.NoError(t, err)
	const lo, hi = 64, 180
	tape.load(got, want, got.Rect)
	tape.lo.Buffer()[0] = lo
	tape.hi.Buffer()[0] = hi
	require.NoError(t, tape.mark.Resize(ndarray.Shape{n}))
	require.NoError(t, tape.mark.Eval(t.Context(), ndarray.CPU, tape.mask[:n]))
	for i := range n {
		e := colorErr(got.NRGBAAt(i, 0), want.NRGBAAt(i, 0))
		wantMark := byte(0)
		if e > lo && e <= hi {
			wantMark = 1
		}
		require.Equal(t, wantMark, tape.mask[i], "pixel %d err %v", i, e)
	}
}

type labelEval struct{ name string }

func (labelEval) Program(context.Context, *ndarray.Kernel) (ndarray.Program, error) {
	return nil, nil
}
func (labelEval) Close() error   { return nil }
func (e labelEval) Name() string { return e.name }

func captureNDLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestLogNDDriverNamesGPU(t *testing.T) {
	buf := captureNDLog(t)
	logNDDriver(labelEval{name: "vulkan:NVIDIA GeForce RTX 4090"}, nil)
	got := buf.String()
	require.Contains(t, got, "ndarray driver")
	require.Contains(t, got, "vulkan:NVIDIA GeForce RTX 4090")
	require.Contains(t, got, "score=device")
	require.Contains(t, got, "prefer=integrated")
}

func TestLatencyWeightsPreferIntegrated(t *testing.T) {
	table := latencyWeightTable()
	var slot map[string]int
	for _, weights := range table {
		if _, ok := weights["vulkan:integrated"]; ok {
			slot = weights
			break
		}
	}
	require.NotNil(t, slot)
	require.Equal(t, 50, slot["vulkan"])
	require.Greater(t, slot["vulkan:integrated"], 70)
	require.NoError(t, applyLatencyWeights())
}

func TestLogNDDriverCPUStaysFloat64(t *testing.T) {
	buf := captureNDLog(t)
	logNDDriver(ndarray.CPU, nil)
	got := buf.String()
	require.Contains(t, got, "name=cpu")
	require.Contains(t, got, "score=float64")
	logNDDriver(nil, errors.New("no device"))
	got = buf.String()
	require.Contains(t, got, "no device")
	require.Equal(t, 2, strings.Count(got, "score=float64"))
}

func TestPixelBudgetLeavesTheDeviceOn(t *testing.T) {
	before := gpuOff.Load()
	_, err := gpuTapeFor(maxDevicePixels + 1)
	require.ErrorIs(t, err, errPixelBudget)
	noteDeviceErr(t.Context(), err)
	require.Equal(t, before, gpuOff.Load())
}

func TestNilContextSkipsDevice(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	require.Nil(t, gpuEvaluator(nil))
	require.Zero(t, Score(nil, img, img))
}

func TestCancelledContextLeavesDeviceAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := gpuOff.Load()
	require.Nil(t, gpuEvaluator(ctx))
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	_, ok := ndScoreImages(ctx, img, img)
	require.False(t, ok)
	require.Equal(t, before, gpuOff.Load())
}
