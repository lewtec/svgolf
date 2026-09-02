package stack

import (
	"image"
	"image/color"

	"github.com/lewtec/svgolf/internal/loss"
)

// DebugFrames is the leftover view. Heat is leftoverHeat 0-1
// (black match, red miss). Island is every leftover-band residual
// (white), the color-glow leftover hypothesis (white), and the
// inscribed triangle (orange).
func DebugFrames(got, want *image.NRGBA, blob, fitted []pix) (heat, island *image.NRGBA) {
	if want == nil {
		return nil, nil
	}
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	heat = image.NewNRGBA(b)
	island = image.NewNRGBA(b)
	if got == nil {
		got = image.NewNRGBA(b)
	}
	gotP := loss.NewPlane(got)
	wantP := loss.NewPlane(want)
	gotP.Ensure()
	wantP.Ensure()
	field := leftoverHeat(gotP, wantP, w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(field[y*w+x] * 255)
			heat.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{R: v, B: 255 - v, A: 255})
			c := color.NRGBA{A: 255}
			if colorErrHSV(gotP.At(x, y), wantP.At(x, y)) > minErr {
				c = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			}
			island.SetNRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
	s := &world{got: got, want: want, gotP: gotP, wantP: wantP, w: w, h: h}
	for _, left := range s.leftovers() {
		if left.region {
			continue
		}
		for _, p := range left.island {
			if p.x >= 0 && p.y >= 0 && p.x < w && p.y < h {
				island.SetNRGBA(b.Min.X+p.x, b.Min.Y+p.y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	for _, p := range blob {
		if p.x >= 0 && p.y >= 0 && p.x < w && p.y < h {
			island.SetNRGBA(b.Min.X+p.x, b.Min.Y+p.y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for _, p := range fitted {
		if p.x >= 0 && p.y >= 0 && p.x < w && p.y < h {
			island.SetNRGBA(b.Min.X+p.x, b.Min.Y+p.y, color.NRGBA{R: 255, G: 80, B: 0, A: 255})
		}
	}
	return heat, island
}
