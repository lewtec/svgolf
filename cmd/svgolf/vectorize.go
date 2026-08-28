package main

import (
	"image/png"
	"os"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func newVectorizeCmd() *cobra.Command {
	var (
		out  string
		algo string
	)
	cmd := &cobra.Command{
		Use:   "vectorize in.png",
		Short: "Write an SVG from a PNG (Search)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := search.New(algo)
			if err != nil {
				return err
			}
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				return err
			}
			want := search.FromImage(img)
			doc, err := search.Last(s.Search(cmd.Context(), want))
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
	cmd.Flags().StringVar(&algo, "search", "dumb", "Search adapter")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
