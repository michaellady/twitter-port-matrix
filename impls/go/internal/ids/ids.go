// Package ids provides a deterministic, monotonically increasing ID generator.
// F8 (tweet ID uniqueness + per-author monotonicity): every issued ID is
// globally unique and per-author monotonic by virtue of being globally
// monotonic.
package ids

import "sync"

// Generator issues monotonically increasing 64-bit IDs starting at 1.
// Zero is never returned (callers may treat zero as "unset").
type Generator struct {
	mu   sync.Mutex
	next int64
}

// New returns a Generator that issues 1, 2, 3, …
// @ ensures acc(g.LockP())
// @ ensures g != nil
func New() (g *Generator) {
	g = &Generator{next: 1}
	// @ fold g.LockP()
	return g
}

// NewAt returns a Generator starting at the given seed (must be > 0).
// @ ensures acc(g.LockP())
// @ ensures g != nil
func NewAt(seed int64) (g *Generator) {
	if seed < 1 {
		seed = 1
	}
	g = &Generator{next: seed}
	// @ fold g.LockP()
	return g
}

// Next issues the next ID and advances by 1.
//
// F8, DISCHARGED. The `// @ trusted` marker is gone. This method has exactly
// the shape `(*clock.Logical).Tick` has carried a real contract since S3P1b --
// take the LockP token, unfold, mutate the one protected field, fold, hand the
// token back -- and it verifies the same way. The sidecar's claim that the
// discharge "happens in Phase 2b" was a statement about work not done, not
// about a tool limitation: nothing in the flat `int64`-behind-a-mutex shape
// needed anything Gobra lacks.
//
// This matters past F8. `(*MemStore).PutTweet` rejects an append whose id does
// not strictly exceed the previous one, and that guard is the difference
// between S_obs's two-outcome stepPostTweet and the store's three. Proving the
// guard never fires needs exactly what this postcondition now gives: every
// issued id strictly exceeds the one before it.
//
// @ requires acc(g.LockP())
// @ ensures acc(g.LockP())
// @ ensures result == old(unfolding acc(g.LockP()) in g.next)
// @ ensures unfolding acc(g.LockP()) in g.next == result + 1
// @ ensures result >= 1
func (g *Generator) Next() (result int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// @ unfold acc(g.LockP())
	result = g.next
	g.next++
	// @ fold acc(g.LockP())
	return result
}

// Peek returns the next ID that would be issued, without advancing.
// @ trusted
func (g *Generator) Peek() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.next
}

// Current returns the highest ID that has been issued so far. Equal to
// Peek()-1; if no ID has been issued yet, returns 0. Used by the
// snapshot contract (Stream 2 Phase 0) to capture generator state.
//
// @ trusted
func (g *Generator) Current() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.next - 1
}

// SetCurrent overwrites the generator's internal counter so that the
// NEXT call to Next() returns value+1. Used by the snapshot contract
// (Stream 2 Phase 0) to restore generator state on the shadow backend.
//
// TRUSTED: this method intentionally bypasses F8's strict-monotonic-
// from-1 invariant. Callers (admin snapshot loader only) are responsible
// for ensuring `value` is at least the current counter to avoid issuing
// duplicate IDs on subsequent Next() calls. Inventoried in TCB.md.
//
// @ trusted
func (g *Generator) SetCurrent(value int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next = value + 1
}
