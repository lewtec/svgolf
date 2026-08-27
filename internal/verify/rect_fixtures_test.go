package verify

import (
	"path/filepath"
	"testing"
)

func TestRectFixturesMatchResvg(t *testing.T) {
	files := []string{
		"empty.svg",
		"rect-full.svg",
		"rect-inset.svg",
		"rect-opacity.svg",
		"rect-edges.svg",
		"rect-rx0.svg",
		"rect-overlap.svg",
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "testdata", "svg", name)
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
				t.Fatalf("%d mismatched pixels", r.DifferingPixels)
			}
		})
	}
}
