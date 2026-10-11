package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lewtec/lewkit/x/cmd"
)

func main() {
	app, err := cmd.Parse[cmd.App[root]](os.Args[1:]...)
	if err != nil {
		exit(err)
	}
	err = app.Run(context.Background())
	if app.Args.stopProf != nil {
		if serr := app.Args.stopProf(); err == nil {
			err = serr
		}
	}
	if err != nil {
		exit(err)
	}
}

func exit(err error) {
	if _, werr := fmt.Fprintln(os.Stderr, err); werr != nil {
		os.Exit(1)
	}
	os.Exit(1)
}
