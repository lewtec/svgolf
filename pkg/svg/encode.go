package svg

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	svgNS       = "http://www.w3.org/2000/svg"
	maxCanvas   = 4096
	maxChildren = 4096
	xmlDecl     = `<?xml version="1.0" encoding="UTF-8"?>`
)

func EncodeToString(d Document) (string, error) {
	var b strings.Builder
	if err := Encode(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func Encode(w io.Writer, d Document) error {
	if err := checkDocSize(d.width, d.height); err != nil {
		return err
	}
	if err := checkChildCount(len(d.children)); err != nil {
		return err
	}
	e := &encoder{w: w}
	e.str(xmlDecl)
	e.str("\n")
	e.str(`<svg xmlns="`)
	e.str(svgNS)
	e.str(`"`)
	e.attr("width", fmtNum(d.width))
	e.attr("height", fmtNum(d.height))
	if d.vb.ok {
		e.attr("viewBox", fmtNum(d.vb.minX)+" "+fmtNum(d.vb.minY)+" "+fmtNum(d.vb.w)+" "+fmtNum(d.vb.h))
	}
	if len(d.children) == 0 {
		e.str("/>\n")
		return e.err
	}
	e.str(">\n")
	for _, n := range d.children {
		if err := e.node(n, 1); err != nil {
			return err
		}
	}
	e.str("</svg>\n")
	return e.err
}

type encoder struct {
	w   io.Writer
	err error
}

func (e *encoder) str(s string) {
	if e.err != nil {
		return
	}
	_, e.err = io.WriteString(e.w, s)
}

func (e *encoder) attr(name, val string) {
	e.str(" ")
	e.str(name)
	e.str(`="`)
	e.str(val)
	e.str(`"`)
}

func (e *encoder) indent(level int) {
	e.str(strings.Repeat("  ", level))
}

func (e *encoder) node(n Node, level int) error {
	switch n.kind {
	case KindGroup:
		return e.group(n.group, level)
	case KindCircle:
		return e.circle(n.circle, level)
	case KindEllipse:
		return e.ellipse(n.ellipse, level)
	case KindRect:
		return e.rect(n.rect, level)
	case KindPolygon:
		return e.polygon(n.polygon, level)
	default:
		return fmt.Errorf("encode: invalid node")
	}
}

func (e *encoder) group(g Group, level int) error {
	if err := checkChildCount(len(g.children)); err != nil {
		return err
	}
	e.indent(level)
	e.str("<g")
	if len(g.children) == 0 {
		e.str("/>\n")
		return e.err
	}
	e.str(">\n")
	for _, n := range g.children {
		if err := e.node(n, level+1); err != nil {
			return err
		}
	}
	e.indent(level)
	e.str("</g>\n")
	return e.err
}

func (e *encoder) circle(c Circle, level int) error {
	if err := checkNonNeg("r", c.r); err != nil {
		return err
	}
	e.indent(level)
	e.str("<circle")
	e.numAttr("cx", c.cx, 0)
	e.numAttr("cy", c.cy, 0)
	e.numAttr("r", c.r, 0)
	e.paint(c.paint)
	e.str("/>\n")
	return e.err
}

func (e *encoder) ellipse(el Ellipse, level int) error {
	if err := checkNonNeg("rx", el.rx); err != nil {
		return err
	}
	if err := checkNonNeg("ry", el.ry); err != nil {
		return err
	}
	e.indent(level)
	e.str("<ellipse")
	e.numAttr("cx", el.cx, 0)
	e.numAttr("cy", el.cy, 0)
	e.numAttr("rx", el.rx, 0)
	e.numAttr("ry", el.ry, 0)
	e.paint(el.paint)
	e.str("/>\n")
	return e.err
}

func (e *encoder) rect(r Rect, level int) error {
	if err := checkNonNeg("width", r.width); err != nil {
		return err
	}
	if err := checkNonNeg("height", r.height); err != nil {
		return err
	}
	if err := checkNonNeg("rx", r.rx); err != nil {
		return err
	}
	if err := checkNonNeg("ry", r.ry); err != nil {
		return err
	}
	e.indent(level)
	e.str("<rect")
	e.numAttr("x", r.x, 0)
	e.numAttr("y", r.y, 0)
	e.numAttr("width", r.width, 0)
	e.numAttr("height", r.height, 0)
	if r.rxSet || r.rySet || r.rx != 0 || r.ry != 0 {
		e.attr("rx", fmtNum(r.rx))
		e.attr("ry", fmtNum(r.ry))
	}
	e.paint(r.paint)
	e.str("/>\n")
	return e.err
}

func (e *encoder) polygon(p Polygon, level int) error {
	if n := len(p.points); n < minPolygonPoints || n > maxPolygonPoints {
		return fmt.Errorf("%w, got %d", ErrPolygonPoints, n)
	}
	e.indent(level)
	e.str("<polygon")
	var b strings.Builder
	for i, pt := range p.points {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(fmtNum(pt[0]))
		b.WriteByte(',')
		b.WriteString(fmtNum(pt[1]))
	}
	e.attr("points", b.String())
	e.paint(p.paint)
	e.str("/>\n")
	return e.err
}

func (e *encoder) numAttr(name string, v, def float64) {
	if v == def {
		return
	}
	e.attr(name, fmtNum(v))
}

func (e *encoder) paint(p paint) {
	if p.fillNone {
		e.attr("fill", "none")
	} else if p.fr != 0 || p.fg != 0 || p.fb != 0 {
		e.attr("fill", hexRGB(p.fr, p.fg, p.fb))
	}
	if op := p.fop.or(255); op != 255 {
		e.attr("fill-opacity", encodeOpacity(op))
	}
	if p.fillRule == FillEvenOdd {
		e.attr("fill-rule", "evenodd")
	}
	if !p.strokeOn {
		return
	}
	e.attr("stroke", hexRGB(p.st.r, p.st.g, p.st.b))
	if op := p.st.op.or(255); op != 255 {
		e.attr("stroke-opacity", encodeOpacity(op))
	}
	if w := p.st.Width(); w != 1 {
		e.attr("stroke-width", fmtNum(w))
	}
	switch p.st.cap {
	case CapRound:
		e.attr("stroke-linecap", "round")
	case CapSquare:
		e.attr("stroke-linecap", "square")
	}
	switch p.st.join {
	case JoinRound:
		e.attr("stroke-linejoin", "round")
	case JoinBevel:
		e.attr("stroke-linejoin", "bevel")
	}
	if m := p.st.MiterLimit(); m != 4 {
		e.attr("stroke-miterlimit", fmtNum(m))
	}
}

func fmtNum(v float64) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func hexRGB(r, g, b uint8) string {
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

func encodeOpacity(u uint8) string {
	if u == 0 {
		return "0"
	}
	if u == 255 {
		return "1"
	}
	unit := float64(u) / 255
	for prec := 1; prec <= 17; prec++ {
		s := strconv.FormatFloat(unit, 'f', prec, 64)
		if op8FromUnit(mustFloat(s)) == u {
			return s
		}
	}
	return strconv.FormatFloat(unit, 'f', -1, 64)
}

func mustFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func checkDocSize(w, h float64) error {
	if err := checkWholeCanvas("width", w); err != nil {
		return err
	}
	return checkWholeCanvas("height", h)
}

func checkWholeCanvas(name string, v float64) error {
	if !isFinite(v) || v <= 0 || v > maxCanvas || v != math.Trunc(v) {
		return fmt.Errorf("document %s %v: must be a whole number in (0, %d]", name, v, maxCanvas)
	}
	return nil
}

func checkNonNeg(name string, v float64) error {
	if !isFinite(v) || v < 0 {
		return fmt.Errorf("%s %v: must be finite and >= 0", name, v)
	}
	return nil
}

func checkChildCount(n int) error {
	if n > maxChildren {
		return fmt.Errorf("too many children: %d (max %d)", n, maxChildren)
	}
	return nil
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
