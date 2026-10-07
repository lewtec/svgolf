package stack

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/lewtec/lewkit/x/driver"
	_ "github.com/lewtec/lewkit/x/driver/ndeval"
	"github.com/lewtec/lewkit/x/ndarray"
)

// gpuHold is the one concrete type stored in gpuEval. atomic.Value
// panics on a nil interface and on a second concrete type.
type gpuHold struct{ ev ndarray.Evaluator }

// gpuEval is set when ndarray.Open returns a device evaluator.
// The CPU tape interprets every pixel and loses to the Go loop, so a
// dry machine leaves this empty and scoring stays in float64.
// A cancelled Open is not remembered; the next live parent tries again.
// A cancelled Eval does not set gpuOff.
var (
	gpuEval  atomic.Value // *gpuHold
	gpuTried atomic.Bool
	gpuOff   atomic.Bool
	gpuMu    sync.Mutex
	gpuTape  *errTape
)

// OpenDriver opens the ndarray evaluator and logs its device once.
// The server calls this before it listens. Search calls it again,
// and that second call reuses the same device without another line.
func OpenDriver(ctx context.Context) {
	_ = gpuEvaluator(ctx)
}

func gpuEvaluator(ctx context.Context) ndarray.Evaluator {
	if ctx == nil || ctx.Err() != nil || gpuOff.Load() {
		return nil
	}
	if ev := loadGPU(); ev != nil {
		return ev
	}
	if gpuTried.Load() {
		return nil
	}
	gpuMu.Lock()
	defer gpuMu.Unlock()
	if gpuOff.Load() {
		return nil
	}
	if ev := loadGPU(); ev != nil {
		return ev
	}
	if gpuTried.Load() {
		return nil
	}
	if err := applyLatencyWeights(); err != nil {
		slog.Info("ndarray driver", "prefer", "integrated", "err", err)
	}
	ev, err := ndarray.Open(ctx)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	gpuTried.Store(true)
	if err != nil || evaluatorLabel(ev) == "cpu" {
		logNDDriver(ev, err)
		return nil
	}
	logNDDriver(ev, nil)
	gpuEval.Store(&gpuHold{ev: ev})
	return ev
}

// evaluatorLabel is the driver name from Open. Metal and Vulkan include
// the device, as in "metal:Apple M1" or "vulkan:NVIDIA GeForce RTX 4090".
func evaluatorLabel(ev ndarray.Evaluator) string {
	if ev == nil || ev == ndarray.CPU {
		return "cpu"
	}
	if named, ok := ev.(interface{ Name() string }); ok {
		if name := named.Name(); name != "" {
			return name
		}
	}
	return fmt.Sprintf("%T", ev)
}

// logNDDriver records the driver once, when Open commits. A later score
// reuses that choice and stays quiet.
func logNDDriver(ev ndarray.Evaluator, err error) {
	name := evaluatorLabel(ev)
	if err != nil {
		slog.Info("ndarray driver", "name", name, "err", err, "score", "float64")
		return
	}
	if name == "cpu" {
		slog.Info("ndarray driver", "name", name, "score", "float64")
		return
	}
	slog.Info("ndarray driver", "name", name, "score", "device", "prefer", "integrated")
}

// applyLatencyWeights keeps every factory at its own weight and raises
// Vulkan's integrated devices above dedicated ones. A score kernel
// returns before a discrete GPU is busy, so the closer device wins
// when both are listed. A machine with only a dedicated GPU still uses it.
func applyLatencyWeights() error {
	return driver.SetWeights(latencyWeightTable())
}

func latencyWeightTable() map[string]map[string]int {
	table := map[string]map[string]int{}
	for ifaceType, factories := range driver.Drivers {
		slot := make(map[string]int, len(factories)+1)
		for id, factory := range factories {
			slot[id] = clampedWeight(factory)
		}
		if _, ok := slot["vulkan"]; ok && ifaceType.Name() == "Device" {
			slot["vulkan:integrated"] = 80
		}
		table[ifaceName(ifaceType)] = slot
	}
	return table
}

func ifaceName(t reflect.Type) string {
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.String()
}

func clampedWeight(factory any) int {
	w, ok := factory.(driver.Weighter)
	if !ok {
		return 0
	}
	n := w.Weight()
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

func loadGPU() ndarray.Evaluator {
	h, _ := gpuEval.Load().(*gpuHold)
	if h == nil {
		return nil
	}
	return h.ev
}

// noteDeviceErr turns the device off after a real failure. Cancel
// and a missing parent leave it alone so the next score can try again.
func noteDeviceErr(ctx context.Context, err error) {
	if err == nil || ctx == nil || ctx.Err() != nil {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	gpuOff.Store(true)
}

// errTape is one device kernel pair over packed RGBA.
// Score squares HSV distance. The leftover mask keeps the distance
// and writes a 0/1 band. Both read the same pixmap leaves.
type errTape struct {
	cap        int
	got, want  *ndarray.Tensor[uint8]
	lo, hi     *ndarray.Tensor[float32]
	err        *ndarray.Tensor[float32]
	mark       *ndarray.Tensor[uint8]
	dst        []float32
	mask       []uint8
	wantKey    pixKey
	wantPacked bool
}

const saturationFloor float32 = 0.08

func pow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func gpuTapeFor(n int) (*errTape, error) {
	if gpuTape != nil && gpuTape.cap >= n {
		if err := gpuTape.err.Resize(ndarray.Shape{n}); err != nil {
			return nil, err
		}
		return gpuTape, nil
	}
	tape, err := newErrTape(pow2(n))
	if err != nil {
		return nil, err
	}
	if err := tape.err.Resize(ndarray.Shape{n}); err != nil {
		return nil, err
	}
	gpuTape = tape
	return tape, nil
}

func newErrTape(n int) (*errTape, error) {
	shape := ndarray.Shape{n, 4}
	plane := func() (*ndarray.Tensor[uint8], error) {
		return ndarray.New(make([]uint8, n*4), shape)
	}
	tape := &errTape{cap: n, dst: make([]float32, n), mask: make([]uint8, n)}
	var err error
	if tape.got, err = plane(); err != nil {
		return nil, err
	}
	if tape.want, err = plane(); err != nil {
		return nil, err
	}
	if tape.lo, err = ndarray.New([]float32{0}, nil); err != nil {
		return nil, err
	}
	if tape.hi, err = ndarray.New([]float32{0}, nil); err != nil {
		return nil, err
	}
	tape.err = squaredRGBAError(tape.got, tape.want)
	if tape.err == nil {
		return nil, ndarray.ErrOp
	}
	tape.mark = residualMark(tape.got, tape.want, tape.lo, tape.hi)
	if tape.mark == nil {
		return nil, ndarray.ErrOp
	}
	if err := tape.err.Resize(ndarray.Shape{n}); err != nil {
		return nil, err
	}
	if err := tape.mark.Resize(ndarray.Shape{n}); err != nil {
		return nil, err
	}
	return tape, nil
}

func squaredRGBAError(got, want *ndarray.Tensor[uint8]) *ndarray.Tensor[float32] {
	dist := rgbaDistance(got, want)
	if dist == nil {
		return nil
	}
	return dist.Mul(dist)
}

func rgbaDistance(got, want *ndarray.Tensor[uint8]) *ndarray.Tensor[float32] {
	gh, gs, gv, ga, ok := hsvRGBA(got)
	if !ok {
		return nil
	}
	wh, ws, wv, wa, ok := hsvRGBA(want)
	if !ok {
		return nil
	}
	ph, ps, pv, ok := hsvFloat(
		ndarray.Const(float32(1)),
		ndarray.Const(float32(1)),
		ndarray.Const(float32(1)),
	)
	if !ok {
		return nil
	}
	dist := hsvDistance(gh, gs, gv, wh, ws, wv)
	paper := hsvDistance(gh, gs, gv, ph, ps, pv)
	zero := ndarray.Const(float32(0))
	miss := ndarray.Const(float32(180))
	err := wa.Equal(zero).Where(paper, dist)
	return ga.Equal(zero).Where(miss, err)
}

func residualMark(got, want *ndarray.Tensor[uint8], lo, hi *ndarray.Tensor[float32]) *ndarray.Tensor[uint8] {
	dist := rgbaDistance(got, want)
	if dist == nil {
		return nil
	}
	above := lo.CmpLt(dist)
	notPast := hi.CmpLt(dist).CmpNe(ndarray.Const(int32(1)))
	return above.And(notPast).Where(ndarray.Const(uint8(1)), ndarray.Const(uint8(0)))
}

func hsvRGBA(px *ndarray.Tensor[uint8]) (h, s, v, a *ndarray.Tensor[float32], ok bool) {
	r, g, b, a, ok := rgbaAxes(px)
	if !ok {
		return nil, nil, nil, nil, false
	}
	h, s, v, ok = hsvFloat(r, g, b)
	if !ok {
		return nil, nil, nil, nil, false
	}
	return h, s, v, a, true
}

func rgbaAxes(px *ndarray.Tensor[uint8]) (r, g, b, a *ndarray.Tensor[float32], ok bool) {
	if px == nil || len(px.Shape()) != 2 || px.Shape()[1] != 4 {
		return nil, nil, nil, nil, false
	}
	n := px.Shape()[0]
	scale := ndarray.Const(float32(1) / 255)
	take := func(channel int, scaled bool) (*ndarray.Tensor[float32], bool) {
		sl, err := px.Shrink([][2]int{{0, n}, {channel, channel + 1}})
		if err != nil {
			return nil, false
		}
		flat, err := sl.Reshape(ndarray.Shape{n})
		if err != nil {
			return nil, false
		}
		f := flat.Cast[float32]()
		if scaled {
			f = f.Mul(scale)
		}
		return f, true
	}
	var good bool
	if r, good = take(0, true); !good {
		return nil, nil, nil, nil, false
	}
	if g, good = take(1, true); !good {
		return nil, nil, nil, nil, false
	}
	if b, good = take(2, true); !good {
		return nil, nil, nil, nil, false
	}
	if a, good = take(3, false); !good {
		return nil, nil, nil, nil, false
	}
	return r, g, b, a, true
}

func hsvFloat(r, g, b *ndarray.Tensor[float32]) (h, s, v *ndarray.Tensor[float32], ok bool) {
	if r == nil || g == nil || b == nil {
		return nil, nil, nil, false
	}
	zero := ndarray.Const(float32(0))
	v = r.Max(g.Max(b))
	delta := v.Add(min32(r, min32(g, b)).Neg())
	s = v.Equal(zero).Where(zero, delta.Mul(guardedRecip(v)))
	scale := guardedRecip(delta)
	sixty := ndarray.Const(float32(60))
	full := ndarray.Const(float32(360))
	hr := g.Add(b.Neg()).Mul(sixty).Mul(scale)
	hr = hr.CmpLt(zero).Where(hr.Add(full), hr)
	hg := b.Add(r.Neg()).Mul(sixty).Mul(scale).Add(ndarray.Const(float32(120)))
	hb := r.Add(g.Neg()).Mul(sixty).Mul(scale).Add(ndarray.Const(float32(240)))
	h = v.Equal(r).Where(hr, v.Equal(g).Where(hg, hb))
	h = delta.Equal(zero).Where(zero, h)
	return h, s, v, true
}

// guardedRecip is 1/x, and 0 where x is 0, so a later multiply stays finite.
func guardedRecip(v *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	zero := ndarray.Const(float32(0))
	return v.Equal(zero).Where(zero, v.Reciprocal())
}

func hsvDistance(h1, s1, v1, h2, s2, v2 *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	scale := ndarray.Const(float32(180))
	dv := abs32(v1.Add(v2.Neg())).Mul(scale)
	ds := abs32(s1.Add(s2.Neg())).Mul(scale)
	colored := hueDistance(h1, h2).Max(ds).Max(dv)
	low := ndarray.Const(saturationFloor)
	low1 := s1.CmpLt(low)
	low2 := s2.CmpLt(low)
	either := low1.Or(low2).Where(scale, colored)
	return low1.And(low2).Where(dv, either)
}

func hueDistance(a, b *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	d := abs32(a.Add(b.Neg()))
	turn := ndarray.Const(float32(180))
	full := ndarray.Const(float32(360))
	return d.CmpLt(turn).Where(d, full.Add(d.Neg()))
}

func abs32(v *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	return v.Max(v.Neg())
}

func min32(a, b *ndarray.Tensor[float32]) *ndarray.Tensor[float32] {
	return a.Neg().Max(b.Neg()).Neg()
}

// pixKey identifies one packed rectangle of an image. The target pixmap
// is not rewritten, so a repeated full-frame score can keep those bytes.
type pixKey struct {
	ptr        uintptr
	n, stride  int
	x, y, w, h int
}

func pixKeyOf(img *image.NRGBA, r image.Rectangle) pixKey {
	if img == nil || len(img.Pix) == 0 {
		return pixKey{}
	}
	return pixKey{
		ptr:    uintptr(unsafe.Pointer(&img.Pix[0])),
		n:      len(img.Pix),
		stride: img.Stride,
		x:      r.Min.X,
		y:      r.Min.Y,
		w:      r.Dx(),
		h:      r.Dy(),
	}
}

func packImage(dst []uint8, img *image.NRGBA, r image.Rectangle) {
	if img == nil {
		return
	}
	r = r.Intersect(img.Rect)
	width := r.Dx()
	rowBytes := width * 4
	di := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := img.PixOffset(r.Min.X, y)
		copy(dst[di:di+rowBytes], img.Pix[so:so+rowBytes])
		di += rowBytes
	}
}

func (tape *errTape) load(got, want *image.NRGBA, r image.Rectangle) {
	packImage(tape.got.Buffer(), got, r)
	key := pixKeyOf(want, r)
	if tape.wantPacked && tape.wantKey == key {
		return
	}
	packImage(tape.want.Buffer(), want, r)
	tape.wantKey = key
	tape.wantPacked = true
}

func (tape *errTape) eval(ctx context.Context, ev ndarray.Evaluator, n int) (float64, error) {
	if err := tape.err.Eval(ctx, ev, tape.dst[:n]); err != nil {
		return 0, err
	}
	var sum float64
	for _, v := range tape.dst[:n] {
		sum += float64(v)
	}
	return sum, nil
}

func ndScoreImages(ctx context.Context, got, want *image.NRGBA) (float64, bool) {
	ev := gpuEvaluator(ctx)
	if ev == nil || got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return 0, false
	}
	n := got.Rect.Dx() * got.Rect.Dy()
	if n == 0 {
		return 0, true
	}
	gpuMu.Lock()
	defer gpuMu.Unlock()
	return ndScoreLocked(ctx, ev, got, want, got.Rect, n)
}

func ndScoreImageRect(ctx context.Context, got, want *image.NRGBA, r image.Rectangle) (float64, bool) {
	ev := gpuEvaluator(ctx)
	if ev == nil || got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return 0, false
	}
	r = r.Intersect(want.Rect)
	if r.Empty() {
		return 0, true
	}
	n := r.Dx() * r.Dy()
	gpuMu.Lock()
	defer gpuMu.Unlock()
	return ndScoreLocked(ctx, ev, got, want, r, n)
}

func ndScoreLocked(ctx context.Context, ev ndarray.Evaluator, got, want *image.NRGBA, r image.Rectangle, n int) (float64, bool) {
	tape, err := gpuTapeFor(n)
	if err != nil {
		noteDeviceErr(ctx, err)
		return 0, false
	}
	tape.load(got, want, r)
	sum, err := tape.eval(ctx, ev, n)
	if err != nil {
		noteDeviceErr(ctx, err)
		return 0, false
	}
	return sum, true
}

// ndStamp fills mark with the (lo, hi] HSV band and family with the
// coarse want color of each marked pixel. ok is false when the device
// path is closed; the caller keeps the float64 walk.
func ndStamp(ctx context.Context, got, want *image.NRGBA, lo, hi float64, mark []byte, family []int) (ok, any bool) {
	ev := gpuEvaluator(ctx)
	if ev == nil || got == nil || want == nil || !got.Rect.Eq(want.Rect) {
		return false, false
	}
	b := want.Rect
	n := b.Dx() * b.Dy()
	if n == 0 || len(mark) < n || len(family) < n {
		return false, false
	}
	gpuMu.Lock()
	tape, err := gpuTapeFor(n)
	if err != nil {
		noteDeviceErr(ctx, err)
		gpuMu.Unlock()
		return false, false
	}
	if err := tape.mark.Resize(ndarray.Shape{n}); err != nil {
		noteDeviceErr(ctx, err)
		gpuMu.Unlock()
		return false, false
	}
	tape.load(got, want, b)
	if buf := tape.lo.Buffer(); len(buf) > 0 {
		buf[0] = float32(lo)
	}
	if buf := tape.hi.Buffer(); len(buf) > 0 {
		buf[0] = float32(hi)
	}
	if err := tape.mark.Eval(ctx, ev, tape.mask[:n]); err != nil {
		noteDeviceErr(ctx, err)
		gpuMu.Unlock()
		return false, false
	}
	copy(mark[:n], tape.mask[:n])
	gpuMu.Unlock()
	w := b.Dx()
	for i := range n {
		if mark[i] == 0 {
			continue
		}
		y := i / w
		x := i - y*w
		family[i] = coarse(want.NRGBAAt(b.Min.X+x, b.Min.Y+y))
		any = true
	}
	return true, any
}
