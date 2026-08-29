package main

import (
	"image/png"
	"os"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

// DocumentRenderer writes one Document to a sink.
type DocumentRenderer interface {
	Render(doc svg.Document) error
}

type SVGFile struct {
	path string
}

func NewSVGFile(path string) SVGFile {
	return SVGFile{path: path}
}

func (f SVGFile) Render(doc svg.Document) error {
	file, err := os.Create(f.path)
	if err != nil {
		return err
	}
	err = svg.Encode(file, doc)
	if c := file.Close(); err == nil {
		err = c
	}
	return err
}

type PNGFile struct {
	path string
}

func NewPNGFile(path string) PNGFile {
	return PNGFile{path: path}
}

func (f PNGFile) Render(doc svg.Document) error {
	got, err := render.Render(doc)
	if err != nil {
		return err
	}
	file, err := os.Create(f.path)
	if err != nil {
		return err
	}
	err = png.Encode(file, got)
	if c := file.Close(); err == nil {
		err = c
	}
	return err
}

var (
	_ DocumentRenderer = SVGFile{}
	_ DocumentRenderer = PNGFile{}
)
