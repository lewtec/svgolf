package search

import (
	"errors"
	"testing"

	"github.com/lewtec/svgolf/pkg/svg"
)

func TestLastNoEpoch(t *testing.T) {
	_, err := Last(func(yield func(Epoch, error) bool) {})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLastStopsOnError(t *testing.T) {
	boom := errors.New("boom")
	seq := func(yield func(Epoch, error) bool) {
		if !yield(Epoch{Document: svg.NewDocument(1, 1), Scale: 1}, nil) {
			return
		}
		yield(Epoch{}, boom)
	}
	_, err := Last(seq)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
