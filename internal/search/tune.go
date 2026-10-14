package search

import (
	"fmt"
	"strings"
)

// Tunable is a Search that accepts opaque key=value knobs.
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

// ParseOptions splits key=value pairs. Duplicate keys are an error.
func ParseOptions(values []string) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for _, raw := range values {
		key, val, ok := strings.Cut(raw, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("search: option %q want key=value", raw)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("search: duplicate option %q", key)
		}
		out[key] = val
	}
	return out, nil
}
