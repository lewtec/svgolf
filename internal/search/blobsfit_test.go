package search

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestNewBlobsFit(t *testing.T) {
	s, err := New("blobsfit")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*BlobsFit); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestNamesHasBlobsFit(t *testing.T) {
	for _, n := range Names() {
		if n == "blobsfit" {
			return
		}
	}
	t.Fatalf("Names=%v", Names())
}

func TestBlobsFitNilPixmap(t *testing.T) {
	t.Parallel()
	_, err := (&BlobsFit{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBlobsFitTwoDisconnected(t *testing.T) {
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
	b := &BlobsFit{Colors: 1}
	doc, err := b.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := svg.PartsDocument(doc); n != 2 {
		t.Fatalf("parts=%d want 2 (disconnected same-color blobs) kinds=%v", n, bfKindsOf(doc.Children()))
	}
	if b.Renders < 0 || b.Renders > bfRenderBudget {
		t.Fatalf("renders=%d", b.Renders)
	}
}

func TestBlobsFitDeletePass(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 180, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			want.SetNRGBA(x, y, red)
		}
	}
	full := svg.NewRect().WithX(0).WithY(0).WithWidth(24).WithHeight(24).WithFill(red).Node()
	extra := svg.NewRect().WithX(4).WithY(4).WithWidth(8).WithHeight(8).WithFill(red).Node()
	kids := []svg.Node{full, extra}
	beforeDoc := svg.NewDocument(24, 24).WithViewBox(0, 0, 24, 24).Append(kids...)
	before, err := loss.OfFit(beforeDoc, want)
	if err != nil {
		t.Fatal(err)
	}
	s := newBFSess(t.Context(), want, 24, 24)
	out := s.prune(kids)
	afterDoc := svg.NewDocument(24, 24).WithViewBox(0, 0, 24, 24).Append(out...)
	after, err := loss.OfFit(afterDoc, want)
	if err != nil {
		t.Fatal(err)
	}
	if after > before {
		t.Fatalf("Fit after delete-pass %v > before %v parts %d→%d", after, before, svg.PartsDocument(beforeDoc), svg.PartsDocument(afterDoc))
	}
	if s.used > bfRenderBudget {
		t.Fatalf("renders=%d", s.used)
	}
}

func TestBlobsFitSolid(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 200, A: 255})
		}
	}
	doc, err := (&BlobsFit{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := svg.PartsDocument(doc); n != 1 {
		t.Fatalf("parts=%d", n)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if r := loss.RMSE(got, img); r != 0 {
		t.Fatalf("RMSE=%v want 0", r)
	}
}

func TestBlobsFitBudget(t *testing.T) {
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
	m := &BlobsFit{Colors: 2}
	if _, err := m.Search(t.Context(), img); err != nil {
		t.Fatal(err)
	}
	if m.Renders > bfRenderBudget {
		t.Fatalf("renders=%d over budget %d", m.Renders, bfRenderBudget)
	}
}

func TestBlobsFitEval(t *testing.T) {
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
			m := &BlobsFit{}
			doc, err := m.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			k := svg.PartsDocument(doc)
			rmse := loss.RMSE(got, want)
			fit := loss.Fit(got, want, k)
			t.Logf("BlobsFit RMSE=%g Fit=%g parts=%d renders=%d canvas=%d×%d kinds=%s",
				rmse, fit, k, m.Renders, w, h, bfCountKinds(doc.Children()))
			if k > 4096 {
				t.Fatalf("parts=%d over Encode cap", k)
			}
			if w > 4096 || h > 4096 {
				t.Fatalf("canvas %d×%d over Encode/Render cap", w, h)
			}
			if m.Renders > bfRenderBudget {
				t.Fatalf("renders=%d over budget", m.Renders)
			}
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}

func bfKindsOf(nodes []svg.Node) []svg.Kind {
	out := make([]svg.Kind, len(nodes))
	for i, n := range nodes {
		out[i] = n.Kind()
	}
	return out
}

func bfCountKinds(nodes []svg.Node) string {
	var nCirc, nEll, nRect int
	for _, n := range nodes {
		switch n.Kind() {
		case svg.KindCircle:
			nCirc++
		case svg.KindEllipse:
			nEll++
		case svg.KindRect:
			nRect++
		}
	}
	return fmt.Sprintf("rect=%d circle=%d ellipse=%d", nRect, nCirc, nEll)
}
