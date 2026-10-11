package stack

import (
	"image"
	"image/color"
	"math"
	"testing"

	_ "github.com/lewtec/lewkit/x/driver/ndeval/vulkan"
	"github.com/lewtec/lewkit/x/ndarray"
)

func TestResidualGPUAgreesWithCPU(t *testing.T) {
	evaluator, err := ndarray.Open(t.Context())
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = evaluator.Close() })
	if evaluator == ndarray.CPU {
		t.Skip("no graphics processor evaluator")
	}
	got := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	want := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			got.SetNRGBA(x, y, color.NRGBA{R: 40, G: 90, B: 20, A: 255})
			want.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + x*8), G: 90, B: 20, A: 255})
		}
	}
	cpu, _, err := confirmResidual(t.Context(), ndarray.CPU, got, want, got.Bounds())
	if err != nil {
		t.Fatal(err)
	}
	gpu, _, err := confirmResidual(t.Context(), evaluator, got, want, got.Bounds())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(cpu-gpu) > 1e-3 {
		t.Fatalf("cpu=%v gpu=%v", cpu, gpu)
	}
}
