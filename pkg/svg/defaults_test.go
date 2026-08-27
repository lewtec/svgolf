package svg

import (
	"image/color"
	"testing"
)

func TestNewZerosAreSVGDefaults(t *testing.T) {
	t.Parallel()

	c := NewCircle()
	if c.CX() != 0 || c.CY() != 0 || c.R() != 0 {
		t.Fatalf("circle geometry = (%v,%v,%v); want zeros", c.CX(), c.CY(), c.R())
	}
	col, ok := c.Fill()
	if !ok {
		t.Fatal("default fill is none; want black")
	}
	if col != (color.NRGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Errorf("Fill() = %+v; want black A=255", col)
	}
	if c.FillOpacity() != 1 {
		t.Errorf("FillOpacity() = %v; want 1", c.FillOpacity())
	}
	if c.FillRule() != FillNonZero {
		t.Errorf("FillRule() = %v; want FillNonZero", c.FillRule())
	}
	if _, on := c.Stroke(); on {
		t.Error("default stroke is on; want none")
	}

	s := NewStroke()
	if s.Color() != (color.NRGBA{A: 255}) {
		t.Errorf("Stroke.Color() = %+v; want black A=255", s.Color())
	}
	if s.Opacity() != 1 {
		t.Errorf("Stroke.Opacity() = %v; want 1", s.Opacity())
	}
	if s.Width() != 1 {
		t.Errorf("Stroke.Width() = %v; want 1", s.Width())
	}
	if s.Cap() != CapButt {
		t.Errorf("Cap() = %v; want CapButt", s.Cap())
	}
	if s.Join() != JoinMiter {
		t.Errorf("Join() = %v; want JoinMiter", s.Join())
	}
	if s.MiterLimit() != 4 {
		t.Errorf("MiterLimit() = %v; want 4", s.MiterLimit())
	}

	e := NewEllipse()
	if e.CX() != 0 || e.CY() != 0 || e.RX() != 0 || e.RY() != 0 {
		t.Fatalf("ellipse geometry not zero: %+v", e)
	}
	r := NewRect()
	if r.X() != 0 || r.Y() != 0 || r.Width() != 0 || r.Height() != 0 || r.RX() != 0 || r.RY() != 0 {
		t.Fatalf("rect geometry not zero: %+v", r)
	}
	d := NewDocument(256, 128)
	if d.Width() != 256 || d.Height() != 128 {
		t.Errorf("NewDocument size = %v×%v", d.Width(), d.Height())
	}
	if d.ViewBox().Set() {
		t.Error("default viewBox is set")
	}
}

func TestPublicConstructorsSetKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  Kind
		want Kind
	}{
		{"group", NewGroup().Node().Kind(), KindGroup},
		{"circle", NewCircle().Node().Kind(), KindCircle},
		{"ellipse", NewEllipse().Node().Kind(), KindEllipse},
		{"rect", NewRect().Node().Kind(), KindRect},
		{"polygon", NewPolygon().Node().Kind(), KindPolygon},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("Kind() = %v; want %v", tt.got, tt.want)
			}
			if tt.got == KindInvalid {
				t.Error("public constructor produced KindInvalid")
			}
		})
	}
	if (Node{}).Kind() != KindInvalid {
		t.Errorf("zero Node Kind() = %v; want KindInvalid", (Node{}).Kind())
	}
}

func TestWithFillAndOpacity(t *testing.T) {
	t.Parallel()
	c := NewCircle().WithFill(color.NRGBA{R: 255, A: 128})
	col, ok := c.Fill()
	if !ok {
		t.Fatal("fill missing after WithFill")
	}
	if col.R != 255 || col.A != 255 {
		t.Errorf("Fill() = %+v; want R=255 A=255 (opacity not folded)", col)
	}
	if c.FillOpacity() != 128.0/255 {
		t.Errorf("FillOpacity() = %v; want 128/255", c.FillOpacity())
	}
	c = c.WithFillOpacity(0.5)
	wantOp := float64(op8FromUnit(0.5)) / 255
	if c.FillOpacity() != wantOp {
		t.Errorf("after WithFillOpacity(0.5) = %v; want %v", c.FillOpacity(), wantOp)
	}
	c = c.WithFillNone()
	if _, ok := c.Fill(); ok {
		t.Error("Fill() present after WithFillNone")
	}
}

func TestStrokeColorSetsOpacity(t *testing.T) {
	t.Parallel()
	s := NewStroke().WithColor(color.NRGBA{B: 255, A: 64})
	if s.Color() != (color.NRGBA{B: 255, A: 255}) {
		t.Errorf("Color() = %+v", s.Color())
	}
	if s.Opacity() != 64.0/255 {
		t.Errorf("Opacity() = %v; want 64/255", s.Opacity())
	}
	s = s.WithOpacity(1)
	if s.Opacity() != 1 {
		t.Errorf("after WithOpacity(1) = %v", s.Opacity())
	}
	c := NewCircle().WithStroke(s)
	got, on := c.Stroke()
	if !on {
		t.Fatal("stroke not on")
	}
	if got.Width() != 1 {
		t.Errorf("Width() = %v; want 1", got.Width())
	}
	c = c.WithoutStroke()
	if _, on := c.Stroke(); on {
		t.Error("stroke still on after WithoutStroke")
	}
}

func TestFillOpacityClamp(t *testing.T) {
	t.Parallel()
	c := NewCircle().WithFillOpacity(-1)
	if c.FillOpacity() != 0 {
		t.Errorf("clamp low = %v; want 0", c.FillOpacity())
	}
	c = NewCircle().WithFillOpacity(2)
	if c.FillOpacity() != 1 {
		t.Errorf("clamp high = %v; want 1", c.FillOpacity())
	}
}
