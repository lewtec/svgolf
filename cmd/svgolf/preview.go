package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lewtec/svgolf/internal/resvg"
	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func newPreviewCmd() *cobra.Command {
	var (
		algo   string
		eval   string
		out    string
		width int
	)
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Run a Search on testdata/eval and write testdata/preview",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := search.New(algo)
			if err != nil {
				return err
			}
			bin, err := resvg.LookPath()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
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
				if err := previewOne(cmd, s, bin, filepath.Join(eval, e.Name()), out, scene, width); err != nil {
					return fmt.Errorf("%s: %w", scene, err)
				}
			}
			if n == 0 {
				return fmt.Errorf("preview: no png in %s", eval)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&algo, "search", "dumb", "registered Search adapter")
	cmd.Flags().StringVar(&eval, "eval", filepath.Join("testdata", "eval"), "eval PNG directory")
	cmd.Flags().StringVar(&out, "out", filepath.Join("testdata", "preview"), "preview output directory")
	cmd.Flags().IntVar(&width, "width", 480, "preview PNG width for resvg")
	return cmd
}

func previewOne(cmd *cobra.Command, s search.Search, resvgBin, src, out, scene string, width int) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	want := search.FitCanvas(search.FromImage(img), search.MaxCanvas)
	if err := writePNGFile(filepath.Join(out, "want-"+scene+".png"), search.FitCanvas(want, width)); err != nil {
		return err
	}
	doc, err := s.Search(cmd.Context(), want)
	if err != nil {
		return err
	}
	svgPath := filepath.Join(out, scene+".svg")
	sf, err := os.Create(svgPath)
	if err != nil {
		return err
	}
	if err := svg.Encode(sf, doc); err != nil {
		sf.Close()
		return err
	}
	sf.Close()
	pngPath := filepath.Join(out, scene+".png")
	c := exec.CommandContext(cmd.Context(), resvgBin, "--width", fmt.Sprint(width), svgPath, pngPath)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("resvg: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s kids=%d -> %s\n", scene, len(doc.Children()), pngPath)
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
