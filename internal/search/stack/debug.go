package stack

import (
	"image"
	"image/color"

	"github.com/lewtec/svgolf/internal/loss"
)

// DebugFrames is the leftover view hottest() uses: heat is ColorAt
// 0–180 (black match, red miss). blob is the leftover this step used;
// if nil, hottest() is computed without skip (debug-only).
func DebugFrames(got, want *image.NRGBA, blob []pix) (heat, island *image.NRGBA) {
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
	mark := make([]byte, w*h)
	for _, p := range blob {
		if p.x >= 0 && p.y >= 0 && p.x < w && p.y < h {
			mark[p.y*w+p.x] = 1
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			e := colorErrHSV(gotP.At(x, y), wantP.At(x, y))
			v := uint8(e / 180 * 255)
			heat.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{R: v, B: 255 - v, A: 255})
			c := want.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if mark[y*w+x] != 0 {
				c = color.NRGBA{R: 255, G: 80, B: 0, A: 255}
			}
			island.SetNRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
	return heat, island
}
