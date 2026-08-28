package search

import "testing"

func TestNewDumb(t *testing.T) {
	s, err := New("dumb")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(Dumb); !ok {
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
