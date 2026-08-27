package main

import (
	"image/png"
	"os"

	"github.com/lewtec/svgolf/internal/gen"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func newVectorizeCmd() *cobra.Command {
	var (
		out    string
		colors int
	)
	cmd := &cobra.Command{
		Use:   "vectorize in.png",
		Short: "Write a stub SVG from a PNG (dumb generator)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				return err
			}
			doc, err := gen.Dumb(img, colors)
			if err != nil {
				return err
			}
			outf, err := os.Create(out)
			if err != nil {
				return err
			}
			defer outf.Close()
			return svg.Encode(outf, doc)
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output SVG path")
	cmd.Flags().IntVar(&colors, "colors", 0, "palette size (0 = auto, cap 8)")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
