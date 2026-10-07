package main

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/internal/search/stack"
	"github.com/lewtec/svgolf/pkg/render"
)

// Trace writes each Search epoch as NNN.svg / NNN.png and last.*.
type Trace struct {
	dir  string
	log  io.Writer
	want *image.NRGBA
	n    int
}

func NewTrace(dir string, log io.Writer, want *image.NRGBA) (*Trace, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Trace{dir: dir, log: log, want: want}, nil
}

func (t *Trace) Record(ctx context.Context, ep search.Epoch) error {
	doc := ep.Document
	scale := ep.Scale
	if scale < 1 {
		scale = 1
	}
	svgPath := filepath.Join(t.dir, fmt.Sprintf("%03d.svg", t.n))
	for _, path := range []string{svgPath, filepath.Join(t.dir, "last.svg")} {
		if err := NewSVGFile(path).Render(doc); err != nil {
			return err
		}
	}
	got, err := render.Render(doc)
	if err != nil {
		return err
	}
	if err := writePNG(filepath.Join(t.dir, fmt.Sprintf("%03d.png", t.n)), got); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(t.dir, "last.png"), got); err != nil {
		return err
	}
	if t.log != nil {
		op := ep.Operator.String()
		if op == "" {
			op = "-"
		}
		fmt.Fprintf(t.log, "epoch %d operator=%s scale=%d elapsed=%.3fs paths=%d vertices=%d score=%.3f -> %s\n",
			t.n, op, scale, ep.Elapsed.Seconds(), documentPaths(doc), documentVertices(doc), stack.Score(ctx, got, t.want), svgPath)
	}
	t.n++
	return nil
}
