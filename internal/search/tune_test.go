package search

import (
	"context"
	"image"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

type tunedSearch struct {
	opts map[string]string
}

func (tunedSearch) Search(context.Context, *image.NRGBA) iter.Seq2[Epoch, error] {
	return func(yield func(Epoch, error) bool) {}
}

func (t *tunedSearch) Tune(opts map[string]string) error {
	t.opts = opts
	return nil
}

type plainSearch struct{}

func (plainSearch) Search(context.Context, *image.NRGBA) iter.Seq2[Epoch, error] {
	return func(yield func(Epoch, error) bool) {}
}

func TestParseOptions(t *testing.T) {
	got, err := ParseOptions([]string{"epochs=3", "beam=2"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"epochs": "3", "beam": "2"}, got)
	_, err = ParseOptions([]string{"epochs"})
	require.Error(t, err)
	_, err = ParseOptions([]string{"epochs=1", "epochs=2"})
	require.Error(t, err)
}

func TestTuneEmptyIsNoop(t *testing.T) {
	require.NoError(t, Tune(plainSearch{}, nil))
}

func TestTuneRejectsPlainAdapter(t *testing.T) {
	require.Error(t, Tune(plainSearch{}, map[string]string{"epochs": "1"}))
}

func TestTuneForwardsPairs(t *testing.T) {
	s := &tunedSearch{}
	require.NoError(t, Tune(s, map[string]string{"beam": "2"}))
	require.Equal(t, map[string]string{"beam": "2"}, s.opts)
}

func TestTuneNil(t *testing.T) {
	require.Error(t, Tune(nil, map[string]string{"beam": "1"}))
}
