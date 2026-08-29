package render

import (
	"image/color"
	"testing"
	"time"

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

func TestRenderLinearLerp(t *testing.T) {
	t.Parallel()
	g := svg.NewLinearFill(0, 8, 64, 8,
		color.NRGBA{R: 255, A: 255},
		color.NRGBA{A: 255},
	)
	d := svg.NewDocument(64, 16).Append(
		svg.NewRect().WithWidth(64).WithHeight(16).WithLinearFill(g).Node(),
	)
	img, err := Render(d)
	if err != nil {
		t.Fatal(err)
	}
	left := img.NRGBAAt(0, 8)
	if left.R < 240 || left.G != 0 || left.B != 0 || left.A != 255 {
		t.Fatalf("left=%+v want red", left)
	}
	right := img.NRGBAAt(63, 8)
	if right.R > 16 || right.G != 0 || right.B != 0 || right.A != 255 {
		t.Fatalf("right=%+v want black", right)
	}
	mid := img.NRGBAAt(32, 8)
	if mid.R < 100 || mid.R > 160 || mid.G != 0 || mid.B != 0 {
		t.Fatalf("mid=%+v want ~#7F0000", mid)
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

func TestAlphaRunsAddPastWidthDoesNotHang(t *testing.T) {
	a := newAlphaRuns(10)
	done := make(chan struct{})
	go func() {
		a.add(0, 0, 1000, 0, 64, 0)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("alphaRuns.add hung on middleCount > width")
	}
}

func TestBlitHPastBBoxDoesNotHang(t *testing.T) {
	// walkEdges blits open winding to the canvas clip; runs are bbox-sized.
	pm := newPixmap(64, 64)
	real := &solidBlitter{pm: pm, pr: 255, pa: 255}
	sb := newSuperBlitter(20, 20, 40, 40, 64, 64, real)
	if sb == nil {
		t.Fatal("superBlitter")
	}
	done := make(chan struct{})
	go func() {
		sb.blitH(0, uint32(20<<supersampleShift), 64<<supersampleShift)
		sb.flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("blitH hung on span wider than bbox")
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
