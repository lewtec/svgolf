package stack

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/lewtec/svgolf/internal/search"
)

func TestRunAgentsStopsWhenContextIsCancelled(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 180, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal("runAgents kept running after the context ended")
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
	if err != nil {
		t.Fatal(err)
	}
	parent := s.snap()
	board := newBeam(parent)
	st := &rankState{head: parent.id, bestByOp: map[Op]*namedPick{}}
	var frames []search.Frame
	ctx := WithFrames(t.Context(), func(frame search.Frame) {
		cp := frame
		cp.Loss = append([]float64(nil), frame.Loss...)
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
	s.rankNotes(ctx, board, st, []agentNote{{pick: full}}, emit)
	if len(st.loss) != 1 || st.loss[0] >= parent.errSum {
		t.Fatalf("kept=%v parent=%v", st.loss, parent.errSum)
	}
	s.rankNotes(ctx, board, st, []agentNote{{pick: full}}, emit)
	if len(st.loss) != 1 {
		t.Fatalf("duplicate loss=%v", st.loss)
	}
	same := nonePick()
	same.doc = parent.doc
	same.op = OpRectangle
	same.parent = parent
	same.dirty = img.Bounds()
	s.rankNotes(ctx, board, st, []agentNote{{pick: same}}, emit)
	if len(st.loss) != 1 {
		t.Fatalf("rejected loss=%v", st.loss)
	}
	halfRing := [][2]float64{{0, 0}, {2, 0}, {2, float64(n)}, {0, float64(n)}}
	half := nonePick()
	half.doc = parent.doc.Append(filledPath(halfRing, red).Node())
	half.op = OpRectangle
	half.parent = parent
	half.dirty = img.Bounds()
	s.rankNotes(ctx, board, st, []agentNote{{pick: half}}, emit)
	if len(st.loss) != 2 || st.loss[1] <= st.loss[0] {
		t.Fatalf("pool loss=%v", st.loss)
	}
	if len(frames) == 0 || len(frames[len(frames)-1].Loss) != len(st.loss) {
		t.Fatalf("frames=%d loss=%v", len(frames), st.loss)
	}
}

func TestInitialLeftoverFeedsTriangle(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	s, err := newWorld(t.Context(), img)
	if err != nil {
		t.Fatal(err)
	}
	lefts := s.leftovers(t.Context())
	if len(lefts) == 0 {
		t.Fatal("no leftover")
	}
	var n int
	fork := s.snap().fork(s.want, s.wantP, s.w, s.h)
	err = fork.emitKind(t.Context(), OpTriangle, lefts, func(p formPick) {
		n++
		t.Logf("triangle ok=%v scored=%v err=%v", p.ok, p.scored, p.errSum)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("triangle emitted nothing, leftovers=%d big=%v", len(lefts), lefts[0].big())
	}
	var epochs int
	yielded, err := s.runAgents(t.Context(), func(id Op, _, _ []pix, _ []search.Rated) bool {
		epochs++
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if !yielded || epochs == 0 || s.paths == 0 {
		t.Fatalf("yielded=%v epochs=%d paths=%d", yielded, epochs, s.paths)
	}
}
