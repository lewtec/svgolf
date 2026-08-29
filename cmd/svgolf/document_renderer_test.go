package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestSVGFileRendersDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.svg")
	if err := NewSVGFile(path).Render(svg.NewDocument(8, 8)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<svg") {
		t.Fatalf("not svg: %s", b)
	}
}

func TestPNGFileRendersDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.png")
	if err := NewPNGFile(path).Render(svg.NewDocument(8, 8)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 8 || string(b[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("not png")
	}
}
