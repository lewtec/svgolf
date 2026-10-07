package loss

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// coreMinimum is the element count where extra cores beat one loop.
const coreMinimum = 16384

var coreLive atomic.Int32

// CoreClaim is a reservation from EnterCores. Release it after the work.
type CoreClaim struct{ held bool }

// Release drops a reservation. A zero claim is a no-op.
func (c CoreClaim) Release() {
	if c.held {
		coreLive.Add(-1)
	}
}

// EnterCores reserves extra cores when n is large and no other call is
// inside the reservation. The bool is false when the caller should stay
// on one core; Release still has to run, including after that inline work,
// so overlapping calls keep the reservation until they finish.
func EnterCores(n int) (bool, CoreClaim) {
	if n < coreMinimum || runtime.GOMAXPROCS(0) < 2 {
		return false, CoreClaim{}
	}
	ticket := coreLive.Add(1)
	claim := CoreClaim{held: true}
	if ticket != 1 {
		return false, claim
	}
	runtime.Gosched()
	if coreLive.Load() != 1 {
		return false, claim
	}
	return true, claim
}

// SplitRange calls fn on disjoint ranges that cover [0, n).
func SplitRange(n int, fn func(lo, hi int)) {
	workers := runtime.GOMAXPROCS(0)
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lo := w * chunk
		hi := lo + chunk
		if hi > n {
			hi = n
		}
		if lo >= hi {
			continue
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			fn(lo, hi)
		}(lo, hi)
	}
	wg.Wait()
}
