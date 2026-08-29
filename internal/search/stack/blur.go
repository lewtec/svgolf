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
	s := m / 4
	if s > 32 {
		s = 32
	}
	if s < 1 {
		return 0
	}
	return s
}

func wantAt(src *image.NRGBA, sigma int) *image.NRGBA {
	if src == nil || sigma < 1 {
		return src
	}
	return boxBlur(src, sigma)
}

// boxBlur is a separable box filter of radius r. RGB is weighted by alpha
// so transparent pixels do not bleed black into the edge.
func boxBlur(src *image.NRGBA, r int) *image.NRGBA {
	if src == nil || r < 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tmp := image.NewNRGBA(b)
	out := image.NewNRGBA(b)
	blur1(src, tmp, w, h, r, true)
	blur1(tmp, out, w, h, r, false)
	return out
}

func blur1(src, dst *image.NRGBA, w, h, r int, horiz bool) {
	span := w
	if !horiz {
		span = h
	}
	outer := h
	if !horiz {
		outer = w
	}
	win := 2*r + 1
	for o := 0; o < outer; o++ {
		var sr, sg, sb, sa int
		for i := -r; i <= r; i++ {
			cr, cg, cb, ca := sample(src, w, h, o, i, horiz)
			sr += cr
			sg += cg
			sb += cb
			sa += ca
		}
		for i := 0; i < span; i++ {
			put(dst, o, i, horiz, sr, sg, sb, sa, win)
			or, og, ob, oa := sample(src, w, h, o, i-r, horiz)
			nr, ng, nb, na := sample(src, w, h, o, i+r+1, horiz)
			sr += nr - or
			sg += ng - og
			sb += nb - ob
			sa += na - oa
		}
	}
}

func sample(src *image.NRGBA, w, h, o, i int, horiz bool) (r, g, b, a int) {
	if i < 0 {
		i = 0
	}
	var x, y int
	if horiz {
		if i >= w {
			i = w - 1
		}
		x, y = i, o
	} else {
		if i >= h {
			i = h - 1
		}
		x, y = o, i
	}
	c := src.NRGBAAt(src.Rect.Min.X+x, src.Rect.Min.Y+y)
	aa := int(c.A)
	return int(c.R) * aa, int(c.G) * aa, int(c.B) * aa, aa
}

func put(dst *image.NRGBA, o, i int, horiz bool, sr, sg, sb, sa, win int) {
	var x, y int
	if horiz {
		x, y = i, o
	} else {
		x, y = o, i
	}
	c := color.NRGBA{}
	if sa > 0 {
		c.R = uint8(sr / sa)
		c.G = uint8(sg / sa)
		c.B = uint8(sb / sa)
		av := sa / win
		if av > 255 {
			av = 255
		}
		c.A = uint8(av)
	}
	dst.SetNRGBA(dst.Rect.Min.X+x, dst.Rect.Min.Y+y, c)
}
