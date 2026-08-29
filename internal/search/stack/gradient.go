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
	parameterAt := func(p pix) float64 {
		if length2 == 0 {
			return 0
		}
		return ((float64(p.x)+0.5-x1)*dx + (float64(p.y)+0.5-y1)*dy) / length2
	}
	minP, maxP := parameterAt(island[0]), parameterAt(island[0])
	for _, p := range island[1:] {
		t := parameterAt(p)
		if t < minP {
			minP = t
		}
		if t > maxP {
			maxP = t
		}
	}
	lo := minP + 0.25*(maxP-minP)
	hi := maxP - 0.25*(maxP-minP)
	var startR, startG, startB, startN int
	var endR, endG, endB, endN int
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		t := parameterAt(p)
		if t <= lo {
			startR += int(c.R)
			startG += int(c.G)
			startB += int(c.B)
			startN++
		}
		if t >= hi {
			endR += int(c.R)
			endG += int(c.G)
			endB += int(c.B)
			endN++
		}
	}
	if startN == 0 || endN == 0 {
		return meanFill(want, island), meanFill(want, island)
	}
	return color.NRGBA{R: uint8(startR / startN), G: uint8(startG / startN), B: uint8(startB / startN), A: 255},
		color.NRGBA{R: uint8(endR / endN), G: uint8(endG / endN), B: uint8(endB / endN), A: 255}
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
