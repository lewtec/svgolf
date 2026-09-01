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
