package stack

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestScoreBatchMatchesScoreDraft(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 180, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	parent := s.snap()
	draft := formPick{
		doc: parent.doc, dirty: img.Bounds(), op: OpWash, parent: parent,
		replace: -1, insert: -1, dropIdx: -1, mergeJ: -1,
	}
	one, err := s.scoreDraft(t.Context(), draft)
	if err != nil {
		t.Fatal(err)
	}
	batch := s.scoreBatch(t.Context(), []formPick{draft, draft})
	if len(batch) != 2 || !batch[0].scored || !batch[1].scored {
		t.Fatalf("batch=%v", batch)
	}
	if math.Abs(batch[0].errSum-one.errSum) > 1e-3 || math.Abs(batch[1].errSum-batch[0].errSum) > 1e-3 {
		t.Fatalf("batch %v %v draft %v", batch[0].errSum, batch[1].errSum, one.errSum)
	}
}

func TestFinishProposeKeepsTheDocumentUnscored(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	s, err := newWorld(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	s.stashOn = true
	doc := svg.NewDocument(4, 4).WithViewBox(0, 0, 4, 4)
	got := s.finishPropose(s.draft(doc, svg.Node{}, grow{i: -1}, OpTriangle))
	if got.scored || got.doc.Width() != 0 {
		t.Fatalf("returned a scored pick: %+v", got)
	}
	if len(s.stash) != 1 || s.stash[0].scored || s.stash[0].doc.Width() == 0 {
		t.Fatalf("stash=%+v", s.stash)
	}
}
