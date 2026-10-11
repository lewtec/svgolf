package search

import (
	"context"
	"errors"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
)

func TestGraphicsProcessorDefaultsToOnboard(t *testing.T) {
	if got := GraphicsProcessor(context.Background()); got != ProcessorIntegrated {
		t.Fatalf("processor=%s", got)
	}
	ctx := WithGraphicsProcessor(context.Background(), ProcessorDedicated)
	if got := GraphicsProcessor(ctx); got != ProcessorDedicated {
		t.Fatalf("processor=%s", got)
	}
	SetGraphicsProcessor(ProcessorSoftware)
	t.Cleanup(func() { SetGraphicsProcessor(ProcessorIntegrated) })
	if got := GraphicsProcessor(context.Background()); got != ProcessorSoftware {
		t.Fatalf("processor=%s", got)
	}
	if got := GraphicsProcessor(ctx); got != ProcessorDedicated {
		t.Fatalf("context processor=%s", got)
	}
}

func TestProcessorEnumArg(t *testing.T) {
	type args struct {
		GPU cmd.EnumArg[Processor] `long:"gpu" default:"integrated" help:"graphics processor"`
	}
	got, err := cmd.Parse[args]("--gpu", "dedicated")
	if err != nil {
		t.Fatal(err)
	}
	if got.GPU.Value() != ProcessorDedicated {
		t.Fatalf("processor=%s", got.GPU.Value())
	}
	bare, err := cmd.Parse[args]()
	if err != nil {
		t.Fatal(err)
	}
	if bare.GPU.Value() != ProcessorIntegrated {
		t.Fatalf("default processor=%s", bare.GPU.Value())
	}
	_, err = cmd.Parse[args]("--gpu", "discrete")
	if !errors.Is(err, cmd.ErrInvalidArgument) {
		t.Fatalf("err=%v", err)
	}
}
