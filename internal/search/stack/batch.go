package stack

import (
	"context"
	"image"
	"sync"

	"github.com/lewtec/svgolf/pkg/render"
)

// maxDevicePixels is the most pixels one score kernel may read.
// The loss is a sum of per-pixel terms, so a larger rectangle is several
// row bands and the bands add. Device buffers only grow, so this cap is
// also the resident size of the tape.
const maxDevicePixels = 1 << 20

// splitRows cuts r into row bands of at most limit pixels. A row wider
// than limit stays one band; the caller scores that band without the device.
func splitRows(r image.Rectangle, limit int) []image.Rectangle {
	if limit < 1 {
		limit = 1
	}
	if r.Empty() || r.Dx() < 1 || r.Dy() < 1 {
		return nil
	}
	var out []image.Rectangle
	for r.Dy() > 0 {
		rows := limit / r.Dx()
		if rows < 1 {
			rows = 1
		}
		if rows > r.Dy() {
			rows = r.Dy()
		}
		out = append(out, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+rows))
		r.Min.Y += rows
	}
	return out
}

// scoreJobs sums every rectangle. A rectangle wider than the device
// budget is split into row bands. The caller adds those bands.
func scoreJobs(ctx context.Context, want *image.NRGBA, gots []*image.NRGBA, rects []image.Rectangle) []float64 {
	return scoreJobsLimited(ctx, want, gots, rects, maxDevicePixels)
}

func scoreJobsLimited(ctx context.Context, want *image.NRGBA, gots []*image.NRGBA, rects []image.Rectangle, limit int) []float64 {
	out := make([]float64, len(rects))
	if limit < 1 {
		limit = 1
	}
	type piece struct {
		job int
		r   image.Rectangle
	}
	var pieces []piece
	for i, r := range rects {
		if want != nil {
			r = r.Intersect(want.Bounds())
		}
		if i < len(gots) && gots[i] != nil {
			r = r.Intersect(gots[i].Bounds())
		}
		for _, band := range splitRows(r, limit) {
			pieces = append(pieces, piece{i, band})
		}
	}
	for start := 0; start < len(pieces); {
		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		pixels := 0
		end := start
		for end < len(pieces) {
			n := pieces[end].r.Dx() * pieces[end].r.Dy()
			if end > start && pixels+n > limit {
				break
			}
			pixels += n
			end++
		}
		partGots := make([]*image.NRGBA, end-start)
		partRects := make([]image.Rectangle, end-start)
		for k := start; k < end; k++ {
			partGots[k-start] = gots[pieces[k].job]
			partRects[k-start] = pieces[k].r
		}
		sums, ok := ndScoreMany(ctx, want, partGots, partRects)
		if !ok {
			if ctx != nil && ctx.Err() != nil {
				return nil
			}
			sums = make([]float64, len(partRects))
			for k, r := range partRects {
				if partGots[k] == nil || want == nil || r.Empty() {
					continue
				}
				sums[k] = ScoreRect(ctx, partGots[k], want, r)
			}
		}
		for k := start; k < end; k++ {
			out[pieces[k].job] += sums[k-start]
		}
		start = end
	}
	return out
}

// scoreBatch renders the drained propositions and scores them together.
// A full proposition uses the whole pixmap. The others use the parent
// sum minus the old dirty rectangle plus the new one. scoreJobs packs
// those rectangles into kernels of at most maxDevicePixels.
func (s *world) scoreBatch(ctx context.Context, drafts []formPick) []formPick {
	out := make([]formPick, len(drafts))
	if ctx != nil && ctx.Err() != nil || len(drafts) == 0 {
		return out
	}
	s.scoreGroup(ctx, drafts, out)
	return out
}

func (s *world) scoreGroup(ctx context.Context, drafts []formPick, out []formPick) {
	if ctx != nil && ctx.Err() != nil {
		return
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
		return
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
	if ctx != nil && ctx.Err() != nil || sums == nil {
		return
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
		if ok {
			d.got = rendered[i]
		}
		d.errSum = nerr
		d.paths = npaths
		d.commands = ncmds
		d.ok = ok
		d.scored = true
		out[i] = d
	}
}
