package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestStrokeCoversLongBar(t *testing.T) {
	red := color.NRGBA{R: 220, G: 30, B: 40, A: 255}
	const w, h = 36, 16
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := paper
			if y >= 7 && y < 9 && x >= 4 && x < 32 {
				c = red
			}
			img.SetNRGBA(x, y, c)
		}
	}
	s, left := strokeLeftover(t, img)
	pick, err := (Stroke{world: s, left: left}).Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("stroke was not accepted")
	}
	path := lastPath(t, pick.doc)
	if _, ok := path.Fill(); ok {
		t.Fatal("stroke path has a fill")
	}
	stroke, ok := path.Stroke()
	if !ok {
		t.Fatal("stroke paint is off")
	}
	if stroke.Width() != 2 {
		t.Fatalf("width=%v want the short side", stroke.Width())
	}
	if stroke.Cap() != svg.CapButt {
		t.Fatalf("cap=%v want butt", stroke.Cap())
	}
	outline, err := (Outline{world: s, left: left}).Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outline.scored || pick.commands >= outline.commands {
		t.Fatalf("stroke commands=%d outline commands=%d", pick.commands, outline.commands)
	}
	got, err := render.Render(pick.doc)
	if err != nil {
		t.Fatal(err)
	}
	if score := Score(nil, got, img); score != 0 {
		t.Fatalf("score=%v want 0", score)
	}
}

func TestStrokeSkipsSquare(t *testing.T) {
	red := color.NRGBA{R: 220, G: 30, B: 40, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	s, left := strokeLeftover(t, img)
	if !(Stroke{world: s, left: left}).Applies() {
		t.Fatal("stroke should still be scheduled on a square")
	}
	pick, err := (Stroke{world: s, left: left}).Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if pick.ok {
		t.Fatal("square became a stroke")
	}
}

func TestStrokeSkipsElbow(t *testing.T) {
	red := color.NRGBA{R: 20, G: 90, B: 180, A: 255}
	const w, h = 16, 16
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var island []pix
	for x := 1; x <= 8; x++ {
		island = append(island, pix{x, 4})
	}
	for y := 5; y <= 8; y++ {
		island = append(island, pix{8, y})
	}
	for _, p := range island {
		img.SetNRGBA(p.x, p.y, red)
	}
	s, err := newWorld(nil, img)
	if err != nil {
		t.Fatal(err)
	}
	left := leftover{island: island, col: red}
	left.fresh = s.seedGrow(grow{i: -1, work: island, fill: red})
	pick, err := (Stroke{world: s, left: left}).Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if pick.ok {
		t.Fatal("elbow became a stroke")
	}
}

func TestStrokeCoversDiagonal(t *testing.T) {
	red := color.NRGBA{R: 20, G: 90, B: 180, A: 255}
	const w, h = 20, 20
	cases := []struct {
		name string
		at   func(i int) (int, int)
	}{
		{name: "down", at: func(i int) (int, int) { return 2 + i, 2 + i }},
		{name: "up", at: func(i int) (int, int) { return 2 + i, 13 - i }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, w, h))
			island := make([]pix, 12)
			for i := range island {
				x, y := tc.at(i)
				island[i] = pix{x, y}
				img.SetNRGBA(x, y, red)
			}
			s, err := newWorld(nil, img)
			if err != nil {
				t.Fatal(err)
			}
			left := leftover{island: island, col: red}
			left.fresh = s.seedGrow(grow{i: -1, work: island, fill: red})
			pick, err := (Stroke{world: s, left: left}).Run(nil)
			if err != nil {
				t.Fatal(err)
			}
			if !pick.ok {
				t.Fatal("diagonal stroke was not accepted")
			}
			path := lastPath(t, pick.doc)
			stroke, ok := path.Stroke()
			if !ok || stroke.Width() != 1 {
				t.Fatalf("stroke=%v on=%v want width 1", stroke.Width(), ok)
			}
			got, err := render.Render(pick.doc)
			if err != nil {
				t.Fatal(err)
			}
			if score := Score(nil, got, img); score != 0 {
				t.Fatalf("score=%v want 0", score)
			}
		})
	}
}

func strokeLeftover(t *testing.T, img *image.NRGBA) (*world, leftover) {
	t.Helper()
	s, err := newWorld(nil, img)
	if err != nil {
		t.Fatal(err)
	}
	var left leftover
	for _, candidate := range s.leftovers(nil) {
		if candidate.paper || candidate.region {
			continue
		}
		if len(candidate.island) > len(left.island) {
			left = candidate
		}
	}
	if len(left.island) < minIsland {
		t.Fatalf("leftover=%d", len(left.island))
	}
	return s, left
}

func lastPath(t *testing.T, doc svg.Document) svg.Path {
	t.Helper()
	kids := doc.Children()
	if len(kids) == 0 {
		t.Fatal("empty document")
	}
	path, ok := kids[len(kids)-1].Path()
	if !ok {
		t.Fatal("last node is not a path")
	}
	return path
}
