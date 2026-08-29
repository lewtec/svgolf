package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func newVectorizeCmd() *cobra.Command {
	var (
		out    string
		algo   string
		epochs string
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
			var trace *Trace
			if epochs != "" {
				trace, err = NewTrace(epochs, cmd.OutOrStdout(), want)
				if err != nil {
					return err
				}
			}
			var last svg.Document
			n := 0
			for ep, err := range s.Search(cmd.Context(), want) {
				if err != nil {
					return err
				}
				last = ep.Document
				if trace != nil {
					if err := trace.Record(ep); err != nil {
						return err
					}
				}
				n++
			}
			if n == 0 {
				return fmt.Errorf("search: no epoch")
			}
			return NewSVGFile(out).Render(last)
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output SVG path")
	cmd.Flags().StringVar(&algo, "search", "dumb", "Search adapter")
	cmd.Flags().StringVar(&epochs, "epochs", "", "directory to write each epoch as N.svg and N.png, plus last.svg and last.png")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
