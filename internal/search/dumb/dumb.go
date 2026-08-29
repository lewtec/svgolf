package dumb

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"iter"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Dumb is the one-shot Search adapter: one rect per palette color, concentric 75%.
type Dumb struct {
	Colors int // 0 = auto, cap 8
}

var _ search.Search = Dumb{}

func init() {
	search.Register("dumb", func() search.Search { return Dumb{} })
}

func (d Dumb) Search(ctx context.Context, target *image.NRGBA) iter.Seq2[search.Epoch, error] {
	return func(yield func(search.Epoch, error) bool) {
		doc, err := d.epoch(ctx, target)
		yield(search.Epoch{Document: doc, Scale: 1}, err)
	}
}

func (d Dumb) epoch(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	b := target.Bounds()
	w, h := b.Dx(), b.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	pal, err := auto(target, d.Colors)
	if err != nil {
		return doc, err
	}
	if len(pal) == 0 {
		return doc, nil
	}
	plate := true
	var minX, minY, maxX, maxY int
	first := true
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			aa := target.NRGBAAt(x, y).A
			if aa != 255 {
				plate = false
			}
			if aa == 0 {
				continue
			}
			ox, oy := x-b.Min.X, y-b.Min.Y
			if first {
				minX, minY, maxX, maxY = ox, oy, ox+1, oy+1
				first = false
			} else {
				if ox < minX {
					minX = ox
				}
				if oy < minY {
					minY = oy
				}
				if ox+1 > maxX {
					maxX = ox + 1
				}
				if oy+1 > maxY {
					maxY = oy + 1
				}
			}
		}
	}
	var x, y, rw, rh float64
	if plate || first {
		x, y, rw, rh = 0, 0, float64(w), float64(h)
	} else {
		x, y = float64(minX), float64(minY)
		rw, rh = float64(maxX-minX), float64(maxY-minY)
	}
	for _, c := range pal {
		r := svg.NewRect().WithX(x).WithY(y).WithWidth(rw).WithHeight(rh).WithFill(color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255})
		if c.A != 255 {
			r = r.WithFillOpacity(float64(c.A) / 255)
		}
		doc = doc.Append(r.Node())
		nw, nh := rw*0.75, rh*0.75
		x = x + (rw-nw)/2
		y = y + (rh-nh)/2
		rw, rh = nw, nh
	}
	return doc, nil
}
