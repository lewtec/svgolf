package search

import (
	"context"
	"fmt"
	"image"
	"iter"
	"time"

	"github.com/lewtec/svgolf/pkg/svg"
)

// Epoch is one Search yield: the Document, the adapter's working scale,
// the operator that produced it, and how long that step took.
// Scale is 1 at native size. Operator is absorb, triangle, grow,
// carve, simplify, wash, join, drop, or empty.
type Epoch struct {
	Document svg.Document
	Scale    int
	Operator string
	Elapsed  time.Duration
	// Heat is ColorAt residual. Island is the leftover this step used.
	// Nil when the adapter does not publish debug frames.
	Heat   *image.NRGBA
	Island *image.NRGBA
	// Rated is every operator that ran this epoch. Score is the
	// proposal's Score when it was valid. Ok is true when it beat
	// the current document.
	Rated []Rated
}

// Rated is one operator's best proposal in an epoch.
type Rated struct {
	Name   string   `json:"name"`
	Score  *float64 `json:"score"`
	Ok     bool     `json:"ok"`
	Best   bool     `json:"best,omitempty"`
	Chosen bool     `json:"chosen,omitempty"`
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
