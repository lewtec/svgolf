package search

import (
	"fmt"
)

// Tunable is a Search that accepts opaque options.
// The CLI forwards the pairs and does not interpret them.
type Tunable interface {
	Tune(map[string]string) error
}

// Tune applies opts to s. An empty map is a no-op.
// An adapter that does not implement Tunable rejects a non-empty map.
func Tune(s Search, opts map[string]string) error {
	if len(opts) == 0 {
		return nil
	}
	if s == nil {
		return fmt.Errorf("search: nil adapter")
	}
	t, ok := s.(Tunable)
	if !ok {
		return fmt.Errorf("search: adapter does not take options")
	}
	return t.Tune(opts)
}
