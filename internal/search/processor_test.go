package search

import (
	"context"
	"testing"
)

func TestGraphicsProcessorDefaultsToOnboard(t *testing.T) {
	if got := GraphicsProcessor(context.Background()); got != "integrated" {
		t.Fatalf("processor=%s", got)
	}
	ctx := WithGraphicsProcessor(context.Background(), "dedicated")
	if got := GraphicsProcessor(ctx); got != "dedicated" {
		t.Fatalf("processor=%s", got)
	}
	SetGraphicsProcessor("vulkan:nvidia:dedicated")
	t.Cleanup(func() { SetGraphicsProcessor("integrated") })
	if got := GraphicsProcessor(context.Background()); got != "vulkan:nvidia:dedicated" {
		t.Fatalf("processor=%s", got)
	}
	if got := GraphicsProcessor(ctx); got != "dedicated" {
		t.Fatalf("context processor=%s", got)
	}
}
