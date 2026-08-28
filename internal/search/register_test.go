package search

import "testing"

func TestNewUnknown(t *testing.T) {
	if _, err := New("nope"); err == nil {
		t.Fatal("expected error")
	}
}
