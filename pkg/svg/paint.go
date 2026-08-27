package svg

import "image/color"

type FillRule int

const (
	FillNonZero FillRule = iota
	FillEvenOdd
)

type LineCap int

const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

type LineJoin int

const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

type optionalF64 struct {
	v   float64
	set bool
}

func (o optionalF64) or(def float64) float64 {
	if !o.set {
		return def
	}
	return o.v
}

type optionalU8 struct {
	v   uint8
	set bool
}

func (o optionalU8) or(def uint8) uint8 {
	if !o.set {
		return def
	}
	return o.v
}

func clamp01(a float64) float64 {
	if a < 0 {
		return 0
	}
	if a > 1 {
		return 1
	}
	return a
}

func op8FromUnit(a float64) uint8 {
	return uint8(clamp01(a)*255 + 0.5)
}

type paint struct {
	fr, fg, fb uint8
	fop        optionalU8
	fillNone   bool
	fillRule   FillRule
	strokeOn   bool
	st         Stroke
}

func (p paint) withFill(col color.NRGBA) paint {
	p.fr, p.fg, p.fb = col.R, col.G, col.B
	p.fop = optionalU8{v: col.A, set: true}
	p.fillNone = false
	return p
}

func (p paint) withFillOpacity(a float64) paint {
	p.fop = optionalU8{v: op8FromUnit(a), set: true}
	return p
}

func (p paint) withFillNone() paint {
	p.fillNone = true
	return p
}

func (p paint) withFillRule(r FillRule) paint {
	p.fillRule = r
	return p
}

func (p paint) withStroke(s Stroke) paint {
	p.strokeOn = true
	p.st = s
	return p
}

func (p paint) withoutStroke() paint {
	p.strokeOn = false
	p.st = Stroke{}
	return p
}

func (p paint) fill() (color.NRGBA, bool) {
	if p.fillNone {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: p.fr, G: p.fg, B: p.fb, A: 255}, true
}

func (p paint) fillOpacity() float64 {
	return float64(p.fop.or(255)) / 255
}

func (p paint) stroke() (Stroke, bool) {
	if !p.strokeOn {
		return Stroke{}, false
	}
	return p.st, true
}

// Stroke is outline paint. Zero value is SVG default stroke paint
// (black, width 1, opacity 1, butt, miter, miterlimit 4), not “none”.
// Presence of a stroke on a shape is strokeOn, not this type.
type Stroke struct {
	r, g, b uint8
	op      optionalU8
	width   optionalF64
	cap     LineCap
	join    LineJoin
	miter   optionalF64
}

func NewStroke() Stroke {
	return Stroke{}
}

func (s Stroke) WithColor(col color.NRGBA) Stroke {
	s.r, s.g, s.b = col.R, col.G, col.B
	s.op = optionalU8{v: col.A, set: true}
	return s
}

func (s Stroke) WithOpacity(a float64) Stroke {
	s.op = optionalU8{v: op8FromUnit(a), set: true}
	return s
}

func (s Stroke) WithWidth(w float64) Stroke {
	s.width = optionalF64{v: w, set: true}
	return s
}

func (s Stroke) WithCap(c LineCap) Stroke {
	s.cap = c
	return s
}

func (s Stroke) WithJoin(j LineJoin) Stroke {
	s.join = j
	return s
}

func (s Stroke) WithMiterLimit(m float64) Stroke {
	s.miter = optionalF64{v: m, set: true}
	return s
}

func (s Stroke) Color() color.NRGBA {
	return color.NRGBA{R: s.r, G: s.g, B: s.b, A: 255}
}

func (s Stroke) Opacity() float64 {
	return float64(s.op.or(255)) / 255
}

func (s Stroke) Width() float64 {
	return s.width.or(1)
}

func (s Stroke) Cap() LineCap {
	return s.cap
}

func (s Stroke) Join() LineJoin {
	return s.join
}

func (s Stroke) MiterLimit() float64 {
	return s.miter.or(4)
}
