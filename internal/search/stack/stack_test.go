package stack

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

func forms(d svg.Document) []svg.Node {
	kids := d.Children()
	if len(kids) == 0 {
		return kids
	}
	return kids[1:]
}

func TestSimplifyDropsUselessHole(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	outer := [][2]float64{{0, 0}, {16, 0}, {16, 16}, {0, 16}}
	hole := [][2]float64{{4, 4}, {8, 4}, {8, 8}, {4, 8}}
	doc := svg.NewDocument(16, 16).WithViewBox(0, 0, 16, 16)
	doc = doc.Append(whitePane(16, 16).Node())
	doc = doc.Append(withHoles(filledPath(outer, red), [][][2]float64{hole}).Node())
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	owner := make([]uint16, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			owner[y*16+x] = 1
		}
	}
	s := &world{
		want:   img,
		got:    got,
		wantP:  loss.NewPlane(img),
		gotP:   loss.NewPlane(got),
		doc:    doc,
		owner:  owner,
		fills:  []color.NRGBA{red},
		paths:  1,
		w:      16,
		h:      16,
		errSum: Score(got, img, 0),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	pick, err := (Simplify{world: s, buckets: [][]pix{nil}}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("simplify=false want drop the hole over red")
	}
	s.apply(pick)
	p, ok := s.doc.Children()[1].Path()
	if !ok || p.FillRule() == svg.FillEvenOdd {
		t.Fatal("hole still punched")
	}
}

func TestScoreCandStoresFullError(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			want.SetNRGBA(x, y, red)
		}
	}
	s, err := newWorld(want)
	if err != nil {
		t.Fatal(err)
	}
	lefts := s.leftovers()
	if len(lefts) == 0 {
		t.Fatal("no leftover")
	}
	pick, err := (Cover{world: s, left: lefts[0]}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("cover=false")
	}
	full := ScoreOn(loss.NewPlane(pick.got), s.wantP, 0)
	if pick.errSum != full {
		t.Fatalf("errSum=%v full=%v (dirty rect lied)", pick.errSum, full)
	}
}

func TestTryDropRedundant(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, red)
		}
	}
	doc := svg.NewDocument(16, 16).WithViewBox(0, 0, 16, 16)
	doc = doc.Append(whitePane(16, 16).Node())
	full := filledPath([][2]float64{{0, 0}, {16, 0}, {16, 16}, {0, 16}}, red)
	crumb := filledPath([][2]float64{{2, 2}, {6, 2}, {6, 6}, {2, 6}}, red)
	doc = doc.Append(full.Node()).Append(crumb.Node())
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	owner := make([]uint16, 16*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			owner[y*16+x] = 1
		}
	}
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			owner[y*16+x] = 2
		}
	}
	fills := []color.NRGBA{red, red}
	s := &world{
		want:   img,
		got:    got,
		wantP:  loss.NewPlane(img),
		doc:    doc,
		owner:  owner,
		fills:  fills,
		paths:  2,
		w:      16,
		h:      16,
		errSum: Score(got, img, 0),
	}
	pick, err := (Delete{world: s, i: 1}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("delete=false want drop the covered crumb")
	}
	s.apply(pick)
	if s.paths != 1 {
		t.Fatalf("paths=%d want 1", s.paths)
	}
}

func TestHottestIslandPrefersFullMiss(t *testing.T) {
	// 20×20 almost-white leftover is more pixels; 8×8 black is more Score.
	got := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	want := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			got.SetNRGBA(x, y, paper)
			want.SetNRGBA(x, y, paper)
		}
	}
	pale := color.NRGBA{R: 242, G: 242, B: 242, A: 255}
	for y := 4; y < 24; y++ {
		for x := 4; x < 24; x++ {
			want.SetNRGBA(x, y, pale)
		}
	}
	black := color.NRGBA{A: 255}
	for y := 30; y < 38; y++ {
		for x := 30; x < 38; x++ {
			want.SetNRGBA(x, y, black)
		}
	}
	col, island := (&world{got: got, want: want}).hottest()
	if len(island) != 64 {
		t.Fatalf("island=%d want 64 (black miss), fill=%+v", len(island), col)
	}
	if col.R > 16 || col.G > 16 || col.B > 16 {
		t.Fatalf("fill=%+v want black", col)
	}
}

func TestHottestNKeepsThree(t *testing.T) {
	got := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	want := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			got.SetNRGBA(x, y, paper)
			want.SetNRGBA(x, y, paper)
		}
	}
	pale := color.NRGBA{R: 242, G: 242, B: 242, A: 255}
	for y := 4; y < 24; y++ {
		for x := 4; x < 24; x++ {
			want.SetNRGBA(x, y, pale)
		}
	}
	mid := color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	for y := 4; y < 16; y++ {
		for x := 30; x < 42; x++ {
			want.SetNRGBA(x, y, mid)
		}
	}
	black := color.NRGBA{A: 255}
	for y := 30; y < 38; y++ {
		for x := 30; x < 38; x++ {
			want.SetNRGBA(x, y, black)
		}
	}
	top := (&world{got: got, want: want}).hottestN(3)
	if len(top) != 3 {
		t.Fatalf("hottestN=%d want 3", len(top))
	}
	seen := map[int]bool{}
	for i, b := range top {
		seen[len(b.island)] = true
		if i > 0 && top[i-1].errSum < b.errSum {
			t.Fatalf("not ranked by Σerr: %v", []float64{top[0].errSum, top[1].errSum, top[2].errSum})
		}
	}
	if !seen[64] || !seen[144] || !seen[400] {
		t.Fatalf("islands=%v want 64, 144, 400", []int{len(top[0].island), len(top[1].island), len(top[2].island)})
	}
}

func TestRankGenerationKeepsYBest(t *testing.T) {
	pool := []formPick{
		{ok: true, a: 30, op: "swap"},
		{ok: true, a: 10, op: "delete"},
		{ok: false, a: 1, op: "grow"},
		{ok: true, a: 50, op: "worse"},
		{ok: true, a: 20, op: "rectangle"},
	}
	got := rankGeneration(pool, 40, 3)
	if len(got) != 3 {
		t.Fatalf("kept=%d want 3", len(got))
	}
	if got[0].a != 10 || got[1].a != 20 || got[2].a != 30 {
		t.Fatalf("order=%v", []float64{got[0].a, got[1].a, got[2].a})
	}
}

func TestRankGenerationOnePerParent(t *testing.T) {
	pool := []formPick{
		{ok: true, a: 10, parent: snapshot{id: 1}},
		{ok: true, a: 11, parent: snapshot{id: 1}},
		{ok: true, a: 20, parent: snapshot{id: 2}},
		{ok: true, a: 30, parent: snapshot{id: 3}},
	}
	got := rankGeneration(pool, 40, 3)
	if len(got) != 3 {
		t.Fatalf("kept=%d want 3", len(got))
	}
	if got[0].a != 10 || got[1].a != 20 || got[2].a != 30 {
		t.Fatalf("order=%v want 10,20,30 (one per parent)", []float64{got[0].a, got[1].a, got[2].a})
	}
}

func TestStackRampOnePathNative(t *testing.T) {
	// coarse() splits this ramp. Score must still keep one linear
	// instead of a second flat for the light band.
	a := color.NRGBA{R: 40, G: 80, B: 200, A: 255}
	b := color.NRGBA{R: 180, G: 220, B: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		t := float64(y) / 47
		c := color.NRGBA{
			R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
			G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
			B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
			A: 255,
		}
		for x := 0; x < 48; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var ops []string
	var doc svg.Document
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, fmt.Sprintf("%s/%d", ep.Operator, len(forms(ep.Document))))
		doc = ep.Document
	}
	fs := forms(doc)
	if len(fs) != 1 {
		t.Fatalf("paths=%d want 1 gradient ops=%v", len(fs), ops)
	}
	if _, ok := fs[0].LinearFill(); !ok {
		t.Fatal("ramp stayed stacked flats")
	}
}

func TestStackGapGetsRectangle(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 8; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 40, G: 80, B: 200, A: 255})
		}
	}
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		fk := forms(ep.Document)
		if len(fk) == 0 {
			continue
		}
		if ep.Operator != "cover" {
			t.Fatalf("operator=%s want cover", ep.Operator)
		}
		p, ok := fk[0].Path()
		if !ok {
			t.Fatal("not a path")
		}
		n := 0
		for _, c := range p.Commands() {
			if c.Kind != svg.CmdClose {
				n++
			}
		}
		if n < 3 || n > 6 {
			t.Fatalf("points=%d want 3-6 (cover on leftover)", n)
		}
		return
	}
	t.Fatal("no form")
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
	if n := len(forms(doc)); n != 1 {
		t.Fatalf("paths=%d want 1", n)
	}
	if _, ok := forms(doc)[0].Path(); !ok {
		t.Fatal("not a path")
	}
}

func TestStackUnblurDoesNotRestack(t *testing.T) {
	// leftover of the same plate must polish path 0, not stack more navy.
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
	if n := len(forms(doc)); n != 1 {
		t.Fatalf("paths=%d want 1 (polished plate)", n)
	}
}

func TestStackLargePlateUnderSmall(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for y := 8; y < 16; y++ {
		for x := 8; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	fs := forms(doc)
	if len(fs) < 2 {
		t.Fatalf("paths=%d want plate + mark", len(fs))
	}
	if pathArea(fs[0]) < pathArea(fs[1]) {
		t.Fatalf("drawn order small-then-large: %v then %v", pathArea(fs[0]), pathArea(fs[1]))
	}
}

func TestSwapUncoversMark(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			want.SetNRGBA(x, y, red)
		}
	}
	for y := 8; y < 16; y++ {
		for x := 8; x < 16; x++ {
			want.SetNRGBA(x, y, blue)
		}
	}
	doc := svg.NewDocument(32, 32).WithViewBox(0, 0, 32, 32)
	doc = doc.Append(whitePane(32, 32).Node())
	mark := filledPath([][2]float64{{8, 8}, {16, 8}, {16, 16}, {8, 16}}, blue)
	plate := filledPath([][2]float64{{0, 0}, {32, 0}, {32, 32}, {0, 32}}, red)
	doc = doc.Append(mark.Node()).Append(plate.Node())
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	owner := make([]uint16, 32*32)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			owner[y*32+x] = 2
		}
	}
	for y := 8; y < 16; y++ {
		for x := 8; x < 16; x++ {
			owner[y*32+x] = 1
		}
	}
	s := &world{
		want:   want,
		got:    got,
		wantP:  loss.NewPlane(want),
		gotP:   loss.NewPlane(got),
		doc:    doc,
		owner:  owner,
		fills:  []color.NRGBA{blue, red},
		paths:  2,
		w:      32,
		h:      32,
		errSum: Score(got, want, 0),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	pick, err := (Swap{world: s, i: 0, j: 1}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("swap=false want large under small")
	}
	s.apply(pick)
	kids := s.doc.Children()
	if pathArea(kids[1]) < pathArea(kids[2]) {
		t.Fatalf("still small-then-large: %v then %v", pathArea(kids[1]), pathArea(kids[2]))
	}
	if c := s.got.NRGBAAt(12, 12); c.B < 200 {
		t.Fatalf("mark still hidden %+v", c)
	}
}

func TestSlidePullsTowardLeftover(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			want.SetNRGBA(x, y, red)
		}
	}
	plate := filledPath([][2]float64{{0, 0}, {16, 0}, {16, 16}, {0, 16}}, red)
	doc := svg.NewDocument(32, 16).WithViewBox(0, 0, 32, 16)
	doc = doc.Append(whitePane(32, 16).Node()).Append(plate.Node())
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := &world{
		want:   want,
		got:    got,
		wantP:  loss.NewPlane(want),
		gotP:   loss.NewPlane(got),
		doc:    doc,
		owner:  make([]uint16, 32*16),
		fills:  []color.NRGBA{red},
		paths:  1,
		w:      32,
		h:      16,
		errSum: Score(got, want, 0),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	lefts := s.leftovers()
	if len(lefts) == 0 {
		t.Fatal("no leftover")
	}
	pick, err := (Slide{world: s, left: lefts[0]}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("slide=false want pull toward leftover")
	}
}

func TestBendPutsCubicTowardLeftover(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			want.SetNRGBA(x, y, red)
		}
	}
	plate := filledPath([][2]float64{{0, 0}, {16, 0}, {16, 16}, {0, 16}}, red)
	doc := svg.NewDocument(32, 16).WithViewBox(0, 0, 32, 16)
	doc = doc.Append(whitePane(32, 16).Node()).Append(plate.Node())
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	s := &world{
		want:   want,
		got:    got,
		wantP:  loss.NewPlane(want),
		gotP:   loss.NewPlane(got),
		doc:    doc,
		owner:  make([]uint16, 32*16),
		fills:  []color.NRGBA{red},
		paths:  1,
		w:      32,
		h:      16,
		errSum: Score(got, want, 0),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	lefts := s.leftovers()
	if len(lefts) == 0 {
		t.Fatal("no leftover")
	}
	pick, err := (Bend{world: s, left: lefts[0]}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("bend=false want cubic toward leftover")
	}
	p, ok := pick.doc.Children()[1].Path()
	if !ok {
		t.Fatal("not a path")
	}
	cubics := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdCubic {
			cubics++
		}
	}
	if cubics == 0 {
		t.Fatal("bend added no cubic")
	}
}

func TestCrossoverRectangleUsesSiblingLeftover(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	want := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			want.SetNRGBA(x, y, red)
		}
	}
	for y := 8; y < 16; y++ {
		for x := 8; x < 16; x++ {
			want.SetNRGBA(x, y, blue)
		}
	}
	plate := filledPath([][2]float64{{0, 0}, {32, 0}, {32, 32}, {0, 32}}, red)
	bdoc := svg.NewDocument(32, 32).WithViewBox(0, 0, 32, 32)
	bdoc = bdoc.Append(whitePane(32, 32).Node()).Append(plate.Node())
	bgot, err := render.Render(bdoc)
	if err != nil {
		t.Fatal(err)
	}
	s := &world{
		want:   want,
		got:    bgot,
		wantP:  loss.NewPlane(want),
		gotP:   loss.NewPlane(bgot),
		doc:    bdoc,
		skip:   make([]byte, 32*32),
		owner:  make([]uint16, 32*32),
		fills:  []color.NRGBA{red},
		paths:  1,
		w:      32,
		h:      32,
		errSum: Score(bgot, want, 0),
	}
	s.wantP.Ensure()
	s.gotP.Ensure()
	lefts := s.leftovers()
	if len(lefts) == 0 || !lefts[0].big() {
		t.Fatal("sibling leftover empty")
	}
	adoc := svg.NewDocument(32, 32).WithViewBox(0, 0, 32, 32)
	adoc = adoc.Append(whitePane(32, 32).Node())
	agot, err := render.Render(adoc)
	if err != nil {
		t.Fatal(err)
	}
	s.doc, s.got, s.fills, s.owner, s.paths = adoc, agot, nil, make([]uint16, 32*32), 0
	s.errSum = Score(agot, want, 0)
	s.gotP.Reset(agot)
	s.gotP.Ensure()
	lefts = s.bindLeftovers(lefts)
	pick, err := (Cover{world: s, left: lefts[0]}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !pick.ok {
		t.Fatal("cover=false want sibling leftover on this parent")
	}
}

func TestStackTwoColorGetsBoth(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(forms(doc)); n < 2 {
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
	if n := len(forms(doc)); n < 2 {
		t.Fatalf("paths=%d want >=2 (plate + mark)", n)
	}
	empty := image.NewNRGBA(img.Rect)
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if Score(got, img, len(forms(doc))) >= Score(empty, img, 0) {
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
	if n := len(forms(doc)); n < 3 {
		t.Fatalf("paths=%d want >=3", n)
	}
}

func TestVoidsInnerCyanRing(t *testing.T) {
	var island []pix
	for y := 10; y < 22; y++ {
		for x := 10; x < 22; x++ {
			if x >= 13 && x < 19 && y >= 13 && y < 19 {
				continue
			}
			island = append(island, pix{x, y})
		}
	}
	if n := len(voids(island)); n != 1 {
		t.Fatalf("voids=%d want 1 (inner hole)", n)
	}
	hs := holeRings(island)
	if len(hs) != 1 {
		t.Fatalf("holeRings=%d want 1", len(hs))
	}
	p := withHoles(filledPath(convexHull(islandPoints(island)), color.NRGBA{B: 255, A: 255}), hs)
	if p.FillRule() != svg.FillEvenOdd {
		t.Fatal("punched form not evenodd")
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
	if c.B > 40 && c.R < 200 {
		t.Fatalf("hole still navy %+v paths=%d", c, len(forms(doc)))
	}
}

func TestStackVisorIsRing(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	cyan := color.NRGBA{R: 5, G: 176, B: 247, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 4; y < 44; y++ {
		for x := 4; x < 44; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 12; y < 36; y++ {
		for x := 12; x < 36; x++ {
			onRing := x < 16 || x >= 32 || y < 16 || y >= 32
			if onRing {
				img.SetNRGBA(x, y, cyan)
			}
		}
	}
	var first, doc svg.Document
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		if len(forms(ep.Document)) > 0 && len(forms(first)) == 0 {
			first = ep.Document
		}
		doc = ep.Document
	}
	if len(forms(doc)) == 0 {
		t.Fatal("no form")
	}
	if p, ok := forms(first)[0].Path(); !ok || p.FillRule() == svg.FillEvenOdd {
		t.Fatal("first form must be a solid plate")
	}
	got, err := render.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	ring := got.NRGBAAt(14, 24)
	if lossColorFar(ring, cyan) {
		t.Fatalf("visor ring %+v want cyan", ring)
	}
	inner := got.NRGBAAt(24, 24)
	if lossColorFar(inner, navy) {
		t.Fatalf("visor interior %+v want navy, not a filled hull", inner)
	}
}

func lossColorFar(got, want color.NRGBA) bool {
	dr := int(got.R) - int(want.R)
	dg := int(got.G) - int(want.G)
	db := int(got.B) - int(want.B)
	if dr < 0 {
		dr = -dr
	}
	if dg < 0 {
		dg = -dg
	}
	if db < 0 {
		db = -db
	}
	return dr > 40 || dg > 40 || db > 40
}

func TestStackFirstFormIsSolid(t *testing.T) {
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
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		doc := ep.Document
		fk := forms(doc)
		if len(fk) == 0 {
			continue
		}
		p, ok := fk[0].Path()
		if !ok {
			t.Fatal("not a path")
		}
		if p.FillRule() == svg.FillEvenOdd {
			t.Fatal("first form carved holes; want a solid plate")
		}
		return
	}
	t.Fatal("no form")
}

func TestStackShrinksHoleNotCover(t *testing.T) {
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
	hole := got.NRGBAAt(16, 16)
	if hole.B > 40 && hole.R < 200 {
		t.Fatalf("hole still navy %+v", hole)
	}
	fk := forms(doc)
	if len(fk) == 0 {
		t.Fatal("no form")
	}
	p, ok := fk[0].Path()
	if !ok {
		t.Fatal("not a path")
	}
	if len(fk) > 1 && p.FillRule() != svg.FillEvenOdd {
		t.Fatalf("hole covered by %d layers; want the plate to shrink", len(fk))
	}
}

func TestStackShrinksInnerNotOuter(t *testing.T) {
	navy := color.NRGBA{R: 12, G: 52, B: 88, A: 255}
	cyan := color.NRGBA{R: 5, G: 176, B: 247, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 6; y < 42; y++ {
		for x := 6; x < 42; x++ {
			img.SetNRGBA(x, y, navy)
		}
	}
	for y := 16; y < 32; y++ {
		for x := 16; x < 32; x++ {
			img.SetNRGBA(x, y, cyan)
		}
	}
	for y := 20; y < 28; y++ {
		for x := 20; x < 28; x++ {
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
	fk := forms(doc)
	rules := make([]svg.FillRule, 0, len(fk))
	for _, n := range fk {
		if p, ok := n.Path(); ok {
			rules = append(rules, p.FillRule())
		}
	}
	hole := got.NRGBAAt(24, 24)
	if hole.B > 40 && hole.R < 200 {
		t.Fatalf("hole still painted %+v paths=%d rules=%v", hole, len(fk), rules)
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
	for y := 16; y < 24; y++ {
		for x := 16; x < 24; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	doc, err := search.Last((Stack{}).Search(t.Context(), img))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(forms(doc)); n < 2 {
		t.Fatalf("paths=%d want plate + mark", n)
	}
}

func TestCandidateLogLine(t *testing.T) {
	var buf bytes.Buffer
	LogCandidates(&buf)
	t.Cleanup(func() { LogCandidates(nil) })
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	if _, err := search.Last((Stack{}).Search(t.Context(), img)); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "\tcover elapsed=") || !strings.Contains(got, "score=") {
		t.Fatalf("candidate log=%q", got)
	}
}

func TestStackNilPixmap(t *testing.T) {
	_, err := search.Last((Stack{}).Search(t.Context(), nil))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStackEpochOperator(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var ops []string
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		if len(forms(ep.Document)) == 0 {
			continue
		}
		ops = append(ops, ep.Operator)
	}
	if len(ops) == 0 || ops[0] != "cover" {
		t.Fatalf("operators=%v want cover first", ops)
	}
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		if ep.Elapsed <= 0 {
			t.Fatal("epoch elapsed not set")
		}
		break
	}
}

func TestStackExpandHasNoLinear(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	a := color.NRGBA{R: 40, G: 80, B: 200, A: 255}
	b := color.NRGBA{R: 180, G: 220, B: 255, A: 255}
	for y := 0; y < 48; y++ {
		t := float64(y) / 47
		c := color.NRGBA{
			R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
			G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
			B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
			A: 255,
		}
		for x := 0; x < 48; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		if ep.Operator != "cover" && ep.Operator != "grow" {
			continue
		}
		for _, n := range forms(ep.Document) {
			if _, ok := n.LinearFill(); ok {
				t.Fatal("cover/grow fitted a linear; leftover stairs are wash")
			}
		}
	}
}

func TestEpochOfNativeScale(t *testing.T) {
	if got := epochOf(svg.NewDocument(1, 1), "cover").Scale; got != 1 {
		t.Fatalf("scale=%d want 1", got)
	}
}

func TestStackCoversBeforeRefine(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var kids []int
	var first []int
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		doc := ep.Document
		kids = append(kids, len(forms(doc)))
		if fk := forms(doc); len(fk) > 0 {
			first = append(first, pathPts(fk[0]))
		} else {
			first = append(first, 0)
		}
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

func TestStackBiteStaysOtherColor(t *testing.T) {
	blue := color.NRGBA{B: 255, A: 255}
	red := color.NRGBA{R: 255, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			if y < 6 && x >= 8 && x < 24 {
				img.SetNRGBA(x, y, red)
				continue
			}
			img.SetNRGBA(x, y, blue)
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
	c := got.NRGBAAt(16, 3)
	if c.R < 200 {
		t.Fatalf("bite %+v want red, not covered by the blue plate", c)
	}
}

func TestStackFirstFormIsPoly(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		fk := forms(ep.Document)
		if len(fk) == 0 {
			t.Fatal("no form")
		}
		p, ok := fk[0].Path()
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

func TestCoverRingKeepsConcaveL(t *testing.T) {
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x < 3 || y < 3 {
				island = append(island, pix{x, y})
			}
		}
	}
	ring := coverRing(island, 6)
	if pointInRing(ring, 5.5, 5.5) {
		t.Fatalf("notch filled, still a hull? %v", ring)
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

func TestRingCrossesBowtie(t *testing.T) {
	bow := [][2]float64{{0, 0}, {2, 2}, {0, 2}, {2, 0}}
	if !ringCrosses(bow) {
		t.Fatal("bowtie not detected")
	}
	// from 20260831-111731 outer ring (slide/bend zigzag)
	job := [][2]float64{{322, 524}, {357, 100}, {121, 554}, {528, 563}, {124, 564}, {120, 94}, {298, 438}}
	if !ringCrosses(job) {
		t.Fatal("job ring not detected")
	}
	if ringCrosses([][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}}) {
		t.Fatal("square flagged as crossing")
	}
	got := uncross(bow)
	if len(got) < 3 || ringCrosses(got) {
		t.Fatalf("uncross=%v", got)
	}
}

func TestFitPolyCollapsesStair(t *testing.T) {
	var ring [][2]float64
	for x := 0; x < 40; x++ {
		ring = append(ring, [2]float64{float64(x), float64(x % 2)})
	}
	for y := 0; y < 40; y++ {
		ring = append(ring, [2]float64{40 + float64(y%2), float64(y)})
	}
	got := fitPoly(ring, 2)
	if len(got) > 8 {
		t.Fatalf("stair fitPoly=%d want a short edge", len(got))
	}
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
	var first, last svg.Document
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		if len(forms(ep.Document)) > 0 && len(forms(first)) == 0 {
			first = ep.Document
		}
		last = ep.Document
	}
	if cubics(forms(first)[0]) != 0 {
		t.Fatal("cover carved cubics; want a rectangle first")
	}
	p := forms(last)[0]
	ncmd := 0
	for _, c := range mustPath(t, p).Commands() {
		if c.Kind != svg.CmdClose {
			ncmd++
		}
	}
	if cubics(p) < 4 && ncmd < 8 {
		t.Fatalf("disk collapsed to lines=%d cubics=%d", ncmd, cubics(p))
	}
}

func mustPath(t *testing.T, n svg.Node) svg.Path {
	t.Helper()
	p, ok := n.Path()
	if !ok {
		t.Fatal("not a path")
	}
	return p
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
	if n := len(forms(doc)); n > 8 {
		t.Fatalf("paths=%d, restacking the ramp", n)
	}
}

func TestStackRampUsesLinear(t *testing.T) {
	// stay saturated so the dark end is still red, not a grey bin.
	img := image.NewNRGBA(image.Rect(0, 0, 80, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + x*2), A: 255})
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	doc, err := search.Last((Stack{}).Search(ctx, img))
	if err != nil {
		t.Fatal(err)
	}
	fs := forms(doc)
	if len(fs) != 1 {
		t.Fatalf("paths=%d want 1 gradient", len(fs))
	}
	if _, ok := fs[0].LinearFill(); !ok {
		t.Fatal("ramp stayed a solid fill")
	}
}

func TestFitLinearHorizontal(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 8))
	var island []pix
	for y := 0; y < 8; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 6), A: 255})
			island = append(island, pix{x, y})
		}
	}
	g, ok := fitLinearFill(island, img)
	if !ok {
		t.Fatal("fitLinearFill rejected a ramp")
	}
	if g.C0().R >= g.C1().R {
		t.Fatalf("ends %+v → %+v want dark→bright or the reverse axis", g.C0(), g.C1())
	}
}

func TestFitLinearFillLargeIsFast(t *testing.T) {
	const w, h = 400, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	island := make([]pix, 0, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / w), A: 255})
			island = append(island, pix{x, y})
		}
	}
	start := time.Now()
	if _, ok := fitLinearFill(island, img); !ok {
		t.Fatal("rejected a ramp")
	}
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Fatalf("fitLinearFill(%d px) took %s; insertion sort is back", w*h, d)
	}
}

func TestFitLinearFillRejectsTwoFlats(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	var island []pix
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 16 {
				c = color.NRGBA{R: 40, A: 255}
			}
			img.SetNRGBA(x, y, c)
			island = append(island, pix{x, y})
		}
	}
	if _, ok := fitLinearFill(island, img); ok {
		t.Fatal("two flats became a smear")
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
	if n := len(forms(doc)); n > 2 {
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

func TestHullSquare(t *testing.T) {
	h := convexHull([][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5}})
	if len(h) != 4 {
		t.Fatalf("hull=%v", h)
	}
}

func TestQuadRingDiagonalIsTilted(t *testing.T) {
	var work []pix
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if x-y < 3 && y-x < 3 {
				work = append(work, pix{x, y})
			}
		}
	}
	q := quadRing(work)
	if len(q) != 4 {
		t.Fatalf("quad=%d want 4: %v", len(q), q)
	}
	axis := 0
	for i := 0; i < 4; i++ {
		a, b := q[i], q[(i+1)%4]
		if a[0] == b[0] || a[1] == b[1] {
			axis++
		}
	}
	if axis == 4 {
		t.Fatalf("quad is axis-aligned %v", q)
	}
}

func TestStackDiagonalGapIsQuad(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if x-y < 4 && y-x < 4 {
				img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
			}
		}
	}
	for ep, err := range (Stack{}).Search(t.Context(), img) {
		if err != nil {
			t.Fatal(err)
		}
		fk := forms(ep.Document)
		if len(fk) == 0 {
			continue
		}
		if ep.Operator != "cover" {
			t.Fatalf("operator=%s want cover", ep.Operator)
		}
		p, ok := fk[0].Path()
		if !ok {
			t.Fatal("not a path")
		}
		n := 0
		axis := 0
		var last [2]float64
		started := false
		for _, c := range p.Commands() {
			if c.Kind == svg.CmdClose {
				continue
			}
			pt := [2]float64{c.X, c.Y}
			if started && (pt[0] == last[0] || pt[1] == last[1]) {
				axis++
			}
			started = true
			last = pt
			n++
		}
		if n < 3 || n > 6 {
			t.Fatalf("points=%d want 3-6", n)
		}
		if n == 4 && axis == 4 {
			t.Fatal("cover stayed axis-aligned on a diagonal leftover")
		}
		return
	}
	t.Fatal("no form")
}
