package stack

import (
	"math"
	"sort"
)

type maskVertex struct {
	p        [2]float64
	response float64
}

// maskVertices are leftover-outline corners. Shi-Tomasi
// min-eigenvalue on the binary mask; a vertex is a local
// maximum along the walk. Score ranks plates built from them.
func maskVertices(island []pix) []maskVertex {
	if len(island) == 0 {
		return nil
	}
	border := contour(island)
	if len(border) < 4 {
		return nil
	}
	set := pixSet(island)
	defer releaseBits(set)
	ring := coverRing(island)
	n := len(border)
	resp := make([]float64, n)
	for i, q := range border {
		resp[i] = shiTomasi(set, pix{int(q[0]), int(q[1])})
	}
	var verts []maskVertex
	for i := 0; i < n; i++ {
		if resp[i] <= 0 || resp[i] <= resp[(i-1+n)%n] || resp[i] < resp[(i+1)%n] {
			continue
		}
		q := nearest(ring, border[i])
		verts = append(verts, maskVertex{q, resp[i]})
	}
	return verts
}

func shiTomasi(set *pixBits, p pix) float64 {
	var A, B, C float64
	bit := func(x, y int) float64 {
		if set.has(pix{x, y}) {
			return 1
		}
		return 0
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := p.x+dx, p.y+dy
			ix := bit(x+1, y) - bit(x-1, y)
			iy := bit(x, y+1) - bit(x, y-1)
			A += ix * ix
			B += ix * iy
			C += iy * iy
		}
	}
	tr := A + C
	det := A*C - B*B
	disc := tr*tr/4 - det
	if disc < 0 {
		disc = 0
	}
	return tr/2 - math.Sqrt(disc)
}

// oneMaskRectangle is the leftover plate from the four
// strongest mask vertices, fan-ordered.
func oneMaskRectangle(island []pix) [][2]float64 {
	verts := maskVertices(island)
	if len(verts) < 4 {
		return nil
	}
	sort.Slice(verts, func(i, j int) bool { return verts[i].response > verts[j].response })
	var pts [][2]float64
	seen := map[[2]float64]bool{}
	for _, v := range verts {
		if seen[v.p] {
			continue
		}
		seen[v.p] = true
		pts = append(pts, v.p)
		if len(pts) == 4 {
			break
		}
	}
	if len(pts) < 4 {
		return nil
	}
	ring := uncross(fanOrder(pts))
	if len(ring) != 4 || quadArea2(ring) == 0 {
		return nil
	}
	return ring
}

func quadArea2(ring [][2]float64) float64 {
	if len(ring) < 4 {
		return 0
	}
	return triangleArea2(ring[0], ring[1], ring[2]) + triangleArea2(ring[0], ring[2], ring[3])
}

func rectanglePix(ring [][2]float64) []pix {
	if len(ring) < 4 {
		return nil
	}
	minX, maxX := ring[0][0], ring[0][0]
	minY, maxY := ring[0][1], ring[0][1]
	for _, p := range ring[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	var out []pix
	for y := int(minY); y < int(maxY)+1; y++ {
		for x := int(minX); x < int(maxX)+1; x++ {
			if pointInRing(ring, float64(x)+0.5, float64(y)+0.5) {
				out = append(out, pix{x, y})
			}
		}
	}
	return out
}
