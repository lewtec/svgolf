package main

import (
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

func (t *Trace) Record(ep search.Epoch) error {
	doc := ep.Document
	scale := ep.Scale
	if scale < 1 {
		scale = 1
	}
	svgPath := filepath.Join(t.dir, fmt.Sprintf("%03d.svg", t.n))
	for _, r := range []DocumentRenderer{
		NewSVGFile(svgPath),
		NewSVGFile(filepath.Join(t.dir, "last.svg")),
		NewPNGFile(filepath.Join(t.dir, fmt.Sprintf("%03d.png", t.n))),
		NewPNGFile(filepath.Join(t.dir, "last.png")),
	} {
		if err := r.Render(doc); err != nil {
			return err
		}
	}
	if t.log != nil {
		got, err := render.Render(doc)
		if err != nil {
			return err
		}
		op := ep.Operator
		if op == "" {
			op = "-"
		}
		fmt.Fprintf(t.log, "epoch %d operator=%s scale=%d elapsed=%.3fs paths=%d score=%.3f -> %s\n",
			t.n, op, scale, ep.Elapsed.Seconds(), len(doc.Children()), stack.Score(got, t.want, len(doc.Children())), svgPath)
	}
	t.n++
	return nil
}
