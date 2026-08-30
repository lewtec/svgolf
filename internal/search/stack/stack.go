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
	maxPaths  = 512
	minIsland = 8
	minErr    = 8
)

// Stack covers leftover with hulls (and a linear if Score takes it),
// then refines existing paths (cubics, a better linear, drop).
// New objects only while cover still has a Score win. Want stays native.
type Stack struct{}

var _ search.Search = Stack{}

func init() {
	search.Register("stack", func() search.Search { return Stack{} })
}

func (Stack) Search(ctx context.Context, target *image.NRGBA) iter.Seq2[search.Epoch, error] {
	return func(yield func(search.Epoch, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(search.Epoch{}, err)
			return
		}
		if target == nil {
			yield(search.Epoch{}, fmt.Errorf("search: nil pixmap"))
			return
		}
		b := target.Bounds()
		w, h := b.Dx(), b.Dy()
		doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
		doc = doc.Append(whitePane(w, h).Node())
		got, err := render.Render(doc)
		if err != nil {
			yield(search.Epoch{}, err)
			return
		}
		skip := make([]byte, w*h)
		owner := make([]uint16, w*h)
		var fills []color.NRGBA
		yielded := false
		n := 0
		want := target
		errSum := Score(got, want, 0)
		for {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(search.Epoch{}, err)
				}
				return
			}
			if n < maxPaths {
				col, island := hottestIsland(got, want, skip)
				if len(island) >= minIsland {
					pick, err := pickForm(doc, got, want, island, col, owner, fills, n, errSum, w, h, false)
					if err != nil {
						yield(search.Epoch{}, err)
						return
					}
					if !pick.ok && n > 0 {
						pick, err = pickForm(doc, got, want, island, col, owner, fills, n, errSum, w, h, true)
						if err != nil {
							yield(search.Epoch{}, err)
							return
						}
					}
					markSkip(skip, island, w)
					if pick.ok {
						doc, got, errSum, yielded = pick.doc, pick.got, pick.errSum, true
						if !yield(epochOf(doc), nil) {
							return
						}
						if len(pick.reclaims) > 0 {
							for i, work := range pick.reclaims {
								if work == nil {
									continue
								}
								id := uint16(i + 1)
								clearOwner(owner, id)
								claim(owner, work, w, id)
							}
						} else if pick.replace >= 0 {
							id := uint16(pick.replace + 1)
							clearOwner(owner, id)
							claim(owner, pick.work, w, id)
							fills[pick.replace] = pick.fill
						} else {
							claim(owner, pick.work, w, uint16(n+1))
							fills = append(fills, pick.fill)
							n++
						}
					}
					continue
				}
			}
			dropped, err := tryDrop(&doc, &got, want, owner, &fills, &n, &errSum)
			if err != nil {
				yield(search.Epoch{}, err)
				return
			}
			if dropped {
				yielded = true
				if !yield(epochOf(doc), nil) {
					return
				}
				continue
			}
			break
		}
		if !yielded {
			yield(epochOf(doc), nil)
		}
	}
}

func epochOf(doc svg.Document) search.Epoch {
	return search.Epoch{Document: doc, Scale: 1}
}

type formPick struct {
	doc      svg.Document
	got      *image.NRGBA
	errSum   float64
	replace  int
	work     []pix
	fill     color.NRGBA
	reclaims [][]pix
	ok       bool
}

// pickForm: cover (refine=false) grows a hull or adds a hull.
// refine rewrites a connecting path (filledFit / linear). Score picks.
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
	refine bool,
) (formPick, error) {
	best := formPick{replace: -1}
	curA := errSum + pathCost*float64(n) + cmdCost*float64(docCmdLen(doc))
	bestA := curA
	var bestLen int
	consider := func(work []pix, fill color.NRGBA, replace int, refine bool) error {
		parts := n
		dirty0 := islandRect(work)
		if replace >= 0 {
			dirty0 = dirty0.Union(nodeRect(doc.Children()[replace+1]))
		} else {
			parts = n + 1
		}
		for _, cand := range formPaths(work, fill, refine, want) {
			var next svg.Document
			if replace >= 0 {
				next = replaceAt(doc, replace+1, cand.Node())
			} else {
				next = doc.Append(cand.Node())
			}
			ngot, err := render.Render(next)
			if err != nil {
				return err
			}
			dirty := dirty0.Union(nodeRect(cand.Node())).Inset(-2)
			nerr := errSum + ScoreRect(ngot, want, dirty) - ScoreRect(got, want, dirty)
			plen := pathLen(cand.Node())
			cmds := docCmdLen(next)
			if replace >= 0 && cand.FillRule() == svg.FillEvenOdd {
				cmds = docCmdLen(doc)
			}
			a := nerr + pathCost*float64(parts) + cmdCost*float64(cmds)
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
	if paperLeftover(col) && n > 0 {
		if err := punchThrough(&best, &bestA, curA, doc, want, island, owner, fills, n, errSum, w); err != nil {
			return formPick{}, err
		}
		return best, nil
	}
	for i := range fills {
		work := ownedUnion(owner, island, w, h, uint16(i+1))
		if len(work) <= len(island) && !paintsIsland(doc.Children()[i+1], island, w, h) {
			continue
		}
		if err := consider(work, meanFill(want, work), i, refine); err != nil {
			return formPick{}, err
		}
	}
	if !refine {
		if err := consider(island, col, -1, false); err != nil {
			return formPick{}, err
		}
	}
	return best, nil
}

// punchThrough shrinks every path that covers a paper leftover so the
// hole opens to the pane. Punching only the top layer reveals the
// plate underneath and Score gets worse.
func paintsIsland(node svg.Node, island []pix, w, h int) bool {
	d := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	d = d.Append(whitePane(w, h).Node()).Append(node)
	img, err := render.Render(d)
	if err != nil {
		return false
	}
	for _, p := range island {
		if colorErr(img.NRGBAAt(p.x, p.y), paper) > minErr {
			return true
		}
	}
	return false
}

func punchThrough(
	best *formPick,
	bestA *float64,
	curA float64,
	doc svg.Document,
	want *image.NRGBA,
	island []pix,
	owner []uint16,
	fills []color.NRGBA,
	n int,
	errSum float64,
	w int,
) error {
	next := doc
	reclaims := make([][]pix, n)
	any := false
	for i := 0; i < n; i++ {
		if !ownsAny(owner, island, w, uint16(i+1)) && !paintsIsland(doc.Children()[i+1], island, w, int(doc.Height())) {
			continue
		}
		work := ownedMinus(owner, island, w, uint16(i+1))
		ring := convexHull(islandPoints(work))
		if len(ring) < 3 {
			continue
		}
		// Punch only this leftover. holeRings(work) would also
		// carve every other void in the plate (trees, scale blocks).
		cand := filledPath(ring, fills[i])
		if hole := convexHull(islandPoints(island)); len(hole) >= 3 {
			cand = withHoles(cand, [][][2]float64{hole})
		}
		next = replaceAt(next, i+1, cand.Node())
		reclaims[i] = work
		any = true
	}
	if !any {
		return nil
	}
	ngot, err := render.Render(next)
	if err != nil {
		return err
	}
	nerr := Score(ngot, want, 0)
	a := nerr + pathCost*float64(n) + cmdCost*float64(docCmdLen(doc))
	if a >= curA || nerr >= errSum {
		return nil
	}
	*bestA = a
	*best = formPick{doc: next, got: ngot, errSum: nerr, replace: -1, work: island, reclaims: reclaims, ok: true}
	return nil
}

// tryDrop removes the smallest claimed path if Score does not rise.
// owner[] is place-time area, not paint after overlays; the accept
// test is a full re-Score without that child.
func tryDrop(doc *svg.Document, got **image.NRGBA, want *image.NRGBA, owner []uint16, fills *[]color.NRGBA, n *int, errSum *float64) (bool, error) {
	if *n < 2 {
		return false, nil
	}
	idx, ok := smallestOwner(owner, *n)
	if !ok {
		return false, nil
	}
	next := dropAt(*doc, idx+1)
	ngot, err := render.Render(next)
	if err != nil {
		return false, err
	}
	nerr := Score(ngot, want, 0)
	curA := *errSum + pathCost*float64(*n) + cmdCost*float64(docCmdLen(*doc))
	a := nerr + pathCost*float64(*n-1) + cmdCost*float64(docCmdLen(next))
	// cmd tax can exceed a real mark's pixel win. Do not drop if
	// the pixmap gets worse — only redundant paint.
	if a >= curA || nerr > *errSum {
		return false, nil
	}
	*doc, *got, *errSum = next, ngot, nerr
	dropOwner(owner, uint16(idx+1), *n)
	f := *fills
	*fills = append(f[:idx], f[idx+1:]...)
	*n--
	return true, nil
}

func smallestOwner(owner []uint16, n int) (int, bool) {
	if n <= 0 {
		return 0, false
	}
	cnt := make([]int, n+1)
	for _, id := range owner {
		if id > 0 && int(id) <= n {
			cnt[id]++
		}
	}
	best, bestN := 0, -1
	for i := 1; i <= n; i++ {
		if bestN < 0 || cnt[i] < bestN {
			best, bestN = i-1, cnt[i]
		}
	}
	return best, bestN >= 0
}

func dropOwner(owner []uint16, id uint16, n int) {
	for i, v := range owner {
		switch {
		case v == id:
			owner[i] = 0
		case v > id:
			owner[i]--
		}
	}
}

func dropAt(d svg.Document, i int) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	for j, k := range kids {
		if j == i {
			continue
		}
		out = out.Append(k)
	}
	return out
}

func markSkip(skip []byte, island []pix, w int) {
	for _, p := range island {
		skip[p.y*w+p.x] = 1
	}
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

func whitePane(w, h int) svg.Path {
	return filledPath([][2]float64{
		{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)},
	}, paper)
}

func formPaths(island []pix, col color.NRGBA, refine bool, want *image.NRGBA) []svg.Path {
	// A new plate is the convex hull. Holes and marks are later
	// layers, not carved into the plate.
	ring := convexHull(islandPoints(island))
	if len(ring) < 3 {
		return nil
	}
	out := []svg.Path{filledPath(ring, col)}
	if refine {
		out = append(out, filledFit(island, ring, col))
	}
	if gradient, ok := fitLinearFill(island, want); ok {
		n := len(out)
		for i := 0; i < n; i++ {
			out = append(out, out[i].WithLinearFill(gradient))
		}
	}
	return out
}

func holeRings(island []pix) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		r := convexHull(islandPoints(h))
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func withHoles(p svg.Path, holes [][][2]float64) svg.Path {
	for _, h := range holes {
		p = appendRing(p, h)
	}
	return p.WithFillRule(svg.FillEvenOdd)
}

func pathLen(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	return len(p.Commands())
}

func pathCommandWeight(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	w := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdLine {
			w += 2
			continue
		}
		w++
	}
	return w
}

func docCmdLen(d svg.Document) int {
	n := 0
	for _, c := range d.Children() {
		n += pathCommandWeight(c)
	}
	return n
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
	return appendRing(svg.NewPath(), ring).WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
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
