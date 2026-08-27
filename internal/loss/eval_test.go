package loss

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/internal/search"
)

func TestEvalScenes(t *testing.T) {
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
			doc, err := (search.Dumb{}).Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if w > 4096 || h > 4096 {
				t.Logf("canvas %d×%d over Render cap; Search kids=%d", w, h, len(doc.Children()))
				return
			}
			s, err := Of(doc, want)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Dumb Of=%g kids=%d canvas=%d×%d", s, len(doc.Children()), w, h)
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
