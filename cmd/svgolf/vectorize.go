package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/lewtec/svgolf/internal/loss"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/render"
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
			if epochs != "" {
				if err := os.MkdirAll(epochs, 0o755); err != nil {
					return err
				}
			}
			var last svg.Document
			n := 0
			for doc, err := range s.Search(cmd.Context(), want) {
				if err != nil {
					return err
				}
				last = doc
				if epochs != "" {
					if err := writeEpoch(cmd, epochs, n, doc, want); err != nil {
						return err
					}
				}
				n++
			}
			if n == 0 {
				return fmt.Errorf("search: no epoch")
			}
			outf, err := os.Create(out)
			if err != nil {
				return err
			}
			defer outf.Close()
			return svg.Encode(outf, last)
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output SVG path")
	cmd.Flags().StringVar(&algo, "search", "dumb", "Search adapter")
	cmd.Flags().StringVar(&epochs, "epochs", "", "directory to write each epoch as N.svg and N.png")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func writeEpoch(cmd *cobra.Command, dir string, i int, doc svg.Document, want *image.NRGBA) error {
	svgPath := filepath.Join(dir, fmt.Sprintf("%03d.svg", i))
	sf, err := os.Create(svgPath)
	if err != nil {
		return err
	}
	if err := svg.Encode(sf, doc); err != nil {
		sf.Close()
		return err
	}
	sf.Close()
	got, err := render.Render(doc)
	if err != nil {
		return err
	}
	pngPath := filepath.Join(dir, fmt.Sprintf("%03d.png", i))
	pf, err := os.Create(pngPath)
	if err != nil {
		return err
	}
	if err := png.Encode(pf, got); err != nil {
		pf.Close()
		return err
	}
	pf.Close()
	fmt.Fprintf(cmd.OutOrStdout(), "epoch %d paths=%d hue=%.3f rmse=%.4f pixels=%.0f -> %s\n",
		i, len(doc.Children()), loss.Hue(got, want), loss.RMSE(got, want), loss.Pixels(got, want), svgPath)
	return nil
}
