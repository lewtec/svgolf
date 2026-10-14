package main

import (
	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/internal/search"
)

func tuneSearch(s search.Search, opts []cmd.StringArg) error {
	vals := make([]string, len(opts))
	for i := range opts {
		vals[i] = opts[i].Value()
	}
	parsed, err := search.ParseOptions(vals)
	if err != nil {
		return err
	}
	return search.Tune(s, parsed)
}
