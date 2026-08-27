package main

import (
	"fmt"

	"github.com/lewtec/svgolf/internal/verify"
	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var diff string
	cmd := &cobra.Command{
		Use:   "verify in.svg",
		Short: "Compare the in-process render to resvg",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := args[0]
			if diff == "" {
				diff = in + ".diff.png"
			}
			r, err := verify.File(cmd.Context(), in)
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
		},
	}
	cmd.Flags().StringVar(&diff, "diff", "", "diff PNG path (default: <input>.diff.png)")
	return cmd
}
