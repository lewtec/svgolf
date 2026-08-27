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
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestSplitTwoColor(t *testing.T) {
	const w, h = 32, 16
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

	doc, renders, err := (Split{Colors: 2}).search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loss.Of(doc, img)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := loss.Of(onePlate(w, h, red), img)
	if err != nil {
		t.Fatal(err)
	}

	kids := doc.Children()
	t.Logf("Of=%g kids=%d renders=%d wrongPlate=%g", got, len(kids), renders, wrong)

	if len(kids) <= 1 {
		t.Fatalf("Split kept %d rect(s); Of=%g wrongPlate=%g", len(kids), got, wrong)
	}
	if got >= wrong {
		t.Fatalf("Of=%g not better than wrong plate %g", got, wrong)
	}
}

func TestSplitSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := (Split{}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d", n)
	}
	s, err := loss.Of(doc, img)
	if err != nil {
		t.Fatal(err)
	}
	if s != 0 {
		t.Fatalf("Of=%g", s)
	}
}

func TestSplitNilPixmap(t *testing.T) {
	_, err := (Split{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSplitEval(t *testing.T) {
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
			if name == "bliss.png" {
				t.Log("bliss 4510×3627 over Render cap; skip native Render")
				return
			}
			f, err := os.Open(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			want := toNRGBA(img)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			doc, renders, err := (Split{}).search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if w > 4096 || h > 4096 {
				t.Logf("canvas %d×%d over Render cap; Search kids=%d renders=%d", w, h, len(doc.Children()), renders)
				return
			}
			s, err := loss.Of(doc, want)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Split Of=%g kids=%d renders=%d canvas=%d×%d", s, len(doc.Children()), renders, w, h)
		})
	}
	if n == 0 {
		t.Fatal("no eval pngs")
	}
}

func onePlate(w, h int, c color.NRGBA) svg.Document {
	return svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h)).Append(
		svg.NewRect().WithWidth(float64(w)).WithHeight(float64(h)).WithFill(c).Node(),
	)
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
