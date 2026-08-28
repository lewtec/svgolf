package main

import (
	"strings"
	"testing"
)

func TestVectorizeUnknownSearch(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"vectorize", "x.png", "-o", "y.svg", "--search", "nope"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("got %v", err)
	}
}
