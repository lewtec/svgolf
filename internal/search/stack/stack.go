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
	minErr     = 8
)

// Stack covers leftover regions. Each island keeps the best of box / ellipse /
// polygon (evenodd if it has holes). Accept if Score drops; on a tie, fewer
// commands. Stop after stallLimit failed regions.
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
		n := 0
		for n < maxPaths && stall < stallLimit {
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
			if transparentIsland(target, island) || thinIsland(island) {
				for _, p := range island {
					skip[p.y*w+p.x] = 1
				}
				continue
			}
			if len(island) < minIsland+2*n {
				break
			}
			placed := false
			var last svg.Node
			for _, cand := range formPaths(island, col) {
				var next svg.Document
				if !placed {
					next = doc.Append(cand.Node())
				} else {
					next = replaceAt(doc, n, cand.Node())
				}
				ngot, err := render.Render(next)
				if err != nil {
					yield(svg.Document{}, err)
					return
				}
				nn := n
				if !placed {
					nn++
				}
				if !accept(got, ngot, target, n, nn, last, cand.Node()) {
					continue
				}
				doc, got, last = next, ngot, cand.Node()
				placed = true
			}
			if placed {
				n++
				stall = 0
				yielded = true
				if !yield(doc, nil) {
					return
				}
			} else {
				stall++
			}
			for _, p := range island {
				skip[p.y*w+p.x] = 1
			}
		}
		if !yielded {
			yield(doc, nil)
		}
	}
}

func accept(got, ngot, want *image.NRGBA, parts, nparts int, old, cand svg.Node) bool {
	a, b := Score(got, want, parts), Score(ngot, want, nparts)
	if b < a {
		return true
	}
	if b > a || old.Kind() == svg.KindInvalid {
		return false
	}
	return pathLen(cand) < pathLen(old)
}

func formPaths(island []pix, col color.NRGBA) []svg.Path {
	bb := bbox(island)
	c := contour(island)
	poly := fitPoly(c, 6)
	hs := holeRings(island)
	tight := fitPoly(c, 3)
	if len(hs) > 0 && len(poly) >= 3 {
		out := []svg.Path{withHoles(filledPath(poly, col), hs), withFitHoles(island, tight, hs, col)}
		if cx, cy, rx, ry, ok := fitEllipse(island); ok {
			out = append(out, withHoles(filledEllipse(cx, cy, rx, ry, col), hs))
		}
		return out
	}
	boxA := (bb[1][0] - bb[0][0]) * (bb[2][1] - bb[1][1])
	if boxA > 2*float64(len(island)) {
		return []svg.Path{filledPath(poly, col), filledFit(island, tight, col)}
	}
	out := []svg.Path{filledPath(bb, col)}
	if cx, cy, rx, ry, ok := fitEllipse(island); ok {
		out = append(out, filledEllipse(cx, cy, rx, ry, col))
	}
	if len(poly) >= 3 {
		out = append(out, filledPath(poly, col), filledFit(island, tight, col))
	}
	return out
}

func holeRings(island []pix) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		r := fitPoly(contour(h), 6)
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func thinIsland(island []pix) bool {
	if len(island) == 0 {
		return false
	}
	bb := bbox(island)
	w := bb[1][0] - bb[0][0]
	h := bb[2][1] - bb[1][1]
	return w <= 1 || h <= 1
}

func withHoles(p svg.Path, holes [][][2]float64) svg.Path {
	for _, h := range holes {
		p = appendRing(p, h)
	}
	return p.WithFillRule(svg.FillEvenOdd)
}

func appendRing(p svg.Path, ring [][2]float64) svg.Path {
	if len(ring) < 3 {
		return p
	}
	cmds := p.Commands()
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdMove, X: ring[0][0], Y: ring[0][1]})
	for _, q := range ring[1:] {
		cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: q[0], Y: q[1]})
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	p, _ = p.WithCommands(cmds)
	return p
}

func filledPath(ring [][2]float64, col color.NRGBA) svg.Path {
	p := appendRing(svg.NewPath(), ring).WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
	if col.A != 255 {
		p = p.WithFillOpacity(float64(col.A) / 255)
	}
	return p
}

func replaceAt(d svg.Document, i int, n svg.Node) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	for j, k := range kids {
		if j == i {
			out = out.Append(n)
			continue
		}
		out = out.Append(k)
	}
	return out
}
