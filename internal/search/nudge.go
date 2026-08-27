package search

import (
	"context"
	"image"
	"math"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Nudge hill-climbs Dumb's rects. Color is not a gene; nodes are not added or deleted.
type Nudge struct {
	Colors int // 0 = auto, cap 8
}

var _ Search = Nudge{}

const maxRenders = 200

// renderCap matches pkg/render.checkCanvas; bliss is over this and must not be Rendered.
const renderCap = 4096

var nudgeSteps = []float64{-8, 8, -4, 4, -2, 2, -1, 1}

func (n Nudge) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	run, err := n.climb(ctx, target)
	return run.doc, err
}

type climbResult struct {
	doc     svg.Document
	seed    svg.Document
	renders int
}

func (n Nudge) climb(ctx context.Context, target *image.NRGBA) (climbResult, error) {
	doc, err := (Dumb{Colors: n.Colors}).Search(ctx, target)
	if err != nil {
		return climbResult{doc: doc, seed: doc}, err
	}
	out := climbResult{doc: doc, seed: doc}
	if target.Bounds().Dx() > renderCap || target.Bounds().Dy() > renderCap {
		return out, nil
	}

	best, err := renderDeviate(doc, target)
	out.renders = 1
	if err != nil {
		return out, nil
	}

	cw, ch := doc.Width(), doc.Height()
	kids := doc.Children()

	for out.renders < maxRenders {
		if err := ctx.Err(); err != nil {
			return out, nil
		}
		improved := false
		for i := len(kids) - 1; i >= 0; i-- {
			r, ok := kids[i].Rect()
			if !ok {
				continue
			}
			for dim := 0; dim < 4; dim++ {
				for _, delta := range nudgeSteps {
					for out.renders < maxRenders {
						if err := ctx.Err(); err != nil {
							return out, nil
						}
						cand, ok := stepRect(r, dim, delta, cw, ch)
						if !ok {
							break
						}
						old := kids[i]
						kids[i] = cand.Node()
						trial := documentWith(out.doc, kids)
						s, err := renderDeviate(trial, target)
						out.renders++
						if err != nil || !(s < best) {
							kids[i] = old
							break
						}
						out.doc = trial
						best = s
						r = cand
						improved = true
					}
				}
			}
		}
		if !improved {
			break
		}
	}
	return out, nil
}

// renderDeviate is the Search circuit: Render, then Pixels (want.A==0 skip).
// Cost is fixed (no add/delete/rx), so this ranking matches loss.Of.
func renderDeviate(doc svg.Document, want *image.NRGBA) (float64, error) {
	got, err := render.Render(doc)
	if err != nil {
		return math.Inf(1), err
	}
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1), nil
	}
	n := 0
	for y := want.Rect.Min.Y; y < want.Rect.Max.Y; y++ {
		for x := want.Rect.Min.X; x < want.Rect.Max.X; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			if got.NRGBAAt(x, y) != q {
				n++
			}
		}
	}
	return float64(n), nil
}

func stepRect(r svg.Rect, dim int, delta, cw, ch float64) (svg.Rect, bool) {
	x, y, w, h := r.X(), r.Y(), r.Width(), r.Height()
	switch dim {
	case 0:
		x += delta
	case 1:
		y += delta
	case 2:
		w += delta
	default:
		h += delta
	}
	if w <= 0 || h <= 0 || x < 0 || y < 0 || x+w > cw || y+h > ch {
		return svg.Rect{}, false
	}
	return r.WithX(x).WithY(y).WithWidth(w).WithHeight(h), true
}

func documentWith(src svg.Document, kids []svg.Node) svg.Document {
	out := svg.NewDocument(src.Width(), src.Height())
	if vb := src.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	return out.Append(kids...)
}
