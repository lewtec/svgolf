package main

import (
	"errors"

	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var diff string
	cmd := &cobra.Command{
		Use:   "verify in.svg",
		Short: "Compare the in-process render to resvg",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return errors.New("verify: not implemented")
		},
	}
	cmd.Flags().StringVar(&diff, "diff", "", "diff PNG path (default: <input>.diff.png)")
	return cmd
}
