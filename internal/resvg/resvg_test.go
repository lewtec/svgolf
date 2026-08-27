package resvg

import (
	"bytes"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestLookPath(t *testing.T) {
	if _, err := LookPath(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderEmpty(t *testing.T) {
	xml, err := svg.EncodeToString(svg.NewDocument(256, 256))
	if err != nil {
		t.Fatal(err)
	}
	img, err := Render(t.Context(), []byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if img.Rect.Dx() != 256 || img.Rect.Dy() != 256 {
		t.Fatalf("size = %v", img.Rect)
	}
	if !bytes.Equal(img.Pix, make([]byte, len(img.Pix))) {
		t.Fatal("empty svg is not all zeros")
	}
}
