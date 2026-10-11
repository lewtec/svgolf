package stack

import (
	"context"
	"image"
	"sync"

	"github.com/lewtec/svgolf/pkg/render"
)

// scoreJobs is one device evaluation for every rectangle, or the
// float64 loop when the device path is closed.
func scoreJobs(ctx context.Context, want *image.NRGBA, gots []*image.NRGBA, rects []image.Rectangle) []float64 {
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	if sums, ok := ndScoreMany(ctx, want, gots, rects); ok {
		return sums
	}
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	out := make([]float64, len(rects))
	for i := range rects {
		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		if gots[i] == nil || want == nil || rects[i].Empty() {
			continue
		}
		if rects[i].Eq(want.Bounds()) && rects[i].Eq(gots[i].Bounds()) {
			out[i] = Score(ctx, gots[i], want)
			continue
		}
		out[i] = ScoreRect(ctx, gots[i], want, rects[i])
	}
	return out
}

// scoreBatch renders every unscored proposition and scores the batch
// with one device call. A full proposition uses the whole pixmap.
// The others use the parent sum minus the old dirty rect plus the new one.
func (s *world) scoreBatch(ctx context.Context, drafts []formPick) []formPick {
	out := make([]formPick, len(drafts))
	if ctx != nil && ctx.Err() != nil {
		return out
	}
	rendered := make([]*image.NRGBA, len(drafts))
	var wg sync.WaitGroup
	for i := range drafts {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if drafts[i].scored || drafts[i].doc.Width() == 0 {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if ctx != nil && ctx.Err() != nil {
				return
			}
			img, err := render.Render(drafts[i].doc)
			if err != nil {
				return
			}
			rendered[i] = img
		}(i)
	}
	wg.Wait()
	if ctx != nil && ctx.Err() != nil {
		return out
	}

	type part struct {
		got *image.NRGBA
		r   image.Rectangle
	}
	var jobs []part
	type use struct {
		old, neu, full int
	}
	uses := make([]use, len(drafts))
	for i := range uses {
		uses[i] = use{-1, -1, -1}
	}
	for i, d := range drafts {
		if d.scored || rendered[i] == nil || s.want == nil {
			continue
		}
		if d.full || d.parent.got == nil {
			uses[i].full = len(jobs)
			jobs = append(jobs, part{rendered[i], s.want.Bounds()})
			continue
		}
		dirty := d.dirty
		if dirty.Empty() && len(d.work) > 0 {
			dirty = islandRect(d.work)
		}
		dirty = dirty.Inset(-2).Intersect(s.want.Bounds())
		if dirty.Empty() {
			uses[i].full = len(jobs)
			jobs = append(jobs, part{rendered[i], s.want.Bounds()})
			continue
		}
		uses[i].old = len(jobs)
		jobs = append(jobs, part{d.parent.got, dirty})
		uses[i].neu = len(jobs)
		jobs = append(jobs, part{rendered[i], dirty})
	}
	gots := make([]*image.NRGBA, len(jobs))
	rects := make([]image.Rectangle, len(jobs))
	for i, job := range jobs {
		gots[i] = job.got
		rects[i] = job.r
	}
	sums := scoreJobs(ctx, s.want, gots, rects)
	if ctx != nil && ctx.Err() != nil {
		return out
	}
	for i, d := range drafts {
		if d.scored {
			out[i] = d
			continue
		}
		if rendered[i] == nil {
			out[i] = nonePick()
			continue
		}
		var nerr float64
		switch {
		case uses[i].full >= 0:
			nerr = sums[uses[i].full]
		case uses[i].old >= 0:
			nerr = d.parent.errSum - sums[uses[i].old] + sums[uses[i].neu]
		default:
			out[i] = nonePick()
			continue
		}
		npaths := docPaths(d.doc)
		ncmds := docCmdLen(d.doc)
		parentCmds := d.parent.commands
		if parentCmds == 0 && d.parent.doc.Width() != 0 {
			parentCmds = docCmdLen(d.parent.doc)
		}
		ok := acceptLexicographic(nerr, npaths, ncmds, d.parent.errSum, d.parent.paths, parentCmds)
		d.got = rendered[i]
		d.errSum = nerr
		d.paths = npaths
		d.commands = ncmds
		d.ok = ok
		d.scored = true
		out[i] = d
	}
	return out
}
