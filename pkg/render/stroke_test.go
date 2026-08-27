package render

import (
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestStrokeRectRingDetectsBox(t *testing.T) {
	r := svg.NewRect().WithWidth(100).WithHeight(80)
	p, ok := flattenRect(r)
	if !ok {
		t.Fatal("flatten")
	}
	rp, ok := strokeRectRing(p, 2)
	if !ok {
		t.Fatal("expected axis-aligned ring")
	}
	if len(rp.segs) < 8 {
		t.Fatalf("segs=%d", len(rp.segs))
	}
}
