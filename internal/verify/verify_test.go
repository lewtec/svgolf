package verify

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/resvg"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestCompareEmptyMatchesOracle(t *testing.T) {
	doc := svg.NewDocument(256, 256)
	ours, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	xml, err := svg.EncodeToString(doc)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := resvg.Render(t.Context(), []byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Compare(ours, oracle)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match || r.DifferingPixels != 0 {
		t.Fatalf("empty mismatch: match=%v pixels=%d", r.Match, r.DifferingPixels)
	}
}

func TestCompareSizeMismatch(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	b := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Match || r.DifferingPixels != -1 || r.Diff != nil {
		t.Fatalf("got %+v", r)
	}
}

func TestComparePixelDiff(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	b := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	a.SetNRGBA(0, 0, color.NRGBA{R: 10, A: 255})
	b.SetNRGBA(0, 0, color.NRGBA{R: 4, A: 255})
	r, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Match || r.DifferingPixels != 1 || r.Diff == nil {
		t.Fatalf("got match=%v n=%d diff=%v", r.Match, r.DifferingPixels, r.Diff)
	}
	if r.Diff.NRGBAAt(0, 0) != (color.NRGBA{R: 6, A: 255}) {
		t.Fatalf("diff pixel = %+v", r.Diff.NRGBAAt(0, 0))
	}
}
