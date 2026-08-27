package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func newRenderCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "render in.svg",
		Short: "Rasterize an SVG to PNG with the in-process renderer",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return errors.New("render: not implemented")
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "output PNG path")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
