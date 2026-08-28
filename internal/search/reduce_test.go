package search

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/pkg/render"
)

func TestReduceSSE(t *testing.T) {
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

func TestReduceNilPixmap(t *testing.T) {
	t.Parallel()
	_, err := (&Reduce{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReduceTwoBlobs(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 200, A: 255}
	bg := color.NRGBA{G: 80, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, bg)
		}
	}
	for y := 2; y < 14; y++ {
		for x := 2; x < 12; x++ {
			img.SetNRGBA(x, y, red)
		}
		for x := 24; x < 34; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	r := &Reduce{Colors: 2}
	doc, err := r.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("kids=%d want ≥2 (disconnected blobs survive reduce)", n)
	}
	if r.Seeded < 2 {
		t.Fatalf("seeded=%d", r.Seeded)
	}
	if r.Renders <= 0 || r.Renders > reduceRenderBudget {
		t.Fatalf("renders=%d", r.Renders)
	}
}

func TestReduceOverlapMerge(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 180, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	cover := redPrim{x: 0, y: 0, w: 16, h: 16, fill: red}
	inner := redPrim{x: 2, y: 3, w: 8, h: 7, fill: red}
	shift := redPrim{x: 4, y: 1, w: 12, h: 10, fill: red}
	got, used := reducePrims(t.Context(), img, []redPrim{cover, inner, shift}, reduceRenderBudget)
	if len(got) != 1 {
		t.Fatalf("kids=%d want 1 (overlapping same-color rects merged/deleted) used=%d", len(got), used)
	}
	if got[0].w != 16 || got[0].h != 16 {
		t.Fatalf("remaining %+v", got[0])
	}
}

func TestReduceSolid(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 200, A: 255})
		}
	}
	r := &Reduce{Colors: 1}
	doc, err := r.Search(t.Context(), img)
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

func TestReduceBudget(t *testing.T) {
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
	m := &Reduce{Colors: 2}
	if _, err := m.Search(t.Context(), img); err != nil {
		t.Fatal(err)
	}
	if m.Renders > reduceRenderBudget {
		t.Fatalf("renders=%d over budget %d", m.Renders, reduceRenderBudget)
	}
}

func TestNewReduce(t *testing.T) {
	s, err := New("reduce")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*Reduce); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestReduceEval(t *testing.T) {
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
			want := FitCanvas(FromImage(img), MaxCanvas)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			m := &Reduce{}
			doc, err := m.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			s := ssePixels(got, want)
			t.Logf("Reduce SSE=%g seeded=%d kids=%d renders=%d canvas=%d×%d",
				s, m.Seeded, len(doc.Children()), m.Renders, w, h)
			if len(doc.Children()) > reduceMaxKids {
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
