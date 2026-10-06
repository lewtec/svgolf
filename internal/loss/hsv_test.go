package loss

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestByteUnitMatchesDivision(t *testing.T) {
	for i := 0; i < 256; i++ {
		if byteUnit[i] != float64(i)/255 {
			t.Fatalf("byteUnit[%d]=%v", i, byteUnit[i])
		}
	}
}

func TestHSVMatchesChannelDivision(t *testing.T) {
	var c color.NRGBA
	c.A = 255
	for r := 0; r < 256; r += 15 {
		for g := 0; g < 256; g += 15 {
			for b := 0; b < 256; b += 15 {
				c.R, c.G, c.B = uint8(r), uint8(g), uint8(b)
				h, s, v := hsv(c)
				wh, ws, wv := hsvByDivision(c)
				if h != wh || s != ws || v != wv {
					t.Fatalf("%v hsv=%v %v %v division=%v %v %v", c, h, s, v, wh, ws, wv)
				}
			}
		}
	}
}

func hsvByDivision(c color.NRGBA) (h, s, v float64) {
	r := float64(c.R) / 255
	g := float64(c.G) / 255
	b := float64(c.B) / 255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	v = max
	if max == 0 {
		return 0, 0, 0
	}
	s = (max - min) / max
	if max == min {
		return 0, s, v
	}
	switch max {
	case r:
		h = 60 * (g - b) / (max - min)
		if h < 0 {
			h += 360
		}
	case g:
		h = 60*(b-r)/(max-min) + 120
	default:
		h = 60*(r-g)/(max-min) + 240
	}
	return h, s, v
}

func TestEnsureRectSkipsConvertedPlane(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
	p := NewPlane(img)
	p.Ensure()
	p.pix[0].H = 123
	p.EnsureRect(img.Rect)
	if p.pix[0].H != 123 {
		t.Fatalf("EnsureRect rewrote a converted plane: %v", p.pix[0].H)
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	p.Reset(img)
	p.EnsureRect(image.Rect(0, 0, 1, 1))
	want, _, _ := hsv(color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	if p.pix == nil || p.pix[0].H != want {
		t.Fatalf("Reset+EnsureRect pix=%v want H=%v", p.pix, want)
	}
}

func TestColorAtHSVMatchesColorAt(t *testing.T) {
	pairs := [][2]color.NRGBA{
		{{R: 255, A: 255}, {R: 255, A: 255}},
		{{R: 80, A: 255}, {R: 255, A: 255}},
		{{R: 255, A: 255}, {B: 255, A: 255}},
		{{A: 255}, {R: 255, G: 255, B: 255, A: 255}},
	}
	for _, p := range pairs {
		a, b := ColorAt(p[0], p[1]), ColorAtHSV(HSVOf(p[0]), HSVOf(p[1]))
		if a != b {
			t.Fatalf("ColorAt=%v ColorAtHSV=%v for %+v %+v", a, b, p[0], p[1])
		}
	}
}

func TestPlaneEnsureRectLeavesRestUnset(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(3, 1, color.NRGBA{B: 255, A: 255})
	p := NewPlane(img)
	p.EnsureRect(image.Rect(0, 0, 1, 1))
	if p.At(0, 0).H > 10 {
		t.Fatalf("red hsv=%+v", p.At(0, 0))
	}
	if p.pix[3+1*4].A != 0 {
		t.Fatal("EnsureRect converted the whole plane")
	}
}

func TestAcquireReusesBuffer(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := 0; i < planeWorkers(); i++ {
		p := Acquire(img)
		p.Ensure()
		Release(p)
	}
	allocs := testing.AllocsPerRun(50, func() {
		p := Acquire(img)
		p.Ensure()
		Release(p)
	})
	if allocs != 0 {
		t.Fatalf("Acquire+Ensure allocs=%v want 0 after the pool is warm", allocs)
	}
}

func TestPlaneResetReusesBuffer(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	p := NewPlane(img)
	p.Ensure()
	buf := p.pix
	p.Reset(img)
	p.EnsureRect(image.Rect(0, 0, 2, 2))
	if cap(p.pix) != cap(buf) {
		t.Fatalf("EnsureRect cap=%d want %d", cap(p.pix), cap(buf))
	}
	if p.pix[3+1*8].A != 0 {
		t.Fatal("reused EnsureRect did not clear the rest")
	}
	p.Reset(img)
	p.Ensure()
	if cap(p.pix) != cap(buf) {
		t.Fatalf("Ensure cap=%d want %d", cap(p.pix), cap(buf))
	}
}

func TestPlaneConvertMatchesNRGBAAt(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(2, 1, color.NRGBA{B: 200, G: 10, A: 255})
	p := NewPlane(img)
	p.Ensure()
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			want := HSVOf(img.NRGBAAt(x, y))
			got := p.At(x, y)
			if got != want {
				t.Fatalf("(%d,%d) %+v want %+v", x, y, got, want)
			}
		}
	}
}

func TestPlaneConvertLargeMatchesHSV(t *testing.T) {
	const n = 200
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(i)
		img.Pix[i+1] = uint8(i * 3)
		img.Pix[i+2] = 80
		img.Pix[i+3] = 255
	}
	p := NewPlane(img)
	p.Ensure()
	for i, got := range p.Slice() {
		x, y := i%n, i/n
		want := HSVOf(img.NRGBAAt(x, y))
		if got != want {
			t.Fatalf("(%d,%d) %+v want %+v", x, y, got, want)
		}
	}
}

func TestPlaneConvertsOnce(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	p := NewPlane(img)
	p.Ensure()
	if p.At(0, 0).H > 10 {
		t.Fatalf("red hsv=%+v", p.At(0, 0))
	}
	img.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	if p.At(0, 0).H > 10 {
		t.Fatal("plane converted again without Reset")
	}
	p.Reset(img)
	if p.At(0, 0).H < 200 {
		t.Fatalf("after Reset hsv=%+v want blue", p.At(0, 0))
	}
}
