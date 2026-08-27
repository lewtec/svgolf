package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "svgolf",
		Short: "Turn a PNG into a simple editable SVG",
		Long: `svgolf turns a PNG, especially a model-generated logo, into a simple editable SVG.

Commands:
  render      SVG file to PNG (in-process renderer)
  verify      compare in-process render to resvg
  vectorize   PNG to a stub SVG (dumb generator)

v1 does not search. Run tests with: mise run test`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newVerifyCmd())
	cmd.AddCommand(newVectorizeCmd())
	return cmd
}
