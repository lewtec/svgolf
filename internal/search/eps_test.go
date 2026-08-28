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
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestNewEps(t *testing.T) {
	s, err := New("eps")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*Eps); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestNamesHasEps(t *testing.T) {
	for _, n := range Names() {
		if n == "eps" {
			return
		}
	}
	t.Fatalf("Names=%v", Names())
}

func TestEpsSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	e := &Eps{}
	doc, err := e.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := svg.PartsDocument(doc); n != 1 {
		t.Fatalf("parts=%d want 1", n)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if r := loss.RMSE(got, img); r != 0 {
		t.Fatalf("RMSE=%v want 0", r)
	}
	if g := loss.EpsFit(got, img, svg.PartsDocument(doc)); g != 1 {
		t.Fatalf("EpsFit=%v want 1", g)
	}
	if e.Renders > epsRenderBudget {
		t.Fatalf("Renders=%d", e.Renders)
	}
}

func halfPlane(w, h int) *image.NRGBA {
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
	return img
}

func TestEpsHalfPlane(t *testing.T) {
	img := halfPlane(8, 8)
	e := &Eps{Colors: 2}
	doc, err := e.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := svg.PartsDocument(doc); n != 2 {
		t.Fatalf("parts=%d want 2 kids=%d", n, len(doc.Children()))
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if r := loss.RMSE(got, img); r != 0 {
		t.Fatalf("RMSE=%v want 0", r)
	}
	if g := loss.EpsFit(got, img, 2); g != 2 {
		t.Fatalf("EpsFit=%v want 2", g)
	}
}

func TestEpsRejectsUselessRect(t *testing.T) {
	img := halfPlane(8, 8)
	e := &Eps{Colors: 2}
	doc, err := e.Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := svg.PartsDocument(doc); n != 2 {
		t.Fatalf("parts=%d want 2", n)
	}
	cur, err := loss.OfEps(doc, img)
	if err != nil {
		t.Fatal(err)
	}
	if cur != 2 {
		t.Fatalf("OfEps=%v want 2", cur)
	}
	extra := svg.NewRect().WithX(1).WithY(1).WithWidth(2).WithHeight(2).
		WithFill(color.NRGBA{R: 255, A: 255}).Node()
	worse := doc.Append(extra)
	next, err := loss.OfEps(worse, img)
	if err != nil {
		t.Fatal(err)
	}
	if !(cur < next) {
		t.Fatalf("third rect not rejected: cur=%v next=%v", cur, next)
	}
	if next != 3 {
		t.Fatalf("EpsFit with extra rect=%v want 3", next)
	}
}

func TestEpsNilPixmap(t *testing.T) {
	_, err := (&Eps{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEpsBudget(t *testing.T) {
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
	e := &Eps{Colors: 2}
	if _, err := e.Search(t.Context(), img); err != nil {
		t.Fatal(err)
	}
	if e.Renders > epsRenderBudget {
		t.Fatalf("renders=%d over budget", e.Renders)
	}
}

func TestEpsEval(t *testing.T) {
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
			m := &Eps{}
			doc, err := m.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			parts := svg.PartsDocument(doc)
			rmse := loss.RMSE(got, want)
			fit := loss.EpsFit(got, want, parts)
			t.Logf("Eps RMSE=%.3f parts=%d EpsFit=%.4f renders=%d canvas=%d×%d",
				rmse, parts, fit, m.Renders, w, h)
			if parts > epsMaxKids {
				t.Fatalf("parts=%d over Encode cap", parts)
			}
			if m.Renders > epsRenderBudget {
				t.Fatalf("renders=%d over budget", m.Renders)
			}
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}
