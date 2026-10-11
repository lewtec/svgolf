package main

import (
	_ "github.com/lewtec/svgolf/internal/search/prelude"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	var (
		profileDir string
		gpu        string
		stopProf   func() error
	)
	cmd := &cobra.Command{
		Use:   "svgolf",
		Short: "Turn a PNG into a simple editable SVG",
		Long: `svgolf turns a PNG, especially a model-generated logo, into a simple editable SVG.

Commands:
  render      SVG file to PNG (in-process renderer)
  verify      compare in-process render to resvg
  vectorize   PNG to SVG (--search NAME)
  preview     run Search on testdata/eval → testdata/preview
  server      watch Search from a --cache folder

Register a Search in its package init. Import search/prelude so it Registers. Then:
  mise exec -- go run ./cmd/svgolf preview --search NAME

v1 ships Dumb via prelude. Run tests with: mise run test`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			search.SetGraphicsProcessor(gpu)
			cmd.SetContext(search.WithGraphicsProcessor(cmd.Context(), gpu))
			if profileDir == "" {
				return nil
			}
			var err error
			stopProf, err = startProfiles(profileDir)
			return err
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if stopProf == nil {
				return nil
			}
			return stopProf()
		},
	}
	cmd.PersistentFlags().StringVar(&profileDir, "profile", "", "directory to write cpu, heap, allocs, goroutine, mutex, block profiles and an execution trace")
	cmd.PersistentFlags().StringVar(&gpu, "gpu", "integrated", "graphics processor for Search: integrated (onboard), dedicated, or a driver id such as vulkan:nvidia:dedicated")
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newVerifyCmd())
	cmd.AddCommand(newVectorizeCmd())
	cmd.AddCommand(newPreviewCmd())
	cmd.AddCommand(newServerCmd())
	return cmd
}
