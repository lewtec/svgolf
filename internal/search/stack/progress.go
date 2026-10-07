package stack

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lewtec/lewkit/x/taskgroup"
)

type jobStatusKey struct{}
type candNoteKey struct{}

// WithJobStatus lets Search write the task row that owns this run.
// A nil status leaves the context unchanged.
func WithJobStatus(ctx context.Context, st *taskgroup.Status) context.Context {
	if ctx == nil || st == nil {
		return ctx
	}
	return context.WithValue(ctx, jobStatusKey{}, st)
}

func jobNote(ctx context.Context, msg string) {
	if ctx == nil || msg == "" {
		return
	}
	st, _ := ctx.Value(jobStatusKey{}).(*taskgroup.Status)
	if st != nil {
		st.Update(msg)
	}
}

func bandLabel(band int) string {
	if band == polishBand {
		return "polish"
	}
	return fmt.Sprintf("band %d", band)
}

// candNote is the generator task for one Operator.Run.
type candNote struct {
	st    *taskgroup.Status
	tried int
}

func (s *world) noteCandidate(id Op, p *formPick, err *error) {
	if s == nil || s.ctx == nil || p == nil || err == nil {
		return
	}
	n, _ := s.ctx.Value(candNoteKey{}).(*candNote)
	if n == nil || n.st == nil {
		return
	}
	if *err != nil {
		n.st.Update((*err).Error())
		return
	}
	n.tried++
	n.st.Progress(int64(n.tried), -1)
	name := id.String()
	if name == "" {
		name = "candidate"
	}
	n.st.Update(fmt.Sprintf("%s · tried %d · %s", name, n.tried, pickLine(*p)))
}

func pickLine(p formPick) string {
	if !p.scored {
		return "no score"
	}
	state := "reject"
	if p.ok {
		state = "keep"
	}
	return fmt.Sprintf("%.3f %s · %d paths · %d cmds", p.errSum, state, p.paths, p.commands)
}

func rankSummary(rows []candResult, limit int) string {
	if len(rows) == 0 {
		return "no candidates"
	}
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		if i > 0 {
			b.WriteString(" · ")
		}
		fmt.Fprintf(&b, "#%d %s %s", i+1, rows[i].name, pickLine(rows[i].pick))
	}
	if len(rows) > limit {
		fmt.Fprintf(&b, " · +%d", len(rows)-limit)
	}
	return b.String()
}

func archiveLine(sn []snapshot) string {
	if len(sn) == 0 {
		return "empty"
	}
	var b strings.Builder
	for i, s := range sn {
		if i > 0 {
			b.WriteString(" · ")
		}
		name := s.operator.String()
		if name == "" {
			name = "plate"
		}
		fmt.Fprintf(&b, "%s %.3f/%dp/%dc", name, s.errSum, s.paths, s.commands)
	}
	return b.String()
}

// fork carries this generator's task on ctx. Pixmaps stay shared.
func (s *world) fork(ctx context.Context) *world {
	return &world{
		ctx:          ctx,
		want:         s.want,
		got:          s.got,
		wantP:        s.wantP,
		gotP:         s.gotP,
		doc:          s.doc,
		owner:        s.owner,
		fills:        s.fills,
		scratch:      s.scratch,
		errSum:       s.errSum,
		paths:        s.paths,
		w:            s.w,
		h:            s.h,
		candidateLog: s.candidateLog,
		snapID:       s.snapID,
		simplifyMiss: s.simplifyMiss,
		rank:         s.rank,
	}
}

// liveRank keeps the last ranking on the tree until the next wave.
type liveRank struct {
	done chan struct{}
}

func (r *liveRank) show(ctx context.Context, msg string) {
	if r == nil || msg == "" || taskgroup.FromContext(ctx) == nil {
		return
	}
	if r.done != nil {
		close(r.done)
	}
	done := make(chan struct{})
	r.done = done
	taskgroup.Go(ctx, "rank", taskgroup.Control, func(ctx context.Context, st *taskgroup.Status) error {
		st.Update(msg)
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	})
}

func (r *liveRank) stop() {
	if r == nil || r.done == nil {
		return
	}
	close(r.done)
	r.done = nil
}

func watchStep(ctx context.Context, name string, fn func(*taskgroup.Status)) {
	if taskgroup.FromContext(ctx) == nil {
		fn(nil)
		return
	}
	release := make(chan struct{})
	started := make(chan *taskgroup.Status, 1)
	taskgroup.Go(ctx, name, taskgroup.Control, func(ctx context.Context, st *taskgroup.Status) error {
		st.Update(name)
		started <- st
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	})
	var st *taskgroup.Status
	select {
	case st = <-started:
	case <-ctx.Done():
	}
	fn(st)
	close(release)
}

type candJob struct {
	op    Operator
	left  leftover
	bound bool
}

type candResult struct {
	name    string
	id      Op
	pick    formPick
	elapsed time.Duration
	err     error
	st      *taskgroup.Status
}

func candidateName(op Operator, left leftover, bound bool) string {
	name := op.ID().String()
	if name == "" {
		name = "candidate"
	}
	if bound {
		return fmt.Sprintf("%s · %dpx", name, len(left.island))
	}
	return name
}

func (s *world) runCandidate(ctx context.Context, job candJob, st *taskgroup.Status, parent snapshot) (formPick, time.Duration, error) {
	if st != nil {
		st.Update("running")
		st.Progress(0, -1)
	}
	impl := job.op
	if st != nil {
		note := &candNote{st: st}
		fork := s.fork(context.WithValue(ctx, candNoteKey{}, note))
		if bound, ok := impl.(op); ok {
			bound.world = fork
			impl = bound
		}
	}
	started := time.Now()
	pick, err := impl.Run()
	elapsed := time.Since(started)
	if err != nil {
		if st != nil {
			st.Update(err.Error())
		}
		return nonePick(), elapsed, err
	}
	if pick.ok {
		pick.parent = parent
		if job.bound {
			pick.island = job.left.glow
			if len(pick.island) == 0 {
				pick.island = job.left.island
			}
		}
	}
	if st != nil {
		st.Update(pickLine(pick))
	}
	return pick, elapsed, nil
}

func (s *world) runCandidateTasks(ctx context.Context, jobs []candJob, parent snapshot, band int) ([]candResult, error) {
	release := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(release) }) }
	defer stop()

	out := make(chan candResult, len(jobs))
	for _, job := range jobs {
		job := job
		name := candidateName(job.op, job.left, job.bound)
		taskgroup.Go(ctx, name, taskgroup.Control, func(ctx context.Context, st *taskgroup.Status) error {
			pick, elapsed, err := s.runCandidate(ctx, job, st, parent)
			out <- candResult{name: name, id: job.op.ID(), pick: pick, elapsed: elapsed, err: err, st: st}
			if err != nil {
				return err
			}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
	}
	got := make([]candResult, 0, len(jobs))
	for len(got) < len(jobs) {
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case row := <-out:
			got = append(got, row)
			if row.err != nil {
				return nil, row.err
			}
		}
	}
	ranked := append([]candResult(nil), got...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return betterPick(ranked[i].pick, ranked[j].pick)
	})
	for i, row := range ranked {
		if row.st == nil {
			continue
		}
		row.st.Update(fmt.Sprintf("#%d · %s", i+1, pickLine(row.pick)))
		row.st.Progress(int64(i+1), int64(len(ranked)))
	}
	line := fmt.Sprintf("%s · %s", bandLabel(band), rankSummary(ranked, 3))
	jobNote(ctx, line)
	if s.rank != nil {
		s.rank.show(ctx, line)
	}
	return got, nil
}
