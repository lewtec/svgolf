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
		p.pix = make([]Pix, b.Dx()*b.Dy())
	}
	w := b.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := (y - b.Min.Y) * w
		for x := r.Min.X; x < r.Max.X; x++ {
			p.pix[row+(x-b.Min.X)] = HSVOf(p.img.NRGBAAt(x, y))
		}
	}
}

func (p *Plane) convert() {
	b := p.img.Rect
	n := b.Dx() * b.Dy()
	p.pix = make([]Pix, n)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p.pix[i] = HSVOf(p.img.NRGBAAt(x, y))
			i++
		}
	}
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
