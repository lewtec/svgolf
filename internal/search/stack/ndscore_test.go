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
	if err != nil {
		t.Fatal(err)
	}
	tape.load(got, want, got.Rect)
	if _, err := tape.eval(t.Context(), ndarray.CPU, n); err != nil {
		t.Fatal(err)
	}
	for i := range n {
		scalar := errAt(got.NRGBAAt(i, 0), want.NRGBAAt(i, 0))
		nd := float64(tape.dst[i])
		if scalar == 0 {
			if nd != 0 {
				t.Fatalf("pixel %d: scalar 0 tape %v", i, nd)
			}
			continue
		}
		rel := math.Abs(nd-scalar) / scalar
		if rel > 1e-4 && math.Abs(nd-scalar) > 1e-2 {
			t.Fatalf("pixel %d: scalar %v tape %v got %v want %v", i, scalar, nd, got.NRGBAAt(i, 0), want.NRGBAAt(i, 0))
		}
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
	if err != nil {
		t.Fatal(err)
	}
	tape.load(got, want, r)
	sum, err := tape.eval(t.Context(), ndarray.CPU, n)
	if err != nil {
		t.Fatal(err)
	}
	var scalar float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			scalar += errAt(got.NRGBAAt(x, y), want.NRGBAAt(x, y))
		}
	}
	if scalar == 0 {
		if sum != 0 {
			t.Fatalf("scalar 0 tape %v", sum)
		}
		return
	}
	rel := math.Abs(sum-scalar) / scalar
	if rel > 1e-4 && math.Abs(sum-scalar) > 1e-2 {
		t.Fatalf("scalar %v tape %v", scalar, sum)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	const lo, hi = 64, 180
	tape.load(got, want, got.Rect)
	tape.lo.Buffer()[0] = lo
	tape.hi.Buffer()[0] = hi
	if err := tape.mark.Resize(ndarray.Shape{n}); err != nil {
		t.Fatal(err)
	}
	if err := tape.mark.Eval(t.Context(), ndarray.CPU, tape.mask[:n]); err != nil {
		t.Fatal(err)
	}
	for i := range n {
		e := colorErr(got.NRGBAAt(i, 0), want.NRGBAAt(i, 0))
		wantMark := byte(0)
		if e > lo && e <= hi {
			wantMark = 1
		}
		if tape.mask[i] != wantMark {
			t.Fatalf("pixel %d: err %v mark %d want %d", i, e, tape.mask[i], wantMark)
		}
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
	if !strings.Contains(got, "ndarray driver") || !strings.Contains(got, "vulkan:NVIDIA GeForce RTX 4090") || !strings.Contains(got, "score=device") {
		t.Fatalf("log=%s", got)
	}
}

func TestLogNDDriverCPUStaysFloat64(t *testing.T) {
	buf := captureNDLog(t)
	logNDDriver(ndarray.CPU, nil)
	got := buf.String()
	if !strings.Contains(got, "name=cpu") || !strings.Contains(got, "score=float64") {
		t.Fatalf("log=%s", got)
	}
	logNDDriver(nil, errors.New("no device"))
	got = buf.String()
	if !strings.Contains(got, "no device") || strings.Count(got, "score=float64") != 2 {
		t.Fatalf("log=%s", got)
	}
}

func TestNilContextSkipsDevice(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if gpuEvaluator(nil) != nil {
		t.Fatal("nil context returned a device")
	}
	if Score(nil, img, img) != 0 {
		t.Fatal("nil context changed the float64 score")
	}
}

func TestCancelledContextLeavesDeviceAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := gpuOff.Load()
	if gpuEvaluator(ctx) != nil {
		t.Fatal("cancelled context returned a device")
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if _, ok := ndScoreImages(ctx, img, img); ok {
		t.Fatal("cancelled context took the device score")
	}
	if gpuOff.Load() != before {
		t.Fatal("cancelled context changed gpuOff")
	}
}
