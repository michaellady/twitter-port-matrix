// Package clock provides an injected, monotonic logical clock.
// F7 (logical clock, non-strict): next() >= prev(). Ties allowed; total
// ordering for F2 is established by tweet_id, not by clock strictness.
package clock

import (
	"log"
	"sync"
)

// Clock issues monotonically non-decreasing logical timestamps.
type Clock interface {
	Now() int64
	Tick()
}

// Logical is a deterministic clock used by tests and conformance replay.
// It only advances when Tick() is called explicitly.
type Logical struct {
	mu  sync.Mutex
	now int64
}

// New returns a Logical clock starting at zero.
// @ ensures acc(l.LockP())
// @ ensures l != nil
func New() (l *Logical) {
	l = &Logical{}
	// @ fold l.LockP()
	return l
}

// NewAt returns a Logical clock starting at the given timestamp.
// @ ensures acc(l.LockP())
// @ ensures l != nil
func NewAt(t int64) (l *Logical) {
	l = &Logical{now: t}
	// @ fold l.LockP()
	return l
}

// Now returns the current logical timestamp without advancing.
//
// Stream 3 Phase 1b: discharged via the LockP() permission predicate
// declared in clock.gobra. The mutex stub itself stays trusted (see
// stubs/sync/sync.gobra) — what's now formally reasoned is the resource
// the mutex protects, namely `l.now`. The unfold/fold pair around the
// l.now read makes the field access legal under Gobra's permission system.
// Wrapper-style: callers transfer the LockP token in and we hand it back.
// @ requires acc(l.LockP())
// @ ensures acc(l.LockP())
// @ ensures unfolding acc(l.LockP()) in result == l.now
func (l *Logical) Now() (result int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// @ unfold acc(l.LockP())
	result = l.now
	// @ fold acc(l.LockP())
	return result
}

// Tick advances the clock by 1.
//
// Stream 3 Phase 1b: discharged via LockP() (see Now() above for rationale).
// F7: Tick monotonically advances `now` by exactly 1.
// @ requires acc(l.LockP())
// @ ensures acc(l.LockP())
// @ ensures unfolding acc(l.LockP()) in l.now == old(unfolding acc(l.LockP()) in l.now) + 1
func (l *Logical) Tick() {
	l.mu.Lock()
	defer l.mu.Unlock()
	// @ unfold acc(l.LockP())
	l.now++
	// @ fold acc(l.LockP())
}

// SetNow sets the clock to `value`, but only if value > current. If
// value <= current, the call logs a warning and leaves the clock alone
// — F7 forbids the logical clock from going backwards.
//
// Used by the snapshot contract (Stream 2 Phase 0) to restore clock
// state on the shadow backend after a load-snapshot. TRUSTED: bypasses
// the Tick()-only mutation discipline. Inventoried in TCB.md.
//
// @ trusted
func (l *Logical) SetNow(value int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if value < l.now {
		log.Printf("clock.SetNow: ignoring backwards set (current=%d, requested=%d)", l.now, value)
		return
	}
	l.now = value
}
