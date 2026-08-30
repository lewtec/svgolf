package main

import (
	_ "github.com/lewtec/svgolf/internal/search/prelude"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	var (
		profileDir string
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

Register a Search in its package init. Import search/prelude so it Registers. Then:
  mise exec -- go run ./cmd/svgolf preview --search NAME

v1 ships Dumb via prelude. Run tests with: mise run test`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newVerifyCmd())
	cmd.AddCommand(newVectorizeCmd())
	cmd.AddCommand(newPreviewCmd())
	return cmd
}
