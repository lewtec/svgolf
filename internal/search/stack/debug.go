package stack

import (
	"image"
	"image/color"

	"github.com/lewtec/svgolf/internal/loss"
)

// DebugFrames is what leftover add sees. Heat is leftoverHeat 0-1
// (black match, red miss). Island is the leftover mask: white
// pixels are inside the set largestTriangle inscribes, orange is
// the inscribed triangle when one was fitted. blob is the leftover
// this step used; if nil, hottest() is computed from the residual
// (debug-only).
func DebugFrames(got, want *image.NRGBA, blob, fitted []pix) (heat, island *image.NRGBA) {
	if want == nil {
		return nil, nil
	}
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	heat = image.NewNRGBA(b)
	island = image.NewNRGBA(b)
	s := &world{got: got, want: want}
	if got == nil {
		s.got = image.NewNRGBA(b)
	}
	if blob == nil {
		_, blob = s.hottest()
	} else {
		s.gotP = loss.NewPlane(s.got)
		s.wantP = loss.NewPlane(want)
	}
	gotP, wantP := s.gotP, s.wantP
	gotP.Ensure()
	wantP.Ensure()
	field := leftoverHeat(gotP, wantP, w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(field[y*w+x] * 255)
			heat.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{R: v, B: 255 - v, A: 255})
			island.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{A: 255})
		}
	}
	paint := func(pts []pix, c color.NRGBA) {
		for _, p := range pts {
			if p.x >= 0 && p.y >= 0 && p.x < w && p.y < h {
				island.SetNRGBA(b.Min.X+p.x, b.Min.Y+p.y, c)
			}
		}
	}
	paint(blob, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	paint(fitted, color.NRGBA{R: 255, G: 80, B: 0, A: 255})
	return heat, island
}
