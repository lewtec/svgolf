package main

import (
	"testing"
)

func TestRootListsCommands(t *testing.T) {
	t.Parallel()
	cmd := newRootCmd()
	got := map[string]bool{}
	for _, c := range cmd.Commands() {
		got[c.Name()] = true
	}
	for _, name := range []string{"render", "verify", "vectorize", "preview"} {
		if !got[name] {
			t.Errorf("missing command %q", name)
		}
	}
}
