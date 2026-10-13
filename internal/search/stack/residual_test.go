package stack

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/stretchr/testify/require"
)

func TestResidualMatchesScore(t *testing.T) {
	got := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	want := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			got.SetNRGBA(x, y, color.NRGBA{R: 40, G: 80, B: 200, A: 255})
			want.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + x*20), G: 80, B: 200, A: 255})
		}
	}
	rect := got.Bounds()
	device, exact, err := confirmResidual(t.Context(), ndarray.CPU, got, want, rect)
	if err != nil {
		t.Fatal(err)
	}
	if !deviceAgrees(device, exact, rect.Dx()*rect.Dy()) {
		t.Fatalf("device=%v exact=%v", device, exact)
	}
	if math.Abs(device-exact) > 1 {
		t.Fatalf("device=%v exact=%v differ by more than one", device, exact)
	}
}

func TestResidualHoleAndMiss(t *testing.T) {
	got := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	want := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	got.SetNRGBA(0, 0, color.NRGBA{})
	want.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	got.SetNRGBA(1, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	want.SetNRGBA(1, 0, color.NRGBA{A: 0})
	device, exact, err := confirmResidual(t.Context(), ndarray.CPU, got, want, got.Bounds())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(device-exact) > 1 {
		t.Fatalf("device=%v exact=%v", device, exact)
	}
}

func TestBeamKeepsTheSingleBestDocument(t *testing.T) {
	b := newBeam(snapshot{id: 1, errSum: 50})
	for i, errSum := range []float64{40, 30, 20, 10, 5} {
		require.Truef(t, b.offer(snapshot{id: i + 2, errSum: errSum}), "offer %d", i)
	}
	list := b.list()
	require.Len(t, list, beamWidth)
	require.Equal(t, 5.0, list[0].errSum)
	require.False(t, b.offer(snapshot{id: 99, errSum: 45}))
	first, ok := b.claim(OpTriangle)
	require.True(t, ok)
	second, ok := b.claim(OpRectangle)
	require.True(t, ok)
	require.Equal(t, first.id, second.id)
}
