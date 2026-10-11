package search

import (
	"context"
	"sync"
)

// The onboard graphics processor is the default. The dedicated card is an option.
const defaultGraphicsProcessor = "integrated"

type processorKey struct{}

var (
	processorMu   sync.Mutex
	processorName = defaultGraphicsProcessor
)

// SetGraphicsProcessor chooses the processor for searches that do not carry one
// on their context. Empty means the onboard processor. Names are integrated,
// dedicated, software, virtual, other, or a driver id such as vulkan:nvidia:dedicated.
func SetGraphicsProcessor(name string) {
	if name == "" {
		name = defaultGraphicsProcessor
	}
	processorMu.Lock()
	processorName = name
	processorMu.Unlock()
}

// WithGraphicsProcessor returns a context that names the processor for Search.
func WithGraphicsProcessor(ctx context.Context, name string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" {
		name = defaultGraphicsProcessor
	}
	return context.WithValue(ctx, processorKey{}, name)
}

// GraphicsProcessor is the processor named on ctx, or the process default.
func GraphicsProcessor(ctx context.Context) string {
	if ctx != nil {
		if name, ok := ctx.Value(processorKey{}).(string); ok && name != "" {
			return name
		}
	}
	processorMu.Lock()
	defer processorMu.Unlock()
	return processorName
}
