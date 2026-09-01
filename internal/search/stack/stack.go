package stack

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"iter"
	"math"
	"math/rand/v2"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

const (
	maxPaths      = 512
	minIsland     = 8
	minErr        = 8
	polyFit       = 2
	// leftoverPicks = 3
	leftoverPicks = 1
	survivorPicks = 3
)

// Stack is variable neighborhood descent on a tiny non-dominated archive.
// Want stays native.
type Stack struct{}

var _ search.Search = Stack{}

func init() {
	search.Register("stack", func() search.Search { return Stack{} })
}

// world is the accepted document and the pixmap it paints.
// leftover is this epoch's hottest miss. grow is one existing
// path union that leftover. formPick is one scored operator.
type world struct {
	want, got    *image.NRGBA
	wantP, gotP  *loss.Plane
	doc          svg.Document
	owner        []uint16
	fills        []color.NRGBA
	scratch      scratch // leftover hottest() only
	errSum       float64
	paths        int
	w, h         int
	candidateLog io.Writer
	logMu        sync.Mutex
	snapID       int
}

var candidateLog io.Writer

var (
	planesOnce sync.Once
	planes     chan *loss.Plane
)

func initPlanes() {
	planesOnce.Do(func() {
		n := runtime.GOMAXPROCS(0)
		if n < 1 {
			n = 1
		}
		planes = make(chan *loss.Plane, n)
		for i := 0; i < n; i++ {
			planes <- &loss.Plane{}
		}
	})
}

func acquirePlane(img *image.NRGBA) *loss.Plane {
	initPlanes()
	p := <-planes
	p.Reset(img)
	return p
}

func releasePlane(p *loss.Plane) {
	if p == nil {
		return
	}
	p.Reset(nil)
	planes <- p
}

// LogCandidates writes one tab-indented line per scored candidate.
func LogCandidates(w io.Writer) {
	candidateLog = w
}

// leftover is the hottest residual blob and the paths that already
// touch it. paper leftovers carve; others may cover or grow.
// fresh is the new-plate grow (work=island, i=-1).
type leftover struct {
	island []pix
	col    color.NRGBA
	paper  bool
	grows  []grow
	fresh  grow
}

type grow struct {
	i      int
	work   []pix
	fill   color.NRGBA
	ring   [][2]float64
	dirty0 image.Rectangle
	oldErr float64
}

type formPick struct {
	doc      svg.Document
	got      *image.NRGBA
	errSum   float64
	paths    int
	commands int
	replace  int
	insert   int
	work     []pix
	fill     color.NRGBA
	reclaims [][]pix
	dropIdx  int
	mergeJ   int
	op       Op
	ok       bool
	scored   bool
	island   []pix
	fills    []color.NRGBA
	owner    []uint16
	parent   snapshot
}

type snapshot struct {
	id       int
	doc      svg.Document
	got      *image.NRGBA
	fills    []color.NRGBA
	owner    []uint16
	errSum   float64
	paths    int
	commands int
	operator Op
}

func nonePick() formPick {
	return formPick{replace: -1, insert: -1, dropIdx: -1, mergeJ: -1}
}

// betterPick is lexicographic (errSum, paths, commands). Accepts beat
// rejects. Unscored loses.
func betterPick(n, old formPick) bool {
	if !n.scored {
		return false
	}
	if !old.scored {
		return true
	}
	if n.ok != old.ok {
		return n.ok
	}
	return lexicographicLessPick(n, old)
}

func lexicographicLessPick(n, old formPick) bool {
	if n.errSum != old.errSum {
		return n.errSum < old.errSum
	}
	if n.paths != old.paths {
		return n.paths < old.paths
	}
	return n.commands < old.commands
}

// acceptLexicographic is true when the candidate beats the parent: lower
// pixel error, or equal error and fewer paths, or equal error and paths
// and fewer commands. Equal-error equal-complexity is a no-op.
func acceptLexicographic(errSum float64, paths, commands int, parentErr float64, parentPaths, parentCommands int) bool {
	if errSum < parentErr {
		return true
	}
	if errSum != parentErr {
		return false
	}
	if paths < parentPaths {
		return true
	}
	if paths != parentPaths {
		return false
	}
	return commands < parentCommands
}

func lexicographicLessSnapshot(a, b snapshot) bool {
	if a.errSum != b.errSum {
		return a.errSum < b.errSum
	}
	if a.paths != b.paths {
		return a.paths < b.paths
	}
	return a.commands < b.commands
}

func samePoint(a, b snapshot) bool {
	return a.errSum == b.errSum && a.paths == b.paths && a.commands == b.commands
}

// dominates is true when a is at least as good as b on error, paths,
// and commands, and strictly better on at least one.
func dominates(a, b snapshot) bool {
	if a.errSum > b.errSum || a.paths > b.paths || a.commands > b.commands {
		return false
	}
	return a.errSum < b.errSum || a.paths < b.paths || a.commands < b.commands
}

func mergeArchive(archive []snapshot, cands []snapshot) []snapshot {
	next := append([]snapshot(nil), archive...)
	for _, cand := range cands {
		skip := false
		for _, a := range next {
			if samePoint(a, cand) || dominates(a, cand) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		kept := make([]snapshot, 0, len(next)+1)
		for _, a := range next {
			if !dominates(cand, a) {
				kept = append(kept, a)
			}
		}
		next = append(kept, cand)
	}
	sort.SliceStable(next, func(i, j int) bool {
		return lexicographicLessSnapshot(next[i], next[j])
	})
	if len(next) > survivorPicks {
		next = next[:survivorPicks]
	}
	return next
}

func archiveChanged(old, next []snapshot) bool {
	if len(old) != len(next) {
		return true
	}
	for i := range old {
		if old[i].id != next[i].id {
			return true
		}
	}
	return false
}

func newWorld(target *image.NRGBA) (*world, error) {
	if target == nil {
		return nil, fmt.Errorf("search: nil pixmap")
	}
	b := target.Bounds()
	w, h := b.Dx(), b.Dy()
	doc := svg.NewDocument(float64(w), float64(h)).WithViewBox(0, 0, float64(w), float64(h))
	doc = doc.Append(whitePane(w, h).Node())
	got, err := render.Render(doc)
	if err != nil {
		return nil, err
	}
	wantP := loss.NewPlane(target)
	gotP := loss.NewPlane(got)
	wantP.Ensure()
	gotP.Ensure()
	return &world{
		want:   target,
		got:    got,
		wantP:  wantP,
		gotP:   gotP,
		doc:    doc,
		owner:  make([]uint16, w*h),
		w:      w,
		h:      h,
		errSum: ScoreOn(gotP, wantP),
	}, nil
}

func (Stack) Search(ctx context.Context, target *image.NRGBA) iter.Seq2[search.Epoch, error] {
	return func(yield func(search.Epoch, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(search.Epoch{}, err)
			return
		}
		s, err := newWorld(target)
		if err != nil {
			yield(search.Epoch{}, err)
			return
		}
		s.candidateLog = candidateLog
		started := time.Now()
		emit := func(id Op, blob []pix, rated []search.Rated) bool {
			ep := epochOf(s.doc, id)
			ep.Elapsed = time.Since(started)
			ep.Heat, ep.Island = DebugFrames(s.got, s.want, blob)
			ep.Rated = rated
			started = time.Now()
			return yield(ep, nil)
		}
		archive := []snapshot{s.snap()}
		band := 1
		yielded := false
		for {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(search.Epoch{}, err)
				}
				return
			}
			var pool []formPick
			var rated []search.Rated
			if band <= 3 {
				for _, member := range archive {
					s.load(member)
					picks, pr, err := s.choose(ctx, s.leftovers(), member, band)
					if err != nil {
						yield(search.Epoch{}, err)
						return
					}
					pool = append(pool, picks...)
					rated = mergeRated(rated, pr)
				}
			} else {
				miss := make([][]leftover, len(archive))
				for j, member := range archive {
					s.load(member)
					miss[j] = s.leftovers()
				}
				for i, member := range archive {
					for j := range archive {
						if i == j {
							continue
						}
						s.load(member)
						picks, pr, err := s.choose(ctx, s.bindLeftovers(miss[j]), member, 4)
						if err != nil {
							yield(search.Epoch{}, err)
							return
						}
						pool = append(pool, picks...)
						rated = mergeRated(rated, pr)
					}
				}
			}
			next, improved := s.archiveUpdate(archive, pool, band)
			if improved {
				archive = next
				s.load(archive[0])
				yielded = true
				markKept(rated, []formPick{{op: archive[0].operator}})
				if !emit(archive[0].operator, islandOf(archive[0], pool), rated) {
					return
				}
				// A new plate just landed. Try wash/join before
				// another leftover add, or a ramp stacks flats forever.
				if leftoverAdd(archive[0].operator) {
					band = 2
				} else {
					band = 1
				}
				continue
			}
			if band < 3 {
				band++
				continue
			}
			if band == 3 {
				band = 4
				continue
			}
			break
		}
		if !yielded {
			emit(OpNone, nil, nil)
		}
	}
}

func epochOf(doc svg.Document, id Op) search.Epoch {
	return search.Epoch{Document: doc, Scale: 1, Operator: id}
}

func (left leftover) big() bool {
	return len(left.island) >= minIsland
}

func (s *world) leftovers() []leftover {
	blobs := s.hottestN(leftoverPicks)
	out := make([]leftover, 0, len(blobs))
	for _, b := range blobs {
		out = append(out, leftover{island: b.island, col: b.col, paper: paperLeftover(b.col)})
	}
	return s.bindLeftovers(out)
}

// bindLeftovers scores leftover islands against the loaded document
// so a sibling's miss can be applied onto this parent.
func (s *world) bindLeftovers(lefts []leftover) []leftover {
	out := make([]leftover, len(lefts))
	for i, left := range lefts {
		left.grows = nil
		if left.big() {
			left.fresh = s.seedGrow(grow{i: -1, work: left.island, fill: left.col})
		}
		out[i] = left
	}
	return out
}

func (s *world) seedGrow(g grow) grow {
	g.dirty0 = islandRect(g.work)
	if g.i >= 0 {
		g.dirty0 = g.dirty0.Union(nodeRect(s.doc.Children()[g.i+1]))
	}
	g.oldErr = ScoreRectOn(s.gotP, s.wantP, g.dirty0.Inset(-2))
	return g
}

func (s *world) snap() snapshot {
	s.snapID++
	return snapshot{
		id:       s.snapID,
		doc:      s.doc,
		got:      s.got,
		fills:    append([]color.NRGBA(nil), s.fills...),
		owner:    append([]uint16(nil), s.owner...),
		errSum:   s.errSum,
		paths:    s.paths,
		commands: docCmdLen(s.doc),
	}
}

func (s *world) load(sn snapshot) {
	s.doc = sn.doc
	s.got = sn.got
	s.fills = append([]color.NRGBA(nil), sn.fills...)
	s.owner = append([]uint16(nil), sn.owner...)
	s.errSum = sn.errSum
	s.paths = sn.paths
	if s.gotP == nil {
		s.gotP = loss.NewPlane(s.got)
	} else {
		s.gotP.Reset(s.got)
	}
	s.gotP.Ensure()
}

func leftoverAdd(id Op) bool {
	return id == OpTriangle || id == OpRing
}

func (s *world) archiveUpdate(archive []snapshot, pool []formPick, band int) ([]snapshot, bool) {
	var cands []snapshot
	for _, p := range pool {
		if !p.ok || !p.scored {
			continue
		}
		// Score leftover add every leftover band. Skip accept on
		// the wash band so a ramp can land a linear instead of
		// stacking another flat.
		if leftoverAdd(p.op) && band == 2 {
			continue
		}
		s.load(p.parent)
		s.apply(p)
		sn := s.snap()
		sn.operator = p.op
		cands = append(cands, sn)
	}
	next := mergeArchive(archive, cands)
	return next, archiveChanged(archive, next)
}

func islandOf(best snapshot, pool []formPick) []pix {
	for _, p := range pool {
		if p.op == best.operator {
			return p.island
		}
	}
	return nil
}

func markKept(rated []search.Rated, kept []formPick) {
	chosen := OpNone
	if len(kept) > 0 {
		chosen = kept[0].op
	}
	type row struct {
		i int
		a float64
	}
	var ranked []row
	for i, r := range rated {
		if r.Score != nil {
			ranked = append(ranked, row{i, *r.Score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].a < ranked[j].a })
	n := survivorPicks
	if n > len(ranked) {
		n = len(ranked)
	}
	for k := 0; k < n; k++ {
		rated[ranked[k].i].Best = true
	}
	for i := range rated {
		if rated[i].Name == chosen.String() {
			rated[i].Chosen = true
		}
	}
}

func (s *world) apply(pick formPick) {
	s.doc, s.got, s.errSum = pick.doc, pick.got, pick.errSum
	if pick.owner != nil {
		s.owner = pick.owner
		s.fills = pick.fills
		s.paths = len(s.fills)
	} else if pick.dropIdx >= 0 {
		dropOwner(s.owner, uint16(pick.dropIdx+1), s.paths)
		s.fills = append(s.fills[:pick.dropIdx], s.fills[pick.dropIdx+1:]...)
		s.paths--
	} else if pick.mergeJ >= 0 {
		j := pick.mergeJ
		i := pick.replace
		for k, v := range s.owner {
			if v == uint16(j+1) {
				s.owner[k] = uint16(i + 1)
			}
		}
		dropOwner(s.owner, uint16(j+1), s.paths)
		s.fills[i] = pick.fill
		s.fills = append(s.fills[:j], s.fills[j+1:]...)
		clearOwner(s.owner, uint16(i+1))
		claim(s.owner, pick.work, s.w, uint16(i+1))
		s.paths--
	} else if len(pick.reclaims) > 0 {
		for i, work := range pick.reclaims {
			if work == nil {
				continue
			}
			id := uint16(i + 1)
			clearOwner(s.owner, id)
			claim(s.owner, work, s.w, id)
		}
	} else if pick.replace >= 0 {
		id := uint16(pick.replace + 1)
		clearOwner(s.owner, id)
		claim(s.owner, pick.work, s.w, id)
		s.fills[pick.replace] = pick.fill
	} else if pick.insert >= 0 && pick.insert < s.paths {
		id := uint16(pick.insert + 1)
		for i, v := range s.owner {
			if v >= id {
				s.owner[i] = v + 1
			}
		}
		s.fills = append(s.fills, color.NRGBA{})
		copy(s.fills[pick.insert+1:], s.fills[pick.insert:])
		s.fills[pick.insert] = pick.fill
		claim(s.owner, pick.work, s.w, id)
		s.paths++
	} else {
		claim(s.owner, pick.work, s.w, uint16(s.paths+1))
		s.fills = append(s.fills, pick.fill)
		s.paths++
	}
	if s.gotP == nil {
		s.gotP = loss.NewPlane(s.got)
	} else {
		s.gotP.Reset(s.got)
	}
	s.gotP.Ensure()
}

// swapPaths exchanges path i with path j. Pane stays first.
func (s *world) swapPaths(i, j int) (svg.Document, []color.NRGBA, []uint16, bool) {
	if i < 0 || j < 0 || i >= s.paths || j >= s.paths || i == j {
		return svg.Document{}, nil, nil, false
	}
	if i > j {
		i, j = j, i
	}
	kids := s.doc.Children()
	out := svg.NewDocument(s.doc.Width(), s.doc.Height())
	if vb := s.doc.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	out = out.Append(kids[0])
	for k := 0; k < s.paths; k++ {
		src := k
		if k == i {
			src = j
		} else if k == j {
			src = i
		}
		out = out.Append(kids[src+1])
	}
	fills := append([]color.NRGBA(nil), s.fills...)
	fills[i], fills[j] = fills[j], fills[i]
	owner := append([]uint16(nil), s.owner...)
	idA, idB := uint16(i+1), uint16(j+1)
	for k, v := range owner {
		switch v {
		case idA:
			owner[k] = idB
		case idB:
			owner[k] = idA
		}
	}
	return out, fills, owner, true
}

func (s *world) connecting(island []pix, seen []byte) []grow {
	var out []grow
	for i := range s.fills {
		work := ownedUnion(s.owner, island, s.w, s.h, uint16(i+1), seen)
		if len(work) <= len(island) && !s.paintsIsland(s.doc.Children()[i+1], island) {
			continue
		}
		out = append(out, s.seedGrow(grow{i: i, work: work, fill: s.fills[i]}))
	}
	return out
}

func (s *world) logCandidate(id Op, elapsed time.Duration, p formPick) {
	if s == nil || s.candidateLog == nil {
		return
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if !p.scored {
		fmt.Fprintf(s.candidateLog, "\t%s elapsed=%.3fs score=-\n", id, elapsed.Seconds())
		return
	}
	fmt.Fprintf(s.candidateLog, "\t%s elapsed=%.3fs score=%.3f\n", id, elapsed.Seconds(), p.errSum)
}

func (s *world) scoreCand(next svg.Document, cand svg.Node, g grow, id Op) (formPick, error) {
	if p, ok := cand.Path(); ok {
		for _, r := range pathRings(p) {
			if ringCrosses(r) {
				return nonePick(), nil
			}
		}
	}
	ngot, err := render.Scratch(next)
	if err != nil {
		return nonePick(), err
	}
	defer render.Release(ngot)
	gotP := acquirePlane(ngot)
	defer releasePlane(gotP)
	nerr := ScoreOn(gotP, s.wantP)
	npaths := docPaths(next)
	ncmds := docCmdLen(next)
	ok := acceptLexicographic(nerr, npaths, ncmds, s.errSum, s.paths, docCmdLen(s.doc))
	var got *image.NRGBA
	if ok {
		got = render.Keep(ngot)
	}
	return formPick{doc: next, got: got, errSum: nerr, paths: npaths, commands: ncmds, replace: g.i, insert: -1, work: g.work, fill: g.fill, dropIdx: -1, mergeJ: -1, op: id, ok: ok, scored: true}, nil
}

// addLayer scores a new path on top and at one random existing
// slot. A background plate loses on top; Score keeps it if the
// random slot is behind the thing it must not cover.
func (s *world) addLayer(cand svg.Path, g grow, id Op) (formPick, error) {
	node := cand.Node()
	best := nonePick()
	slots := []int{s.paths}
	if s.paths > 0 {
		slots = append(slots, rand.IntN(s.paths))
	}
	for _, at := range slots {
		var next svg.Document
		if at >= s.paths {
			next = s.doc.Append(node)
		} else {
			next = insertAt(s.doc, at+1, node)
		}
		pick, err := s.scoreCand(next, node, g, id)
		if err != nil {
			return nonePick(), err
		}
		if pick.ok && at < s.paths {
			pick.insert = at
		}
		if betterPick(pick, best) {
			best = pick
		}
	}
	return best, nil
}

func (s *world) paintsIsland(node svg.Node, island []pix) bool {
	if !nodeRect(node).Overlaps(islandRect(island)) {
		return false
	}
	d := svg.NewDocument(float64(s.w), float64(s.h)).WithViewBox(0, 0, float64(s.w), float64(s.h))
	d = d.Append(whitePane(s.w, s.h).Node()).Append(node)
	img, err := render.Scratch(d)
	if err != nil {
		return false
	}
	defer render.Release(img)
	for _, p := range island {
		if colorErr(img.NRGBAAt(p.x, p.y), paper) > minErr {
			return true
		}
	}
	return false
}

func ownerBucket(owner []uint16, w int, id uint16) []pix {
	var out []pix
	for i, v := range owner {
		if v == id {
			out = append(out, pix{i % w, i / w})
		}
	}
	return out
}

func fillBuckets(owner []uint16, w, n int, buckets [][]pix) [][]pix {
	if cap(buckets) < n {
		buckets = make([][]pix, n)
	} else {
		buckets = buckets[:n]
		for i := range buckets {
			buckets[i] = buckets[i][:0]
		}
	}
	for i, v := range owner {
		if v == 0 || int(v) > n {
			continue
		}
		id := int(v) - 1
		buckets[id] = append(buckets[id], pix{i % w, i / w})
	}
	return buckets
}

func dropOwner(owner []uint16, id uint16, n int) {
	for i, v := range owner {
		switch {
		case v == id:
			owner[i] = 0
		case v > id:
			owner[i]--
		}
	}
}

func dropAt(d svg.Document, i int) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	for j, k := range kids {
		if j == i {
			continue
		}
		out = out.Append(k)
	}
	return out
}

func whitePane(w, h int) svg.Path {
	return filledPath([][2]float64{
		{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)},
	}, paper)
}

// leftoverRings are enclosed voids of this leftover that are not paper
// and are already painted. A paper hole waits for carve. A painted
// interior is part of the leftover outline (visor on a plate). An
// unpainted void is another leftover, not a hole in this plate.
func leftoverRings(island []pix, got, want *image.NRGBA, col color.NRGBA) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		if paperLeftover(modeFill(want, h)) {
			continue
		}
		if loss.ColorAt(modeFill(want, h), col) <= minErr {
			continue
		}
		if !holePainted(got, want, h) {
			continue
		}
		r := hullRing(h)
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func sameColorHollow(island []pix, want *image.NRGBA, col color.NRGBA) bool {
	for _, h := range voids(island) {
		if paperLeftover(modeFill(want, h)) {
			continue
		}
		if loss.ColorAt(modeFill(want, h), col) <= minErr {
			return true
		}
	}
	return false
}

func holePainted(got, want *image.NRGBA, hole []pix) bool {
	if got == nil || want == nil || len(hole) == 0 {
		return false
	}
	w := want.Bounds().Dx()
	for _, p := range hole {
		if residual(got, want, p.x, p.y, w) {
			return false
		}
	}
	return true
}

func holeRings(island []pix) [][][2]float64 {
	var rings [][][2]float64
	for _, h := range voids(island) {
		r := convexHull(islandPoints(h))
		if len(r) >= 3 {
			rings = append(rings, r)
		}
	}
	return rings
}

func withHoles(p svg.Path, holes [][][2]float64) svg.Path {
	for _, h := range holes {
		p = appendRing(p, h)
	}
	return p.WithFillRule(svg.FillEvenOdd)
}

func pathArea(n svg.Node) float64 {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	rings := pathRings(p)
	if len(rings) == 0 {
		return 0
	}
	return shoelace(rings[0])
}

func shoelace(ring [][2]float64) float64 {
	n := len(ring)
	if n < 3 {
		return 0
	}
	var a float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a += ring[i][0]*ring[j][1] - ring[j][0]*ring[i][1]
	}
	if a < 0 {
		a = -a
	}
	return a / 2
}

func pathOuter(n svg.Node) [][2]float64 {
	p, ok := n.Path()
	if !ok {
		return nil
	}
	rings := pathRings(p)
	if len(rings) == 0 {
		return nil
	}
	return rings[0]
}

func pathOuters(doc svg.Document, n int) [][][2]float64 {
	out := make([][][2]float64, n)
	kids := doc.Children()
	for i := 0; i < n && i+1 < len(kids); i++ {
		out[i] = pathOuter(kids[i+1])
	}
	return out
}

// pathRing is one closed subpath. edges[i] goes from verts[i] to
// verts[(i+1)%n] and keeps the original Line or Cubic.
type pathRing struct {
	verts [][2]float64
	edges []svg.PathCmd
}

func (r pathRing) points() [][2]float64 {
	return r.verts
}

func parsePathRings(p svg.Path) []pathRing {
	var rings []pathRing
	var cur pathRing
	has := false
	flush := func() {
		if has && len(cur.verts) >= 3 {
			if len(cur.edges) == len(cur.verts)-1 {
				cur.edges = append(cur.edges, svg.PathCmd{Kind: svg.CmdClose, X: cur.verts[0][0], Y: cur.verts[0][1]})
			}
			if len(cur.edges) == len(cur.verts) {
				rings = append(rings, cur)
			}
		}
		cur = pathRing{}
		has = false
	}
	for _, c := range p.Commands() {
		switch c.Kind {
		case svg.CmdMove:
			flush()
			cur.verts = [][2]float64{{c.X, c.Y}}
			has = true
		case svg.CmdClose:
			if has && len(cur.verts) >= 1 && len(cur.edges) == len(cur.verts)-1 {
				cur.edges = append(cur.edges, svg.PathCmd{Kind: svg.CmdClose, X: cur.verts[0][0], Y: cur.verts[0][1]})
			}
			flush()
		default:
			cur.edges = append(cur.edges, c)
			cur.verts = append(cur.verts, [2]float64{c.X, c.Y})
		}
	}
	flush()
	return rings
}

func pathRings(p svg.Path) [][][2]float64 {
	rings := parsePathRings(p)
	out := make([][][2]float64, len(rings))
	for i, r := range rings {
		out[i] = r.points()
	}
	return out
}

func polylineRing(pts [][2]float64) pathRing {
	if len(pts) < 3 {
		return pathRing{}
	}
	r := pathRing{verts: append([][2]float64{}, pts...)}
	for i := 0; i < len(pts); i++ {
		dest := pts[(i+1)%len(pts)]
		kind := svg.CmdLine
		if i == len(pts)-1 {
			kind = svg.CmdClose
		}
		r.edges = append(r.edges, svg.PathCmd{Kind: kind, X: dest[0], Y: dest[1]})
	}
	return r
}

func (r pathRing) appendTo(p svg.Path) svg.Path {
	if len(r.verts) < 3 {
		return p
	}
	cmds := p.Commands()
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdMove, X: r.verts[0][0], Y: r.verts[0][1]})
	for i, e := range r.edges {
		if e.Kind == svg.CmdClose {
			continue
		}
		if i == len(r.edges)-1 && e.Kind == svg.CmdLine && e.X == r.verts[0][0] && e.Y == r.verts[0][1] {
			continue
		}
		cmds = append(cmds, e)
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	p, _ = p.WithCommands(cmds)
	return p
}

func filledRings(outer pathRing, holes []pathRing, col color.NRGBA) svg.Path {
	p := outer.appendTo(svg.NewPath())
	for _, h := range holes {
		p = h.appendTo(p)
	}
	p = p.WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
	if len(holes) > 0 {
		p = p.WithFillRule(svg.FillEvenOdd)
	}
	return p
}

func (r pathRing) dropVertex(i int) pathRing {
	n := len(r.verts)
	if n < 4 || i < 0 || i >= n || len(r.edges) != n {
		return pathRing{}
	}
	prev := (i - 1 + n) % n
	next := (i + 1) % n
	verts := append([][2]float64{}, r.verts[:i]...)
	verts = append(verts, r.verts[i+1:]...)
	var edges []svg.PathCmd
	for k := 0; k < n; k++ {
		if k == prev {
			dest := r.verts[next]
			edges = append(edges, svg.PathCmd{Kind: svg.CmdLine, X: dest[0], Y: dest[1]})
			continue
		}
		if k == i {
			continue
		}
		edges = append(edges, r.edges[k])
	}
	return pathRing{verts: verts, edges: edges}
}

func (r pathRing) moveVertex(i int, to [2]float64) pathRing {
	n := len(r.verts)
	if i < 0 || i >= n || len(r.edges) != n {
		return pathRing{}
	}
	verts := append([][2]float64{}, r.verts...)
	edges := append([]svg.PathCmd{}, r.edges...)
	verts[i] = to
	prev := (i - 1 + n) % n
	edges[prev].X, edges[prev].Y = to[0], to[1]
	return pathRing{verts: verts, edges: edges}
}

func (r pathRing) setEdge(i int, cmd svg.PathCmd) pathRing {
	n := len(r.verts)
	if i < 0 || i >= n || len(r.edges) != n {
		return pathRing{}
	}
	edges := append([]svg.PathCmd{}, r.edges...)
	edges[i] = cmd
	return pathRing{verts: append([][2]float64{}, r.verts...), edges: edges}
}

func (r pathRing) spliceAfter(ei int, chain [][2]float64) pathRing {
	n := len(r.verts)
	if ei < 0 || ei >= n || len(r.edges) != n {
		return pathRing{}
	}
	dest := r.verts[(ei+1)%n]
	verts := append([][2]float64{}, r.verts[:ei+1]...)
	verts = append(verts, chain...)
	if ei+1 < n {
		verts = append(verts, r.verts[ei+1:]...)
	}
	var edges []svg.PathCmd
	for k := 0; k < n; k++ {
		if k == ei {
			for _, p := range chain {
				edges = append(edges, svg.PathCmd{Kind: svg.CmdLine, X: p[0], Y: p[1]})
			}
			edges = append(edges, svg.PathCmd{Kind: svg.CmdLine, X: dest[0], Y: dest[1]})
			continue
		}
		edges = append(edges, r.edges[k])
	}
	return pathRing{verts: verts, edges: edges}
}

// regionWorthTrying is true when a drop can still change the picture:
// the polygon is empty of raster pixels (command golf) or it covers a
// mismatch. A region of already-correct pixels is skipped so simplify
// does not re-render every load-bearing ear.
func regionWorthTrying(ring [][2]float64, got, want *loss.Plane) bool {
	if len(ring) < 3 || got == nil || want == nil || got.Image() == nil || want.Image() == nil {
		return true
	}
	minX, minY := ring[0][0], ring[0][1]
	maxX, maxY := minX, minY
	for _, p := range ring[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	box := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY)))
	box = box.Intersect(want.Image().Rect)
	if box.Empty() {
		return true
	}
	got.EnsureRect(box)
	want.EnsureRect(box)
	seen := false
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			if !pointInRing(ring, float64(x)+0.5, float64(y)+0.5) {
				continue
			}
			seen = true
			if errAtHSV(got.At(x, y), want.At(x, y)) > 0 {
				return true
			}
		}
	}
	return !seen
}

func (r pathRing) collapseColinearLines() pathRing {
	out := r
	for len(out.verts) > 3 && len(out.edges) == len(out.verts) {
		n := len(out.verts)
		drop := -1
		for i := 0; i < n; i++ {
			prev := (i - 1 + n) % n
			if out.edges[prev].Kind != svg.CmdLine && out.edges[prev].Kind != svg.CmdClose {
				continue
			}
			if out.edges[i].Kind != svg.CmdLine && out.edges[i].Kind != svg.CmdClose {
				continue
			}
			a, b, c := out.verts[prev], out.verts[i], out.verts[(i+1)%n]
			if (b[0]-a[0])*(c[1]-a[1]) == (b[1]-a[1])*(c[0]-a[0]) {
				drop = i
				break
			}
		}
		if drop < 0 {
			break
		}
		out = out.dropVertex(drop)
	}
	return out
}

func pathCommandWeight(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	w := 0
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdLine {
			w += 2
			continue
		}
		w++
	}
	return w
}

func docCmdLen(d svg.Document) int {
	n := 0
	for _, c := range d.Children() {
		n += pathCommandWeight(c)
	}
	return n
}

func docPaths(d svg.Document) int {
	n := len(d.Children()) - 1
	if n < 0 {
		return 0
	}
	return n
}

func thinIsland(island []pix) bool {
	if len(island) == 0 {
		return false
	}
	bb := bbox(island)
	w := bb[1][0] - bb[0][0]
	h := bb[2][1] - bb[1][1]
	return w <= 1 || h <= 1
}

func appendRing(p svg.Path, ring [][2]float64) svg.Path {
	if len(ring) < 3 {
		return p
	}
	cmds := p.Commands()
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdMove, X: ring[0][0], Y: ring[0][1]})
	for _, q := range ring[1:] {
		cmds = append(cmds, svg.PathCmd{Kind: svg.CmdLine, X: q[0], Y: q[1]})
	}
	cmds = append(cmds, svg.PathCmd{Kind: svg.CmdClose})
	p, _ = p.WithCommands(cmds)
	return p
}

func filledPath(ring [][2]float64, col color.NRGBA) svg.Path {
	return appendRing(svg.NewPath(), ring).WithFill(color.NRGBA{R: col.R, G: col.G, B: col.B, A: 255})
}

func islandRect(island []pix) image.Rectangle {
	if len(island) == 0 {
		return image.Rectangle{}
	}
	r := image.Rect(island[0].x, island[0].y, island[0].x+1, island[0].y+1)
	for _, p := range island[1:] {
		r = r.Union(image.Rect(p.x, p.y, p.x+1, p.y+1))
	}
	return r
}

func nodeRect(ns ...svg.Node) image.Rectangle {
	var r image.Rectangle
	for _, n := range ns {
		p, ok := n.Path()
		if !ok {
			continue
		}
		for _, c := range p.Commands() {
			q := image.Rect(int(c.X)-1, int(c.Y)-1, int(c.X)+2, int(c.Y)+2)
			if c.Kind == svg.CmdCubic {
				q = q.Union(image.Rect(int(c.X1)-1, int(c.Y1)-1, int(c.X1)+2, int(c.Y1)+2))
				q = q.Union(image.Rect(int(c.X2)-1, int(c.Y2)-1, int(c.X2)+2, int(c.Y2)+2))
			}
			if r.Empty() {
				r = q
			} else {
				r = r.Union(q)
			}
		}
	}
	return r
}

func insertAt(d svg.Document, i int, n svg.Node) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	if i < 0 {
		i = 0
	}
	for j, k := range kids {
		if j == i {
			out = out.Append(n)
		}
		out = out.Append(k)
	}
	if i >= len(kids) {
		out = out.Append(n)
	}
	return out
}

func replaceAt(d svg.Document, i int, n svg.Node) svg.Document {
	kids := d.Children()
	out := svg.NewDocument(d.Width(), d.Height())
	if vb := d.ViewBox(); vb.Set() {
		out = out.WithViewBox(vb.MinX(), vb.MinY(), vb.Width(), vb.Height())
	}
	for j, k := range kids {
		if j == i {
			out = out.Append(n)
			continue
		}
		out = out.Append(k)
	}
	return out
}
