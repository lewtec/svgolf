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
	maxPaths   = 512
	stallLimit = 24
	minIsland  = 8
	minErr     = 8
	// leftover closer than this to the overlapping path is a polish,
	// not a new plate. 90 is half of ColorAt's 180.
	recolorAt = 90
)

// Stack covers leftover regions on a blurred copy of the pixmap, then halves
// the blur when the search stalls, down to the original. Each island keeps the
// best of polygon / ellipse (evenodd if it has holes). Accept if Score drops.
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
		owner := make([]uint16, w*h)
		var fills []color.NRGBA
		stall := 0
		yielded := false
		n := 0
		sigma := startSigma(w, h)
		want := wantAt(target, sigma)
		errSum := Score(got, want, 0)
		for n < maxPaths {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(svg.Document{}, err)
				}
				return
			}
			col, island := largestIsland(got, want, skip)
			if len(island) < minIsland {
				if !unblur(target, &sigma, &want, skip, &stall, &errSum, got) {
					break
				}
				continue
			}
			if transparentIsland(want, island) || thinIsland(island) {
				markSkip(skip, island, w)
				continue
			}

			pick, err := pickForm(doc, got, want, island, col, owner, fills, n, errSum, w, h)
			if err != nil {
				yield(svg.Document{}, err)
				return
			}
			if pick.ok {
				doc, got, errSum, yielded = pick.doc, pick.got, pick.errSum, true
				if !yield(doc, nil) {
					return
				}
				stall = 0
				if pick.replace >= 0 {
					clearOwner(owner, uint16(pick.replace+1))
					claim(owner, pick.work, w, uint16(pick.replace+1))
					fills[pick.replace] = pick.fill
				} else {
					claim(owner, pick.work, w, uint16(n+1))
					fills = append(fills, pick.fill)
					n++
				}
			}
			// one cover per CC at this blur. leftover of a flat fill
			// on a gradient is the same island; without skip we restack
			// it up to maxPaths. unblur clears skip.
			markSkip(skip, island, w)
			if !pick.ok {
				stall++
				if stall >= stallLimit && !unblur(target, &sigma, &want, skip, &stall, &errSum, got) {
					break
				}
			}
		}
		if !yielded {
			yield(doc, nil)
		}
	}
}

type formPick struct {
	doc     svg.Document
	got     *image.NRGBA
	errSum  float64
	replace int
	work    []pix
	fill    color.NRGBA
	ok      bool
}

// pickForm scores a polish of the overlapping path (same parts) against
// a new path. Leftover of the same body should refine; a mark on top
// should pay pathCost and append.
func pickForm(
	doc svg.Document,
	got, want *image.NRGBA,
	island []pix,
	col color.NRGBA,
	owner []uint16,
	fills []color.NRGBA,
	n int,
	errSum float64,
	w, h int,
) (formPick, error) {
	best := formPick{replace: -1}
	bestA := errSum + pathCost*float64(n)
	curA := bestA
	var bestLen int
	consider := func(work []pix, fill color.NRGBA, replace int) error {
		parts := n
		dirty0 := islandRect(work)
		if replace >= 0 {
			dirty0 = dirty0.Union(nodeRect(doc.Children()[replace]))
		} else {
			parts = n + 1
		}
		for _, cand := range formPaths(work, fill) {
			var next svg.Document
			if replace >= 0 {
				next = replaceAt(doc, replace, cand.Node())
			} else {
				next = doc.Append(cand.Node())
			}
			ngot, err := render.Render(next)
			if err != nil {
				return err
			}
			dirty := dirty0.Union(nodeRect(cand.Node())).Inset(-2)
			nerr := errSum + ScoreRect(ngot, want, dirty) - ScoreRect(got, want, dirty)
			a := nerr + pathCost*float64(parts)
			plen := pathLen(cand.Node())
			if a > bestA || a > curA {
				continue
			}
			if a == bestA && (!best.ok || plen >= bestLen) {
				continue
			}
			bestA = a
			bestLen = plen
			best = formPick{doc: next, got: ngot, errSum: nerr, replace: replace, work: work, fill: fill, ok: true}
		}
		return nil
	}
	if idx, ok := majorityOwner(owner, island, w); ok && loss.ColorAt(fills[idx], col) < recolorAt {
		union := ownedUnion(owner, island, w, h, uint16(idx+1))
		if err := consider(union, meanFill(want, union), idx); err != nil {
			return formPick{}, err
		}
		return best, nil
	}
	if err := consider(island, col, -1); err != nil {
		return formPick{}, err
	}
	return best, nil
}

func markSkip(skip []byte, island []pix, w int) {
	for _, p := range island {
		skip[p.y*w+p.x] = 1
	}
}

func unblur(orig *image.NRGBA, sigma *int, want **image.NRGBA, skip []byte, stall *int, errSum *float64, got *image.NRGBA) bool {
	if *sigma <= 0 {
		return false
	}
	*sigma /= 2
	*want = wantAt(orig, *sigma)
	for i := range skip {
		skip[i] = 0
	}
	*stall = 0
	*errSum = Score(got, *want, 0)
	return true
}

func acceptSum(err0, err1 float64, parts, nparts int, old, cand svg.Node) bool {
	a := err0 + pathCost*float64(parts)
	b := err1 + pathCost*float64(nparts)
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
	poly := fitPoly(contour(island), 2)
	hs := holeRings(island)
	if len(hs) > 0 && len(poly) >= 3 {
		out := []svg.Path{withHoles(filledPath(poly, col), hs), withFitHoles(island, poly, hs, col)}
		if cx, cy, rx, ry, ok := fitEllipse(island); ok {
			out = append(out, withHoles(filledEllipse(cx, cy, rx, ry, col), hs))
		}
		return out
	}
	var out []svg.Path
	if len(poly) >= 3 {
		out = append(out, filledPath(poly, col), filledFit(island, poly, col))
	}
	boxA := (bb[1][0] - bb[0][0]) * (bb[2][1] - bb[1][1])
	if boxA <= 2*float64(len(island)) {
		if cx, cy, rx, ry, ok := fitEllipse(island); ok {
			out = append(out, filledEllipse(cx, cy, rx, ry, col))
		}
	}
	out = append(out, filledPath(bb, col))
	return out
}

func holeRings(island []pix) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		r := fitPoly(contour(h), 2)
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

func islandRect(island []pix) image.Rectangle {
	if len(island) == 0 {
		return image.Rectangle{}
	}
	r := image.Rect(island[0].x, island[0].y, island[0].x+1, island[0].y+1)
	for _, p := range island[1:] {
		r = r.Union(image.Rect(p.x, p.y, p.x+1, p.y+1))
	}
	return r
}

func nodeRect(ns ...svg.Node) image.Rectangle {
	var r image.Rectangle
	for _, n := range ns {
		p, ok := n.Path()
		if !ok {
			continue
		}
		for _, c := range p.Commands() {
			q := image.Rect(int(c.X)-1, int(c.Y)-1, int(c.X)+2, int(c.Y)+2)
			if c.Kind == svg.CmdCubic {
				q = q.Union(image.Rect(int(c.X1)-1, int(c.Y1)-1, int(c.X1)+2, int(c.Y1)+2))
				q = q.Union(image.Rect(int(c.X2)-1, int(c.Y2)-1, int(c.X2)+2, int(c.Y2)+2))
			}
			if r.Empty() {
				r = q
			} else {
				r = r.Union(q)
			}
		}
	}
	return r
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
