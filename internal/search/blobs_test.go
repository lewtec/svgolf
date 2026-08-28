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

func TestNewBlobs(t *testing.T) {
	s, err := New("blobs")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*Blobs); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestNamesHasBlobs(t *testing.T) {
	for _, n := range Names() {
		if n == "blobs" {
			return
		}
	}
	t.Fatalf("Names=%v", Names())
}

func TestBlobsSSE(t *testing.T) {
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

func TestBlobsNilPixmap(t *testing.T) {
	t.Parallel()
	_, err := (&Blobs{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBlobsTwoDisconnected(t *testing.T) {
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
	b := &Blobs{Colors: 1}
	doc, err := b.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 2 {
		t.Fatalf("kids=%d want 2 (disconnected same-color blobs) kinds=%v", len(kids), kindsOf(kids))
	}
	if b.Renders <= 0 || b.Renders > blobRenderBudget {
		t.Fatalf("renders=%d", b.Renders)
	}
}

func TestBlobsSpeckleLargeCanvas(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 180, A: 255}
	// 800×800 → speckle = max(32, 640000/5000) = 128. A 10×10 island is ignored;
	// a fixed 32-px cut would have kept it.
	img := image.NewNRGBA(image.Rect(0, 0, 800, 800))
	for y := 40; y < 100; y++ {
		for x := 40; x < 100; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	for y := 400; y < 410; y++ {
		for x := 400; x < 410; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	img.SetNRGBA(10, 10, red)
	img.SetNRGBA(11, 10, red)
	img.SetNRGBA(790, 790, red)
	b := &Blobs{Colors: 1}
	doc, err := b.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d want 1 (area-fraction speckle) kinds=%v", n, kindsOf(doc.Children()))
	}
}

func TestBlobsSolid(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 200, A: 255})
		}
	}
	doc, err := (&Blobs{Colors: 1}).Search(t.Context(), img)
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

func TestBlobsBudget(t *testing.T) {
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
	m := &Blobs{Colors: 2}
	if _, err := m.Search(t.Context(), img); err != nil {
		t.Fatal(err)
	}
	if m.Renders > blobRenderBudget {
		t.Fatalf("renders=%d over budget %d", m.Renders, blobRenderBudget)
	}
}

func TestBlobsEval(t *testing.T) {
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
			m := &Blobs{}
			doc, err := m.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			s := ssePixels(got, want)
			t.Logf("Blobs SSE=%g kids=%d renders=%d canvas=%d×%d kinds=%s",
				s, len(doc.Children()), m.Renders, w, h, countKinds(doc.Children()))
			if len(doc.Children()) > 4096 {
				t.Fatalf("kids=%d over Encode cap", len(doc.Children()))
			}
			if w > 4096 || h > 4096 {
				t.Fatalf("canvas %d×%d over Encode/Render cap", w, h)
			}
			if m.Renders > blobRenderBudget {
				t.Fatalf("renders=%d over budget", m.Renders)
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
