package stack

import (
	"image"
	"image/color"
	"testing"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestChainStraightDiagonalStair(t *testing.T) {
	stair := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {3, 2}, {3, 3}}
	if !chainStraight(stair) {
		t.Fatal("unit diagonal stair is one digital line")
	}
	ell := [][2]float64{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {3, 1}, {3, 2}, {3, 3}}
	if chainStraight(ell) {
		t.Fatal("an ell bend is not a straight run")
	}
}

func TestStraightFormTriangleOutline(t *testing.T) {
	const n = 12
	var island []pix
	for y := 0; y < n; y++ {
		for x := 0; x < n-y; x++ {
			island = append(island, pix{x, y})
		}
	}
	outline := coverRing(island)
	ring := polylineRing(outline)
	for steps := 0; len(ring.verts) > 3 && steps < len(outline); steps++ {
		next := dropFirstStraight(ring)
		if len(next.verts) < 3 {
			break
		}
		ring = next
	}
	if len(ring.verts) != 3 {
		t.Fatalf("verts=%d want the three corners, outline=%d %v", len(ring.verts), len(outline), ring.verts)
	}
}

func dropFirstStraight(r pathRing) pathRing {
	for i := 0; i < len(r.verts); i++ {
		if r.straightVertex(i) {
			return r.dropVertex(i)
		}
	}
	return pathRing{}
}

func TestStraightFormKeepsEll(t *testing.T) {
	var island []pix
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if x < 6 || y >= 10 {
				island = append(island, pix{x, y})
			}
		}
	}
	outline := coverRing(island)
	ring := outline
	if dropped := dropFirstStraight(polylineRing(outline)); len(dropped.verts) >= 3 {
		ring = dropped.verts
	}
	if pointInRing(ring, 10.5, 4.5) {
		t.Fatalf("notch filled by straight form %v", ring)
	}
	if len(ring) != len(outline) {
		t.Fatalf("ell verts=%d outline=%d want the corners left alone", len(ring), len(outline))
	}
}

func TestSimplifyCollapsesStairToTriangle(t *testing.T) {
	const n = 16
	red := color.NRGBA{R: 255, A: 255}
	tri := [][2]float64{{0, 0}, {float64(n), 0}, {0, float64(n)}}
	doc := svg.NewDocument(float64(n), float64(n)).WithViewBox(0, 0, float64(n), float64(n))
	doc = doc.Append(whitePane(n, n).Node()).Append(filledPath(tri, red).Node())
	want, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	// The hard mask of that triangle. Its outline is the staircase
	// coverRing keeps; the form underneath is the three corners.
	var island []pix
	for y := 0; y < n; y++ {
		for x := 0; x < n-y; x++ {
			island = append(island, pix{x, y})
		}
	}
	outline := coverRing(island)
	if len(outline) <= 3 {
		t.Fatalf("outline=%d want a stair to collapse", len(outline))
	}
	stair := svg.NewDocument(float64(n), float64(n)).WithViewBox(0, 0, float64(n), float64(n))
	stair = stair.Append(whitePane(n, n).Node()).Append(filledPath(outline, red).Node())
	got, err := render.Render(stair)
	if err != nil {
		t.Fatal(err)
	}
	s := &world{
		want:   want,
		got:    got,
		wantP:  loss.NewPlane(want),
		gotP:   loss.NewPlane(got),
		doc:    stair,
		owner:  make([]uint16, n*n),
		fills:  []color.NRGBA{red},
		paths:  1,
		w:      n,
		h:      n,
		errSum: Score(got, want),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	before := s.errSum
	var last svg.Path
	for step := 0; step < len(outline); step++ {
		pick, err := (Simplify{world: s, buckets: [][]pix{nil}}).Run()
		if err != nil {
			t.Fatal(err)
		}
		if !pick.ok {
			break
		}
		if pick.errSum > s.errSum {
			t.Fatalf("step %d score %.3f → %.3f", step, s.errSum, pick.errSum)
		}
		s.apply(pick)
		p, ok := s.doc.Children()[1].Path()
		if !ok {
			t.Fatal("not a path")
		}
		last = p
	}
	if s.errSum > before {
		t.Fatalf("score %.3f → %.3f", before, s.errSum)
	}
	if verts := pathPts(last.Node()); verts != 3 {
		t.Fatalf("verts=%d want 3 after one vertex per step, outline was %d", verts, len(outline))
	}
}

func TestSimplifyDoesNotFillEll(t *testing.T) {
	red := color.NRGBA{R: 220, G: 30, B: 40, A: 255}
	const w, h = 24, 24
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var island []pix
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			inside := x < 8 || y >= 16
			c := paper
			if inside {
				c = red
				island = append(island, pix{x, y})
			}
			img.SetNRGBA(x, y, c)
		}
	}
	outline := coverRing(island)
	doc := svg.NewDocument(w, h).WithViewBox(0, 0, w, h)
	doc = doc.Append(whitePane(w, h).Node()).Append(filledPath(outline, red).Node())
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := &world{
		want:   img,
		got:    got,
		wantP:  loss.NewPlane(img),
		gotP:   loss.NewPlane(got),
		doc:    doc,
		owner:  make([]uint16, w*h),
		fills:  []color.NRGBA{red},
		paths:  1,
		w:      w,
		h:      h,
		errSum: Score(got, img),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	pick, err := (Simplify{world: s, buckets: [][]pix{nil}}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if pick.ok {
		s.apply(pick)
		p, _ := s.doc.Children()[1].Path()
		if pointInRing(pathOuter(p.Node()), 14.5, 6.5) {
			t.Fatal("simplify filled the ell notch")
		}
		if pathPts(p.Node()) < len(outline) {
			t.Fatalf("verts %d → %d on an outline that is already its corners", len(outline), pathPts(p.Node()))
		}
	}
}
