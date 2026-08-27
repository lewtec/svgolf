package verify

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/lewtec/svgolf/internal/resvg"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

type Result struct {
	Match           bool
	Ours            *image.NRGBA
	Oracle          *image.NRGBA
	Diff            *image.NRGBA
	DifferingPixels int
	EncodeDrift     bool
}

func Compare(ours, oracle *image.NRGBA) (Result, error) {
	r := Result{Ours: ours, Oracle: oracle}
	if ours == nil || oracle == nil {
		return r, fmt.Errorf("verify: nil pixmap")
	}
	if !ours.Rect.Eq(oracle.Rect) {
		r.DifferingPixels = -1
		return r, nil
	}
	if ours.Stride == oracle.Stride && ours.Rect.Min == (image.Point{}) && oracle.Rect.Min == (image.Point{}) &&
		bytes.Equal(ours.Pix, oracle.Pix) {
		r.Match = true
		return r, nil
	}
	w, h := ours.Rect.Dx(), ours.Rect.Dy()
	diff := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := ours.NRGBAAt(ours.Rect.Min.X+x, ours.Rect.Min.Y+y)
			q := oracle.NRGBAAt(oracle.Rect.Min.X+x, oracle.Rect.Min.Y+y)
			if o == q {
				continue
			}
			n++
			i := diff.PixOffset(x, y)
			diff.Pix[i] = absDiff(o.R, q.R)
			diff.Pix[i+1] = absDiff(o.G, q.G)
			diff.Pix[i+2] = absDiff(o.B, q.B)
			diff.Pix[i+3] = 255
		}
	}
	r.DifferingPixels = n
	r.Match = n == 0
	if n > 0 {
		r.Diff = diff
	}
	return r, nil
}

func File(ctx context.Context, path string) (Result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	tree, err := svg.Parse(bytes.NewReader(raw))
	if err != nil {
		return Result{}, err
	}
	ours, err := render.Render(tree)
	if err != nil {
		return Result{}, err
	}
	oracle, err := resvg.Render(ctx, raw)
	if err != nil {
		return Result{}, err
	}
	r, err := Compare(ours, oracle)
	if err != nil {
		return r, err
	}
	enc, err := svg.EncodeToString(tree)
	if err != nil {
		return r, err
	}
	encOracle, err := resvg.Render(ctx, []byte(enc))
	if err != nil {
		return r, err
	}
	drift, err := Compare(encOracle, oracle)
	if err != nil {
		return r, err
	}
	if r.Match && !drift.Match {
		r.EncodeDrift = true
		r.Match = false
		if r.DifferingPixels == 0 {
			r.DifferingPixels = drift.DifferingPixels
			r.Diff = drift.Diff
		}
	}
	return r, nil
}

func WriteDiff(path string, diff *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, diff)
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
