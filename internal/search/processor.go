package search

import (
	"context"
	"fmt"
	"sync"

	"github.com/lewtec/lewkit/x/cmd"
)

// Processor is the kind of graphics processor Search should use.
// The zero value is the onboard processor. String tokens match cmd.EnumArg.
type Processor uint8

const (
	ProcessorIntegrated Processor = iota
	ProcessorDedicated
	ProcessorVirtual
	ProcessorSoftware
	ProcessorOther
)

func (p Processor) String() string {
	switch p {
	case ProcessorDedicated:
		return "dedicated"
	case ProcessorVirtual:
		return "virtual"
	case ProcessorSoftware:
		return "software"
	case ProcessorOther:
		return "other"
	default:
		return "integrated"
	}
}

// Values lists the processors cmd.EnumArg accepts.
func (Processor) Values() []Processor {
	return []Processor{
		ProcessorIntegrated,
		ProcessorDedicated,
		ProcessorVirtual,
		ProcessorSoftware,
		ProcessorOther,
	}
}

// ParseProcessor maps a String token. Unknown tokens are an error.
func ParseProcessor(name string) (Processor, error) {
	var zero Processor
	for _, v := range zero.Values() {
		if v.String() == name {
			return v, nil
		}
	}
	return ProcessorIntegrated, fmt.Errorf("graphics processor %q", name)
}

type processorKey struct{}

var (
	processorMu   sync.Mutex
	processorKind = ProcessorIntegrated
)

// SetGraphicsProcessor chooses the processor for searches that do not carry one
// on their context.
func SetGraphicsProcessor(kind Processor) {
	processorMu.Lock()
	processorKind = kind
	processorMu.Unlock()
}

// WithGraphicsProcessor returns a context that names the processor for Search.
func WithGraphicsProcessor(ctx context.Context, kind Processor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, processorKey{}, kind)
}

// GraphicsProcessor is the processor named on ctx, or the process default.
// An x/cmd value under "gpu" wins. That is what EnumArg stores.
func GraphicsProcessor(ctx context.Context) Processor {
	if ctx != nil {
		if kind, ok := cmd.Lookup[Processor](ctx, "gpu"); ok {
			return kind
		}
		if kind, ok := ctx.Value(processorKey{}).(Processor); ok {
			return kind
		}
	}
	processorMu.Lock()
	defer processorMu.Unlock()
	return processorKind
}
