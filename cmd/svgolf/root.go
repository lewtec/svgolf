package main

import (
	_ "github.com/lewtec/svgolf/internal/search/prelude"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/lewkit/x/driver/ndeval"
)

// root is the svgolf command. Flags are x/cmd fields. gpu is one ndarray
// evaluator, resolved from an expression and stored on the context.
type root struct {
	GPU     ndeval.Arg    `long:"gpu" help:"evaluator expression, for example vulkan:integrated | cpu" ctx:"gpu"`
	Profile cmd.StringArg `long:"profile" default:"" help:"directory to write cpu, heap, allocs, goroutine, mutex, block profiles and an execution trace"`

	Render    *renderCmd
	Verify    *verifyCmd
	Vectorize *vectorizeCmd
	Preview   *previewCmd
	Server    *serverCmd

	stopProf func() error
}

func (root) Description() string {
	return `svgolf turns a PNG, especially a model-generated logo, into a simple editable SVG.

Register a Search in its package init. Import search/prelude so it Registers. Then:
  mise exec -- go run ./cmd/svgolf preview --search NAME

v1 ships Dumb via prelude. Run tests with: mise run test`
}

func (r *root) Setup() error {
	if r.Profile.Value() == "" {
		return nil
	}
	stop, err := startProfiles(r.Profile.Value())
	if err != nil {
		return err
	}
	r.stopProf = stop
	return nil
}
