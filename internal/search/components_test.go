package search

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestComponentsSSE(t *testing.T) {
	t.Parallel()
	if !math.IsInf(ssePixels(nil, image.NewNRGBA(image.Rect(0, 0, 1, 1))), 1) {
		t.Fatal("nil got")
	}
	a := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	b := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	if !math.IsInf(ssePixels(a, b), 1) {
		t.Fatal("size mismatch")
	}
	want := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	got := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	want.SetNRGBA(0, 0, color.NRGBA{R: 10, A: 255})
	want.SetNRGBA(1, 0, color.NRGBA{R: 0, A: 0})
	got.SetNRGBA(0, 0, color.NRGBA{R: 13, A: 255})
	got.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
	if s := ssePixels(got, want); s != 9 {
		t.Fatalf("sse=%v want 9 (don't-care ignored)", s)
	}
}

func TestComponentsNilPixmap(t *testing.T) {
	t.Parallel()
	_, err := (&Components{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestComponentsTwoBlobs(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 200, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 16))
	for y := 2; y < 14; y++ {
		for x := 2; x < 12; x++ {
			img.SetNRGBA(x, y, red)
		}
		for x := 24; x < 34; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	c := &Components{Colors: 1}
	doc, err := c.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 2 {
		t.Fatalf("kids=%d want 2 (disconnected same-color blobs) kinds=%v", len(kids), kindsOf(kids))
	}
	if c.Renders <= 0 || c.Renders > compRenderBudget {
		t.Fatalf("renders=%d", c.Renders)
	}
}

func TestComponentsSpeckle(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 180, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	for y := 2; y < 14; y++ {
		for x := 2; x < 14; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	img.SetNRGBA(20, 1, red)
	img.SetNRGBA(21, 3, red)
	img.SetNRGBA(22, 20, red)
	img.SetNRGBA(23, 20, red)
	img.SetNRGBA(22, 21, red)
	img.SetNRGBA(23, 21, red)
	img.SetNRGBA(0, 23, red)
	c := &Components{Colors: 1}
	doc, err := c.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d want 1 (1–4 px speckles dropped) kinds=%v", n, kindsOf(doc.Children()))
	}
}

func TestComponentsSolid(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 200, A: 255})
		}
	}
	doc, err := (&Components{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d", n)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if s := ssePixels(got, img); s != 0 {
		t.Fatalf("sse=%v want 0", s)
	}
}

func TestComponentsDontCare(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 12))
	for y := 2; y < 10; y++ {
		for x := 3; x < 13; x++ {
			img.SetNRGBA(x, y, color.NRGBA{G: 180, A: 255})
		}
	}
	doc, err := (&Components{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d", n)
	}
}

func TestComponentsBudget(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if (x-16)*(x-16)+(y-16)*(y-16) <= 64 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	m := &Components{Colors: 2}
	if _, err := m.Search(t.Context(), img); err != nil {
		t.Fatal(err)
	}
	if m.Renders > compRenderBudget {
		t.Fatalf("renders=%d over budget %d", m.Renders, compRenderBudget)
	}
}

func TestComponentsEval(t *testing.T) {
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
			want := toNRGBAComp(img)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			if w > 4096 || h > 4096 {
				want = capLongEdge(want, 4096)
				w, h = want.Rect.Dx(), want.Rect.Dy()
			}
			m := &Components{}
			doc, err := m.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			s := ssePixels(got, want)
			t.Logf("Components SSE=%g kids=%d renders=%d canvas=%d×%d kinds=%s",
				s, len(doc.Children()), m.Renders, w, h, countKinds(doc.Children()))
			if len(doc.Children()) > 4096 {
				t.Fatalf("kids=%d over Encode cap", len(doc.Children()))
			}
			if w > 4096 || h > 4096 {
				t.Fatalf("canvas %d×%d over Encode/Render cap", w, h)
			}
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}

func kindsOf(nodes []svg.Node) []svg.Kind {
	out := make([]svg.Kind, len(nodes))
	for i, n := range nodes {
		out[i] = n.Kind()
	}
	return out
}

func countKinds(nodes []svg.Node) string {
	var nCirc, nEll, nRect, nPoly int
	for _, n := range nodes {
		switch n.Kind() {
		case svg.KindCircle:
			nCirc++
		case svg.KindEllipse:
			nEll++
		case svg.KindRect:
			nRect++
		case svg.KindPolygon:
			nPoly++
		}
	}
	return fmt.Sprintf("rect=%d circle=%d ellipse=%d poly=%d", nRect, nCirc, nEll, nPoly)
}

func toNRGBAComp(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
	return out
}
