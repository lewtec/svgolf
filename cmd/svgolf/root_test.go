package main

import (
	"errors"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
)

func TestRootListsCommands(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"render", "verify", "vectorize", "preview", "server"} {
		_, err := cmd.Parse[cmd.App[root]](name, "--help")
		if errors.Is(err, cmd.ErrUnknownCommand) {
			t.Errorf("missing command %q", name)
		}
	}
	_, err := cmd.Parse[cmd.App[root]]("nope")
	if !errors.Is(err, cmd.ErrUnknownCommand) {
		t.Fatalf("err=%v", err)
	}
}
