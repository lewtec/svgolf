package search

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestSplitSSETwoColor(t *testing.T) {
	const w, h = 32, 32
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.SetNRGBA(x, y, red)
			} else {
				img.SetNRGBA(x, y, blue)
			}
		}
	}

	doc, renders, err := (SplitSSE{Colors: 2}).search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	s := mustSSE(t, doc, img)
	t.Logf("SSE=%g kids=%d renders=%d", s, len(kids), renders)

	if len(kids) != 2 {
		t.Fatalf("kids=%d want 2 (SSE gate, not PerCost packing)", len(kids))
	}
	if len(kids) >= 200 {
		t.Fatalf("kids=%d; PerCost cheat still packing to the Render cap", len(kids))
	}
	if s > 256 {
		t.Fatalf("SSE=%g want ~0", s)
	}
}

func TestSplitSSESolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := (SplitSSE{}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d", n)
	}
	if s := mustSSE(t, doc, img); s != 0 {
		t.Fatalf("SSE=%g", s)
	}
}

func TestSplitSSESameFillStops(t *testing.T) {
	// One palette color: a cut cannot drop SSE, so PerCost-style packing is rejected.
	const w, h = 32, 32
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= w/2 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, renders, err := (SplitSSE{Colors: 1}).search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d renders=%d; same-fill cut must be rejected", n, renders)
	}
}

func TestSplitSSENilPixmap(t *testing.T) {
	_, err := (SplitSSE{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSplitSSEEval(t *testing.T) {
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
			want := toNRGBATest(img)
			doc, renders, err := (SplitSSE{}).search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			gotW, gotH := int(doc.Width()), int(doc.Height())
			scoreWant := sseFit(want)
			s := mustSSE(t, doc, scoreWant)
			of, err := loss.Of(doc, scoreWant)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("SplitSSE SSE=%g Of=%g kids=%d renders=%d canvas=%d×%d (src %d×%d)",
				s, of, len(doc.Children()), renders, gotW, gotH, want.Rect.Dx(), want.Rect.Dy())
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}

func mustSSE(t *testing.T, doc svg.Document, want *image.NRGBA) float64 {
	t.Helper()
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	return sseNRGBA(got, want)
}

func toNRGBATest(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
