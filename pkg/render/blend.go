package render

// tiny-skia 0.12 src/color.rs premultiply_u8 (PNG encode/decode, ColorU8).
func premultiplyU8(c, a uint8) uint8 {
	prod := uint32(c)*uint32(a) + 128
	return uint8((prod + (prod >> 8)) >> 8)
}

// tiny-skia 0.12 src/pipeline/lowp.rs div255 — this is what the raster pipeline uses.
// (v + 255) >> 8  ≈  v/256, not premultiply_u8's v/255.
func div255(v uint32) uint8 {
	return uint8((v + 255) >> 8)
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
	inv := uint32(255 - sa)
	// RGB: tiny-skia lowp s + div255(d * inv(sa)).
	// Alpha: 255 - div255(inv(sa)*inv(da)) — the integer form of
	// 1-(1-sa)(1-da). sa+div255(da*inv(sa)) is 192 for two 128 hairline
	// caps; resvg stores 191 there and also for two 127s (both map to 191
	// only with the complement form).
	return sr + div255(uint32(dr)*inv),
		sg + div255(uint32(dg)*inv),
		sb + div255(uint32(db)*inv),
		255 - div255(inv*uint32(255-da))
}

func scalePremul(r, g, b, a, cov uint8) (uint8, uint8, uint8, uint8) {
	c := uint32(cov)
	return div255(uint32(r) * c), div255(uint32(g) * c), div255(uint32(b) * c), div255(uint32(a) * c)
}
