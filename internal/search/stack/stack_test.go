package stack

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestBoxBlurSpreads(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 5, 5))
	img.SetNRGBA(2, 2, color.NRGBA{R: 255, A: 255})
	out := boxBlur(img, 1)
	n := out.NRGBAAt(1, 2)
	if n.A == 0 || n.R == 0 {
		t.Fatalf("blur did not spread: %+v", n)
	}
}

func TestStartSigma(t *testing.T) {
	if startSigma(8, 8) != 2 {
		t.Fatalf("small=%d", startSigma(8, 8))
	}
	if startSigma(1000, 800) != 32 {
		t.Fatalf("large=%d", startSigma(1000, 800))
	}
	if startSigma(1, 1) != 0 {
		t.Fatalf("tiny=%d", startSigma(1, 1))
	}
}

func TestRecolorAtSplitsVisor(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	darker := color.NRGBA{R: 8, G: 24, B: 40, A: 255}
	cyan := color.NRGBA{R: 5, G: 176, B: 247, A: 255}
	if loss.ColorAt(navy, darker) >= recolorAt {
		t.Fatalf("darker navy=%v should polish", loss.ColorAt(navy, darker))
	}
	if loss.ColorAt(navy, cyan) < recolorAt {
		t.Fatalf("cyan=%v should be a new path", loss.ColorAt(navy, cyan))
	}
}

func TestStackSolid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("paths=%d want 1", n)
	}
	if _, ok := doc.Children()[0].Path(); !ok {
		t.Fatal("not a path")
	}
}

func TestStackUnblurDoesNotRestack(t *testing.T) {
	// startSigma(48)=12, so Search unblurs. leftover of the same
	// plate must polish path 0, not stack more navy.
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n != 1 {
		t.Fatalf("paths=%d want 1 (polished plate)", n)
	}
}

func TestStackTwoColorGetsBoth(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want >=2", n)
	}
}

func TestStackMarkAfterPlate(t *testing.T) {
	// navy field + black block: global hue must not block the mark
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := navy
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want >=2 (plate + mark)", n)
	}
	empty := image.NewNRGBA(img.Rect)
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if Score(got, img, len(doc.Children())) >= Score(empty, img, 0) {
		t.Fatalf("final score not better than empty")
	}
}

func TestStackKeepsGoingAfterReject(t *testing.T) {
	// three disjoint blobs: a rejected hull must not end the run
	img := image.NewNRGBA(image.Rect(0, 0, 24, 8))
	paint := func(x0, x1 int, c color.NRGBA) {
		for y := 1; y < 7; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	paint(1, 7, color.NRGBA{R: 255, A: 255})
	paint(9, 15, color.NRGBA{G: 255, A: 255})
	paint(17, 23, color.NRGBA{B: 255, A: 255})
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 3 {
		t.Fatalf("paths=%d want >=3", n)
	}
}

func TestVoidsFindsEnclosedHole(t *testing.T) {
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				continue
			}
			island = append(island, pix{x, y})
		}
	}
	hs := voids(island)
	if len(hs) != 1 {
		t.Fatalf("voids=%d want 1", len(hs))
	}
	if len(hs[0]) != 16 {
		t.Fatalf("hole px=%d want 16", len(hs[0]))
	}
}

func TestStackRingKeepsInterior(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	cyan := color.NRGBA{R: 5, G: 176, B: 247, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	ring := func(x, y int) bool {
		dx, dy := float64(x)-15.5, float64(y)-15.5
		d := dx*dx + dy*dy
		return d >= 8*8 && d < 12*12
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if ring(x, y) {
				img.SetNRGBA(x, y, cyan)
			}
		}
	}
	for y := 14; y < 18; y++ {
		for x := 14; x < 20; x++ {
			img.SetNRGBA(x, y, cyan)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	c := got.NRGBAAt(15, 5)
	if c.B < 80 {
		t.Fatalf("ring missing %+v paths=%d", c, len(doc.Children()))
	}
	mid := got.NRGBAAt(15, 11)
	if mid.G > 80 && mid.B > 180 {
		t.Fatalf("interior filled with cyan %+v", mid)
	}
	if mid.A > 200 && mid.R < 20 && mid.G < 20 && mid.B < 20 {
		t.Fatalf("interior painted black %+v", mid)
	}
	mark := got.NRGBAAt(16, 16)
	if mark.B < 80 {
		t.Fatalf("inner mark missing %+v paths=%d", mark, len(doc.Children()))
	}
}

func TestStackDoesNotKeepFilledHoles(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 4; y < 28; y++ {
		for x := 4; x < 28; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 12; y < 20; y++ {
		for x := 12; x < 20; x++ {
			img.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	c := got.NRGBAAt(16, 16)
	if c.A != 0 && c.B > 40 {
		t.Fatalf("hole still navy %+v paths=%d", c, len(doc.Children()))
	}
}

func TestStackTinyMark(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 18; y < 22; y++ {
		for x := 18; x < 22; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n < 2 {
		t.Fatalf("paths=%d want plate + 16px mark", n)
	}
}

func TestStackNilPixmap(t *testing.T) {
	_, err := search.Last((Stack{}).Search(t.Context(), nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStackCoversBeforeRefine(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var kids []int
	var first []int
	for doc, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		kids = append(kids, len(doc.Children()))
		first = append(first, pathPts(doc.Children()[0]))
	}
	grew := false
	for i := 1; i < len(kids); i++ {
		if kids[i] > kids[i-1] {
			grew = true
			if first[i] != first[i-1] {
				t.Fatalf("epoch %d: first path changed while adding a path", i)
			}
		}
	}
	if !grew {
		t.Fatalf("never covered both, kids=%v", kids)
	}
}

func pathPts(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	ncmd := 0
	for _, c := range p.Commands() {
		if c.Kind != svg.CmdClose {
			ncmd++
		}
	}
	return ncmd
}

func TestFitPolyRect(t *testing.T) {
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			island = append(island, pix{x, y})
		}
	}
	c := contour(island)
	got := fitPoly(c, 2)
	if len(got) != 4 {
		t.Fatalf("contour=%d fitPoly=%d %v", len(c), len(got), got)
	}
}

func TestStackFirstFormIsBBox(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for doc, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		p, ok := doc.Children()[0].Path()
		if !ok {
			t.Fatal("not a path")
		}
		n := 0
		for _, c := range p.Commands() {
			if c.Kind != svg.CmdClose {
				n++
			}
		}
		if n < 4 || n > 6 {
			t.Fatalf("first form points=%d want 4-6", n)
		}
		return
	}
	t.Fatal("no epoch")
}

func TestSmoothPullsStairInward(t *testing.T) {
	stair := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {3, 2}}
	got := smooth(stair, 2)
	if len(got) != len(stair) {
		t.Fatalf("len=%d", len(got))
	}
	// the (1,0) corner should move up/right toward the diagonal
	if got[1][1] <= 0 {
		t.Fatalf("stair still on axis: %v", got[1])
	}
}

func TestFitPolyKeepsConcaveL(t *testing.T) {
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x < 3 || y < 3 {
				island = append(island, pix{x, y})
			}
		}
	}
	ring := fitPoly(contour(island), 2)
	if pointInRing(ring, 5.5, 5.5) {
		t.Fatalf("notch filled, fan-order hull? %v", ring)
	}
}

func pointInRing(ring [][2]float64, x, y float64) bool {
	in := false
	n := len(ring)
	for i := 0; i < n; i++ {
		a, b := ring[i], ring[(i+1)%n]
		if (a[1] > y) != (b[1] > y) {
			t := (y - a[1]) / (b[1] - a[1])
			if x < a[0]+t*(b[0]-a[0]) {
				in = !in
			}
		}
	}
	return in
}

func TestFanOrderUncrossesBowtie(t *testing.T) {
	// two triangles sharing a vertex, edges would cross if walked 0-1-2-3
	bow := [][2]float64{{0, 0}, {2, 2}, {0, 2}, {2, 0}}
	got := fanOrder(bow)
	if len(got) != 4 {
		t.Fatalf("fanOrder=%v", got)
	}
	// no edge of the rewritten ring should properly intersect another
	for i := 0; i < 4; i++ {
		a, b := got[i], got[(i+1)%4]
		for j := i + 1; j < 4; j++ {
			c, d := got[j], got[(j+1)%4]
			if i == j || (i+1)%4 == j || (j+1)%4 == i {
				continue
			}
			if edgesCross(a, b, c, d) {
				t.Fatalf("still crosses: %v", got)
			}
		}
	}
}

func edgesCross(a, b, c, d [2]float64) bool {
	cross := func(p, q, r [2]float64) float64 {
		return (q[0]-p[0])*(r[1]-p[1]) - (q[1]-p[1])*(r[0]-p[0])
	}
	d1, d2 := cross(a, b, c), cross(a, b, d)
	d3, d4 := cross(c, d, a), cross(c, d, b)
	return d1*d2 < 0 && d3*d4 < 0
}

func TestFitPolyShorterThanContour(t *testing.T) {
	var ring [][2]float64
	for i := 0; i < 40; i++ {
		ring = append(ring, [2]float64{float64(i), 0})
	}
	for i := 0; i < 40; i++ {
		ring = append(ring, [2]float64{40, float64(i)})
	}
	got := fitPoly(ring, 4)
	if len(got) >= len(ring) {
		t.Fatalf("fitPoly=%d contour=%d", len(got), len(ring))
	}
	if len(got) < 3 {
		t.Fatalf("fitPoly=%v", got)
	}
}

func TestThinIsland(t *testing.T) {
	var bar []pix
	for x := 0; x < 12; x++ {
		bar = append(bar, pix{x, 3})
	}
	if !thinIsland(bar) {
		t.Fatal("1px bar should be scatter")
	}
	var box []pix
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			box = append(box, pix{x, y})
		}
	}
	if thinIsland(box) {
		t.Fatal("4x4 not scatter")
	}
}

func TestFitSharpStaysLines(t *testing.T) {
	var island []pix
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			island = append(island, pix{x, y})
		}
	}
	sq := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	p := filledFit(island, sq, color.NRGBA{A: 255})
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			t.Fatalf("square used a cubic: %+v", p.Commands())
		}
	}
}

func TestFitBendHasCubic(t *testing.T) {
	var island []pix
	for y := 0; y <= 16; y++ {
		for x := 0; x <= 16; x++ {
			if x*x+y*y <= 16*16 && x >= 0 && y >= 0 {
				island = append(island, pix{x, y})
			}
		}
	}
	s := math.Sqrt(2) / 2
	bend := [][2]float64{{16, 0}, {16 * math.Cos(math.Pi/6), 16 * math.Sin(math.Pi/6)}, {16 * s, 16 * s}, {16 * math.Cos(math.Pi/3), 16 * math.Sin(math.Pi/3)}, {0, 16}, {0, 0}}
	p := filledFit(island, bend, color.NRGBA{A: 255})
	n := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			n++
		}
	}
	if n == 0 {
		t.Fatal("arc used only lines")
	}
}

func TestRDPClosedRectangle(t *testing.T) {
	var ring [][2]float64
	for x := 0; x < 10; x++ {
		ring = append(ring, [2]float64{float64(x), 0})
	}
	for y := 1; y < 8; y++ {
		ring = append(ring, [2]float64{9, float64(y)})
	}
	for x := 8; x >= 0; x-- {
		ring = append(ring, [2]float64{float64(x), 7})
	}
	for y := 6; y >= 1; y-- {
		ring = append(ring, [2]float64{0, float64(y)})
	}
	got := rdpClosed(ring, 1)
	if len(got) != 4 {
		t.Fatalf("rdpClosed rect=%v want 4 corners", got)
	}
}

func TestRDPCollinear(t *testing.T) {
	got := rdp([][2]float64{{0, 0}, {1, 0}, {2, 0}, {3, 0}}, 0.5)
	if len(got) != 2 {
		t.Fatalf("rdp=%v", got)
	}
}

func TestStackDiskUsesCubics(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			if dx*dx+dy*dy <= 12*12 {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			}
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := cubics(doc.Children()[0]); n < 4 {
		t.Fatalf("cubics=%d want >=4", n)
	}
}

func TestStackGradientDoesNotRestack(t *testing.T) {
	// flat fill never matches a ramp. without skip-after-accept the same
	// CC is covered again until maxPaths.
	img := image.NewNRGBA(image.Rect(0, 0, 80, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 3), A: 255})
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	doc, err := search.Last((Stack{}).Search(ctx, img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n > 8 {
		t.Fatalf("paths=%d, restacking the ramp", n)
	}
}

func TestStackSkipsSpeckles(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for _, o := range [][2]int{{4, 4}, {20, 6}, {36, 5}, {8, 22}, {28, 24}, {12, 38}} {
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				img.SetNRGBA(o[0]+x, o[1]+y, color.NRGBA{A: 255})
			}
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(doc.Children()); n > 2 {
		t.Fatalf("paths=%d want plate, not 3x3 sprinkles", n)
	}
}

func cubics(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	k := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			k++
		}
	}
	return k
}

func TestFilledEllipseUsesCubics(t *testing.T) {
	p := filledEllipse(10, 10, 8, 8, color.NRGBA{R: 255, A: 255})
	n := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("cubics=%d want 4", n)
	}
}

func TestHullSquare(t *testing.T) {
	h := convexHull([][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}})
	if len(h) != 4 {
		t.Fatalf("hull=%v", h)
	}
}
