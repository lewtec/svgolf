package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"iter"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

const (
	maxPaths   = 256
	stallLimit = 8
	minIsland  = 8
)

// Stack covers a coarse residual region with a simple path, then tightens that same path.
// A new path is accepted on island-local hue drop (global mean traps after a plate).
// Tightening is accepted on global hue drop. Stop after stallLimit failed regions.
type Stack struct{}

var _ search.Search = Stack{}

func init() {
	search.Register("stack", func() search.Search { return Stack{} })
}

func (Stack) Search(ctx context.Context, target *image.NRGBA) iter.Seq2[svg.Document, error] {
	return func(yield func(svg.Document, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(svg.Document{}, err)
			return
		}
		if target == nil {
			yield(svg.Document{}, fmt.Errorf("search: nil pixmap"))
			return
		}
		b := target.Bounds()
		w, h := b.Dx(), b.Dy()
		doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
		got, err := render.Render(doc)
		if err != nil {
			yield(svg.Document{}, err)
			return
		}
		best := loss.Hue(got, target)
		skip := make([]byte, w*h)
		stall := 0
		yielded := false
		for i := 0; i < maxPaths && stall < stallLimit; i++ {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(svg.Document{}, err)
				}
				return
			}
			col, island := largestIsland(got, target, skip)
			need := minIsland
			if n := w * h / 20000; n > need {
				need = n
			}
			if len(island) < need {
				break
			}
			placed := false
			local := hueOn(got, target, island)
			for _, ring := range forms(island) {
				if len(ring) < 3 {
					continue
				}
				var next svg.Document
				if !placed {
					next = doc.Append(filledPath(ring, col).Node())
				} else {
					next = replaceLast(doc, filledPath(ring, col).Node())
				}
				ngot, err := render.Render(next)
				if err != nil {
					yield(svg.Document{}, err)
					return
				}
				if !placed {
					if !(hueOn(ngot, target, island) < local) {
						continue
					}
				} else if !(loss.Hue(ngot, target) < best) {
					continue
				}
				doc, got, best = next, ngot, loss.Hue(ngot, target)
				placed = true
				stall = 0
				yielded = true
				if !yield(doc, nil) {
					return
				}
			}
			for _, p := range island {
				skip[p.y*w+p.x] = 1
			}
			if !placed {
				stall++
			}
		}
		if !yielded {
			yield(doc, nil)
		}
	}
}

func forms(island []pix) [][][2]float64 {
	c := contour(island)
	return [][][2]float64{
		bbox(island),
		convexHull(corners(island)),
		rdp(c, 8),
		rdp(c, 2),
	}
}

func filledPath(ring [][2]float64, col color.NRGBA) svg.Path {
	p := svg.NewPath().MoveTo(ring[0][0], ring[0][1])
	for _, q := range ring[1:] {
		p = p.LineTo(q[0], q[1])
	}
	p = p.Close().WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
	if col.A != 255 {
		p = p.WithFillOpacity(float64(col.A) / 255)
	}
	return p
}

func replaceLast(d svg.Document, n svg.Node) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	if len(kids) > 1 {
		out = out.Append(kids[:len(kids)-1]...)
	}
	return out.Append(n)
}
