package search

import (
	"context"
	"fmt"
	"image"
	"iter"

	"github.com/lewtec/svgolf/pkg/svg"
)

// Search is the whole problem: want pixmap → one Document per epoch.
// Host does not scale the pixmap. Only this adapter may scale, as its own algorithm.
// A one-shot adapter yields once and stops. Palette, Cost, Loss, and mutate stay inside the adapter.
type Search interface {
	Search(ctx context.Context, target *image.NRGBA) iter.Seq2[svg.Document, error]
}

// Last returns the last successful epoch. Zero epochs or a yielded error fail.
func Last(seq iter.Seq2[svg.Document, error]) (svg.Document, error) {
	var last svg.Document
	n := 0
	for doc, err := range seq {
		if err != nil {
			return svg.Document{}, err
		}
		last = doc
		n++
	}
	if n == 0 {
		return svg.Document{}, fmt.Errorf("search: no epoch")
	}
	return last, nil
}
