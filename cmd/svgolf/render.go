package main

import (
	"context"
	"fmt"
	"image/png"
	"os"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
)

type renderCmd struct {
	In  cmd.StringArg
	Out cmd.StringArg `short:"o" long:"out" help:"output PNG path"`
}

func (renderCmd) Description() string {
	return "Rasterize an SVG to PNG with the in-process renderer"
}

func (c *renderCmd) Run(context.Context) error {
	if c.Out.Value() == "" {
		return fmt.Errorf("render: --out is required")
	}
	doc, err := svg.ParseFile(c.In.Value())
	if err != nil {
		return err
	}
	img, err := render.Render(doc)
	if err != nil {
		return err
	}
	f, err := os.Create(c.Out.Value())
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("png: %w", err)
	}
	return nil
}
