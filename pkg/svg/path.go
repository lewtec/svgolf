package svg

import (
	"fmt"
	"image/color"
	"slices"
)

const maxPathCmds = 4096

type PathCmdKind uint8

const (
	CmdMove PathCmdKind = iota
	CmdLine
	CmdCubic
	CmdClose
)

// PathCmd is absolute. Cubics store controls in X1,Y1,X2,Y2 and end in X,Y.
type PathCmd struct {
	Kind           PathCmdKind
	X, Y           float64
	X1, Y1, X2, Y2 float64
}

type Path struct {
	cmds  []PathCmd
	paint paint
}

func NewPath() Path { return Path{} }

func (p Path) Commands() []PathCmd {
	return slices.Clone(p.cmds)
}

func (p Path) withCmd(c PathCmd) (Path, error) {
	if len(p.cmds)+1 > maxPathCmds {
		return p, fmt.Errorf("path: more than %d commands", maxPathCmds)
	}
	p.cmds = append(slices.Clone(p.cmds), c)
	return p, nil
}

func (p Path) MoveTo(x, y float64) Path {
	p, _ = p.withCmd(PathCmd{Kind: CmdMove, X: x, Y: y})
	return p
}

func (p Path) LineTo(x, y float64) Path {
	p, _ = p.withCmd(PathCmd{Kind: CmdLine, X: x, Y: y})
	return p
}

func (p Path) CubicTo(x1, y1, x2, y2, x, y float64) Path {
	p, _ = p.withCmd(PathCmd{Kind: CmdCubic, X: x, Y: y, X1: x1, Y1: y1, X2: x2, Y2: y2})
	return p
}

func (p Path) Close() Path {
	p, _ = p.withCmd(PathCmd{Kind: CmdClose})
	return p
}

func (p Path) WithCommands(cmds []PathCmd) (Path, error) {
	if len(cmds) > maxPathCmds {
		return p, fmt.Errorf("path: more than %d commands", maxPathCmds)
	}
	p.cmds = slices.Clone(cmds)
	return p, nil
}

func (p Path) Node() Node {
	return Node{kind: KindPath, path: p}
}

func (p Path) WithFill(col color.NRGBA) Path {
	p.paint = p.paint.withFill(col)
	return p
}

func (p Path) WithLinearFill(g LinearFill) Path {
	p.paint = p.paint.withLinear(g)
	return p
}

func (p Path) WithFillOpacity(a float64) Path {
	p.paint = p.paint.withFillOpacity(a)
	return p
}

func (p Path) WithFillNone() Path {
	p.paint = p.paint.withFillNone()
	return p
}

func (p Path) WithFillRule(r FillRule) Path {
	p.paint = p.paint.withFillRule(r)
	return p
}

func (p Path) WithStroke(s Stroke) Path {
	p.paint = p.paint.withStroke(s)
	return p
}

func (p Path) WithoutStroke() Path {
	p.paint = p.paint.withoutStroke()
	return p
}

func (p Path) Fill() (color.NRGBA, bool)      { return p.paint.fill() }
func (p Path) LinearFill() (LinearFill, bool) { return p.paint.linearFill() }
func (p Path) FillOpacity() float64           { return p.paint.fillOpacity() }
func (p Path) FillRule() FillRule             { return p.paint.fillRule }
func (p Path) Stroke() (Stroke, bool)         { return p.paint.stroke() }
