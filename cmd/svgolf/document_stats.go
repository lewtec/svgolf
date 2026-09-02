package main

import "github.com/lewtec/svgolf/pkg/svg"

func documentPaths(d svg.Document) int {
	return len(documentForms(d))
}

func documentVertices(d svg.Document) int {
	n := 0
	for _, child := range documentForms(d) {
		n += nodeVertices(child)
	}
	return n
}

// documentForms skips the full-frame paper pane stack puts first.
func documentForms(d svg.Document) []svg.Node {
	kids := d.Children()
	if len(kids) == 0 {
		return nil
	}
	if fullFramePane(kids[0], d.Width(), d.Height()) {
		return kids[1:]
	}
	return kids
}

func fullFramePane(n svg.Node, w, h float64) bool {
	p, ok := n.Path()
	if !ok {
		return false
	}
	seen := map[[2]float64]bool{}
	for _, c := range p.Commands() {
		if c.Kind == svg.CmdClose {
			continue
		}
		seen[[2]float64{c.X, c.Y}] = true
	}
	if len(seen) != 4 {
		return false
	}
	for _, q := range [][2]float64{{0, 0}, {w, 0}, {w, h}, {0, h}} {
		if !seen[q] {
			return false
		}
	}
	return true
}

func nodeVertices(n svg.Node) int {
	if g, ok := n.Group(); ok {
		sum := 0
		for _, child := range g.Children() {
			sum += nodeVertices(child)
		}
		return sum
	}
	if p, ok := n.Path(); ok {
		sum := 0
		for _, c := range p.Commands() {
			if c.Kind != svg.CmdClose {
				sum++
			}
		}
		return sum
	}
	if p, ok := n.Polygon(); ok {
		return len(p.Points())
	}
	if _, ok := n.Rect(); ok {
		return 4
	}
	return 0
}
