package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartProfilesWrites(t *testing.T) {
	dir := t.TempDir()
	stop, err := startProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"cpu.pprof", "trace.out",
		"heap.pprof", "allocs.pprof",
		"goroutine.pprof", "mutex.pprof", "block.pprof",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
