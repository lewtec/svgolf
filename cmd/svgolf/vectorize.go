package main

import (
	"errors"

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
		RunE: func(*cobra.Command, []string) error {
			return errors.New("vectorize: not implemented")
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output SVG path")
	cmd.Flags().IntVar(&colors, "colors", 0, "palette size (0 = auto, cap 8)")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
