package main

import "github.com/lewtec/svgolf/pkg/svg"

func documentPaths(d svg.Document) int {
	return len(d.Children())
}

func documentVertices(d svg.Document) int {
	n := 0
	for _, child := range d.Children() {
		n += nodeVertices(child)
	}
	return n
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
