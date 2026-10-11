package main

import (
	_ "github.com/lewtec/svgolf/internal/search/prelude"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/svgolf/internal/search"
)

// root is the svgolf command. Flags are x/cmd fields. gpu is stored on the
// context under "gpu" as a Processor for Search.
type root struct {
	GPU     cmd.EnumArg[search.Processor] `long:"gpu" default:"integrated" help:"graphics processor: integrated (onboard), dedicated, virtual, software, or other" ctx:"gpu"`
	Profile cmd.StringArg                 `long:"profile" default:"" help:"directory to write cpu, heap, allocs, goroutine, mutex, block profiles and an execution trace"`

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
