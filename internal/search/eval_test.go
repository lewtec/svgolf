package search

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
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
			want := FromImage(img)
			w, h := want.Rect.Dx(), want.Rect.Dy()
			doc, err := (Dumb{}).Search(t.Context(), want)
			if err != nil {
				t.Fatal(err)
			}
			if w > MaxCanvas || h > MaxCanvas {
				t.Logf("canvas %d×%d over cap; kids=%d", w, h, len(doc.Children()))
				return
			}
			s, err := loss.Of(doc, want)
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
