package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func rectIsland(x0, y0, x1, y1 int) []pix {
	var island []pix
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			island = append(island, pix{x, y})
		}
	}
	return island
}

func TestMaskVerticesFindsRectangleCorners(t *testing.T) {
	island := rectIsland(3, 2, 11, 10)
	verts := maskVertices(island)
	if len(verts) < 4 {
		t.Fatalf("vertices=%d want the four leftover corners: %v", len(verts), verts)
	}
	want := map[[2]float64]bool{
		{3, 2}:   true,
		{11, 2}:  true,
		{11, 10}: true,
		{3, 10}:  true,
	}
	hit := 0
	for _, v := range verts {
		if want[v.p] {
			hit++
		}
	}
	if hit < 4 {
		t.Fatalf("corners %v missed leftover rectangle corners %v", verts, want)
	}
}

func TestMaskVerticesIgnoresStairTeeth(t *testing.T) {
	var island []pix
	for y := 0; y < 16; y++ {
		for x := y; x < 16; x++ {
			island = append(island, pix{x, y})
		}
	}
	verts := maskVertices(island)
	if len(verts) > 8 {
		t.Fatalf("vertices=%d want the plate corners, not every stair tooth: %v", len(verts), verts)
	}
	if len(verts) < 3 {
		t.Fatalf("vertices=%d want the stair plate corners", len(verts))
	}
}

func TestOneMaskRectangleIsQuad(t *testing.T) {
	island := rectIsland(3, 2, 11, 10)
	ring := oneMaskRectangle(island)
	if len(ring) != 4 {
		t.Fatalf("ring=%d want a rectangle: %v", len(ring), ring)
	}
	want := map[[2]float64]bool{
		{3, 2}:   true,
		{11, 2}:  true,
		{11, 10}: true,
		{3, 10}:  true,
	}
	for _, q := range ring {
		if !want[q] {
			t.Fatalf("vertex %v is not a leftover rectangle corner", q)
		}
	}
	if quadArea2(ring) == 0 {
		t.Fatalf("degenerate rectangle: %v", ring)
	}
}

func TestRectanglePlacesPlate(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 2; y < 10; y++ {
		for x := 2; x < 14; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	s, err := newWorld(img)
	if err != nil {
		t.Fatal(err)
	}
	lefts := s.leftovers()
	if len(lefts) == 0 {
		t.Fatal("no leftover")
	}
	pick, err := (Rectangle{world: s, left: lefts[0]}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("rectangle=false want leftover plate")
	}
	p, ok := pick.doc.Children()[len(pick.doc.Children())-1].Path()
	if !ok {
		t.Fatal("not a path")
	}
	n := 0
	for _, c := range p.Commands() {
		if c.Kind != svg.CmdClose {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("cmds=%d want one leftover rectangle", n)
	}
}
