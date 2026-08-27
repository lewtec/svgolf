package search

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/lewtec/svgolf/internal/palette"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// maxRenders is the per-scene Render budget.
const maxRenders = 200

// scoreMaxEdge: Render rejects canvases larger than this. Score a downsampled copy.
const scoreMaxEdge = 4096

// Greedy covers the scene with axis-aligned filled rects from the palette.
// Color is not a gene; fills come from palette.Auto.
type Greedy struct {
	Colors  int // 0 = auto, cap 8
	Renders int // set by Search
}

var _ Search = (*Greedy)(nil)

func (g *Greedy) Search(ctx context.Context, target *image.NRGBA) (svg.Document, error) {
	if g == nil {
		return svg.Document{}, fmt.Errorf("search: nil Greedy")
	}
	g.Renders = 0
	if err := ctx.Err(); err != nil {
		return svg.Document{}, err
	}
	if target == nil {
		return svg.Document{}, fmt.Errorf("search: nil pixmap")
	}
	want := origin0(target)
	w, h := want.Rect.Dx(), want.Rect.Dy()
	out := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	if w <= 0 || h <= 0 {
		return out, nil
	}

	_, pal, err := palette.Auto(want, g.Colors)
	if err != nil {
		return out, err
	}
	if len(pal) == 0 {
		return out, nil
	}

	scoreWant, sx, sy := scoringWant(want)
	sw, sh := scoreWant.Rect.Dx(), scoreWant.Rect.Dy()
	scoreDoc := svg.NewDocument(float64(sw), float64(sh)).WithViewBox(0, 0, float64(sw), float64(sh))

	got, err := g.render(ctx, scoreDoc)
	if err != nil {
		return out, err
	}
	if got == nil {
		return out, nil
	}
	curN := pixelsLoss(got, scoreWant)
	if curN == 0 {
		return out, nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if g.Renders >= maxRenders {
			return out, nil
		}

		cands := proposeRects(got, scoreWant, pal, len(scoreDoc.Children()) == 0)
		bestN := curN
		bestS := perCost(curN, svg.CostDocument(scoreDoc))
		var best svg.Rect
		var bestGot *image.NRGBA
		found := false
		for _, r := range cands {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			trial := scoreDoc.Append(r.Node())
			tg, err := g.render(ctx, trial)
			if err != nil {
				return out, err
			}
			if tg == nil {
				break
			}
			n := pixelsLoss(tg, scoreWant)
			if n >= curN {
				continue
			}
			s := perCost(n, svg.CostDocument(trial))
			if !found || s < bestS || (s == bestS && n < bestN) {
				bestN, bestS, best, bestGot, found = n, s, r, tg, true
			}
		}
		if !found {
			return out, nil
		}

		scoreDoc = scoreDoc.Append(best.Node())
		out = out.Append(scaleRect(best, sx, sy).Node())
		curN = bestN
		got = bestGot
		if curN == 0 {
			return out, nil
		}
	}
}

func (g *Greedy) render(ctx context.Context, doc svg.Document) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g.Renders >= maxRenders {
		return nil, nil
	}
	img, err := render.Render(doc)
	g.Renders++
	return img, err
}

// pixelsLoss / perCost match loss.Pixels and loss.PerCost without importing loss
// (loss eval tests import search).
func pixelsLoss(got, want *image.NRGBA) float64 {
	if got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return math.Inf(1)
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
	return float64(n)
}

func perCost(deviate float64, complexity int) float64 {
	if math.IsInf(deviate, 0) || math.IsNaN(deviate) {
		return deviate
	}
	if complexity <= 0 {
		if deviate == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return deviate / float64(complexity)
}

func origin0(img *image.NRGBA) *image.NRGBA {
	if img.Rect.Min == (image.Point{}) {
		return img
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func scoringWant(want *image.NRGBA) (*image.NRGBA, float64, float64) {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	if w <= scoreMaxEdge && h <= scoreMaxEdge {
		return want, 1, 1
	}
	sw, sh := w, h
	if w >= h {
		sw = scoreMaxEdge
		sh = h * scoreMaxEdge / w
	} else {
		sh = scoreMaxEdge
		sw = w * scoreMaxEdge / h
	}
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	return resizeNN(want, sw, sh), float64(w) / float64(sw), float64(h) / float64(sh)
}

func resizeNN(src *image.NRGBA, nw, nh int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for y := 0; y < nh; y++ {
		sy := src.Rect.Min.Y + y*sh/nh
		for x := 0; x < nw; x++ {
			sx := src.Rect.Min.X + x*sw/nw
			dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
		}
	}
	return dst
}

func filledRect(x, y, w, h float64, c color.NRGBA) svg.Rect {
	r := svg.NewRect().WithX(x).WithY(y).WithWidth(w).WithHeight(h).WithFill(color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255})
	if c.A != 255 {
		r = r.WithFillOpacity(float64(c.A) / 255)
	}
	return r
}

func scaleRect(r svg.Rect, sx, sy float64) svg.Rect {
	if sx == 1 && sy == 1 {
		return r
	}
	c, _ := r.Fill()
	out := filledRect(r.X()*sx, r.Y()*sy, r.Width()*sx, r.Height()*sy, c)
	if op := r.FillOpacity(); op != 1 {
		out = out.WithFillOpacity(op)
	}
	return out
}

func proposeRects(got, want *image.NRGBA, pal []color.NRGBA, plate bool) []svg.Rect {
	w, h := want.Rect.Dx(), want.Rect.Dy()
	var out []svg.Rect
	if plate {
		for _, c := range pal {
			out = append(out, filledRect(0, 0, float64(w), float64(h), c))
		}
	}
	masks := make([][]bool, len(pal))
	for i := range pal {
		masks[i] = make([]bool, w*h)
	}
	idx := make(map[color.NRGBA]int, len(pal))
	for i, c := range pal {
		painted := color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
		if c.A != 255 {
			continue
		}
		idx[painted] = i
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			q := want.NRGBAAt(x, y)
			if q.A == 0 {
				continue
			}
			if got.NRGBAAt(x, y) == q {
				continue
			}
			i, ok := idx[q]
			if !ok {
				continue
			}
			masks[i][y*w+x] = true
		}
	}
	for i, mask := range masks {
		x, y, rw, rh, area := largestOnes(mask, w, h)
		if area <= 0 {
			continue
		}
		out = append(out, filledRect(float64(x), float64(y), float64(rw), float64(rh), pal[i]))
	}
	return out
}

func largestOnes(mask []bool, w, h int) (x, y, rw, rh, area int) {
	hist := make([]int, w)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			if mask[row*w+col] {
				hist[col]++
			} else {
				hist[col] = 0
			}
		}
		stack := make([]int, 0, w)
		for col := 0; col <= w; col++ {
			cur := 0
			if col < w {
				cur = hist[col]
			}
			for len(stack) > 0 && hist[stack[len(stack)-1]] > cur {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				height := hist[top]
				left := 0
				if len(stack) > 0 {
					left = stack[len(stack)-1] + 1
				}
				width := col - left
				a := height * width
				if a > area {
					area = a
					x, y, rw, rh = left, row-height+1, width, height
				}
			}
			if col < w {
				stack = append(stack, col)
			}
		}
	}
	return x, y, rw, rh, area
}
