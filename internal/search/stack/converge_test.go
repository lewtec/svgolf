package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/pkg/render"
)

// Flat shapes that are not one triangle must reach Score zero.
// The largest-area ear spills, and a smoothed polygon changes
// pixels, so Score keeps the mask outline unless a simpler
// plate matches it exactly.
func TestOutlineConvergesFlatShapes(t *testing.T) {
	red := color.NRGBA{R: 220, G: 30, B: 40, A: 255}
	blue := color.NRGBA{R: 20, G: 90, B: 180, A: 255}
	cases := []struct {
		name    string
		w, h    int
		inside  func(x, y int) bool
		outline bool
	}{
		{
			name: "ell", w: 40, h: 40, outline: true,
			inside: func(x, y int) bool {
				return (x >= 6 && x < 16 && y >= 6 && y < 34) || (x >= 6 && x < 34 && y >= 24 && y < 34)
			},
		},
		{
			name: "disk", w: 40, h: 40, outline: true,
			inside: func(x, y int) bool {
				dx, dy := x-20, y-20
				return dx*dx+dy*dy <= 12*12
			},
		},
		{
			name: "annulus", w: 40, h: 40, outline: true,
			inside: func(x, y int) bool {
				dx, dy := x-20, y-20
				rr := dx*dx + dy*dy
				return rr <= 14*14 && rr >= 6*6
			},
		},
		{
			name: "stroke", w: 36, h: 16, outline: true,
			inside: func(x, y int) bool {
				return y >= 7 && y < 9 && x >= 4 && x < 32
			},
		},
		{
			name: "two", w: 48, h: 32,
			inside: func(x, y int) bool {
				if y < 6 || y >= 26 {
					return false
				}
				return (x >= 4 && x < 20) || (x >= 28 && x < 44)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h))
			for y := 0; y < tc.h; y++ {
				for x := 0; x < tc.w; x++ {
					c := paper
					if tc.inside(x, y) {
						c = red
						if tc.name == "two" && x >= 28 {
							c = blue
						}
					}
					img.SetNRGBA(x, y, c)
				}
			}
			outlined := false
			var got *image.NRGBA
			epochs := 0
			for ep, err := range (Stack{}).Search(mustCtx(t), img) {
				if err != nil {
					t.Fatal(err)
				}
				epochs++
				if ep.Operator == OpOutline {
					outlined = true
				}
				got, err = render.Render(ep.Document)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got == nil {
				t.Fatal("no epoch")
			}
			if score := Score(got, img); score != 0 {
				t.Fatalf("score=%.3f epochs=%d want 0", score, epochs)
			}
			if tc.outline && !outlined {
				t.Fatal("outline was not accepted")
			}
		})
	}
}
