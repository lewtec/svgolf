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
	for _, name := range []string{"render", "verify", "vectorize"} {
		if !got[name] {
			t.Errorf("missing command %q", name)
		}
	}
}

func TestCommandsNotImplemented(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "render", args: []string{"render", "in.svg", "-o", "out.png"}, want: "render: not implemented"},
		{name: "verify", args: []string{"verify", "in.svg"}, want: "verify: not implemented"},
		{name: "vectorize", args: []string{"vectorize", "in.png", "-o", "out.svg"}, want: "vectorize: not implemented"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := newRootCmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.want {
				t.Errorf("Execute() = %q; want %q", err.Error(), tt.want)
			}
		})
	}
}
