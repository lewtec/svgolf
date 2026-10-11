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

// chooseProcessor picks a Vulkan device. choice is a device type or a driver id.
// integrated is the onboard processor. When that type is absent, the highest-weight
// non-software device is used so a machine with one graphics processor still runs.
// An explicit type or id that is not listed is an error.
func chooseProcessor(handles []driver.Handle[drvvulkan.Device], choice string) (driver.Handle[drvvulkan.Device], error) {
	if choice == "" {
		choice = "integrated"
	}
	for _, handle := range handles {
		if handle.ID == choice {
			return handle, nil
		}
	}
	kind, err := drvvulkan.ParseDeviceType(choice)
	if err != nil {
		return driver.Handle[drvvulkan.Device]{}, fmt.Errorf("graphics processor %q: %w", choice, err)
	}
	choice = kind.String()
	var matches []driver.Handle[drvvulkan.Device]
	for _, handle := range handles {
		if processorType(handle.ID) == choice {
			matches = append(matches, handle)
		}
	}
	if len(matches) > 0 {
		return highestWeight(matches), nil
	}
	if choice == "integrated" {
		var rest []driver.Handle[drvvulkan.Device]
		for _, handle := range handles {
			if processorType(handle.ID) == "software" {
				continue
			}
			rest = append(rest, handle)
		}
		if len(rest) > 0 {
			return highestWeight(rest), nil
		}
	}
	return driver.Handle[drvvulkan.Device]{}, fmt.Errorf("no %s graphics processor", choice)
}

func processorType(id string) string {
	parts := strings.Split(id, ":")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
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
	evalCache = map[string]ndarray.Evaluator{}
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
