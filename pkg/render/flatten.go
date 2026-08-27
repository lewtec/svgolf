package render

import "github.com/lewtec/svgolf/pkg/svg"

type pathSegKind uint8

const (
	segMove pathSegKind = iota
	segLine
	segClose
)

type pathSeg struct {
	kind pathSegKind
	x, y float32
}

type path struct {
	segs                   []pathSeg
	minX, minY, maxX, maxY float32
	empty                  bool
}

func (p *path) add(kind pathSegKind, x, y float32) {
	if p.empty {
		p.minX, p.maxX, p.minY, p.maxY = x, x, y, y
		p.empty = false
	} else {
		if x < p.minX {
			p.minX = x
		}
		if x > p.maxX {
			p.maxX = x
		}
		if y < p.minY {
			p.minY = y
		}
		if y > p.maxY {
			p.maxY = y
		}
	}
	p.segs = append(p.segs, pathSeg{kind: kind, x: x, y: y})
}

func flattenRect(r svg.Rect) (path, bool) {
	w := float32(r.Width())
	h := float32(r.Height())
	if w <= 0 || h <= 0 {
		return path{}, false
	}
	rx, ry := r.RX(), r.RY()
	hw := r.Width() / 2
	hh := r.Height() / 2
	if hw >= 0 && rx > hw {
		rx = hw
	}
	if hh >= 0 && ry > hh {
		ry = hh
	}
	if rx != 0 || ry != 0 {
		return path{}, false // rounded: PR 9
	}
	x := float32(r.X())
	y := float32(r.Y())
	var p path
	p.empty = true
	p.add(segMove, x, y)
	p.add(segLine, x+w, y)
	p.add(segLine, x+w, y+h)
	p.add(segLine, x, y+h)
	p.add(segClose, x, y)
	return p, true
}
