package stack

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/stretchr/testify/require"
)

func TestBeamSlotCountsVertices(t *testing.T) {
	doc := svg.NewDocument(4, 4).WithViewBox(0, 0, 4, 4).Append(whitePane(4, 4).Node())
	doc = doc.Append(filledPath([][2]float64{{0, 0}, {4, 0}, {2, 3}}, color.NRGBA{R: 255, A: 255}).Node())
	slots := beamSlots([]snapshot{{doc: doc, paths: 1, commands: 3, errSum: 1, operator: OpTriangle}})
	require.Len(t, slots, 1)
	require.Equal(t, 3, slots[0].Vertices)
	require.Equal(t, "triangle", slots[0].Operator)
}

func TestRunAgentsStopsWhenContextIsCancelled(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 180, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = s.runAgents(ctx, func(Op, []pix, []pix, []search.Rated) bool { return true })
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		require.Fail(t, "runAgents kept running after the context ended")
	}
}

func TestKeptProposalPushesLoss(t *testing.T) {
	const n = 4
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
		img.Pix[i+3] = 255
	}
	s, err := newWorld(t.Context(), img)
	require.NoError(t, err)
	parent := s.snap()
	board := newBeam(parent)
	st := &rankState{head: parent.id, bestByOp: map[Op]*namedPick{}}
	var frames []search.Frame
	ctx := WithFrames(t.Context(), func(frame search.Frame) {
		cp := frame
		cp.Loss = append([]float64(nil), frame.Loss...)
		cp.Proposals = append([]search.Proposal(nil), frame.Proposals...)
		frames = append(frames, cp)
	})
	red := color.NRGBA{R: 255, A: 255}
	cover := [][2]float64{{0, 0}, {float64(n), 0}, {float64(n), float64(n)}, {0, float64(n)}}
	full := nonePick()
	full.doc = parent.doc.Append(filledPath(cover, red).Node())
	full.op = OpRectangle
	full.parent = parent
	full.dirty = img.Bounds()
	emit := func(Op, []pix, []pix, []search.Rated) bool { return true }
	s.rankNotes(ctx, board, st, []agentNote{{pick: full}}, nil, nil, nil, emit)
	require.Len(t, st.loss, 1)
	require.Less(t, st.loss[0], parent.errSum)
	require.NotEmpty(t, frames)
	require.NotEmpty(t, frames[0].Proposals)
	require.Equal(t, docVertices(full.doc), frames[0].Proposals[0].Vertices)
	s.rankNotes(ctx, board, st, []agentNote{{pick: full}}, nil, nil, nil, emit)
	require.Len(t, st.loss, 1, "duplicate loss=%v", st.loss)
	same := nonePick()
	same.doc = parent.doc
	same.op = OpRectangle
	same.parent = parent
	same.dirty = img.Bounds()
	s.rankNotes(ctx, board, st, []agentNote{{pick: same}}, nil, nil, nil, emit)
	require.Len(t, st.loss, 1, "rejected loss=%v", st.loss)
	halfRing := [][2]float64{{0, 0}, {2, 0}, {2, float64(n)}, {0, float64(n)}}
	half := nonePick()
	half.doc = parent.doc.Append(filledPath(halfRing, red).Node())
	half.op = OpRectangle
	half.parent = parent
	half.dirty = img.Bounds()
	s.rankNotes(ctx, board, st, []agentNote{{pick: half}}, nil, nil, nil, emit)
	require.Len(t, st.loss, 2)
	require.Greater(t, st.loss[1], st.loss[0])
	require.NotEmpty(t, frames)
	require.Len(t, frames[len(frames)-1].Loss, len(st.loss))
}

func TestInitialLeftoverFeedsTriangle(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	require.NoError(t, err)
	lefts := s.leftovers(t.Context())
	require.NotEmpty(t, lefts)
	var n int
	fork := s.snap().fork(s.want, s.wantP, s.w, s.h)
	err = fork.emitKind(t.Context(), OpTriangle, lefts, func(p formPick) {
		n++
		t.Logf("triangle ok=%v scored=%v err=%v", p.ok, p.scored, p.errSum)
	})
	require.NoError(t, err)
	require.NotZero(t, n, "leftovers=%d big=%v", len(lefts), lefts[0].big())
	var epochs int
	yielded, err := s.runAgents(t.Context(), func(id Op, _, _ []pix, _ []search.Rated) bool {
		epochs++
		return true
	})
	require.NoError(t, err)
	require.True(t, yielded)
	require.NotZero(t, epochs)
	require.NotZero(t, s.paths)
}
