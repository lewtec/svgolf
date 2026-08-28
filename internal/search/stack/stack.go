package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"iter"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

const (
	maxPaths   = 512
	stallLimit = 24
	minIsland  = 8
)

// Stack covers a leftover region with a simple path, then tightens that same path.
// Accept if Score drops; on a tie, fewer path commands. Stop after stallLimit failed regions.
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
			if len(island) < minIsland {
				break
			}
			placed := false
			for _, cand := range formPaths(island, col) {
				var next svg.Document
				if !placed {
					next = doc.Append(cand.Node())
				} else {
					next = replaceLast(doc, cand.Node())
				}
				ngot, err := render.Render(next)
				if err != nil {
					yield(svg.Document{}, err)
					return
				}
				if !accept(got, ngot, target, doc, cand.Node(), placed) {
					continue
				}
				doc, got = next, ngot
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

func accept(got, ngot, want *image.NRGBA, cur svg.Document, cand svg.Node, placed bool) bool {
	n := len(cur.Children())
	nn := n
	if !placed {
		nn++
	}
	a, b := Score(got, want, n), Score(ngot, want, nn)
	if b < a {
		return true
	}
	if b > a || !placed {
		return false
	}
	kids := cur.Children()
	if len(kids) == 0 {
		return false
	}
	return pathLen(cand) < pathLen(kids[len(kids)-1])
}

func formPaths(island []pix, col color.NRGBA) []svg.Path {
	bb := bbox(island)
	boxA := (bb[1][0] - bb[0][0]) * (bb[2][1] - bb[1][1])
	c := contour(island)
	if boxA > 2*float64(len(island)) {
		return []svg.Path{filledPath(rdp(c, 4), col), filledPath(rdp(c, 1), col)}
	}
	out := []svg.Path{filledPath(bb, col)}
	if cx, cy, rx, ry, ok := fitEllipse(island); ok {
		out = append(out, filledEllipse(cx, cy, rx, ry, col))
		if r := (rx + ry) / 2; r >= 1 {
			out = append(out, filledEllipse(cx, cy, r, r, col))
		}
	}
	out = append(out,
		filledPath(convexHull(corners(island)), col),
		filledPath(rdp(c, 8), col),
		filledPath(rdp(c, 2), col),
	)
	return out
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
