package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func newVectorizeCmd() *cobra.Command {
	var (
		out    string
		colors int
		algo   string
	)
	cmd := &cobra.Command{
		Use:   "vectorize in.png",
		Short: "Write an SVG from a PNG (Search)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				return err
			}
			var s search.Search
			switch algo {
			case "dumb":
				s = search.Dumb{Colors: colors}
			case "greedy":
				s = &search.Greedy{Colors: colors}
			default:
				return fmt.Errorf("search: unknown adapter %q", algo)
			}
			doc, err := s.Search(cmd.Context(), toNRGBA(img))
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
	cmd.Flags().StringVar(&algo, "search", "dumb", "search adapter (dumb|greedy)")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
