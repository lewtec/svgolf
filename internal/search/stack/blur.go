package stack

import (
	"image"
	"image/color"
)

func startSigma(w, h int) int {
	m := w
	if h > m {
		m = h
	}
	if m < 64 {
		return 0
	}
	s := m / 8
	if s > 32 {
		s = 32
	}
	if s < 2 {
		return 0
	}
	return s
}

func wantAt(src *image.NRGBA, scale int) *image.NRGBA {
	if src == nil || scale < 2 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	return upscaleNearest(downscaleArea(src, scale), w, h)
}

// downscaleArea averages each scale×scale block. RGB is weighted by
// alpha so transparent pixels do not bleed black into the edge.
func downscaleArea(src *image.NRGBA, scale int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	sw := (w + scale - 1) / scale
	sh := (h + scale - 1) / scale
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	out := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			var sr, sg, sb, sa, n int
			x0, y0 := x*scale, y*scale
			x1, y1 := x0+scale, y0+scale
			if x1 > w {
				x1 = w
			}
			if y1 > h {
				y1 = h
			}
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.NRGBAAt(b.Min.X+xx, b.Min.Y+yy)
					aa := int(c.A)
					sr += int(c.R) * aa
					sg += int(c.G) * aa
					sb += int(c.B) * aa
					sa += aa
					n++
				}
			}
			px := color.NRGBA{}
			if sa > 0 {
				px.R = uint8(sr / sa)
				px.G = uint8(sg / sa)
				px.B = uint8(sb / sa)
				av := sa / n
				if av > 255 {
					av = 255
				}
				px.A = uint8(av)
			}
			out.SetNRGBA(x, y, px)
		}
	}
	return out
}

func upscaleNearest(src *image.NRGBA, w, h int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if sw < 1 || sh < 1 {
		return out
	}
	for y := 0; y < h; y++ {
		sy := y * sh / h
		for x := 0; x < w; x++ {
			sx := x * sw / w
			out.SetNRGBA(x, y, src.NRGBAAt(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return out
}
