package main

import (
	"context"
	"fmt"
	"image/png"
	"os"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/internal/search/stack"
	"github.com/lewtec/svgolf/pkg/svg"
)

type vectorizeCmd struct {
	In     cmd.StringArg
	Out    cmd.StringArg                    `short:"o" long:"out" help:"output SVG path"`
	Algo   cmd.StringArg                    `long:"search" default:"dumb" help:"Search adapter"`
	Opt    []cmd.KV[string, *cmd.StringArg] `long:"search-opt" help:"Search option key and value"`
	Epochs cmd.StringArg                    `long:"epochs" default:"" help:"directory to write each epoch as N.svg and N.png, plus last.svg and last.png"`
}

func (vectorizeCmd) Description() string {
	return "Write an SVG from a PNG (Search)"
}

func (c *vectorizeCmd) Run(ctx context.Context) error {
	if c.Out.Value() == "" {
		return fmt.Errorf("vectorize: --out is required")
	}
	s, err := search.New(c.Algo.Value())
	if err != nil {
		return err
	}
	if err := tuneSearch(s, c.Opt); err != nil {
		return err
	}
	f, err := os.Open(c.In.Value())
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return err
	}
	want := search.FromImage(img)
	stack.LogCandidates(os.Stdout)
	defer stack.LogCandidates(nil)
	var trace *Trace
	if c.Epochs.Value() != "" {
		trace, err = NewTrace(c.Epochs.Value(), os.Stdout, want)
		if err != nil {
			return err
		}
	}
	var last svg.Document
	n := 0
	for ep, err := range s.Search(ctx, want) {
		if err != nil {
			return err
		}
		last = ep.Document
		if trace != nil {
			if err := trace.Record(ctx, ep); err != nil {
				return err
			}
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("search: no epoch")
	}
	return NewSVGFile(c.Out.Value()).Render(last)
}
