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

func TestRenderViewBoxIdentity(t *testing.T) {
	t.Parallel()
	d := svg.NewDocument(256, 256).WithViewBox(0, 0, 256, 256)
	if _, err := Render(d); err != nil {
		t.Fatal(err)
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

func TestCircleEdgeCount(t *testing.T) {
	p, ok := flattenEllipse(0, 0, 10, 10)
	if !ok {
		t.Fatal("flatten")
	}
	t.Logf("segs=%d bounds=(%v,%v)-(%v,%v)", len(p.segs), p.minX, p.minY, p.maxX, p.maxY)
	e := buildLineEdges(p, supersampleShift)
	t.Logf("edges=%d", len(e))
	for i, ed := range e {
		t.Logf("edge[%d] firstY=%d lastY=%d x=%d dx=%d wind=%d cub=%v", i, ed.firstY, ed.lastY, ed.x, ed.dx, ed.winding, ed.cub != nil)
	}
	if len(e) < 2 {
		t.Fatalf("too few edges")
	}
}

func TestFillPathSmallCircle(t *testing.T) {
	p, ok := flattenEllipse(0, 0, 10, 10)
	if !ok {
		t.Fatal("flatten")
	}
	pm := newPixmap(256, 256)
	fillPath(pm, p, true, color.NRGBA{A: 255}, 255)
	img := pm.toNRGBA()
	var nz int
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			nz++
		}
	}
	if nz == 0 {
		t.Fatal("fillPath painted nothing")
	}
	t.Logf("nonzero=%d a(5,5)=%d", nz, img.NRGBAAt(5, 5).A)
}

func TestNestedCircleFillsOrigin(t *testing.T) {
	t.Parallel()
	d := svg.NewDocument(256, 256).Append(
		svg.NewGroup().Append(svg.NewCircle().WithR(10).Node()).Node(),
	)
	img, err := Render(d)
	if err != nil {
		t.Fatal(err)
	}
	c := img.NRGBAAt(5, 5)
	if c.A == 0 {
		t.Fatalf("expected fill at (5,5), got %+v", c)
	}
	c0 := img.NRGBAAt(0, 0)
	if c0.A == 0 {
		t.Fatalf("expected fill at (0,0), got %+v", c0)
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
