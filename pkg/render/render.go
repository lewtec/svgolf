package render

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/lewtec/svgolf/pkg/svg"
)

// Render walks the tree and paints an NRGBA pixmap. Identity viewport only.
func Render(d svg.Document) (*image.NRGBA, error) {
	if d.ViewBox().Set() {
		return nil, fmt.Errorf("viewBox not implemented")
	}
	w, h := d.Width(), d.Height()
	if err := checkCanvas(w, h); err != nil {
		return nil, err
	}
	iw, ih := int(w), int(h)
	pm := newPixmap(iw, ih)
	if err := paintNodes(pm, d.Children()); err != nil {
		return nil, err
	}
	return pm.toNRGBA(), nil
}

func checkCanvas(w, h float64) error {
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 || w > 4096 || h > 4096 || w != math.Trunc(w) || h != math.Trunc(h) {
		return fmt.Errorf("render: canvas %v×%v", w, h)
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func paintNodes(pm *pixmap, nodes []svg.Node) error {
	for _, n := range nodes {
		if err := paintNode(pm, n); err != nil {
			return err
		}
	}
	return nil
}

func paintNode(pm *pixmap, n svg.Node) error {
	switch n.Kind() {
	case svg.KindGroup:
		g, _ := n.Group()
		return paintNodes(pm, g.Children())
	case svg.KindRect:
		r, _ := n.Rect()
		return paintRect(pm, r)
	case svg.KindCircle, svg.KindEllipse, svg.KindPolygon:
		return nil
	case svg.KindInvalid:
		return fmt.Errorf("render: invalid node")
	default:
		return fmt.Errorf("render: unknown node")
	}
}

func paintRect(pm *pixmap, r svg.Rect) error {
	p, ok := flattenRect(r)
	if !ok {
		return nil
	}
	col, on := r.Fill()
	if !on {
		return nil
	}
	a := uint8(r.FillOpacity()*255 + 0.5)
	fillPath(pm, p, r.FillRule() != svg.FillEvenOdd, col, a)
	return nil
}

func fillPath(pm *pixmap, p path, nonzero bool, col color.NRGBA, a uint8) {
	pr := premultiplyU8(col.R, a)
	pg := premultiplyU8(col.G, a)
	pb := premultiplyU8(col.B, a)
	bl := &solidBlitter{pm: pm, pr: pr, pg: pg, pb: pb, pa: a}
	fillPathAA(p, nonzero, uint32(pm.w), uint32(pm.h), bl)
}

func (p *pixmap) toNRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, p.w, p.h))
	for i := 0; i < p.w*p.h; i++ {
		pr, pg, pb, pa := p.pix[i*4], p.pix[i*4+1], p.pix[i*4+2], p.pix[i*4+3]
		off := i * 4
		img.Pix[off] = demultiplyU8(pr, pa)
		img.Pix[off+1] = demultiplyU8(pg, pa)
		img.Pix[off+2] = demultiplyU8(pb, pa)
		img.Pix[off+3] = pa
	}
	return img
}
