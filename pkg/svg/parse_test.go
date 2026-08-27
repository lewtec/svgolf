package svg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRoundTripGoldens(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "testdata", "svg")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if !strings.HasSuffix(ent.Name(), ".svg") {
			continue
		}
		t.Run(ent.Name(), func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Parse(strings.NewReader(string(raw)))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, err := EncodeToString(doc)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if got != string(raw) {
				t.Errorf("Encode(Parse(golden)) mismatch\n--- got ---\n%s--- want ---\n%s", got, raw)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	t.Parallel()
	doc, err := ParseFile(filepath.Join("..", "..", "testdata", "svg", "empty.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Width() != 256 || doc.Height() != 256 {
		t.Errorf("size = %v×%v", doc.Width(), doc.Height())
	}
}

func TestParseRXCopyWhenOmitted(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256">
  <rect width="10" height="10" rx="5"/>
</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	n := doc.Children()
	if len(n) != 1 {
		t.Fatalf("children = %d", len(n))
	}
	r, ok := n[0].Rect()
	if !ok {
		t.Fatal("not a rect")
	}
	if r.RX() != 5 || r.RY() != 5 {
		t.Errorf("RX,RY = %v,%v; want 5,5", r.RX(), r.RY())
	}
}

func TestParseRXBothPresentKeepsZeroRY(t *testing.T) {
	t.Parallel()
	doc, err := ParseFile(filepath.Join("..", "..", "testdata", "svg", "rx-only.svg"))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := doc.Children()[0].Rect()
	if !ok {
		t.Fatal("not a rect")
	}
	if r.RX() != 5 || r.RY() != 0 {
		t.Errorf("RX,RY = %v,%v; want 5,0", r.RX(), r.RY())
	}
}

func TestParseNoXMLNS(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(`<svg width="256" height="256"/>`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := EncodeToString(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := readGolden(t, "empty.svg")
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestParsePxUnits(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(`<svg width="256px" height="256px"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Width() != 256 || doc.Height() != 256 {
		t.Errorf("size = %v×%v", doc.Width(), doc.Height())
	}
}

func TestParseRRGGBBAATimesOpacity(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(
		`<svg width="256" height="256"><circle r="1" fill="#FF000080" fill-opacity="0.5"/></svg>`,
	))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := doc.Children()[0].Circle()
	if !ok {
		t.Fatal("not a circle")
	}
	col, present := c.Fill()
	if !present || col.R != 255 || col.G != 0 || col.B != 0 {
		t.Errorf("Fill() = %+v, %v", col, present)
	}
	want := mul8(0x80, op8FromUnit(0.5))
	got := op8FromUnit(c.FillOpacity())
	if got != want {
		t.Errorf("fill opacity 8-bit = %d; want mul8(128, 128)=%d", got, want)
	}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		xml  string
	}{
		{name: "fill on g", xml: `<svg width="256" height="256"><g fill="#FF0000"/></svg>`},
		{name: "style", xml: `<svg width="256" height="256" style="x"/></svg>`},
		{name: "transform", xml: `<svg width="256" height="256"><circle r="1" transform="scale(2)"/></svg>`},
		{name: "named color", xml: `<svg width="256" height="256"><circle r="1" fill="red"/></svg>`},
		{name: "rgb()", xml: `<svg width="256" height="256"><circle r="1" fill="rgb(0,0,0)"/></svg>`},
		{name: "winding", xml: `<svg width="256" height="256"><polygon points="0,0 1,0 0,1" fill-rule="winding"/></svg>`},
		{name: "text", xml: `<svg width="256" height="256">hello</svg>`},
		{name: "path", xml: `<svg width="256" height="256"><path d="M0 0"/></svg>`},
		{name: "em unit", xml: `<svg width="10em" height="256"/></svg>`},
		{name: "non-integer canvas", xml: `<svg width="256.5" height="256"/>`},
		{name: "negative r", xml: `<svg width="256" height="256"><circle r="-1"/></svg>`},
		{name: "too many points", xml: tooManyPointsXML()},
		{name: "stroke width only", xml: `<svg width="256" height="256"><circle r="1" stroke-width="2"/></svg>`},
		{name: "doctype", xml: `<!DOCTYPE svg><svg width="256" height="256"/>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(strings.NewReader(tt.xml))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseCommentsAndSpace(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(`
		<!-- logo -->
		<svg width="256" height="256">
		  <!-- inner -->
		  <circle r="1"/>
		</svg>
	`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children()) != 1 {
		t.Fatalf("children = %d", len(doc.Children()))
	}
}

func TestParsePolygonPointsGrammar(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(
		`<svg width="256" height="256"><polygon points="0,0, 10 0,0,10"/></svg>`,
	))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := doc.Children()[0].Polygon()
	if !ok {
		t.Fatal("not a polygon")
	}
	got := p.Points()
	want := [][2]float64{{0, 0}, {10, 0}, {0, 10}}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("points = %v; want %v", got, want)
	}
}

func TestParseOddPoints(t *testing.T) {
	t.Parallel()
	_, err := Parse(strings.NewReader(
		`<svg width="256" height="256"><polygon points="0,0 1,0 2"/></svg>`,
	))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseEmptyPolygonRejectedOnEncode(t *testing.T) {
	t.Parallel()
	doc, err := Parse(strings.NewReader(`<svg width="256" height="256"><polygon/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = EncodeToString(doc)
	if !errors.Is(err, ErrPolygonPoints) {
		t.Errorf("Encode empty polygon: %v; want ErrPolygonPoints", err)
	}
}

func tooManyPointsXML() string {
	var b strings.Builder
	b.WriteString(`<svg width="256" height="256"><polygon points="`)
	for i := 0; i < 1025; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("0,0")
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}
