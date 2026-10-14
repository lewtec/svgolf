package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/internal/resvg"
	"github.com/lewtec/svgolf/internal/search"
)

type previewCmd struct {
	Algo cmd.StringArg                    `long:"search" default:"dumb" help:"registered Search adapter"`
	Opt  []cmd.KV[string, *cmd.StringArg] `long:"search-opt" help:"Search option key and value"`
	Eval cmd.StringArg                    `long:"eval" default:"testdata/eval" help:"eval PNG directory"`
	Out  cmd.StringArg                    `long:"out" default:"testdata/preview" help:"preview output directory"`
}

func (previewCmd) Description() string {
	return "Run a Search on testdata/eval and write testdata/preview"
}

func (c *previewCmd) Run(ctx context.Context) error {
	s, err := search.New(c.Algo.Value())
	if err != nil {
		return err
	}
	if err := tuneSearch(s, c.Opt); err != nil {
		return err
	}
	bin, err := resvg.LookPath()
	if err != nil {
		return err
	}
	out := c.Out.Value()
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	eval := c.Eval.Value()
	ents, err := os.ReadDir(eval)
	if err != nil {
		return err
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		n++
		scene := strings.TrimSuffix(e.Name(), ".png")
		if err := previewOne(ctx, s, bin, filepath.Join(eval, e.Name()), out, scene); err != nil {
			return fmt.Errorf("%s: %w", scene, err)
		}
	}
	if n == 0 {
		return fmt.Errorf("preview: no png in %s", eval)
	}
	return nil
}

func previewOne(ctx context.Context, s search.Search, resvgBin, src, out, scene string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	want := search.FromImage(img)
	if err := writePNGFile(filepath.Join(out, "want-"+scene+".png"), want); err != nil {
		return err
	}
	doc, err := search.Last(s.Search(ctx, want))
	if err != nil {
		return err
	}
	svgPath := filepath.Join(out, scene+".svg")
	if err := NewSVGFile(svgPath).Render(doc); err != nil {
		return err
	}
	pngPath := filepath.Join(out, scene+".png")
	command := exec.CommandContext(ctx, resvgBin, svgPath, pngPath)
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("resvg: %w", err)
	}
	fmt.Fprintf(os.Stdout, "%s kids=%d -> %s\n", scene, len(doc.Children()), pngPath)
	return nil
}

func writePNGFile(path string, img *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
