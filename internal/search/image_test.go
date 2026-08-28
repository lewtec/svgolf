package search

import (
	"image"
	"testing"
)

func TestFromImageKeepsSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5000, 4000))
	got := FromImage(src)
	if got.Rect.Dx() != 5000 || got.Rect.Dy() != 4000 {
		t.Fatalf("got %dx%d", got.Rect.Dx(), got.Rect.Dy())
	}
}
