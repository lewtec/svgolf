package search

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestNewSimplify(t *testing.T) {
	s, err := New("simplify")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(Simplify); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestNamesHasSimplify(t *testing.T) {
	for _, n := range Names() {
		if n == "simplify" {
			return
		}
	}
	t.Fatalf("Names=%v", Names())
}

func TestSimplifyNilPixmap(t *testing.T) {
	if _, err := (Simplify{}).Search(t.Context(), nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestSimplifySolidIsBoxPath(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := (Simplify{}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 1 {
		t.Fatalf("kids=%d", len(kids))
	}
	p, ok := kids[0].Path()
	if !ok {
		t.Fatalf("kind=%v", kids[0].Kind())
	}
	if c := svg.Cost(p.Node()); c != 1 {
		t.Fatalf("Cost=%d cmds=%+v", c, p.Commands())
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if rmse := loss.RMSE(got, img); rmse > 2 {
		t.Fatalf("RMSE=%g", rmse)
	}
}

func TestSimplifyTwoIslands(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 8))
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
			}
		}
	}
	fill(1, 1, 5, 5)
	fill(12, 1, 16, 5)
	doc, err := (Simplify{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 2 {
		t.Fatalf("kids=%d", n)
	}
	for i, n := range doc.Children() {
		if _, ok := n.Path(); !ok {
			t.Fatalf("kid %d kind=%v", i, n.Kind())
		}
	}
}

func TestSimplifyCircleConverges(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	cx, cy, r := 32.0, 32.0, 26.0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				img.SetNRGBA(x, y, color.NRGBA{B: 180, A: 255})
			}
		}
	}
	doc, err := (Simplify{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children()) != 1 {
		t.Fatalf("kids=%d", len(doc.Children()))
	}
	p, ok := doc.Children()[0].Path()
	if !ok {
		t.Fatal("not path")
	}
	n := 0
	for _, c := range p.Commands() {
		if c.Kind != svg.CmdClose {
			n++
		}
	}
	if n > 20 {
		t.Fatalf("circle did not converge: %d cmds %+v", n, p.Commands())
	}
}

func TestSimplifyDropsStairPoints(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x <= y; x++ {
			img.SetNRGBA(x, y, color.NRGBA{G: 180, A: 255})
		}
	}
	doc, err := (Simplify{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children()) != 1 {
		t.Fatalf("kids=%d", len(doc.Children()))
	}
	p, ok := doc.Children()[0].Path()
	if !ok {
		t.Fatal("not path")
	}
	n := 0
	for _, c := range p.Commands() {
		if c.Kind != svg.CmdClose {
			n++
		}
	}
	if n > 6 {
		t.Fatalf("still dense: %d points cmds=%+v", n, p.Commands())
	}
}

func TestSimplifyDropsFringeColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	for y := 1; y < 4; y++ {
		for x := 1; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{G: 200, A: 255})
		}
	}
	doc, err := (Simplify{}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d; fringe green should not add a cycle", n)
	}
}

func TestSimplifyKeepsSecondColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	for y := 5; y < 25; y++ {
		for x := 5; x < 25; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 200, A: 255})
		}
	}
	doc, err := (Simplify{}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 2 {
		t.Fatalf("kids=%d; large second color should stay", n)
	}
}

func TestSimplifyRingHasHole(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			if x >= 3 && x < 9 && y >= 3 && y < 9 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	doc, err := (Simplify{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children()) != 1 {
		t.Fatalf("kids=%d", len(doc.Children()))
	}
	p, ok := doc.Children()[0].Path()
	if !ok {
		t.Fatal("not path")
	}
	if p.FillRule() != svg.FillEvenOdd {
		t.Fatalf("fill-rule=%v", p.FillRule())
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	c := got.NRGBAAt(6, 6)
	if c.A != 0 {
		t.Fatalf("hole painted %+v", c)
	}
	c = got.NRGBAAt(1, 1)
	if c.A == 0 {
		t.Fatal("frame empty")
	}
}
