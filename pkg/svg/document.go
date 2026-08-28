package svg

import "slices"

type ViewBox struct {
	minX, minY, w, h float64
	ok               bool
}

func (v ViewBox) MinX() float64   { return v.minX }
func (v ViewBox) MinY() float64   { return v.minY }
func (v ViewBox) Width() float64  { return v.w }
func (v ViewBox) Height() float64 { return v.h }
func (v ViewBox) Set() bool       { return v.ok }

type Document struct {
	width, height float64
	vb            ViewBox
	children      []Node
}

func NewDocument(width, height float64) Document {
	return Document{width: width, height: height}
}

func (d Document) WithViewBox(minX, minY, w, h float64) Document {
	d.vb = ViewBox{minX: minX, minY: minY, w: w, h: h, ok: true}
	return d
}

func (d Document) ClearViewBox() Document {
	d.vb = ViewBox{}
	return d
}

func (d Document) Append(nodes ...Node) Document {
	n := len(d.children)
	out := make([]Node, n, n+len(nodes))
	copy(out, d.children)
	d.children = append(out, nodes...)
	return d
}

func (d Document) Width() float64   { return d.width }
func (d Document) Height() float64  { return d.height }
func (d Document) ViewBox() ViewBox { return d.vb }

func (d Document) Children() []Node {
	return slices.Clone(d.children)
}

type Kind int

const (
	KindInvalid Kind = iota
	KindGroup
	KindCircle
	KindEllipse
	KindRect
	KindPolygon
	KindPath
)

type Node struct {
	kind    Kind
	group   Group
	circle  Circle
	ellipse Ellipse
	rect    Rect
	polygon Polygon
	path    Path
}

func (n Node) Kind() Kind { return n.kind }

func (n Node) Group() (Group, bool) {
	if n.kind != KindGroup {
		return Group{}, false
	}
	return n.group, true
}

func (n Node) Circle() (Circle, bool) {
	if n.kind != KindCircle {
		return Circle{}, false
	}
	return n.circle, true
}

func (n Node) Ellipse() (Ellipse, bool) {
	if n.kind != KindEllipse {
		return Ellipse{}, false
	}
	return n.ellipse, true
}

func (n Node) Rect() (Rect, bool) {
	if n.kind != KindRect {
		return Rect{}, false
	}
	return n.rect, true
}

func (n Node) Polygon() (Polygon, bool) {
	if n.kind != KindPolygon {
		return Polygon{}, false
	}
	return n.polygon, true
}

func (n Node) Path() (Path, bool) {
	if n.kind != KindPath {
		return Path{}, false
	}
	return n.path, true
}

type Group struct {
	children []Node
}

func NewGroup() Group { return Group{} }

func (g Group) Append(nodes ...Node) Group {
	n := len(g.children)
	out := make([]Node, n, n+len(nodes))
	copy(out, g.children)
	g.children = append(out, nodes...)
	return g
}

func (g Group) Children() []Node {
	return slices.Clone(g.children)
}

func (g Group) Node() Node {
	return Node{kind: KindGroup, group: g}
}
