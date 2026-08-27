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

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestMaskScore(t *testing.T) {
	t.Parallel()
	if got := maskScore(10, 2); got != 5 {
		t.Fatalf("maskScore(10,2)=%v want 5", got)
	}
	if got := maskScore(0, 0); got != 0 {
		t.Fatalf("maskScore(0,0)=%v want 0", got)
	}
	if got := maskScore(3, 0); !math.IsInf(got, 1) {
		t.Fatalf("maskScore(3,0)=%v want +Inf", got)
	}
}

func TestMaskNilPixmap(t *testing.T) {
	t.Parallel()
	_, err := (&Mask{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMaskTwoBlobs(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 200, A: 255}
	blue := color.NRGBA{B: 200, A: 255}
	base := svg.NewDocument(48, 32).WithViewBox(0, 0, 48, 32).Append(
		svg.NewRect().WithX(2).WithY(4).WithWidth(16).WithHeight(12).WithFill(red).Node(),
		svg.NewCircle().WithCX(36).WithCY(16).WithR(8).WithFill(blue).Node(),
	)
	want, err := render.Render(base)
	if err != nil {
		t.Fatal(err)
	}
	m := &Mask{Colors: 2}
	doc, err := m.Search(t.Context(), want)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 2 {
		t.Fatalf("kids=%d want 2 kinds=%v", len(kids), kindsOf(kids))
	}
	for _, n := range kids {
		if n.Kind() == svg.KindPolygon {
			t.Fatalf("kinds=%v; boxy/round blobs should stay rect or circle", kindsOf(kids))
		}
	}
	if m.Renders <= 0 || m.Renders > maskRenderBudget {
		t.Fatalf("renders=%d", m.Renders)
	}
}

func TestMaskPrefersRect(t *testing.T) {
	t.Parallel()
	red := color.NRGBA{R: 255, A: 255}
	base := svg.NewDocument(24, 16).WithViewBox(0, 0, 24, 16).Append(
		svg.NewRect().WithX(3).WithY(2).WithWidth(14).WithHeight(10).WithFill(red).Node(),
	)
	want, err := render.Render(base)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := (&Mask{Colors: 1}).Search(t.Context(), want)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 1 {
		t.Fatalf("kids=%d want 1", len(kids))
	}
	if _, ok := kids[0].Rect(); !ok {
		t.Fatalf("kind=%v want rect", kids[0].Kind())
	}
	if _, ok := kids[0].Polygon(); ok {
		t.Fatal("boxy blob escalated to polygon")
	}
}

func TestMaskPrefersCircle(t *testing.T) {
	t.Parallel()
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	// Opaque plate so overpaint in the bbox corners is scored; a transparent
	// field would treat those corners as don't-care and keep the seed rect.
	base := svg.NewDocument(32, 32).WithViewBox(0, 0, 32, 32).Append(
		svg.NewRect().WithWidth(32).WithHeight(32).WithFill(white).Node(),
		svg.NewCircle().WithCX(16).WithCY(16).WithR(11).WithFill(blue).Node(),
	)
	want, err := render.Render(base)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := (&Mask{Colors: 2}).Search(t.Context(), want)
	if err != nil {
		t.Fatal(err)
	}
	kids := doc.Children()
	if len(kids) != 2 {
		t.Fatalf("kids=%d want 2 kinds=%v", len(kids), kindsOf(kids))
	}
	var sawCircle bool
	for _, n := range kids {
		if n.Kind() == svg.KindPolygon {
			t.Fatalf("kinds=%v; round blob should not escalate to polygon", kindsOf(kids))
		}
		if _, ok := n.Circle(); ok {
			sawCircle = true
		}
	}
	if !sawCircle {
		t.Fatalf("kinds=%v want a circle", kindsOf(kids))
	}
}

func TestMaskDontCare(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 10))
	for y := 2; y < 8; y++ {
		for x := 3; x < 9; x++ {
			img.SetNRGBA(x, y, color.NRGBA{G: 180, A: 255})
		}
	}
	doc, err := (&Mask{Colors: 1}).Search(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("kids=%d", n)
	}
	r, ok := doc.Children()[0].Rect()
	if !ok {
		t.Fatalf("kind=%v want rect", doc.Children()[0].Kind())
	}
	if r.X() != 3 || r.Y() != 2 || r.Width() != 6 || r.Height() != 6 {
		t.Fatalf("bbox rect x=%v y=%v w=%v h=%v", r.X(), r.Y(), r.Width(), r.Height())
	}
}

func TestMaskBudget(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if (x-10)*(x-10)+(y-10)*(y-10) <= 25 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	m := &Mask{Colors: 2}
	if _, err := m.Search(t.Context(), img); err != nil {
		t.Fatal(err)
	}
	if m.Renders > maskRenderBudget {
		t.Fatalf("renders=%d over budget %d", m.Renders, maskRenderBudget)
	}
}

func TestMaskEval(t *testing.T) {
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
			want := toNRGBAEval(img)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			if w > 4096 || h > 4096 {
				t.Fatal("eval scene over Render cap; skip Of/Render (bliss should be excluded)")
			}
			m := &Mask{}
			doc, err := m.Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := render.Render(doc)
			if err != nil {
				t.Fatal(err)
			}
			dev := (loss.Pixels{}).Loss(got, want)
			sc := maskScore(dev, rankSum(doc.Children()))
			of, err := loss.Of(doc, want)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Mask score=%g Of=%g kids=%d renders=%d canvas=%d×%d kinds=%v",
				sc, of, len(doc.Children()), m.Renders, w, h, kindsOf(doc.Children()))
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

func toNRGBAEval(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	drawSrc(out, img, b)
	return out
}

func drawSrc(dst *image.NRGBA, src image.Image, b image.Rectangle) {
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
}
