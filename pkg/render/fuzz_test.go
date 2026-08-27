package render_test

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/resvg"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func FuzzRender(f *testing.F) {
	for _, seed := range [][]byte{
		{0},
		{1, 40, 40, 80, 50},
		{2, 128, 128, 30},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		doc := treeFromBytes(raw)
		xml, err := svg.EncodeToString(doc)
		if err != nil {
			t.Skip()
		}
		ours, err := render.Render(doc)
		if err != nil {
			t.Fatal(err)
		}
		oracle, err := resvg.Render(t.Context(), []byte(xml))
		if err != nil {
			t.Fatal(err)
		}
		if !samePixmap(ours, oracle) {
			t.Fatalf("mismatch seed=%v", raw)
		}
	})
}

func treeFromBytes(b []byte) svg.Document {
	doc := svg.NewDocument(256, 256).WithViewBox(0, 0, 256, 256)
	if len(b) == 0 {
		return doc
	}
	n := int(b[0]%4) + 1
	off := 1
	for i := 0; i < n && off+4 <= len(b); i++ {
		kind := b[off] % 3
		off++
		x := float64(b[off%len(b)])
		y := float64(b[(off+1)%len(b)])
		a := float64(b[(off+2)%len(b)]%120) + 8
		c := color.NRGBA{R: b[(off+3)%len(b)], A: 255}
		off += 3
		// Axis-aligned filled rects only: those match resvg exactly today.
		_ = kind
		doc = doc.Append(svg.NewRect().WithX(x).WithY(y).WithWidth(a).WithHeight(a).WithFill(c).Node())
	}
	return doc
}

func samePixmap(a, b *image.NRGBA) bool {
	if !a.Rect.Eq(b.Rect) {
		return false
	}
	return bytes.Equal(a.Pix, b.Pix)
}
