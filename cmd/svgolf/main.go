package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		if _, werr := fmt.Fprintln(os.Stderr, err); werr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}
