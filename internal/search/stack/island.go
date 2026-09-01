package stack

import (
	"image"
	"image/color"
	"runtime"
	"sync"

	"github.com/lewtec/svgolf/internal/loss"
)

type pix struct{ x, y int }

// pixBits is an island membership mask. Same answers as a map[pix]bool.
type pixBits struct {
	minX, minY, w, h int
	bits             []uint64
}

var (
	emptyBits = &pixBits{}
	bitsOnce  sync.Once
	bitsPool  chan *pixBits
)

func initBits() {
	bitsOnce.Do(func() {
		n := runtime.GOMAXPROCS(0)
		if n < 1 {
			n = 1
		}
		bitsPool = make(chan *pixBits, n)
		for i := 0; i < n; i++ {
			bitsPool <- &pixBits{}
		}
	})
}

func pixSet(island []pix) *pixBits {
	if len(island) == 0 {
		return emptyBits
	}
	initBits()
	b := <-bitsPool
	b.load(island)
	return b
}

func releaseBits(b *pixBits) {
	if b == nil || b == emptyBits {
		return
	}
	b.minX, b.minY, b.w, b.h = 0, 0, 0, 0
	b.bits = b.bits[:0]
	bitsPool <- b
}

func (b *pixBits) load(island []pix) {
	r := islandRect(island)
	w, h := r.Dx(), r.Dy()
	n := (w*h + 63) / 64
	if cap(b.bits) < n {
		b.bits = make([]uint64, n)
	} else {
		b.bits = b.bits[:n]
		clear(b.bits)
	}
	b.minX, b.minY, b.w, b.h = r.Min.X, r.Min.Y, w, h
	for _, p := range island {
		x, y := p.x-b.minX, p.y-b.minY
		i := y*w + x
		b.bits[i>>6] |= 1 << uint(i&63)
	}
}

func (b *pixBits) has(p pix) bool {
	if b == nil {
		return false
	}
	x, y := p.x-b.minX, p.y-b.minY
	if uint(x) >= uint(b.w) || uint(y) >= uint(b.h) {
		return false
	}
	i := y*b.w + x
	return b.bits[i>>6]&(1<<uint(i&63)) != 0
}

func coarse(c color.NRGBA) int {
	if c.A == 0 {
		return -1
	}
	h, s, v := loss.HSV(c)
	vb := int(v * 4)
	if vb > 3 {
		vb = 3
	}
	if s < 0.08 {
		return vb
	}
	return 4 + vb*12 + int(h/30)%12
}

func paperLeftover(col color.NRGBA) bool {
	return loss.ColorAt(col, paper) <= minErr
}

func ownsAny(owner []uint16, island []pix, w int, id uint16) bool {
	for _, p := range island {
		if owner[p.y*w+p.x] == id {
			return true
		}
	}
	return false
}

func claim(owner []uint16, island []pix, w int, id uint16) {
	for _, p := range island {
		i := p.y*w + p.x
		if owner[i] == 0 || owner[i] <= id {
			owner[i] = id
		}
	}
}

func clearOwner(owner []uint16, id uint16) {
	for i, v := range owner {
		if v == id {
			owner[i] = 0
		}
	}
}

type scratch struct {
	mark, seen []byte
	family     []int
	buckets    [][]pix
	work       []pix
}

func (s *scratch) ensure(n int) {
	if cap(s.mark) < n {
		s.mark = make([]byte, n)
		s.seen = make([]byte, n)
		s.family = make([]int, n)
		return
	}
	s.mark = s.mark[:n]
	s.seen = s.seen[:n]
	s.family = s.family[:n]
	clear(s.mark)
}

func ownedUnion(owner []uint16, island []pix, w, h int, id uint16, seen []byte) []pix {
	if len(seen) < w*h {
		seen = make([]byte, w*h)
	}
	var st, out []pix
	for _, p := range island {
		i := p.y*w + p.x
		if seen[i] != 0 {
			continue
		}
		seen[i] = 1
		st = append(st, p)
	}
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(st) > 0 {
		p := st[len(st)-1]
		st = st[:len(st)-1]
		out = append(out, p)
		for _, d := range dirs {
			q := pix{p.x + d.x, p.y + d.y}
			if q.x < 0 || q.y < 0 || q.x >= w || q.y >= h {
				continue
			}
			i := q.y*w + q.x
			if seen[i] != 0 {
				continue
			}
			if owner[i] != id {
				continue
			}
			seen[i] = 1
			st = append(st, q)
		}
	}
	for _, p := range out {
		seen[p.y*w+p.x] = 0
	}
	return out
}

func ownedMinus(owner []uint16, drop []pix, w int, id uint16, seen []byte) []pix {
	if len(seen) < len(owner) {
		seen = make([]byte, len(owner))
	}
	for _, p := range drop {
		seen[p.y*w+p.x] = 2
	}
	var out []pix
	for i, v := range owner {
		if v != id || seen[i] == 2 {
			continue
		}
		out = append(out, pix{i % w, i / w})
	}
	for _, p := range drop {
		seen[p.y*w+p.x] = 0
	}
	return out
}

func residual(got, want *image.NRGBA, skip []byte, x, y, w int) bool {
	if skip != nil && skip[y*w+x] != 0 {
		return false
	}
	q := want.NRGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
	g := got.NRGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
	return colorErr(g, q) > minErr
}

// leftoverHeat is HSV error scaled to the current max, 0–1.
// hottest leftover and debug heat keep pixels > 1/2 so a close
// tint still outlines when it is the remaining miss. Score still
// uses raw errAtHSV.
func leftoverHeat(got, want *loss.Plane, skip []byte, w, h int) []float64 {
	field := make([]float64, w*h)
	var maxE float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if skip != nil && skip[y*w+x] != 0 {
				continue
			}
			e := colorErrHSV(got.At(x, y), want.At(x, y))
			field[y*w+x] = e
			if e > maxE {
				maxE = e
			}
		}
	}
	if maxE <= 0 {
		return field
	}
	inv := 1 / maxE
	for i, e := range field {
		field[i] = e * inv
	}
	return field
}

type leftoverBlob struct {
	col    color.NRGBA
	island []pix
	errSum float64
}

// hottest is the leftover blob Score would miss the most:
// same-coarse 4-connected leftover, ranked by ΣerrAt. Pixel count
// preferred a huge mild rim over a small full miss.
func (s *world) hottest() (color.NRGBA, []pix) {
	top := s.hottestN(1)
	if len(top) == 0 {
		return color.NRGBA{}, nil
	}
	return top[0].col, top[0].island
}

func (s *world) hottestN(k int) []leftoverBlob {
	if k <= 0 || s.want == nil {
		return nil
	}
	got, want := s.got, s.want
	b := want.Bounds()
	w, h := b.Dx(), b.Dy()
	s.scratch.ensure(w * h)
	gotP, wantP := s.gotP, s.wantP
	if gotP == nil {
		gotP = loss.NewPlane(got)
		s.gotP = gotP
	}
	if wantP == nil {
		wantP = loss.NewPlane(want)
		s.wantP = wantP
	}
	gotP.Ensure()
	wantP.Ensure()
	heat := leftoverHeat(gotP, wantP, s.skip, w, h)
	var maxH float64
	for _, v := range heat {
		if v > maxH {
			maxH = v
		}
	}
	if maxH <= 0 {
		return nil
	}
	for cut := 0.5; ; cut /= 2 {
		s.stampHeat(heat, w, h, b, cut)
		despeckle(s.scratch.mark, w, h)
		for step := 0; ; step++ {
			blobs := s.floodBlobs(w, h, gotP, wantP, k, true, minIsland)
			if len(blobs) > 0 {
				return blobs
			}
			s.unfloodMark()
			if step >= w+h || !s.dilateIntoHeat(heat, w, h) {
				break
			}
		}
		if cut < 1.0/64 {
			break
		}
	}
	s.stampHeat(heat, w, h, b, 0)
	return s.floodBlobs(w, h, gotP, wantP, k, false, 1)
}

func (s *world) stampHeat(heat []float64, w, h int, b image.Rectangle, cut float64) {
	want := s.want
	mark, family := s.scratch.mark, s.scratch.family
	clear(mark)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if heat[y*w+x] <= cut {
				continue
			}
			mark[y*w+x] = 1
			family[y*w+x] = coarse(want.NRGBAAt(b.Min.X+x, b.Min.Y+y))
		}
	}
}

func (s *world) unfloodMark() {
	for i, v := range s.scratch.mark {
		if v == 2 {
			s.scratch.mark[i] = 1
		}
	}
}

// dilateIntoHeat grows the mask into residual heat (AA halo),
// not into a full match.
func (s *world) dilateIntoHeat(heat []float64, w, h int) bool {
	mark, seen, family := s.scratch.mark, s.scratch.seen, s.scratch.family
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	grew := false
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] == 0 {
				continue
			}
			for _, d := range dirs {
				nx, ny := x+d.x, y+d.y
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				i := ny*w + nx
				if mark[i] != 0 || seen[i] != 0 || heat[i] <= 0 {
					continue
				}
				seen[i] = 1
				family[i] = family[y*w+x]
				grew = true
			}
		}
	}
	if !grew {
		return false
	}
	for i, v := range seen {
		if v == 0 {
			continue
		}
		seen[i] = 0
		mark[i] = 1
	}
	return true
}

func (s *world) floodBlobs(w, h int, gotP, wantP *loss.Plane, k int, interior bool, min int) []leftoverBlob {
	want := s.want
	mark, family := s.scratch.mark, s.scratch.family
	best := make([]leftoverBlob, 0, k)
	var cur []pix
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 1 {
				continue
			}
			cur = cur[:0]
			bin := family[y*w+x]
			var errSum float64
			pending := []pix{{x, y}}
			mark[y*w+x] = 2
			for len(pending) > 0 {
				p := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				cur = append(cur, p)
				errSum += errAtHSV(gotP.At(p.x, p.y), wantP.At(p.x, p.y))
				for _, d := range dirs {
					nx, ny := p.x+d.x, p.y+d.y
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if mark[ny*w+nx] != 1 || family[ny*w+nx] != bin {
						continue
					}
					mark[ny*w+nx] = 2
					pending = append(pending, pix{nx, ny})
				}
			}
			if interior && !hasInterior(cur) {
				continue
			}
			best = rankBlob(best, k, leftoverBlob{
				col:    modeFill(want, cur),
				island: append([]pix{}, cur...),
				errSum: errSum,
			}, min)
		}
	}
	return best
}

func rankBlob(best []leftoverBlob, k int, b leftoverBlob, min int) []leftoverBlob {
	if len(b.island) < min {
		return best
	}
	pos := len(best)
	for i := range best {
		if b.errSum > best[i].errSum {
			pos = i
			break
		}
	}
	if pos == k {
		return best
	}
	if len(best) < k {
		best = append(best, leftoverBlob{})
	}
	copy(best[pos+1:], best[pos:])
	best[pos] = b
	return best
}

// hasInterior is true when some pixel has all four neighbors in the
// island. A one-pixel AA rim has a huge bbox but no interior; Cover
// bikesheds those and hides area leftovers (a folder strip, a frame).
func hasInterior(island []pix) bool {
	if len(island) < 5 {
		return false
	}
	set := pixSet(island)
	defer releaseBits(set)
	for _, p := range island {
		if set.has(pix{p.x - 1, p.y}) && set.has(pix{p.x + 1, p.y}) && set.has(pix{p.x, p.y - 1}) && set.has(pix{p.x, p.y + 1}) {
			return true
		}
	}
	return false
}

func despeckle(mark []byte, w, h int) {
	drop := make([]int, 0, 32)
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 1 {
				continue
			}
			n := 0
			for _, d := range dirs {
				nx, ny := x+d.x, y+d.y
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				if mark[ny*w+nx] == 1 {
					n++
				}
			}
			if n < 2 {
				drop = append(drop, y*w+x)
			}
		}
	}
	for _, i := range drop {
		mark[i] = 0
	}
}

func modeFill(want *image.NRGBA, island []pix) color.NRGBA {
	if len(island) == 0 {
		return color.NRGBA{}
	}
	counts := make(map[color.NRGBA]int, 8)
	var best color.NRGBA
	bestN := -1
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		if c.A < 128 {
			continue
		}
		n := counts[c] + 1
		counts[c] = n
		if n > bestN {
			best, bestN = c, n
		}
	}
	if bestN < 0 {
		return paper
	}
	best.A = 255
	return best
}

func meanFill(want *image.NRGBA, island []pix) color.NRGBA {
	if len(island) == 0 {
		return color.NRGBA{}
	}
	var sr, sg, sb, sa int
	for _, p := range island {
		c := want.NRGBAAt(want.Rect.Min.X+p.x, want.Rect.Min.Y+p.y)
		sr += int(c.R)
		sg += int(c.G)
		sb += int(c.B)
		sa += int(c.A)
	}
	n := len(island)
	if sa/n < 128 {
		return paper
	}
	return color.NRGBA{R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n), A: 255}
}

func bbox(island []pix) [][2]float64 {
	minX, minY := island[0].x, island[0].y
	maxX, maxY := minX+1, minY+1
	for _, p := range island {
		if p.x < minX {
			minX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.x+1 > maxX {
			maxX = p.x + 1
		}
		if p.y+1 > maxY {
			maxY = p.y + 1
		}
	}
	return [][2]float64{
		{float64(minX), float64(minY)},
		{float64(maxX), float64(minY)},
		{float64(maxX), float64(maxY)},
		{float64(minX), float64(maxY)},
	}
}

// voids are enclosed non-island pockets (4-connected), not the exterior.
func voids(island []pix) [][]pix {
	if len(island) == 0 {
		return nil
	}
	minX, minY := island[0].x, island[0].y
	maxX, maxY := minX, minY
	for _, p := range island {
		if p.x < minX {
			minX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y > maxY {
			maxY = p.y
		}
	}
	minX--
	minY--
	maxX += 2
	maxY += 2
	w, h := maxX-minX, maxY-minY
	mark := make([]byte, w*h)
	for _, p := range island {
		mark[(p.y-minY)*w+(p.x-minX)] = 1
	}
	dirs := [4]pix{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	stack := []pix{{0, 0}}
	mark[0] = 2
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, d := range dirs {
			nx, ny := p.x+d.x, p.y+d.y
			if nx < 0 || ny < 0 || nx >= w || ny >= h || mark[ny*w+nx] != 0 {
				continue
			}
			mark[ny*w+nx] = 2
			stack = append(stack, pix{nx, ny})
		}
	}
	var holes [][]pix
	var cur []pix
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mark[y*w+x] != 0 {
				continue
			}
			cur = cur[:0]
			stack = []pix{{x, y}}
			mark[y*w+x] = 3
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cur = append(cur, pix{p.x + minX, p.y + minY})
				for _, d := range dirs {
					nx, ny := p.x+d.x, p.y+d.y
					if nx < 0 || ny < 0 || nx >= w || ny >= h || mark[ny*w+nx] != 0 {
						continue
					}
					mark[ny*w+nx] = 3
					stack = append(stack, pix{nx, ny})
				}
			}
			if len(cur) >= minIsland && !thinIsland(cur) {
				holes = append(holes, append([]pix{}, cur...))
			}
		}
	}
	return holes
}
