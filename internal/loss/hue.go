package loss

import (
	"image"
	"image/color"
	"math"
)

const satMin = 0.08

// Hue is the mean circular HSV hue distance in degrees (0..180) on scored pixels.
// Unsaturated pixels compare value instead (black/white have no hue).
func Hue(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
	}
	var sum float64
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			sum += HueAt(got.NRGBAAt(x, y), q)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// HueAt is circular hue distance in degrees (0..180).
func HueAt(got, want color.NRGBA) float64 {
	hg, sg, vg := hsv(got)
	hw, sw, vw := hsv(want)
	if sg < satMin && sw < satMin {
		return math.Abs(vg-vw) * 180
	}
	if sg < satMin || sw < satMin {
		return 180
	}
	return hueDelta(hg, hw)
}

// ColorAt is HSV distance 0..180. HueAt ignores value on saturated
// pixels, so dark and light navy look like a match.
func ColorAt(got, want color.NRGBA) float64 {
	return ColorAtHSV(HSVOf(got), HSVOf(want))
}

func hueDelta(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// HSV is hue in [0,360), saturation and value in [0,1].
func HSV(c color.NRGBA) (h, s, v float64) { return hsv(c) }

// byteUnit is float64(i)/255. Channel bytes never produce a NaN,
// so the table matches the division for every RGB triple.
var byteUnit = func() (t [256]float64) {
	for i := range t {
		t[i] = float64(i) / 255
	}
	return t
}()

func hsv(c color.NRGBA) (h, s, v float64) {
	r := byteUnit[c.R]
	g := byteUnit[c.G]
	b := byteUnit[c.B]
	max := r
	if g > max {
		max = g
	}
	if b > max {
		max = b
	}
	min := r
	if g < min {
		min = g
	}
	if b < min {
		min = b
	}
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
