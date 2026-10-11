package stack

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/stretchr/testify/require"
)

func TestScoreJobsBandsMatchWholeRect(t *testing.T) {
	gotImg := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	want := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 32; x++ {
			gotImg.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 20), A: 255})
			want.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	whole := ScoreRect(nil, gotImg, want, want.Bounds())
	got := scoreJobsLimited(nil, want, []*image.NRGBA{gotImg}, []image.Rectangle{want.Bounds()}, 32)
	require.Len(t, got, 1)
	require.InDelta(t, whole, got[0], 1e-6*math.Abs(whole))
	device := scoreJobsLimited(t.Context(), want, []*image.NRGBA{gotImg}, []image.Rectangle{want.Bounds()}, 32)
	require.Len(t, device, 1)
	require.InDelta(t, whole, device[0], math.Max(1e-3*math.Abs(whole), 1e-3))
}

func TestScoreJobsPacksRectangles(t *testing.T) {
	want := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	gots := make([]*image.NRGBA, 3)
	rects := []image.Rectangle{
		image.Rect(0, 0, 16, 4),
		image.Rect(0, 4, 16, 8),
		image.Rect(2, 1, 10, 7),
	}
	for i := range gots {
		gots[i] = image.NewNRGBA(want.Bounds())
		for y := 0; y < 8; y++ {
			for x := 0; x < 16; x++ {
				gots[i].SetNRGBA(x, y, color.NRGBA{R: uint8(20*i + x), G: uint8(y * 30), A: 255})
				if i == 0 {
					want.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
				}
			}
		}
	}
	wantSums := make([]float64, len(rects))
	for i, r := range rects {
		wantSums[i] = ScoreRect(nil, gots[i], want, r)
	}
	for _, limit := range []int{16, 64, maxDevicePixels} {
		packed := scoreJobsLimited(nil, want, gots, append([]image.Rectangle(nil), rects...), limit)
		require.Len(t, packed, len(wantSums))
		for i := range wantSums {
			require.InDelta(t, wantSums[i], packed[i], 1e-6*math.Abs(wantSums[i]), "limit %d job %d", limit, i)
		}
		device := scoreJobsLimited(t.Context(), want, gots, append([]image.Rectangle(nil), rects...), limit)
		require.Len(t, device, len(wantSums))
		for i := range wantSums {
			tol := math.Max(1e-3*math.Abs(wantSums[i]), 1e-3)
			require.InDelta(t, wantSums[i], device[i], tol, "device limit %d job %d", limit, i)
		}
	}
}

func TestScoreBatchMatchesScoreDraft(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 180, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	require.NoError(t, err)
	parent := s.snap()
	draft := formPick{
		doc: parent.doc, dirty: img.Bounds(), op: OpWash, parent: parent,
		replace: -1, insert: -1, dropIdx: -1, mergeJ: -1,
	}
	one, err := s.scoreDraft(t.Context(), draft)
	require.NoError(t, err)
	batch := s.scoreBatch(t.Context(), []formPick{draft, draft})
	require.Len(t, batch, 2)
	require.True(t, batch[0].scored, "%+v", batch)
	require.True(t, batch[1].scored, "%+v", batch)
	require.InDelta(t, one.errSum, batch[0].errSum, 1e-3)
	require.InDelta(t, batch[0].errSum, batch[1].errSum, 1e-3)
}

func TestScoreRenderedMatchesScoreBatch(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 180, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	require.NoError(t, err)
	parent := s.snap()
	draft := formPick{
		doc: parent.doc, dirty: img.Bounds(), op: OpWash, parent: parent,
		replace: -1, insert: -1, dropIdx: -1, mergeJ: -1,
	}
	batch := s.scoreBatch(t.Context(), []formPick{draft})
	painted := paintNote(agentNote{pick: draft})
	rendered := s.scoreRendered(t.Context(), []formPick{draft}, []*image.NRGBA{painted})
	require.Len(t, batch, 1)
	require.Len(t, rendered, 1)
	require.Equal(t, batch[0].scored, rendered[0].scored)
	require.Equal(t, batch[0].ok, rendered[0].ok)
	require.InDelta(t, batch[0].errSum, rendered[0].errSum, 1e-3)
}

func TestFinishProposeKeepsTheDocumentUnscored(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	s, err := newWorld(t.Context(), img)
	require.NoError(t, err)
	s.stashOn = true
	doc := svg.NewDocument(4, 4).WithViewBox(0, 0, 4, 4)
	got := s.finishPropose(s.draft(doc, svg.Node{}, grow{i: -1}, OpTriangle))
	require.False(t, got.scored, "%+v", got)
	require.Zero(t, got.doc.Width())
	require.Len(t, s.stash, 1)
	require.False(t, s.stash[0].scored, "%+v", s.stash)
	require.Positive(t, s.stash[0].doc.Width())
}
