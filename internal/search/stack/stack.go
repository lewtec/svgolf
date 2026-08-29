package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"iter"
	"sort"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

const (
	maxPaths   = 512
	stallLimit = 24
	minIsland  = 8
)

// Stack covers every leftover region with a simple path, then tightens those paths.
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
		var layers []layer
		for len(layers) < maxPaths && stall < stallLimit {
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
			if transparentIsland(target, island) {
				for _, p := range island {
					skip[p.y*w+p.x] = 1
				}
				continue
			}
			cands := formPaths(island, col)
			next := doc.Append(cands[0].Node())
			ngot, err := render.Render(next)
			if err != nil {
				yield(svg.Document{}, err)
				return
			}
			if accept(got, ngot, target, len(layers), len(layers)+1, svg.Node{}, cands[0].Node()) {
				doc, got = next, ngot
				layers = append(layers, layer{forms: cands, n: len(island)})
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
		n := len(layers)
		for _, i := range refineOrder(layers) {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(svg.Document{}, err)
				}
				return
			}
			old := doc.Children()[i]
			for _, cand := range layers[i].forms[1:] {
				next := replaceAt(doc, i, cand.Node())
				ngot, err := render.Render(next)
				if err != nil {
					yield(svg.Document{}, err)
					return
				}
				if !accept(got, ngot, target, n, n, old, cand.Node()) {
					continue
				}
				doc, got = next, ngot
				old = cand.Node()
				yielded = true
				if !yield(doc, nil) {
					return
				}
			}
		}
		if !yielded {
			yield(doc, nil)
		}
	}
}

type layer struct {
	forms []svg.Path
	n     int
}

func refineOrder(layers []layer) []int {
	idx := make([]int, len(layers))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return layers[idx[i]].n > layers[idx[j]].n
	})
	return idx
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
	boxA := (bb[1][0] - bb[0][0]) * (bb[2][1] - bb[1][1])
	c := contour(island)
	out := holedForms(island, c, col)
	if boxA > 2*float64(len(island)) {
		return append(out, filledPath(rdp(c, 4), col), filledPath(rdp(c, 1), col))
	}
	out = append(out, filledPath(bb, col))
	if cx, cy, rx, ry, ok := fitEllipse(island); ok {
		el := filledEllipse(cx, cy, rx, ry, col)
		if hs := holeRings(island, 4); len(hs) > 0 {
			out = append(out, withHoles(el, hs))
		}
		out = append(out, el)
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

func holedForms(island []pix, outer [][2]float64, col color.NRGBA) []svg.Path {
	hs := holeRings(island, 4)
	if len(hs) == 0 || len(outer) < 3 {
		return nil
	}
	out := []svg.Path{filledEvenOdd(rdp(outer, 4), hs, col)}
	if tight := holeRings(island, 1); len(tight) > 0 {
		out = append(out, filledEvenOdd(rdp(outer, 1), tight, col))
	}
	return out
}

func holeRings(island []pix, eps float64) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		c := contour(h)
		if len(c) < 3 {
			continue
		}
		r := rdp(c, eps)
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func filledEvenOdd(outer [][2]float64, holes [][][2]float64, col color.NRGBA) svg.Path {
	return withHoles(filledPath(outer, col), holes)
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
	p = p.MoveTo(ring[0][0], ring[0][1])
	for _, q := range ring[1:] {
		p = p.LineTo(q[0], q[1])
	}
	return p.Close()
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
