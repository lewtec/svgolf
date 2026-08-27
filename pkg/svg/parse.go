package svg

import (
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func ParseFile(path string) (Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()
	return Parse(f)
}

func Parse(r io.Reader) (Document, error) {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return Document{}, fmt.Errorf("parse: missing svg element")
		}
		if err != nil {
			return Document{}, fmt.Errorf("parse: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name, err := elemName(t.Name)
			if err != nil {
				return Document{}, err
			}
			if name != "svg" {
				return Document{}, fmt.Errorf("parse: root is %s, want svg", name)
			}
			doc, err := parseSVG(dec, t)
			if err != nil {
				return Document{}, err
			}
			if err := drain(dec); err != nil {
				return Document{}, err
			}
			return doc, nil
		case xml.CharData:
			if !isSpace(t) {
				return Document{}, fmt.Errorf("parse: unexpected text before svg")
			}
		case xml.Comment, xml.ProcInst:
		case xml.Directive:
			return Document{}, fmt.Errorf("parse: unexpected directive")
		default:
			return Document{}, fmt.Errorf("parse: unexpected token %T", tok)
		}
	}
}

func drain(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			if !isSpace(t) {
				return fmt.Errorf("parse: trailing text")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return fmt.Errorf("parse: trailing token after svg")
		}
	}
}

func parseSVG(dec *xml.Decoder, start xml.StartElement) (Document, error) {
	var width, height *float64
	var vb *ViewBox
	for _, a := range start.Attr {
		key, err := attrName(a.Name)
		if err != nil {
			return Document{}, err
		}
		switch key {
		case "xmlns":
			if a.Value != svgNS {
				return Document{}, fmt.Errorf("parse: xmlns %q", a.Value)
			}
		case "width":
			v, err := parseDocLength(a.Value)
			if err != nil {
				return Document{}, err
			}
			width = &v
		case "height":
			v, err := parseDocLength(a.Value)
			if err != nil {
				return Document{}, err
			}
			height = &v
		case "viewBox":
			box, err := parseViewBox(a.Value)
			if err != nil {
				return Document{}, err
			}
			vb = &box
		default:
			return Document{}, fmt.Errorf("parse: unknown attribute %s on svg", key)
		}
	}
	if width == nil || height == nil {
		return Document{}, fmt.Errorf("parse: svg requires width and height")
	}
	if err := checkDocSize(*width, *height); err != nil {
		return Document{}, err
	}
	doc := NewDocument(*width, *height)
	if vb != nil {
		doc = doc.WithViewBox(vb.minX, vb.minY, vb.w, vb.h)
	}
	kids, err := parseChildren(dec, "svg")
	if err != nil {
		return Document{}, err
	}
	return doc.Append(kids...), nil
}

func parseChildren(dec *xml.Decoder, parent string) ([]Node, error) {
	var kids []Node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("parse: unclosed %s", parent)
		}
		if err != nil {
			return nil, fmt.Errorf("parse: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			name, err := elemName(t.Name)
			if err != nil {
				return nil, err
			}
			if name != parent {
				return nil, fmt.Errorf("parse: unexpected </%s> in %s", name, parent)
			}
			if err := checkChildCount(len(kids)); err != nil {
				return nil, err
			}
			return kids, nil
		case xml.StartElement:
			n, err := parseChild(dec, t)
			if err != nil {
				return nil, err
			}
			kids = append(kids, n)
			if err := checkChildCount(len(kids)); err != nil {
				return nil, err
			}
		case xml.CharData:
			if !isSpace(t) {
				return nil, fmt.Errorf("parse: unexpected text in %s", parent)
			}
		case xml.Comment, xml.ProcInst:
		default:
			return nil, fmt.Errorf("parse: unexpected token in %s", parent)
		}
	}
}

func parseChild(dec *xml.Decoder, start xml.StartElement) (Node, error) {
	name, err := elemName(start.Name)
	if err != nil {
		return Node{}, err
	}
	switch name {
	case "g":
		return parseGroup(dec, start)
	case "circle":
		return parseCircle(dec, start)
	case "ellipse":
		return parseEllipse(dec, start)
	case "rect":
		return parseRect(dec, start)
	case "polygon":
		return parsePolygon(dec, start)
	default:
		return Node{}, fmt.Errorf("parse: unknown tag %s", name)
	}
}

func parseGroup(dec *xml.Decoder, start xml.StartElement) (Node, error) {
	if len(start.Attr) > 0 {
		key, err := attrName(start.Attr[0].Name)
		if err != nil {
			return Node{}, err
		}
		return Node{}, fmt.Errorf("parse: unknown attribute %s on g", key)
	}
	kids, err := parseChildren(dec, "g")
	if err != nil {
		return Node{}, err
	}
	return NewGroup().Append(kids...).Node(), nil
}

func parseCircle(dec *xml.Decoder, start xml.StartElement) (Node, error) {
	c := NewCircle()
	var pa paintAttrs
	for _, a := range start.Attr {
		key, err := attrName(a.Name)
		if err != nil {
			return Node{}, err
		}
		switch key {
		case "cx":
			v, err := parseLength(a.Value)
			if err != nil {
				return Node{}, err
			}
			c = c.WithCX(v)
		case "cy":
			v, err := parseLength(a.Value)
			if err != nil {
				return Node{}, err
			}
			c = c.WithCY(v)
		case "r":
			v, err := parseNonNegLength("r", a.Value)
			if err != nil {
				return Node{}, err
			}
			c = c.WithR(v)
		default:
			if !pa.set(key, a.Value) {
				return Node{}, fmt.Errorf("parse: unknown attribute %s on circle", key)
			}
		}
	}
	c, err := applyCirclePaint(c, pa)
	if err != nil {
		return Node{}, err
	}
	if err := skipEmpty(dec, "circle"); err != nil {
		return Node{}, err
	}
	return c.Node(), nil
}

func parseEllipse(dec *xml.Decoder, start xml.StartElement) (Node, error) {
	el := NewEllipse()
	var pa paintAttrs
	for _, a := range start.Attr {
		key, err := attrName(a.Name)
		if err != nil {
			return Node{}, err
		}
		switch key {
		case "cx":
			v, err := parseLength(a.Value)
			if err != nil {
				return Node{}, err
			}
			el = el.WithCX(v)
		case "cy":
			v, err := parseLength(a.Value)
			if err != nil {
				return Node{}, err
			}
			el = el.WithCY(v)
		case "rx":
			v, err := parseNonNegLength("rx", a.Value)
			if err != nil {
				return Node{}, err
			}
			el = el.WithRX(v)
		case "ry":
			v, err := parseNonNegLength("ry", a.Value)
			if err != nil {
				return Node{}, err
			}
			el = el.WithRY(v)
		default:
			if !pa.set(key, a.Value) {
				return Node{}, fmt.Errorf("parse: unknown attribute %s on ellipse", key)
			}
		}
	}
	el, err := applyEllipsePaint(el, pa)
	if err != nil {
		return Node{}, err
	}
	if err := skipEmpty(dec, "ellipse"); err != nil {
		return Node{}, err
	}
	return el.Node(), nil
}

func parseRect(dec *xml.Decoder, start xml.StartElement) (Node, error) {
	r := NewRect()
	var (
		pa     paintAttrs
		rx, ry *float64
	)
	for _, a := range start.Attr {
		key, err := attrName(a.Name)
		if err != nil {
			return Node{}, err
		}
		switch key {
		case "x":
			v, err := parseLength(a.Value)
			if err != nil {
				return Node{}, err
			}
			r = r.WithX(v)
		case "y":
			v, err := parseLength(a.Value)
			if err != nil {
				return Node{}, err
			}
			r = r.WithY(v)
		case "width":
			v, err := parseNonNegLength("width", a.Value)
			if err != nil {
				return Node{}, err
			}
			r = r.WithWidth(v)
		case "height":
			v, err := parseNonNegLength("height", a.Value)
			if err != nil {
				return Node{}, err
			}
			r = r.WithHeight(v)
		case "rx":
			v, err := parseNonNegLength("rx", a.Value)
			if err != nil {
				return Node{}, err
			}
			rx = &v
		case "ry":
			v, err := parseNonNegLength("ry", a.Value)
			if err != nil {
				return Node{}, err
			}
			ry = &v
		default:
			if !pa.set(key, a.Value) {
				return Node{}, fmt.Errorf("parse: unknown attribute %s on rect", key)
			}
		}
	}
	switch {
	case rx != nil && ry != nil:
		r = r.WithRX(*rx).WithRY(*ry)
	case rx != nil:
		r = r.WithRX(*rx).WithRY(*rx)
	case ry != nil:
		r = r.WithRX(*ry).WithRY(*ry)
	}
	r, err := applyRectPaint(r, pa)
	if err != nil {
		return Node{}, err
	}
	if err := skipEmpty(dec, "rect"); err != nil {
		return Node{}, err
	}
	return r.Node(), nil
}

func parsePolygon(dec *xml.Decoder, start xml.StartElement) (Node, error) {
	p := NewPolygon()
	var pa paintAttrs
	var havePts bool
	var pts [][2]float64
	for _, a := range start.Attr {
		key, err := attrName(a.Name)
		if err != nil {
			return Node{}, err
		}
		switch key {
		case "points":
			got, err := parsePoints(a.Value)
			if err != nil {
				return Node{}, err
			}
			pts = got
			havePts = true
		default:
			if !pa.set(key, a.Value) {
				return Node{}, fmt.Errorf("parse: unknown attribute %s on polygon", key)
			}
		}
	}
	if havePts {
		var err error
		p, err = p.WithPoints(pts)
		if err != nil {
			return Node{}, err
		}
	}
	p, err := applyPolygonPaint(p, pa)
	if err != nil {
		return Node{}, err
	}
	if err := skipEmpty(dec, "polygon"); err != nil {
		return Node{}, err
	}
	return p.Node(), nil
}

func skipEmpty(dec *xml.Decoder, tag string) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return fmt.Errorf("parse: unclosed %s", tag)
		}
		if err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		switch t := tok.(type) {
		case xml.EndElement:
			name, err := elemName(t.Name)
			if err != nil {
				return err
			}
			if name != tag {
				return fmt.Errorf("parse: unexpected </%s> in %s", name, tag)
			}
			return nil
		case xml.CharData:
			if !isSpace(t) {
				return fmt.Errorf("parse: unexpected text in %s", tag)
			}
		case xml.Comment, xml.ProcInst:
		case xml.StartElement:
			name, err := elemName(t.Name)
			if err != nil {
				return err
			}
			return fmt.Errorf("parse: unexpected child %s in %s", name, tag)
		default:
			return fmt.Errorf("parse: unexpected token in %s", tag)
		}
	}
}

type paintAttrs struct {
	fill, fillOp, fillRule    *string
	stroke, strokeOp, strokeW *string
	cap, join, miter          *string
	anyStrokeExtra            bool
}

func heapStr(s string) *string { return &s }

func (p *paintAttrs) set(key, val string) bool {
	switch key {
	case "fill":
		p.fill = heapStr(val)
	case "fill-opacity":
		p.fillOp = heapStr(val)
	case "fill-rule":
		p.fillRule = heapStr(val)
	case "stroke":
		p.stroke = heapStr(val)
	case "stroke-opacity":
		p.strokeOp = heapStr(val)
		p.anyStrokeExtra = true
	case "stroke-width":
		p.strokeW = heapStr(val)
		p.anyStrokeExtra = true
	case "stroke-linecap":
		p.cap = heapStr(val)
		p.anyStrokeExtra = true
	case "stroke-linejoin":
		p.join = heapStr(val)
		p.anyStrokeExtra = true
	case "stroke-miterlimit":
		p.miter = heapStr(val)
		p.anyStrokeExtra = true
	default:
		return false
	}
	return true
}

func composeFill(a paintAttrs) (none, apply bool, col color.NRGBA, rule FillRule, err error) {
	if a.fillRule != nil {
		switch *a.fillRule {
		case "nonzero":
			rule = FillNonZero
		case "evenodd":
			rule = FillEvenOdd
		default:
			err = fmt.Errorf("parse: unknown fill-rule %q", *a.fillRule)
			return
		}
	}
	colorA := uint8(255)
	if a.fill != nil {
		var r, g, b, ca uint8
		var isNone bool
		r, g, b, ca, isNone, err = parseColor(*a.fill)
		if err != nil {
			return
		}
		if isNone {
			none = true
			return
		}
		col.R, col.G, col.B = r, g, b
		colorA = ca
		apply = true
	}
	attrA := uint8(255)
	if a.fillOp != nil {
		var v float64
		v, err = parseUnitInterval(*a.fillOp)
		if err != nil {
			return
		}
		attrA = op8FromUnit(v)
		apply = true
	}
	if apply {
		col.A = mul8(colorA, attrA)
	}
	return
}

func composeStroke(a paintAttrs) (on bool, s Stroke, err error) {
	if a.stroke == nil {
		if a.anyStrokeExtra {
			err = fmt.Errorf("parse: stroke presentation without stroke")
		}
		return
	}
	r, g, b, ca, none, err := parseColor(*a.stroke)
	if err != nil {
		return
	}
	if none {
		if a.anyStrokeExtra {
			err = fmt.Errorf("parse: stroke presentation with stroke none")
		}
		return
	}
	attrA := uint8(255)
	if a.strokeOp != nil {
		v, e := parseUnitInterval(*a.strokeOp)
		if e != nil {
			err = e
			return
		}
		attrA = op8FromUnit(v)
	}
	s = NewStroke().WithColor(color.NRGBA{R: r, G: g, B: b, A: mul8(ca, attrA)})
	if a.strokeW != nil {
		w, e := parseNonNegLength("stroke-width", *a.strokeW)
		if e != nil {
			err = e
			return
		}
		s = s.WithWidth(w)
	}
	if a.cap != nil {
		switch *a.cap {
		case "butt":
			s = s.WithCap(CapButt)
		case "round":
			s = s.WithCap(CapRound)
		case "square":
			s = s.WithCap(CapSquare)
		default:
			err = fmt.Errorf("parse: unknown stroke-linecap %q", *a.cap)
			return
		}
	}
	if a.join != nil {
		switch *a.join {
		case "miter":
			s = s.WithJoin(JoinMiter)
		case "round":
			s = s.WithJoin(JoinRound)
		case "bevel":
			s = s.WithJoin(JoinBevel)
		default:
			err = fmt.Errorf("parse: unknown stroke-linejoin %q", *a.join)
			return
		}
	}
	if a.miter != nil {
		m, e := parseLength(*a.miter)
		if e != nil {
			err = e
			return
		}
		if !isFinite(m) || m < 0 {
			err = fmt.Errorf("parse: stroke-miterlimit %v", m)
			return
		}
		s = s.WithMiterLimit(m)
	}
	on = true
	return
}

func applyCirclePaint(c Circle, a paintAttrs) (Circle, error) {
	none, apply, col, rule, err := composeFill(a)
	if err != nil {
		return c, err
	}
	on, st, err := composeStroke(a)
	if err != nil {
		return c, err
	}
	if none {
		c = c.WithFillNone()
	} else if apply {
		c = c.WithFill(col)
	}
	if rule == FillEvenOdd {
		c = c.WithFillRule(FillEvenOdd)
	}
	if on {
		c = c.WithStroke(st)
	}
	return c, nil
}

func applyEllipsePaint(el Ellipse, a paintAttrs) (Ellipse, error) {
	none, apply, col, rule, err := composeFill(a)
	if err != nil {
		return el, err
	}
	on, st, err := composeStroke(a)
	if err != nil {
		return el, err
	}
	if none {
		el = el.WithFillNone()
	} else if apply {
		el = el.WithFill(col)
	}
	if rule == FillEvenOdd {
		el = el.WithFillRule(FillEvenOdd)
	}
	if on {
		el = el.WithStroke(st)
	}
	return el, nil
}

func applyRectPaint(r Rect, a paintAttrs) (Rect, error) {
	none, apply, col, rule, err := composeFill(a)
	if err != nil {
		return r, err
	}
	on, st, err := composeStroke(a)
	if err != nil {
		return r, err
	}
	if none {
		r = r.WithFillNone()
	} else if apply {
		r = r.WithFill(col)
	}
	if rule == FillEvenOdd {
		r = r.WithFillRule(FillEvenOdd)
	}
	if on {
		r = r.WithStroke(st)
	}
	return r, nil
}

func applyPolygonPaint(p Polygon, a paintAttrs) (Polygon, error) {
	none, apply, col, rule, err := composeFill(a)
	if err != nil {
		return p, err
	}
	on, st, err := composeStroke(a)
	if err != nil {
		return p, err
	}
	if none {
		p = p.WithFillNone()
	} else if apply {
		p = p.WithFill(col)
	}
	if rule == FillEvenOdd {
		p = p.WithFillRule(FillEvenOdd)
	}
	if on {
		p = p.WithStroke(st)
	}
	return p, nil
}

func elemName(n xml.Name) (string, error) {
	switch n.Space {
	case "", svgNS:
		return n.Local, nil
	default:
		return "", fmt.Errorf("parse: unknown tag {%s}%s", n.Space, n.Local)
	}
}

func attrName(n xml.Name) (string, error) {
	if n.Space == "xmlns" {
		if n.Local == "" {
			return "xmlns", nil
		}
		return "", fmt.Errorf("parse: unknown attribute xmlns:%s", n.Local)
	}
	if n.Local == "xmlns" && (n.Space == "" || n.Space == svgNS) {
		return "xmlns", nil
	}
	switch n.Space {
	case "", svgNS:
		return n.Local, nil
	default:
		return "", fmt.Errorf("parse: unknown attribute {%s}%s", n.Space, n.Local)
	}
}

func parseColor(s string) (r, g, b, a uint8, none bool, err error) {
	s = strings.TrimSpace(s)
	if s == "none" {
		return 0, 0, 0, 255, true, nil
	}
	if !strings.HasPrefix(s, "#") {
		return 0, 0, 0, 0, false, fmt.Errorf("parse: color %q", s)
	}
	h := s[1:]
	switch len(h) {
	case 3:
		exp := []byte{h[0], h[0], h[1], h[1], h[2], h[2]}
		raw, e := hex.DecodeString(string(exp))
		if e != nil {
			return 0, 0, 0, 0, false, fmt.Errorf("parse: color %q", s)
		}
		return raw[0], raw[1], raw[2], 255, false, nil
	case 6:
		raw, e := hex.DecodeString(h)
		if e != nil {
			return 0, 0, 0, 0, false, fmt.Errorf("parse: color %q", s)
		}
		return raw[0], raw[1], raw[2], 255, false, nil
	case 8:
		raw, e := hex.DecodeString(h)
		if e != nil {
			return 0, 0, 0, 0, false, fmt.Errorf("parse: color %q", s)
		}
		return raw[0], raw[1], raw[2], raw[3], false, nil
	default:
		return 0, 0, 0, 0, false, fmt.Errorf("parse: color %q", s)
	}
}

func mul8(c, a uint8) uint8 {
	prod := uint32(c)*uint32(a) + 128
	return uint8((prod + (prod >> 8)) >> 8)
}

func parseLength(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "px") {
		s = strings.TrimSpace(s[:len(s)-2])
	}
	if s == "" {
		return 0, fmt.Errorf("parse: empty number")
	}
	for _, r := range s {
		if unicode.IsLetter(r) || r == '%' {
			return 0, fmt.Errorf("parse: unsupported unit in %q", s)
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || !isFinite(v) {
		return 0, fmt.Errorf("parse: number %q", s)
	}
	return v, nil
}

func parseNonNegLength(name, s string) (float64, error) {
	v, err := parseLength(s)
	if err != nil {
		return 0, err
	}
	if err := checkNonNeg(name, v); err != nil {
		return 0, err
	}
	return v, nil
}

func parseDocLength(s string) (float64, error) {
	return parseLength(s)
}

func parseUnitInterval(s string) (float64, error) {
	v, err := parseLength(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func parseNumberList(s string) ([]float64, error) {
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		v, err := parseLength(f)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func parseViewBox(s string) (ViewBox, error) {
	nums, err := parseNumberList(s)
	if err != nil {
		return ViewBox{}, err
	}
	if len(nums) != 4 {
		return ViewBox{}, fmt.Errorf("parse: viewBox wants 4 numbers, got %d", len(nums))
	}
	return ViewBox{minX: nums[0], minY: nums[1], w: nums[2], h: nums[3], ok: true}, nil
}

func parsePoints(s string) ([][2]float64, error) {
	nums, err := parseNumberList(s)
	if err != nil {
		return nil, err
	}
	if len(nums)%2 != 0 {
		return nil, fmt.Errorf("parse: odd point list")
	}
	pts := make([][2]float64, 0, len(nums)/2)
	for i := 0; i < len(nums); i += 2 {
		pts = append(pts, [2]float64{nums[i], nums[i+1]})
	}
	return pts, nil
}

func isSpace(b []byte) bool {
	for _, r := range string(b) {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
