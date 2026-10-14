package main

import (
	"fmt"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/internal/search"
)

func tuneSearch(s search.Search, opts []cmd.KV[string, *cmd.StringArg]) error {
	seen := make(map[string]struct{}, len(opts))
	for _, kv := range opts {
		key := kv.K.Value()
		if _, dup := seen[key]; dup {
			return fmt.Errorf("search: duplicate option %q", key)
		}
		seen[key] = struct{}{}
	}
	return search.Tune(s, cmd.Map(opts))
}
