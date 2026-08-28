package search

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestPrimitiveSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	p := &Primitive{}
	doc, err := p.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Renders=%d kids=%d", p.Renders, len(doc.Children()))
	if p.Renders == 0 || p.Renders > primMaxRenders {
		t.Fatalf("Renders=%d", p.Renders)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d want 1 plate", n)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if e := sseNRGBA(got, img); e != 0 {
		t.Fatalf("SSE=%g", e)
	}
}

func TestPrimitiveTwoColor(t *testing.T) {
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
	p := &Primitive{Colors: 2}
	doc, err := p.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("kids=%d want >=2", n)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	gotSSE := sseNRGBA(got, img)
	plate := svg.NewDocument(8, 8).WithViewBox(0, 0, 8, 8).Append(
		svg.NewRect().WithX(0).WithY(0).WithWidth(8).WithHeight(8).WithFill(color.NRGBA{R: 255, A: 255}).Node(),
	)
	pg, err := render.Render(plate)
	if err != nil {
		t.Fatal(err)
	}
	plateSSE := sseNRGBA(pg, img)
	t.Logf("Renders=%d kids=%d SSE=%g plateSSE=%g", p.Renders, len(doc.Children()), gotSSE, plateSSE)
	if !(gotSSE < plateSSE) {
		t.Fatalf("SSE=%g not better than 1 plate %g", gotSSE, plateSSE)
	}
	if p.Renders > primMaxRenders {
		t.Fatalf("Renders=%d", p.Renders)
	}
}

func TestPrimitiveNilPixmap(t *testing.T) {
	_, err := (&Primitive{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPrimitiveEval(t *testing.T) {
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
		n++
		name := e.Name()
		if name == "bliss.png" {
			// Native 4510×3627 is scored on a 4096-capped copy; skip the long eval.
			continue
		}
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
			want := capWant(origin0(toNRGBA(img)))
			w, h := want.Rect.Dx(), want.Rect.Dy()
			p := &Primitive{}
			doc, err := p.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if p.Renders > primMaxRenders {
				t.Fatalf("Renders=%d over budget", p.Renders)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("SSE=%g kids=%d Renders=%d canvas=%d×%d", sseNRGBA(got, want), len(doc.Children()), p.Renders, w, h)
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
