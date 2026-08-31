package loss

import (
	"image"
	"image/color"
	"sync"
)

// Pix is one pixel after a single RGB conversion. Hue in [0,360),
// saturation and value in [0,1]. A is the source alpha.
type Pix struct {
	H, S, V float64
	A       uint8
}

// HSVOf converts one NRGBA pixel.
func HSVOf(c color.NRGBA) Pix {
	h, s, v := hsv(c)
	return Pix{H: h, S: s, V: v, A: c.A}
}

// Plane is an image converted to HSV once. Search holds one for want
// (immutable) and one for got (Reset after each Render).
type Plane struct {
	img  *image.NRGBA
	once sync.Once
	pix  []Pix
	buf  []Pix
}

// NewPlane wraps img. Convert runs on the first At / Ensure.
func NewPlane(img *image.NRGBA) *Plane {
	if img == nil {
		return nil
	}
	return &Plane{img: img}
}

// Image is the source pixmap.
func (p *Plane) Image() *image.NRGBA {
	if p == nil {
		return nil
	}
	return p.img
}

// Ensure converts the whole pixmap if it has not been converted yet.
func (p *Plane) Ensure() {
	if p == nil || p.img == nil {
		return
	}
	p.once.Do(p.convert)
}

// Reset points the plane at a new pixmap and forgets the table.
// The backing buffer is kept for the next Ensure / EnsureRect.
func (p *Plane) Reset(img *image.NRGBA) {
	if p == nil {
		return
	}
	p.img = img
	p.once = sync.Once{}
	p.pix = nil
}

// At is the HSV pixel at (x,y) in image coordinates.
func (p *Plane) At(x, y int) Pix {
	if p == nil || p.img == nil {
		return Pix{}
	}
	if p.pix == nil {
		p.Ensure()
	}
	if p.pix == nil {
		return Pix{}
	}
	b := p.img.Rect
	return p.pix[(y-b.Min.Y)*b.Dx()+(x-b.Min.X)]
}

// EnsureRect converts r. The rest of the table stays unset until Ensure.
func (p *Plane) EnsureRect(r image.Rectangle) {
	if p == nil || p.img == nil {
		return
	}
	b := p.img.Rect
	r = r.Intersect(b)
	if r.Empty() {
		return
	}
	if p.pix == nil {
		p.pix = p.growPix(b.Dx()*b.Dy(), true)
	}
	src := p.img.Pix
	stride := p.img.Stride
	w := b.Dx()
	minX := b.Min.X
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := (y - b.Min.Y) * w
		off := (y-b.Min.Y)*stride + (r.Min.X-minX)*4
		for x := r.Min.X; x < r.Max.X; x++ {
			p.pix[row+(x-minX)] = HSVOf(color.NRGBA{R: src[off], G: src[off+1], B: src[off+2], A: src[off+3]})
			off += 4
		}
	}
}

func (p *Plane) growPix(n int, zero bool) []Pix {
	if cap(p.buf) < n {
		p.buf = make([]Pix, n)
	} else {
		p.buf = p.buf[:n]
		if zero {
			clear(p.buf)
		}
	}
	return p.buf
}

func (p *Plane) convert() {
	b := p.img.Rect
	src := p.img.Pix
	stride := p.img.Stride
	w, h := b.Dx(), b.Dy()
	p.pix = p.growPix(w*h, false)
	i := 0
	for y := 0; y < h; y++ {
		off := y * stride
		for x := 0; x < w; x++ {
			p.pix[i] = HSVOf(color.NRGBA{R: src[off], G: src[off+1], B: src[off+2], A: src[off+3]})
			off += 4
			i++
		}
	}
}

// Slice is the row-major HSV table. Call Ensure or EnsureRect first.
func (p *Plane) Slice() []Pix {
	if p == nil {
		return nil
	}
	return p.pix
}

// ColorAtHSV is ColorAt on already-converted pixels.
func ColorAtHSV(got, want Pix) float64 {
	dv := absF(got.V-want.V) * 180
	ds := absF(got.S-want.S) * 180
	if got.S < satMin && want.S < satMin {
		return dv
	}
	if got.S < satMin || want.S < satMin {
		return 180
	}
	return max(hueDelta(got.H, want.H), ds, dv)
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
