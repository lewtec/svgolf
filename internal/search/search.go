package search

import (
	"context"
	"fmt"
	"image"
	"iter"

	"github.com/lewtec/svgolf/pkg/svg"
)

// Epoch is one Search yield: the Document plus the adapter's working scale.
// Scale is 1 at native size. Adapters that never scale yield 1.
type Epoch struct {
	Document svg.Document
	Scale    int
}

// Search is the whole problem: want pixmap → one Epoch per step.
// Host does not scale the pixmap. Only this adapter may scale, as its own algorithm.
// A one-shot adapter yields once and stops. Palette, Cost, Loss, and mutate stay inside the adapter.
type Search interface {
	Search(ctx context.Context, target *image.NRGBA) iter.Seq2[Epoch, error]
}

// Last returns the last successful epoch's Document. Zero epochs or a yielded error fail.
func Last(seq iter.Seq2[Epoch, error]) (svg.Document, error) {
	var last svg.Document
	n := 0
	for ep, err := range seq {
		if err != nil {
			return svg.Document{}, err
		}
		last = ep.Document
		n++
	}
	if n == 0 {
		return svg.Document{}, fmt.Errorf("search: no epoch")
	}
	return last, nil
}
