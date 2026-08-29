package stack

import (
	"image"
	"image/color"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/svg"
)

// hueFamily is leftover grouping for polish: same hue, any value.
// Greys stay in value bins so black type on a grey plate is not one wash.
func hueFamily(c color.NRGBA) int {
	if c.A == 0 {
		return -1
	}
	hue, saturation, value := loss.HSV(c)
	if saturation < 0.08 {
		bin := int(value * 4)
		if bin > 3 {
			bin = 3
		}
		return bin
	}
	return 4 + int(hue/30)%12
}

// fitLinearFill is a 2-stop userSpaceOnUse candidate. Rejected when a
// solid mean is as good, or when the island is two flats (a hard edge)
// rather than a ramp — a smear would cheat those into one wash.
func fitLinearFill(island []pix, want *image.NRGBA) (svg.LinearFill, bool) {
	if want == nil || len(island) < minIsland {
		return svg.LinearFill{}, false
	}
	solid := meanFill(want, island)
	var solidErr float64
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		e := loss.ColorAt(c, solid)
		solidErr += e * e
	}
	box := bbox(island)
	minX, minY := box[0][0], box[0][1]
	maxX, maxY := box[1][0], box[2][1]
	midX := (minX + maxX) / 2
	midY := (minY + maxY) / 2
	axes := [][4]float64{
		{minX, midY, maxX, midY},
		{midX, minY, midX, maxY},
		{minX, minY, maxX, maxY},
		{minX, maxY, maxX, minY},
	}
	var best svg.LinearFill
	bestErr := solidErr
	found := false
	for _, axis := range axes {
		start, end := endQuartileColors(island, want, axis[0], axis[1], axis[2], axis[3])
		if loss.ColorAt(start, end) < 24 {
			continue
		}
		gradient := svg.NewLinearFill(axis[0], axis[1], axis[2], axis[3], start, end)
		if !rampLike(island, want, gradient) {
			continue
		}
		var errSum float64
		for _, p := range island {
			c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
			e := loss.ColorAt(c, gradient.ColorAt(float64(p.x)+0.5, float64(p.y)+0.5))
			errSum += e * e
		}
		if errSum < bestErr {
			bestErr = errSum
			best = gradient
			found = true
		}
	}
	return best, found
}

// rampLike is true when a quarter of the pixels sit closer to the lerp
// than to either stop. Two flats only have that on the AA seam.
func rampLike(island []pix, want *image.NRGBA, gradient svg.LinearFill) bool {
	start, end := gradient.C0(), gradient.C1()
	closerToLerp := 0
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		lerped := gradient.ColorAt(float64(p.x)+0.5, float64(p.y)+0.5)
		toLerp := loss.ColorAt(c, lerped)
		if toLerp < loss.ColorAt(c, start) && toLerp < loss.ColorAt(c, end) {
			closerToLerp++
		}
	}
	return closerToLerp*4 > len(island)
}

func endQuartileColors(island []pix, want *image.NRGBA, x1, y1, x2, y2 float64) (color.NRGBA, color.NRGBA) {
	dx := x2 - x1
	dy := y2 - y1
	length2 := dx*dx + dy*dy
	samples := make([]gradientSample, 0, len(island))
	for _, p := range island {
		parameter := 0.0
		if length2 > 0 {
			parameter = ((float64(p.x)+0.5-x1)*dx + (float64(p.y)+0.5-y1)*dy) / length2
		}
		samples = append(samples, gradientSample{
			parameter: parameter,
			color:     want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y),
		})
	}
	for i := 1; i < len(samples); i++ {
		j := i
		for j > 0 && samples[j].parameter < samples[j-1].parameter {
			samples[j], samples[j-1] = samples[j-1], samples[j]
			j--
		}
	}
	quarter := len(samples) / 4
	if quarter < 1 {
		quarter = 1
	}
	return meanGradientSample(samples[:quarter]), meanGradientSample(samples[len(samples)-quarter:])
}

func meanGradientSample(samples []gradientSample) color.NRGBA {
	if len(samples) == 0 {
		return color.NRGBA{A: 255}
	}
	var sumR, sumG, sumB int
	for _, s := range samples {
		sumR += int(s.color.R)
		sumG += int(s.color.G)
		sumB += int(s.color.B)
	}
	n := len(samples)
	return color.NRGBA{R: uint8(sumR / n), G: uint8(sumG / n), B: uint8(sumB / n), A: 255}
}

type gradientSample struct {
	parameter float64
	color     color.NRGBA
}

func sameLayer(n svg.Node, fill, col color.NRGBA, island []pix, want *image.NRGBA) bool {
	if gradient, ok := n.LinearFill(); ok && want != nil && len(island) > 0 {
		hit := 0
		for _, p := range island {
			c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
			if loss.ColorAt(c, gradient.ColorAt(float64(p.x)+0.5, float64(p.y)+0.5)) < recolorAt {
				hit++
			}
		}
		return hit*2 > len(island)
	}
	return sameObject(fill, col)
}

func layerSample(n svg.Node, fill color.NRGBA, x, y int) color.NRGBA {
	if gradient, ok := n.LinearFill(); ok {
		return gradient.ColorAt(float64(x)+0.5, float64(y)+0.5)
	}
	return fill
}
