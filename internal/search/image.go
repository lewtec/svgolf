package search

import (
	"image"
	"image/draw"
)

// MaxCanvas is the Render/Encode edge cap. Search a FitCanvas copy above this.
const MaxCanvas = 4096

// FromImage copies img into an origin-zero NRGBA.
func FromImage(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// FitCanvas nearest-neighbor scales so both edges are ≤ max. max≤0 uses MaxCanvas.
func FitCanvas(src *image.NRGBA, max int) *image.NRGBA {
	if src == nil {
		return nil
	}
	if max <= 0 {
		max = MaxCanvas
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w <= max && h <= max && src.Rect.Min == (image.Point{}) {
		return src
	}
	nw, nh := w, h
	if w > max || h > max {
		if w >= h {
			nw = max
			nh = h * max / w
		} else {
			nh = max
			nw = w * max / h
		}
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := src.Rect.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			sx := src.Rect.Min.X + x*w/nw
			dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
		}
	}
	return dst
}
