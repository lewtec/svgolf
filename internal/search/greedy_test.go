package search

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestGreedySolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	g := &Greedy{}
	doc, err := g.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Renders=%d kids=%d", g.Renders, len(doc.Children()))
	if g.Renders == 0 || g.Renders > maxRenders {
		t.Fatalf("Renders=%d", g.Renders)
	}
	kids := doc.Children()
	if len(kids) != 1 {
		t.Fatalf("kids=%d", len(kids))
	}
	r, ok := kids[0].Rect()
	if !ok || r.Width() != 10 || r.Height() != 8 {
		t.Fatalf("rect %+v", r)
	}
	s, err := loss.Of(doc, img)
	if err != nil {
		t.Fatal(err)
	}
	if s != 0 {
		t.Fatalf("Of=%g", s)
	}
}

func TestGreedyTwoColor(t *testing.T) {
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
	g := &Greedy{Colors: 2}
	doc, err := g.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Renders=%d kids=%d Cost=%d", g.Renders, len(doc.Children()), svg.CostDocument(doc))
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("kids=%d want >=2", n)
	}
	s, err := loss.Of(doc, img)
	if err != nil {
		t.Fatal(err)
	}
	if s != 0 {
		t.Fatalf("Of=%g", s)
	}
	if g.Renders > maxRenders {
		t.Fatalf("Renders=%d", g.Renders)
	}
}

func TestGreedyNilPixmap(t *testing.T) {
	_, err := (&Greedy{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGreedyTransparent(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	g := &Greedy{}
	doc, err := g.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 0 {
		t.Fatalf("kids=%d", n)
	}
	t.Logf("Renders=%d", g.Renders)
}

func TestGreedyEval(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "eval")
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		name := e.Name()
		if name == "bliss.png" {
			continue
		}
		n++
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			want := origin0(toNRGBA(img))
			w, h := want.Rect.Dx(), want.Rect.Dy()
			g := &Greedy{}
			doc, err := g.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if g.Renders > maxRenders {
				t.Fatalf("Renders=%d over budget", g.Renders)
			}
			s, err := loss.Of(doc, want)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Of=%g Cost=%d kids=%d Renders=%d canvas=%d×%d", s, svg.CostDocument(doc), len(doc.Children()), g.Renders, w, h)
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}
