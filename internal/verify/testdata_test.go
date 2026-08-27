package verify

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestdataMatchesResvg(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".svg") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	n := len(files)
	for _, path := range files {
		name, err := filepath.Rel(root, path)
		if err != nil {
			name = filepath.Base(path)
		}
		t.Run(name, func(t *testing.T) {
			r, err := File(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if r.DifferingPixels == -1 {
				t.Fatalf("size mismatch: ours %v resvg %v", r.Ours.Rect, r.Oracle.Rect)
			}
			if r.EncodeDrift {
				t.Fatal("encode drift")
			}
			if !r.Match {
				x, y := firstMismatch(r.Ours, r.Oracle)
				t.Fatalf("%d mismatched pixels; first=(%d,%d) ours=%v resvg=%v",
					r.DifferingPixels, x, y, r.Ours.NRGBAAt(x, y), r.Oracle.NRGBAAt(x, y))
			}
		})
	}
	if n == 0 {
		t.Fatal("no svg fixtures")
	}
}

func firstMismatch(a, b *image.NRGBA) (int, int) {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if a.NRGBAAt(x, y) != b.NRGBAAt(x, y) {
				return x, y
			}
		}
	}
	return -1, -1
}
