package main

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
	"github.com/spf13/cobra"
)

func TestWriteEpochOverwritesLast(t *testing.T) {
	dir := t.TempDir()
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	d0 := svg.NewDocument(2, 2)
	d1 := svg.NewDocument(4, 4)
	w0 := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	w1 := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	if err := writeEpoch(cmd, dir, 0, d0, w0); err != nil {
		t.Fatal(err)
	}
	if err := writeEpoch(cmd, dir, 1, d1, w1); err != nil {
		t.Fatal(err)
	}
	sameFile(t, filepath.Join(dir, "001.svg"), filepath.Join(dir, "last.svg"))
	sameFile(t, filepath.Join(dir, "001.png"), filepath.Join(dir, "last.png"))
	b0, err := os.ReadFile(filepath.Join(dir, "000.svg"))
	if err != nil {
		t.Fatal(err)
	}
	last, err := os.ReadFile(filepath.Join(dir, "last.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(b0, last) {
		t.Fatal("last.svg still epoch 0")
	}
}

func sameFile(t *testing.T, a, b string) {
	t.Helper()
	xa, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	xb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(xa, xb) {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestVectorizeUnknownSearch(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"vectorize", "x.png", "-o", "y.svg", "--search", "nope"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("got %v", err)
	}
}
