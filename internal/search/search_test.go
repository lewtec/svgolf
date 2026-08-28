package search

import (
	"errors"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestLastNoEpoch(t *testing.T) {
	_, err := Last(func(yield func(svg.Document, error) bool) {})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLastStopsOnError(t *testing.T) {
	boom := errors.New("boom")
	seq := func(yield func(svg.Document, error) bool) {
		if !yield(svg.NewDocument(1, 1), nil) {
			return
		}
		yield(svg.Document{}, boom)
	}
	_, err := Last(seq)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
