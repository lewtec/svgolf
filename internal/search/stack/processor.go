package stack

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lewtec/lewkit/x/driver"
	ndvulkan "github.com/lewtec/lewkit/x/driver/ndeval/vulkan"
	drvvulkan "github.com/lewtec/lewkit/x/driver/vulkan"
	"github.com/lewtec/lewkit/x/ndarray"
	"github.com/lewtec/svgolf/internal/search"
)

// chooseProcessor picks a Vulkan device of kind. ProcessorIntegrated is the
// onboard processor. When that type is absent, the highest-weight non-software
// device is used so a machine with one graphics processor still runs.
// An explicit type that is not listed is an error.
func chooseProcessor(handles []driver.Handle[drvvulkan.Device], kind search.Processor) (driver.Handle[drvvulkan.Device], error) {
	var matches []driver.Handle[drvvulkan.Device]
	for _, handle := range handles {
		if processorType(handle.ID) == kind {
			matches = append(matches, handle)
		}
	}
	if len(matches) > 0 {
		return highestWeight(matches), nil
	}
	if kind == search.ProcessorIntegrated {
		var rest []driver.Handle[drvvulkan.Device]
		for _, handle := range handles {
			if processorType(handle.ID) == search.ProcessorSoftware {
				continue
			}
			rest = append(rest, handle)
		}
		if len(rest) > 0 {
			return highestWeight(rest), nil
		}
	}
	return driver.Handle[drvvulkan.Device]{}, fmt.Errorf("no %s graphics processor", kind)
}

func processorType(id string) search.Processor {
	parts := strings.Split(id, ":")
	if len(parts) < 3 {
		return search.ProcessorOther
	}
	kind, err := search.ParseProcessor(parts[2])
	if err != nil {
		return search.ProcessorOther
	}
	return kind
}

func highestWeight(handles []driver.Handle[drvvulkan.Device]) driver.Handle[drvvulkan.Device] {
	best := handles[0]
	for _, handle := range handles[1:] {
		if handle.Weight > best.Weight {
			best = handle
		}
	}
	return best
}

var (
	evalMu    sync.Mutex
	evalCache = map[search.Processor]ndarray.Evaluator{}
)

func openEvaluator(ctx context.Context) (ndarray.Evaluator, error) {
	choice := search.GraphicsProcessor(ctx)
	evalMu.Lock()
	defer evalMu.Unlock()
	if evaluator, ok := evalCache[choice]; ok {
		return evaluator, nil
	}
	handles, err := drvvulkan.List(ctx)
	if err != nil {
		return nil, err
	}
	handle, err := chooseProcessor(handles, choice)
	if err != nil {
		return nil, err
	}
	device, err := handle.Open(ctx)
	if err != nil {
		return nil, err
	}
	evaluator := ndvulkan.Bind(device)
	evalCache[choice] = evaluator
	return evaluator, nil
}
