package render

import "image/color"

type gradientBlitter struct {
	pm     *pixmap
	x1, y1 float32
	x2, y2 float32
	dx, dy float32
	inv    float32
	c0, c1 color.NRGBA
	a      uint8
}

func (b *gradientBlitter) sample(x, y float32) (pr, pg, pb, pa uint8) {
	t := float32(1)
	if b.inv > 0 {
		t = ((x-b.x1)*b.dx + (y-b.y1)*b.dy) * b.inv
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
	}
	red := lerp8(b.c0.R, b.c1.R, t)
	green := lerp8(b.c0.G, b.c1.G, t)
	blue := lerp8(b.c0.B, b.c1.B, t)
	pr = div255(uint32(red) * uint32(b.a))
	pg = div255(uint32(green) * uint32(b.a))
	pb = div255(uint32(blue) * uint32(b.a))
	return pr, pg, pb, b.a
}

func lerp8(a, b uint8, t float32) uint8 {
	return uint8(float32(a) + (float32(b)-float32(a))*t + 0.5)
}

func (b *gradientBlitter) blitH(x, y, width uint32) {
	fy := float32(y) + 0.5
	for i := uint32(0); i < width; i++ {
		fx := float32(x+i) + 0.5
		pr, pg, pb, pa := b.sample(fx, fy)
		b.pm.blend(int(x+i), int(y), pr, pg, pb, pa)
	}
}

func (b *gradientBlitter) blitAntiH(x, y uint32, alpha []uint8, runs []uint16) {
	i := 0
	px := x
	fy := float32(y) + 0.5
	for {
		n := runs[i]
		if n == 0 {
			return
		}
		a := alpha[i]
		if a != 0 {
			for k := uint16(0); k < n; k++ {
				fx := float32(px+uint32(k)) + 0.5
				pr, pg, pb, pa := b.sample(fx, fy)
				sr, sg, sb, sa := scalePremul(pr, pg, pb, pa, a)
				b.pm.blend(int(px+uint32(k)), int(y), sr, sg, sb, sa)
			}
		}
		px += uint32(n)
		i += int(n)
	}
}
