package search

import (
	"context"
	"image"
	"iter"
	"testing"
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
	if err != nil {
		t.Fatal(err)
	}
	if got["epochs"] != "3" || got["beam"] != "2" {
		t.Fatalf("opts=%v", got)
	}
	if _, err := ParseOptions([]string{"epochs"}); err == nil {
		t.Fatal("expected key=value error")
	}
	if _, err := ParseOptions([]string{"epochs=1", "epochs=2"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestTuneEmptyIsNoop(t *testing.T) {
	if err := Tune(plainSearch{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestTuneRejectsPlainAdapter(t *testing.T) {
	if err := Tune(plainSearch{}, map[string]string{"epochs": "1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTuneForwardsPairs(t *testing.T) {
	s := &tunedSearch{}
	if err := Tune(s, map[string]string{"beam": "2"}); err != nil {
		t.Fatal(err)
	}
	if s.opts["beam"] != "2" {
		t.Fatalf("opts=%v", s.opts)
	}
}

func TestTuneNil(t *testing.T) {
	if err := Tune(nil, map[string]string{"beam": "1"}); err == nil {
		t.Fatal("expected error")
	}
}
