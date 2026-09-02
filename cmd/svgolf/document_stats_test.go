package main

import (
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestDocumentVerticesCountsPathDestinations(t *testing.T) {
	p := svg.NewPath().MoveTo(0, 0).LineTo(8, 0).LineTo(16, 0).CubicTo(16, 4, 16, 12, 16, 16).Close()
	doc := svg.NewDocument(16, 16).Append(p.Node())
	if got := documentPaths(doc); got != 1 {
		t.Fatalf("paths=%d want 1", got)
	}
	if got := documentVertices(doc); got != 4 {
		t.Fatalf("vertices=%d want 4 (close is not a vertex)", got)
	}
}

func TestDocumentStatsSkipPaperPane(t *testing.T) {
	pane := svg.NewPath().MoveTo(0, 0).LineTo(16, 0).LineTo(16, 16).LineTo(0, 16).Close()
	ear := svg.NewPath().MoveTo(1, 1).LineTo(8, 1).LineTo(8, 2).Close()
	doc := svg.NewDocument(16, 16).Append(pane.Node()).Append(ear.Node())
	if got := documentPaths(doc); got != 1 {
		t.Fatalf("paths=%d want 1 (paper pane is chrome)", got)
	}
	if got := documentVertices(doc); got != 3 {
		t.Fatalf("vertices=%d want the triangle, not 4+3", got)
	}
}
