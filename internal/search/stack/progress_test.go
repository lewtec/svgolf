package stack

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/taskgroup"
)

func TestPickLineRanksKeepAheadOfReject(t *testing.T) {
	keep := formPick{errSum: 4, paths: 1, commands: 4, op: OpTriangle, ok: true, scored: true}
	reject := formPick{errSum: 1, paths: 1, commands: 4, op: OpOutline, ok: false, scored: true}
	none := formPick{op: OpCarve}
	if !strings.Contains(pickLine(keep), "4.000 keep") || !strings.Contains(pickLine(keep), "1 paths") {
		t.Fatalf("keep=%s", pickLine(keep))
	}
	if pickLine(none) != "no score" {
		t.Fatalf("none=%s", pickLine(none))
	}
	rows := []candResult{
		{name: "outline", pick: reject},
		{name: "carve", pick: none},
		{name: "triangle", pick: keep},
	}
	sortResultsForTest(rows)
	line := rankSummary(rows, 3)
	if !strings.HasPrefix(line, "#1 triangle") || !strings.Contains(line, "#2 outline") || !strings.Contains(line, "#3 carve") {
		t.Fatalf("rank=%s", line)
	}
	arch := archiveLine([]snapshot{{operator: OpTriangle, errSum: 4, paths: 1, commands: 4}, {errSum: 9, paths: 1, commands: 1}})
	if !strings.Contains(arch, "triangle 4.000/1p/4c") || !strings.Contains(arch, "plate 9.000/1p/1c") {
		t.Fatalf("archive=%s", arch)
	}
}

func sortResultsForTest(rows []candResult) {
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		j := i
		for j > 0 && betterPick(row.pick, rows[j-1].pick) {
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = row
	}
}

func TestSearchProgressShowsRank(t *testing.T) {
	sess, ctx := taskgroup.New(t.Context(), taskgroup.DefaultLimits())
	t.Cleanup(func() {
		sess.Cancel(context.Canceled)
		_ = sess.Wait()
	})
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 180, A: 255})
		}
	}
	found := make(chan string, 1)
	taskgroup.Go(ctx, "job", taskgroup.Control, func(ctx context.Context, st *taskgroup.Status) error {
		ctx = WithJobStatus(ctx, st)
		for _, err := range (Stack{}).Search(ctx, img) {
			if err != nil {
				return err
			}
			deadline := time.Now().Add(2 * time.Second)
			var lines []string
			for time.Now().Before(deadline) {
				lines = nil
				for _, n := range sess.List(64) {
					lines = append(lines, n.Name+": "+n.Message)
					if n.Name == "rank" && strings.Contains(n.Message, "#1") {
						found <- n.Message
						return nil
					}
				}
				time.Sleep(time.Millisecond)
			}
			found <- "missing rank\n" + strings.Join(lines, "\n")
			return nil
		}
		return nil
	})
	select {
	case msg := <-found:
		if strings.HasPrefix(msg, "missing") {
			t.Fatal(msg)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("search did not publish a ranking")
	}
}
