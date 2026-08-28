package prelude_test

import (
	"testing"

	"github.com/lewtec/svgolf/internal/search"
	_ "github.com/lewtec/svgolf/internal/search/prelude"
)

func TestNewDumb(t *testing.T) {
	s, err := search.New("dumb")
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil Search")
	}
}

func TestNamesHasDumb(t *testing.T) {
	found := false
	for _, n := range search.Names() {
		if n == "dumb" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Names=%v", search.Names())
	}
}
