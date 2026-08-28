package search

import (
	"image"
	"testing"
)

func TestNewDumb(t *testing.T) {
	s, err := New("dumb")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(Dumb); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestNewResidual(t *testing.T) {
	s, err := New("residual")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*Residual); !ok {
		t.Fatalf("got %T", s)
	}
}

func TestNewUnknown(t *testing.T) {
	if _, err := New("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNamesHasDumb(t *testing.T) {
	found := false
	for _, n := range Names() {
		if n == "dumb" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Names=%v", Names())
	}
}

func TestNamesHasResidual(t *testing.T) {
	found := false
	for _, n := range Names() {
		if n == "residual" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Names=%v", Names())
	}
}

func TestFitCanvasCaps(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 5000, 4000))
	got := FitCanvas(src, 4096)
	if got.Rect.Dx() != 4096 || got.Rect.Dy() != 3276 {
		t.Fatalf("got %dx%d", got.Rect.Dx(), got.Rect.Dy())
	}
}
