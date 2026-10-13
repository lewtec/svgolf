package stack

import (
	"image"
	"image/color"
	"testing"
)

func TestPaintChoicesFollowHorizontalRamp(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 8))
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x*5), G: 30, B: 40, A: 255})
			island = append(island, pix{x, y})
		}
	}
	choices := paintChoices(island, img)
	var linear int
	for _, choice := range choices {
		if !choice.linear {
			continue
		}
		linear++
		dx := choice.grad.X2() - choice.grad.X1()
		dy := choice.grad.Y2() - choice.grad.Y1()
		if dx*dx < dy*dy {
			t.Fatalf("axis dx=%v dy=%v want the horizontal ramp", dx, dy)
		}
	}
	if linear == 0 {
		t.Fatal("ramp produced no gradient")
	}
}

func TestMonotoneSpanDropsStair(t *testing.T) {
	verts := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {2, 4}, {0, 4}}
	ring := polylineRing(verts)
	spans := monotoneSpans(ring)
	if len(spans) == 0 {
		t.Fatal("stair produced no span")
	}
	samples := spanPoints(ring.verts, spans[0].from, spans[0].to)
	cmds := spanCommands(samples)
	next := ring.replaceSpan(spans[0].from, spans[0].to, cmds)
	if len(next.verts) >= len(ring.verts) {
		t.Fatalf("verts %d -> %d", len(ring.verts), len(next.verts))
	}
	if len(next.verts) < 3 {
		t.Fatalf("collapsed to %d", len(next.verts))
	}
}

func TestSingleEdgeIsNotASpline(t *testing.T) {
	ring := polylineRing([][2]float64{{0, 0}, {4, 0}, {4, 3}, {0, 3}})
	if spans := monotoneSpans(ring); len(spans) == 0 {
		return
	} else {
		for _, span := range spans {
			if edgesInSpan(span.from, span.to, 4) < 2 {
				t.Fatalf("span %v is a single edge", span)
			}
		}
	}
}
