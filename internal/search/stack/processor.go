package stack

import (
	"context"
	"sync"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/lewkit/x/ndarray"
)

var (
	evalOnce sync.Once
	evalInst ndarray.Evaluator
	evalErr  error
)

// openEvaluator is the one evaluator from the command context, under "gpu".
// Searches that were not started by the command take the driver default.
func openEvaluator(ctx context.Context) (ndarray.Evaluator, error) {
	if ev, ok := cmd.Lookup[ndarray.Evaluator](ctx, "gpu"); ok && ev != nil {
		return ev, nil
	}
	evalOnce.Do(func() {
		evalInst, evalErr = ndarray.Open(ctx)
	})
	return evalInst, evalErr
}
