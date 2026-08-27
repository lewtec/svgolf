package render

// tiny-skia 0.12 src/color.rs premultiply_u8
func premultiplyU8(c, a uint8) uint8 {
	prod := uint32(c)*uint32(a) + 128
	return uint8((prod + (prod >> 8)) >> 8)
}

// tiny-skia 0.12 PremultipliedColorU8::demultiply; a==0 → 0 (PremultipliedColor::demultiply)
func demultiplyU8(c, a uint8) uint8 {
	if a == 255 {
		return c
	}
	if a == 0 {
		return 0
	}
	return uint8(float64(c)/(float64(a)/255.0) + 0.5)
}

func sourceOver(dr, dg, db, da, sr, sg, sb, sa uint8) (uint8, uint8, uint8, uint8) {
	inv := uint8(255 - sa)
	return sr + premultiplyU8(dr, inv),
		sg + premultiplyU8(dg, inv),
		sb + premultiplyU8(db, inv),
		sa + premultiplyU8(da, inv)
}

func scalePremul(r, g, b, a, cov uint8) (uint8, uint8, uint8, uint8) {
	return premultiplyU8(r, cov), premultiplyU8(g, cov), premultiplyU8(b, cov), premultiplyU8(a, cov)
}
