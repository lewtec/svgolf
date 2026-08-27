package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/lewtec/svgolf/pkg/render"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func newRenderCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "render in.svg",
		Short: "Rasterize an SVG to PNG with the in-process renderer",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			doc, err := svg.ParseFile(args[0])
			if err != nil {
				return err
			}
			img, err := render.Render(doc)
			if err != nil {
				return err
			}
			f, err := os.Create(out)
			if err != nil {
				return err
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				return fmt.Errorf("png: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output PNG path")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
