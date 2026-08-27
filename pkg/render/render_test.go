package render

import (
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestRenderEmptyIsZeros(t *testing.T) {
	t.Parallel()
	img, err := Render(svg.NewDocument(256, 256))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != img.Rect || img.Rect.Dx() != 256 || img.Rect.Dy() != 256 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	for i, b := range img.Pix {
		if b != 0 {
			t.Fatalf("pix[%d]=%d; want 0", i, b)
		}
	}
}

func TestRenderViewBoxRejected(t *testing.T) {
	t.Parallel()
	d := svg.NewDocument(256, 256).WithViewBox(0, 0, 256, 256)
	if _, err := Render(d); err == nil {
		t.Fatal("expected viewBox error")
	}
}

func TestRenderRejectsBadCanvas(t *testing.T) {
	t.Parallel()
	if _, err := Render(svg.NewDocument(256.5, 256)); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderFilledRectDoesNotPanic(t *testing.T) {
	t.Parallel()
	d := svg.NewDocument(256, 256).Append(
		svg.NewRect().WithWidth(100).WithHeight(80).WithX(10).WithY(10).
			WithFill(color.NRGBA{R: 255, A: 255}).Node(),
	)
	img, err := Render(d)
	if err != nil {
		t.Fatal(err)
	}
	// Interior sample; not an oracle. Just prove paint happened.
	c := img.NRGBAAt(50, 50)
	if c.R == 0 && c.A == 0 {
		t.Fatalf("interior still empty: %+v", c)
	}
}

func TestPremultiplyU8(t *testing.T) {
	t.Parallel()
	if premultiplyU8(255, 255) != 255 {
		t.Fatal(premultiplyU8(255, 255))
	}
	if premultiplyU8(255, 0) != 0 {
		t.Fatal(premultiplyU8(255, 0))
	}
	if demultiplyU8(0, 0) != 0 {
		t.Fatal("demultiply 0/0")
	}
}
