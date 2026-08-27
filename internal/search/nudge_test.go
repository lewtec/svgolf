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
)

func TestNudgeOfNotWorseThanDumb(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for y := 10; y < 14; y++ {
		for x := 10; x < 14; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}

	ctx := t.Context()
	dumb, err := (Dumb{Colors: 2}).Search(ctx, img)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (Nudge{Colors: 2}).climb(ctx, img)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := loss.Of(dumb, img)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := loss.Of(got.doc, img)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Dumb Of=%g Nudge Of=%g renders=%d", ds, ns, got.renders)
	if ns > ds {
		t.Fatalf("Nudge Of=%g > Dumb Of=%g", ns, ds)
	}
	if n, d := len(got.doc.Children()), len(dumb.Children()); n != d {
		t.Fatalf("kids nudge=%d dumb=%d", n, d)
	}
	if got.renders > maxRenders {
		t.Fatalf("renders=%d > %d", got.renders, maxRenders)
	}
}

func TestNudgeNilPixmap(t *testing.T) {
	_, err := (Nudge{}).Search(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNudgeEvalScenes(t *testing.T) {
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
			want := toNRGBA(img)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			if name == "bliss.png" || w > renderCap || h > renderCap {
				t.Logf("canvas %d×%d over Render cap; skip Of", w, h)
				return
			}
			got, err := (Nudge{}).climb(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			ds, err := loss.Of(got.seed, want)
			if err != nil {
				t.Fatal(err)
			}
			ns, err := loss.Of(got.doc, want)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Dumb Of=%g Nudge Of=%g renders=%d kids=%d canvas=%d×%d", ds, ns, got.renders, len(got.doc.Children()), w, h)
			if ns > ds {
				t.Fatalf("Nudge Of=%g > Dumb seed Of=%g", ns, ds)
			}
			if got.renders > maxRenders {
				t.Fatalf("renders=%d > %d", got.renders, maxRenders)
			}
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
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
