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
	leftoverPicks = 3
	survivorPicks = 3
	streakRate    = 1.1
)

// Stack crosses every pair of survivors: leftover of B on document A,
// plus every world Operator on A. Score keeps the strongest. Want stays native.
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
	skip         []byte
	owner        []uint16
	fills        []color.NRGBA
	scratch      scratch // leftover hottest() only
	errSum       float64
	paths        int
	w, h         int
	candidateLog io.Writer
	logMu        sync.Mutex
	snapID       int
	winOp        string
	winN         int
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
	a        float64
	raw      float64
	replace  int
	insert   int
	work     []pix
	fill     color.NRGBA
	reclaims [][]pix
	dropIdx  int
	mergeJ   int
	op       string
	ok       bool
	scored   bool
	island   []pix
	fills    []color.NRGBA
	owner    []uint16
	parent   snapshot
}

type snapshot struct {
	id     int
	doc    svg.Document
	got    *image.NRGBA
	fills  []color.NRGBA
	owner  []uint16
	skip   []byte
	errSum float64
	paths  int
	winOp  string
	winN   int
}

func nonePick() formPick {
	return formPick{replace: -1, insert: -1, dropIdx: -1, mergeJ: -1}
}

// betterPick is the lower Score. Accepts beat rejects. Unscored loses.
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
	return n.a < old.a
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
		skip:   make([]byte, w*h),
		owner:  make([]uint16, w*h),
		w:      w,
		h:      h,
		errSum: ScoreOn(gotP, wantP, 0),
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
		emit := func(op string, blob []pix, rated []search.Rated) bool {
			ep := epochOf(s.doc, op)
			ep.Elapsed = time.Since(started)
			ep.Heat, ep.Island = DebugFrames(s.got, s.want, blob)
			ep.Rated = rated
			started = time.Now()
			return yield(ep, nil)
		}
		survivors := []snapshot{s.snap()}
		yielded := false
		for {
			if err := ctx.Err(); err != nil {
				if !yielded {
					yield(search.Epoch{}, err)
				}
				return
			}
			bestA := survivors[0].score()
			for _, sv := range survivors[1:] {
				if a := sv.score(); a < bestA {
					bestA = a
				}
			}
			miss := make([][]leftover, len(survivors))
			for j, b := range survivors {
				s.load(b)
				miss[j] = s.leftovers()
			}
			var pool []formPick
			var rated []search.Rated
			for _, a := range survivors {
				s.load(a)
				world, wr, err := s.choose(ctx, nil, a, true)
				if err != nil {
					yield(search.Epoch{}, err)
					return
				}
				pool = append(pool, world...)
				rated = mergeRated(rated, wr)
				for j := range survivors {
					s.load(a)
					picks, pr, err := s.choose(ctx, s.bindLeftovers(miss[j]), a, false)
					if err != nil {
						yield(search.Epoch{}, err)
						return
					}
					pool = append(pool, picks...)
					rated = mergeRated(rated, pr)
				}
			}
			kept := rankGeneration(pool, bestA, survivorPicks)
			if len(kept) == 0 {
				any := false
				for i, sv := range survivors {
					s.load(sv)
					for _, left := range s.leftovers() {
						if !left.big() {
							continue
						}
						s.ignore(left)
						any = true
					}
					survivors[i] = s.snap()
				}
				if !any {
					break
				}
				continue
			}
			next := make([]snapshot, 0, len(kept))
			for _, p := range kept {
				s.load(p.parent)
				s.apply(p)
				s.noteWin(p.op)
				next = append(next, s.snap())
			}
			s.load(next[0])
			survivors = next
			yielded = true
			if !emit(kept[0].op, kept[0].island, rated) {
				return
			}
		}
		if !yielded {
			emit("", nil, nil)
		}
	}
}

func epochOf(doc svg.Document, op string) search.Epoch {
	return search.Epoch{Document: doc, Scale: 1, Operator: op}
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

func (s *world) currentScore() float64 {
	return s.errSum + pathCost*float64(s.paths) + cmdCost*float64(docCmdLen(s.doc))
}

// streak inflates a repeat world-op: Score * streakRate^n.
// Leftover ops stay raw so a second cover can still land.
func (s *world) streak(a float64, op string) float64 {
	if s == nil || s.winN <= 0 || op != s.winOp || !streakable(op) {
		return a
	}
	return a * math.Pow(streakRate, float64(s.winN))
}

func streakable(op string) bool {
	switch op {
	case "simplify", "wash", "hull", "join", "subtract", "swap", "delete":
		return true
	default:
		return false
	}
}

func (s *world) noteWin(op string) {
	if op == s.winOp {
		s.winN++
		return
	}
	s.winOp = op
	s.winN = 1
}

func (sn snapshot) score() float64 {
	return sn.errSum + pathCost*float64(sn.paths) + cmdCost*float64(docCmdLen(sn.doc))
}

func (s *world) snap() snapshot {
	s.snapID++
	return snapshot{
		id:     s.snapID,
		doc:    s.doc,
		got:    s.got,
		fills:  append([]color.NRGBA(nil), s.fills...),
		owner:  append([]uint16(nil), s.owner...),
		skip:   append([]byte(nil), s.skip...),
		errSum: s.errSum,
		paths:  s.paths,
		winOp:  s.winOp,
		winN:   s.winN,
	}
}

func (s *world) load(sn snapshot) {
	s.doc = sn.doc
	s.got = sn.got
	s.fills = append([]color.NRGBA(nil), sn.fills...)
	s.owner = append([]uint16(nil), sn.owner...)
	s.skip = append([]byte(nil), sn.skip...)
	s.errSum = sn.errSum
	s.paths = sn.paths
	s.winOp = sn.winOp
	s.winN = sn.winN
	if s.gotP == nil {
		s.gotP = loss.NewPlane(s.got)
	} else {
		s.gotP.Reset(s.got)
	}
	s.gotP.Ensure()
}

func rankGeneration(pool []formPick, bestA float64, y int) []formPick {
	var out []formPick
	for _, p := range pool {
		if p.ok && p.a < bestA {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].a < out[j].a })
	var kept []formPick
	seen := map[int]bool{}
	for _, p := range out {
		if p.parent.id != 0 && seen[p.parent.id] {
			continue
		}
		if p.parent.id != 0 {
			seen[p.parent.id] = true
		}
		kept = append(kept, p)
		if y > 0 && len(kept) == y {
			break
		}
	}
	return kept
}

func (s *world) ignore(left leftover) {
	markSkip(s.skip, left.island, s.w)
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
	if s.skip != nil {
		clear(s.skip)
	}
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

func (s *world) logCandidate(op string, elapsed time.Duration, p formPick) {
	if s == nil || s.candidateLog == nil {
		return
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if !p.scored {
		fmt.Fprintf(s.candidateLog, "\t%s elapsed=%.3fs score=-\n", op, elapsed.Seconds())
		return
	}
	fmt.Fprintf(s.candidateLog, "\t%s elapsed=%.3fs score=%.3f\n", op, elapsed.Seconds(), p.a)
}

func (s *world) scoreCand(next svg.Document, cand svg.Node, g grow, parts int, op string, curA float64) (formPick, error) {
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
	nerr := ScoreOn(gotP, s.wantP, 0)
	raw := nerr + pathCost*float64(parts) + cmdCost*float64(docCmdLen(next))
	a := s.streak(raw, op)
	ok := a < curA
	var got *image.NRGBA
	if ok {
		got = render.Keep(ngot)
	}
	return formPick{doc: next, got: got, errSum: nerr, a: a, raw: raw, replace: g.i, insert: -1, work: g.work, fill: g.fill, dropIdx: -1, mergeJ: -1, op: op, ok: ok, scored: true}, nil
}

// addLayer scores a new path on top and at one random existing
// slot. A background plate loses on top; Score keeps it if the
// random slot is behind the thing it must not cover.
func (s *world) addLayer(cand svg.Path, g grow, op string) (formPick, error) {
	node := cand.Node()
	curA := s.currentScore()
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
		pick, err := s.scoreCand(next, node, g, s.paths+1, op, curA)
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

func markSkip(skip []byte, island []pix, w int) {
	for _, p := range island {
		skip[p.y*w+p.x] = 1
	}
}

func acceptSum(err0, err1 float64, parts, nparts int, old, cand svg.Node) bool {
	a := err0 + pathCost*float64(parts)
	b := err1 + pathCost*float64(nparts)
	if b < a {
		return true
	}
	if b > a || old.Kind() == svg.KindInvalid {
		return false
	}
	return pathLen(cand) < pathLen(old)
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
		if residual(got, want, nil, p.x, p.y, w) {
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

func pathRings(p svg.Path) [][][2]float64 {
	var rings [][][2]float64
	var cur [][2]float64
	flush := func() {
		if len(cur) >= 3 {
			rings = append(rings, cur)
		}
		cur = nil
	}
	for _, c := range p.Commands() {
		switch c.Kind {
		case svg.CmdMove:
			flush()
			cur = [][2]float64{{c.X, c.Y}}
		case svg.CmdClose:
			flush()
		default:
			cur = append(cur, [2]float64{c.X, c.Y})
		}
	}
	flush()
	return rings
}

func pathLen(n svg.Node) int {
	p, ok := n.Path()
	if !ok {
		return 0
	}
	return len(p.Commands())
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
