package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// Flat shapes that are not one triangle must reach Score zero.
// The largest-area ear spills, and a smoothed polygon changes
// pixels, so Score keeps the mask outline unless a simpler
// plate matches it exactly.
func TestOutlineConvergesFlatShapes(t *testing.T) {
	red := color.NRGBA{R: 220, G: 30, B: 40, A: 255}
	blue := color.NRGBA{R: 20, G: 90, B: 180, A: 255}
	cases := []struct {
		name    string
		w, h    int
		inside  func(x, y int) bool
		outline bool
	}{
		{
			name: "ell", w: 40, h: 40, outline: true,
			inside: func(x, y int) bool {
				return (x >= 6 && x < 16 && y >= 6 && y < 34) || (x >= 6 && x < 34 && y >= 24 && y < 34)
			},
		},
		{
			name: "disk", w: 40, h: 40, outline: true,
			inside: func(x, y int) bool {
				dx, dy := x-20, y-20
				return dx*dx+dy*dy <= 12*12
			},
		},
		{
			name: "annulus", w: 40, h: 40, outline: true,
			inside: func(x, y int) bool {
				dx, dy := x-20, y-20
				rr := dx*dx + dy*dy
				return rr <= 14*14 && rr >= 6*6
			},
		},
		{
			name: "stroke", w: 36, h: 16, outline: true,
			inside: func(x, y int) bool {
				return y >= 7 && y < 9 && x >= 4 && x < 32
			},
		},
		{
			name: "two", w: 48, h: 32,
			inside: func(x, y int) bool {
				if y < 6 || y >= 26 {
					return false
				}
				return (x >= 4 && x < 20) || (x >= 28 && x < 44)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h))
			for y := 0; y < tc.h; y++ {
				for x := 0; x < tc.w; x++ {
					c := paper
					if tc.inside(x, y) {
						c = red
						if tc.name == "two" && x >= 28 {
							c = blue
						}
					}
					img.SetNRGBA(x, y, c)
				}
			}
			outlined := false
			var got *image.NRGBA
			epochs := 0
			for ep, err := range (Stack{}).Search(mustCtx(t), img) {
				if err != nil {
					t.Fatal(err)
				}
				epochs++
				if ep.Operator == OpOutline {
					outlined = true
				}
				got, err = render.Render(ep.Document)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got == nil {
				t.Fatal("no epoch")
			}
			if score := Score(nil, got, img); score != 0 {
				t.Fatalf("score=%.3f epochs=%d want 0", score, epochs)
			}
			if tc.outline && !outlined {
				t.Fatal("outline was not accepted")
			}
		})
	}
}

// A paper letter on a flat field is one plate with a hole. The
// convex hull spills into the missing corner, so the hole has to
// be the mask outline or Score keeps a second path.
func TestPaperLetterConvergesToOneShape(t *testing.T) {
	red := color.NRGBA{R: 172, G: 19, B: 12, A: 255}
	const w, h = 40, 40
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := red
			if (x >= 8 && x < 16 && y >= 6 && y < 32) || (x >= 8 && x < 32 && y >= 24 && y < 32) {
				c = paper
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc := runStack(t, img)
	got := mustRender(t, doc)
	if score := Score(nil, got, img); score != 0 {
		t.Fatalf("score=%.3f want 0", score)
	}
	if n := len(forms(doc)); n != 1 {
		t.Fatalf("paths=%d want one plate", n)
	}
	if c := got.NRGBAAt(12, 12); c != paper {
		t.Fatalf("stem %+v want paper", c)
	}
	if c := got.NRGBAAt(2, 2); loss.ColorAt(c, red) > minErr {
		t.Fatalf("field %+v want red", c)
	}
}

// A paper speck beside the letter is another hole in the same
// plate. A white path for the speck would survive on a short
// sibling, and the letter would come back as a second path.
func TestPaperSpeckStaysOnThePlate(t *testing.T) {
	red := color.NRGBA{R: 172, G: 19, B: 12, A: 255}
	const w, h = 40, 40
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := red
			if (x >= 8 && x < 16 && y >= 6 && y < 32) || (x >= 8 && x < 32 && y >= 24 && y < 32) {
				c = paper
			}
			if x >= 20 && x < 23 && y >= 8 && y < 11 {
				c = paper
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc := runStack(t, img)
	got := mustRender(t, doc)
	if score := Score(nil, got, img); score != 0 {
		t.Fatalf("score=%.3f want 0", score)
	}
	if n := len(forms(doc)); n != 1 {
		t.Fatalf("paths=%d want one plate", n)
	}
	if c := got.NRGBAAt(21, 9); c != paper {
		t.Fatalf("speck %+v want paper", c)
	}
	if c := got.NRGBAAt(12, 12); c != paper {
		t.Fatalf("stem %+v want paper", c)
	}
}

func TestPaperLeftoverDoesNotAddAPath(t *testing.T) {
	s := &world{paths: 1, fills: []color.NRGBA{{R: 172, G: 19, B: 12, A: 255}}}
	island := make([]pix, minIsland)
	for i := range island {
		island[i] = pix{i, 0}
	}
	left := leftover{paper: true, island: island, col: paper}
	for band := 1; band <= 4; band++ {
		for _, op := range s.leftoverOperators(left, band) {
			switch op.ID() {
			case OpTriangle, OpRectangle, OpRing, OpOutline:
				t.Fatalf("band %d scheduled %s for a paper leftover", band, op.ID())
			}
		}
	}
}

// The soft edge of that letter is a blend of the field and the
// pane. It is not another plate.
func TestBlendAroundLetterIsNotAPlate(t *testing.T) {
	red := color.NRGBA{R: 172, G: 19, B: 12, A: 255}
	mid := mixColor(red, paper, 0.5)
	const w, h = 32, 32
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := red
			switch {
			case x >= 12 && x < 20 && y >= 10 && y < 22:
				c = paper
			case x >= 10 && x < 22 && y >= 8 && y < 24:
				c = mid
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var ops []string
	var doc svg.Document
	for ep, err := range (Stack{}).Search(mustCtx(t), img) {
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, ep.Operator.String())
		doc = ep.Document
	}
	if n := len(forms(doc)); n != 1 {
		t.Fatalf("paths=%d ops=%v want one plate, not a rim for the blend", n, ops)
	}
	got := mustRender(t, doc)
	if c := got.NRGBAAt(16, 16); c != paper {
		t.Fatalf("letter %+v want paper", c)
	}
	if c := got.NRGBAAt(1, 1); loss.ColorAt(c, red) > minErr {
		t.Fatalf("field %+v want red", c)
	}
}

func TestOnBlendIsTheSeamNotAThirdFlat(t *testing.T) {
	red := color.NRGBA{R: 172, G: 19, B: 12, A: 255}
	blue := color.NRGBA{R: 20, G: 90, B: 180, A: 255}
	black := color.NRGBA{A: 255}
	paints := []color.NRGBA{red, paper}
	if !onBlend(mixColor(red, paper, 0.5), paints) {
		t.Fatal("mid blend was not a seam")
	}
	if onBlend(red, paints) {
		t.Fatal("field color was called a seam")
	}
	if onBlend(paper, paints) {
		t.Fatal("paper was called a seam")
	}
	if onBlend(blue, paints) {
		t.Fatal("a third flat on the field was called a seam")
	}
	if onBlend(black, paints) {
		t.Fatal("black mark was called a seam")
	}
}

func mixColor(a, b color.NRGBA, t float64) color.NRGBA {
	ch := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t + 0.5)
	}
	return color.NRGBA{R: ch(a.R, b.R), G: ch(a.G, b.G), B: ch(a.B, b.B), A: 255}
}

func runStack(t *testing.T, img *image.NRGBA) svg.Document {
	t.Helper()
	var last svg.Document
	n := 0
	for ep, err := range (Stack{}).Search(mustCtx(t), img) {
		if err != nil {
			t.Fatal(err)
		}
		last = ep.Document
		n++
	}
	if n == 0 {
		t.Fatal("no epoch")
	}
	return last
}

func mustRender(t *testing.T, doc svg.Document) *image.NRGBA {
	t.Helper()
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
