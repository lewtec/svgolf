package main

import (
	"context"
	"fmt"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/internal/verify"
)

type verifyCmd struct {
	In   cmd.StringArg
	Diff cmd.StringArg `long:"diff" default:"" help:"diff PNG path (default: <input>.diff.png)"`
}

func (verifyCmd) Description() string {
	return "Compare the in-process render to resvg"
}

func (c *verifyCmd) Run(ctx context.Context) error {
	in := c.In.Value()
	diff := c.Diff.Value()
	if diff == "" {
		diff = in + ".diff.png"
	}
	r, err := verify.File(ctx, in)
	if err != nil {
		return err
	}
	if r.DifferingPixels == -1 {
		ow, oh := r.Ours.Rect.Dx(), r.Ours.Rect.Dy()
		qw, qh := r.Oracle.Rect.Dx(), r.Oracle.Rect.Dy()
		return fmt.Errorf("verify: size mismatch: ours %dx%d, resvg %dx%d", ow, oh, qw, qh)
	}
	if r.EncodeDrift {
		return fmt.Errorf("verify: encode drift")
	}
	if r.Match {
		return nil
	}
	if r.Diff != nil {
		if err := verify.WriteDiff(diff, r.Diff); err != nil {
			return err
		}
		w, h := r.Ours.Rect.Dx(), r.Ours.Rect.Dy()
		return fmt.Errorf("verify: %d mismatched pixels (%dx%d); wrote %s", r.DifferingPixels, w, h, diff)
	}
	return fmt.Errorf("verify: mismatch")
}
