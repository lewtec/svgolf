package main

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lewtec/svgolf/internal/search"
	"github.com/lewtec/svgolf/pkg/svg"
)

func TestTraceOverwritesLast(t *testing.T) {
	dir := t.TempDir()
	log := &bytes.Buffer{}
	d0 := svg.NewDocument(2, 2)
	d1 := svg.NewDocument(4, 4)
	w0 := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	tr, err := NewTrace(dir, log, w0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Record(search.Epoch{Document: d0, Scale: 8, Operator: "hull", Elapsed: time.Second}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Record(search.Epoch{Document: d1, Scale: 4, Operator: "refit"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "scale=8") {
		t.Fatalf("missing scale in log: %s", log.String())
	}
	if !strings.Contains(log.String(), "operator=hull") || !strings.Contains(log.String(), "operator=refit") {
		t.Fatalf("missing operator in log: %s", log.String())
	}
	if !strings.Contains(log.String(), "elapsed=1.000s") {
		t.Fatalf("missing elapsed in log: %s", log.String())
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
