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
// @ trusted
func (g *Generator) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := g.next
	g.next++
	return id
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
