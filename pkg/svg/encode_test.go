package svg

import (
	"errors"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeGoldens(t *testing.T) {
	t.Parallel()
	poly, err := NewPolygon().WithPoints([][2]float64{{0, 0}, {10, 0}, {0, 10}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		doc  Document
	}{
		{name: "empty.svg", doc: NewDocument(256, 256)},
		{
			name: "rx-only.svg",
			doc: NewDocument(256, 256).Append(
				NewRect().WithWidth(10).WithHeight(10).WithRX(5).Node(),
			),
		},
		{
			name: "circle.svg",
			doc: NewDocument(256, 256).Append(
				NewCircle().WithCX(50).WithCY(50).WithR(40).Node(),
			),
		},
		{
			name: "paint.svg",
			doc: NewDocument(256, 256).Append(
				NewRect().WithWidth(100).WithHeight(80).
					WithFill(color.NRGBA{R: 255, A: 255}).
					WithFillOpacity(0.5).
					WithFillRule(FillEvenOdd).
					WithStroke(NewStroke().WithWidth(2).WithCap(CapRound)).
					Node(),
			),
		},
		{
			name: "nested.svg",
			doc: NewDocument(256, 256).Append(
				NewGroup().Append(NewCircle().WithR(10).Node()).Node(),
			),
		},
		{name: "polygon.svg", doc: NewDocument(256, 256).Append(poly.Node())},
		{
			name: "path-tri.svg",
			doc: NewDocument(256, 256).Append(
				NewPath().MoveTo(40, 40).LineTo(120, 40).LineTo(80, 100).Close().Node(),
			),
		},
		{
			name: "path-cubic.svg",
			doc: NewDocument(256, 256).Append(
				NewPath().MoveTo(40, 120).CubicTo(40, 40, 120, 40, 120, 120).CubicTo(120, 200, 40, 200, 40, 120).Close().Node(),
			),
		},
		{
			name: "path-stroke.svg",
			doc: NewDocument(256, 256).Append(
				NewPath().MoveTo(30, 180).LineTo(125, 40).LineTo(220, 180).
					WithFillNone().
					WithStroke(NewStroke().WithWidth(4)).
					Node(),
			),
		},
		{name: "viewbox.svg", doc: NewDocument(256, 256).WithViewBox(0, 0, 100, 50)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := EncodeToString(tt.doc)
			if err != nil {
				t.Fatal(err)
			}
			want := readGolden(t, tt.name)
			if got != want {
				t.Errorf("Encode mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
			}
		})
	}
}

func TestEncodeOmitsNegZero(t *testing.T) {
	t.Parallel()
	neg := math.Copysign(0, -1)
	doc := NewDocument(256, 256).Append(NewCircle().WithCX(neg).WithCY(neg).WithR(1).Node())
	got, err := EncodeToString(doc)
	if err != nil {
		t.Fatal(err)
	}
	if containsMinusZero(got) {
		t.Errorf("encoded -0: %s", got)
	}
}

func TestEncodeOpacityRoundTrip(t *testing.T) {
	t.Parallel()
	for u := range 256 {
		s := encodeOpacity(uint8(u))
		if op8FromUnit(mustFloat(s)) != uint8(u) {
			t.Fatalf("encodeOpacity(%d)=%q parses to %d", u, s, op8FromUnit(mustFloat(s)))
		}
	}
}

func TestEncodeRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		doc  Document
	}{
		{name: "zero size", doc: NewDocument(0, 256)},
		{name: "non-integer", doc: NewDocument(256.5, 256)},
		{name: "too big", doc: NewDocument(8192, 256)},
		{name: "negative r", doc: NewDocument(256, 256).Append(NewCircle().WithR(-1).Node())},
		{name: "empty polygon", doc: NewDocument(256, 256).Append(NewPolygon().Node())},
		{name: "invalid node", doc: NewDocument(256, 256).Append(Node{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := EncodeToString(tt.doc)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
	_, err := EncodeToString(NewDocument(256, 256).Append(NewPolygon().Node()))
	if !errors.Is(err, ErrPolygonPoints) {
		t.Errorf("empty polygon: %v; want ErrPolygonPoints", err)
	}
}

func TestEncodeStrokeBlackEmitted(t *testing.T) {
	t.Parallel()
	doc := NewDocument(256, 256).Append(
		NewCircle().WithR(5).WithStroke(NewStroke()).Node(),
	)
	got, err := EncodeToString(doc)
	if err != nil {
		t.Fatal(err)
	}
	if want := `stroke="#000000"`; !strings.Contains(got, want) {
		t.Errorf("missing %s in %s", want, got)
	}
	if strings.Contains(got, "stroke-width") {
		t.Errorf("default stroke-width emitted: %s", got)
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "svg", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func containsMinusZero(s string) bool {
	return strings.Contains(s, `="-0"`) || strings.Contains(s, `="-0 `) || strings.Contains(s, ` -0"`) || strings.Contains(s, ` -0 `)
}
