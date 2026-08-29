package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestStackSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("paths=%d want 1", n)
	}
	if _, ok := doc.Children()[0].Path(); !ok {
		t.Fatal("not a path")
	}
}

func TestStackTwoColorGetsBoth(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want >=2", n)
	}
}

func TestStackMarkAfterPlate(t *testing.T) {
	// navy field + black block: global hue must not block the mark
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := navy
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want >=2 (plate + mark)", n)
	}
	empty := image.NewNRGBA(img.Rect)
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if Score(got, img, len(doc.Children())) >= Score(empty, img, 0) {
		t.Fatalf("final score not better than empty")
	}
}

func TestStackKeepsGoingAfterReject(t *testing.T) {
	// three disjoint blobs: a rejected hull must not end the run
	img := image.NewNRGBA(image.Rect(0, 0, 24, 8))
	paint := func(x0, x1 int, c color.NRGBA) {
		for y := 1; y < 7; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	paint(1, 7, color.NRGBA{R: 255, A: 255})
	paint(9, 15, color.NRGBA{G: 255, A: 255})
	paint(17, 23, color.NRGBA{B: 255, A: 255})
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 3 {
		t.Fatalf("paths=%d want >=3", n)
	}
}

func TestVoidsFindsEnclosedHole(t *testing.T) {
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				continue
			}
			island = append(island, pix{x, y})
		}
	}
	hs := voids(island)
	if len(hs) != 1 {
		t.Fatalf("voids=%d want 1", len(hs))
	}
	if len(hs[0]) != 16 {
		t.Fatalf("hole px=%d want 16", len(hs[0]))
	}
}

func TestStackRingKeepsInterior(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	cyan := color.NRGBA{R: 5, G: 176, B: 247, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	ring := func(x, y int) bool {
		dx, dy := float64(x)-15.5, float64(y)-15.5
		d := dx*dx + dy*dy
		return d >= 8*8 && d < 12*12
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if ring(x, y) {
				img.SetNRGBA(x, y, cyan)
			}
		}
	}
	for y := 14; y < 18; y++ {
		for x := 14; x < 20; x++ {
			img.SetNRGBA(x, y, cyan)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	c := got.NRGBAAt(15, 5)
	if c.B < 80 {
		t.Fatalf("ring missing %+v paths=%d", c, len(doc.Children()))
	}
	mid := got.NRGBAAt(15, 11)
	if mid.G > 80 && mid.B > 180 {
		t.Fatalf("interior filled with cyan %+v", mid)
	}
	if mid.A > 200 && mid.R < 20 && mid.G < 20 && mid.B < 20 {
		t.Fatalf("interior painted black %+v", mid)
	}
	mark := got.NRGBAAt(16, 16)
	if mark.B < 80 {
		t.Fatalf("inner mark missing %+v paths=%d", mark, len(doc.Children()))
	}
}

func TestStackDoesNotKeepFilledHoles(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 12; y < 20; y++ {
		for x := 12; x < 20; x++ {
			img.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	c := got.NRGBAAt(16, 16)
	if c.A != 0 && c.B > 40 {
		t.Fatalf("hole still navy %+v paths=%d", c, len(doc.Children()))
	}
}

func TestStackTinyMark(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 18; y < 22; y++ {
		for x := 18; x < 22; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want plate + 16px mark", n)
	}
}

func TestStackNilPixmap(t *testing.T) {
	_, err := search.Last((Stack{}).Search(t.Context(), nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRefineOrderBiggerFirst(t *testing.T) {
	got := refineOrder([]layer{{n: 9}, {n: 9}, {n: 64}, {n: 12}})
	want := []int{2, 3, 0, 1}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order=%v want %v", got, want)
		}
	}
}

func TestStackCoversBeforeRefine(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var kids []int
	var firstPts []int
	for doc, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		kids = append(kids, len(doc.Children()))
		firstPts = append(firstPts, pathPts(doc.Children()[0]))
	}
	lastGrow := 0
	for i := 1; i < len(kids); i++ {
		if kids[i] > kids[i-1] {
			lastGrow = i
		}
	}
	if kids[lastGrow] < 2 {
		t.Fatalf("never covered both, kids=%v", kids)
	}
	for i := 0; i <= lastGrow; i++ {
		if firstPts[i] != 4 {
			t.Fatalf("epoch %d: still covering (kids=%d) but first path pts=%d want 4", i, kids[i], firstPts[i])
		}
	}
}

func pathPts(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	ncmd := 0
	for _, c := range p.Commands() {
		if c.Kind != svg.CmdClose {
			ncmd++
		}
	}
	return ncmd
}

func TestStackFirstFormIsBBox(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for doc, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		p, ok := doc.Children()[0].Path()
		if !ok {
			t.Fatal("not a path")
		}
		n := 0
		for _, c := range p.Commands() {
			if c.Kind != svg.CmdClose {
				n++
			}
		}
		if n != 4 {
			t.Fatalf("first form points=%d want 4 (bbox)", n)
		}
		return
	}
	t.Fatal("no epoch")
}

func TestRDPCollinear(t *testing.T) {
	got := rdp([][2]float64{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, 0.5)
	if len(got) != 2 {
		t.Fatalf("rdp=%v", got)
	}
}

func TestFilledEllipseUsesCubics(t *testing.T) {
	p := filledEllipse(10, 10, 8, 8, color.NRGBA{R: 255, A: 255})
	n := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("cubics=%d want 4", n)
	}
}

func TestHullSquare(t *testing.T) {
	h := convexHull([][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}})
	if len(h) != 4 {
		t.Fatalf("hull=%v", h)
	}
}
