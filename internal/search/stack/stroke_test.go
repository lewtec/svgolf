package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.True(t, pick.ok)
	path := lastPath(t, pick.doc)
	_, filled := path.Fill()
	require.False(t, filled)
	stroke, ok := path.Stroke()
	require.True(t, ok)
	require.Equal(t, 2.0, stroke.Width())
	require.Equal(t, svg.CapButt, stroke.Cap())
	outline, err := (Outline{world: s, left: left}).Run(nil)
	require.NoError(t, err)
	require.True(t, outline.scored)
	require.Less(t, pick.commands, outline.commands)
	got, err := render.Render(pick.doc)
	require.NoError(t, err)
	require.Zero(t, Score(nil, got, img))
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
	require.True(t, (Stroke{world: s, left: left}).Applies())
	pick, err := (Stroke{world: s, left: left}).Run(nil)
	require.NoError(t, err)
	require.False(t, pick.ok)
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
	require.NoError(t, err)
	left := leftover{island: island, col: red}
	left.fresh = s.seedGrow(grow{i: -1, work: island, fill: red})
	pick, err := (Stroke{world: s, left: left}).Run(nil)
	require.NoError(t, err)
	require.False(t, pick.ok)
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
			require.NoError(t, err)
			left := leftover{island: island, col: red}
			left.fresh = s.seedGrow(grow{i: -1, work: island, fill: red})
			pick, err := (Stroke{world: s, left: left}).Run(nil)
			require.NoError(t, err)
			require.True(t, pick.ok)
			path := lastPath(t, pick.doc)
			stroke, ok := path.Stroke()
			require.True(t, ok)
			require.Equal(t, 1.0, stroke.Width())
			got, err := render.Render(pick.doc)
			require.NoError(t, err)
			require.Zero(t, Score(nil, got, img))
		})
	}
}

func strokeLeftover(t *testing.T, img *image.NRGBA) (*world, leftover) {
	t.Helper()
	s, err := newWorld(nil, img)
	require.NoError(t, err)
	var left leftover
	for _, candidate := range s.leftovers(nil) {
		if candidate.paper || candidate.region {
			continue
		}
		if len(candidate.island) > len(left.island) {
			left = candidate
		}
	}
	require.GreaterOrEqual(t, len(left.island), minIsland)
	return s, left
}

func lastPath(t *testing.T, doc svg.Document) svg.Path {
	t.Helper()
	kids := doc.Children()
	require.NotEmpty(t, kids)
	path, ok := kids[len(kids)-1].Path()
	require.True(t, ok)
	return path
}
