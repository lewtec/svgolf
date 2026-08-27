package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderWritesPNG(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.png")
	in := filepath.Join("..", "..", "testdata", "svg", "rect-inset.svg")
	cmd := newRootCmd()
	cmd.SetArgs([]string{"render", in, "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("size = %v", img.Bounds())
	}
}
