package search

import (
	"fmt"
	"sort"
	"sync"
)

var (
	regMu    sync.Mutex
	adapters = map[string]func() Search{}
)

// Register adds a Search constructor. Call from init in the adapter file.
func Register(name string, make func() Search) {
	if name == "" || make == nil {
		panic("search: Register empty name or nil ctor")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := adapters[name]; ok {
		panic("search: Register duplicate " + name)
	}
	adapters[name] = make
}

// New builds a registered adapter. Palette, Cost, and knobs stay inside the adapter.
func New(name string) (Search, error) {
	regMu.Lock()
	fn, ok := adapters[name]
	regMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("search: unknown adapter %q (have %v)", name, Names())
	}
	return fn(), nil
}

// Names returns registered adapter names, sorted.
func Names() []string {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]string, 0, len(adapters))
	for n := range adapters {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
