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

func TestResidualSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	r := &Residual{}
	doc, err := r.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Renders=%d kids=%d SSE=%g", r.Renders, len(doc.Children()), r.SSE)
	if r.Renders == 0 || r.Renders > resMaxRenders {
		t.Fatalf("Renders=%d", r.Renders)
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

func TestResidualTwoColor(t *testing.T) {
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
	r := &Residual{Colors: 2}
	doc, err := r.Search(t.Context(), img)
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
	t.Logf("Renders=%d kids=%d SSE=%g plateSSE=%g", r.Renders, len(doc.Children()), gotSSE, plateSSE)
	if !(gotSSE < plateSSE) {
		t.Fatalf("SSE=%g not better than 1 plate %g", gotSSE, plateSSE)
	}
	if r.Renders > resMaxRenders {
		t.Fatalf("Renders=%d", r.Renders)
	}
}

func TestResidualNilPixmap(t *testing.T) {
	_, err := (&Residual{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResidualEval(t *testing.T) {
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
			want := FitCanvas(FromImage(img), MaxCanvas)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			r := &Residual{}
			doc, err := r.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if r.Renders > resMaxRenders {
				t.Fatalf("Renders=%d over budget", r.Renders)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			kinds := map[svg.Kind]int{}
			for _, c := range doc.Children() {
				kinds[c.Kind()]++
			}
			t.Logf("SSE=%g kids=%d Renders=%d canvas=%d×%d kinds=%v", sseNRGBA(got, want), len(doc.Children()), r.Renders, w, h, kinds)
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}
