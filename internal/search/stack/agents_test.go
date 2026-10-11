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
