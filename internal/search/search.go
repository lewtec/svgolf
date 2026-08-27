package search

import (
	"context"
	"image"

	"github.com/lewtec/svgolf/pkg/svg"
)

// Search turns a target pixmap into a tree. Palette, Loss, and mutate stay inside the adapter.
type Search interface {
	Search(ctx context.Context, target *image.NRGBA) (svg.Document, error)
}
