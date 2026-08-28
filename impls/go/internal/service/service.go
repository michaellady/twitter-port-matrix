// Package service is the verified business logic layer.
//
// It composes clock, ids, domain, and store. The HTTP shim calls into this
// package and only this package.
//
// F-properties dispatched here:
//   - F1: HomeTimeline returns store.HomeTimeline (which maintains visibility)
//   - F2: result is sorted by (created_at desc, tweet_id desc)
//   - F4: PostFollow rejects self-follow via dom.NewFollow
//   - F7: PostTweet uses clock.Now() (which is non-decreasing)
//   - F8: PostTweet uses ids.Next() (strictly monotonic)
package service

import (
	"errors"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/ids"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/store"
)

var (
	ErrEmptyHandle = newErr("empty_handle")
	ErrEmptyText   = newErr("empty_text")
)

// newErr wraps errors.New so the package-level var initializers route
// through a function with an explicit `// @ decreases` clause. See the
// equivalent helpers in internal/store/memstore.go for the rationale.
//
// @ trusted
// @ ensures err != nil
// @ decreases
func newErr(s string) (err error) { return errors.New(s) }

// Service holds the verified core dependencies. All exported methods are
// safe for concurrent use (delegate to MemStore's RWMutex + the clock and
// ids generators' own locks).
type Service struct {
	clk    clock.Clock
	idsTw  *ids.Generator
	idsUsr *ids.Generator
	st     *store.MemStore
}

// New wires up a Service with the given clock and a fresh store + id generators.
//
// Stream 3 Phase 5: folds the new `LockP()` predicate so callers can pass
// the resulting `*Service` into the discharged methods (`Follow`,
// `HasUser`, `CreateUser`, `Tick`). The composed component predicates are
// folded by the underlying constructors (`ids.New()` and `store.New()`)
// and then bundled into `s.LockP()` here. The clock field is the
// interface-typed `clk` and only carries field-existence permission in
// `LockP()` — see service.gobra for why.
//
// @ ensures acc(s.LockP())
// @ ensures s != nil
func New(clk clock.Clock) (s *Service) {
	s = &Service{
		clk:    clk,
		idsTw:  ids.New(),
		idsUsr: ids.New(),
		st:     store.New(),
	}
	// @ fold s.LockP()
	return s
}

// CreateUser registers a new user. Empty handles are rejected.
//
// Stream 3 Phase 5: discharged. The `// @ trusted` marker is gone; the
// contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded so the body can access `s.idsUsr` (call the
// trusted `Next()`) and pass `s.st.LockP()` into the discharged
// `store.PutUser`. Refolded before each return path so the postcondition
// holds. Same explicit unlock-before-return pattern as the discharged
// store methods (no `defer fold`).
//
// Framing-only postcondition: the user-existence postcondition (e.g.
// `handle in domain(s.st.users)`) would require an `unfolding`-clause
// composing the Service-level `LockP()` with the inner `MemStore.LockP()`,
// which is the same nested-permission shape documented across the S3P4
// store sub-PRs. Strengthening would require either (a) extending Service's
// LockP() with an inner `unfolding`-friendly predicate over `s.st.users`,
// or (b) reshaping the store. Both are out of scope for the per-method
// discharge cadence of S3P5. F9 (no orphan users at creation) is not a
// thing — F9 forbids orphan-edge follow rows; CreateUser's only invariant
// is empty-handle rejection, which is enforced at runtime by the explicit
// `if handle == "" { return ErrEmptyHandle }` guard.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *Service) CreateUser(handle string) (u dom.User, err error) {
	if handle == "" {
		return dom.User{}, ErrEmptyHandle
	}
	// @ unfold s.LockP()
	u = dom.User{ID: s.idsUsr.Next(), Handle: handle}
	err = s.st.PutUser(u)
	// @ fold s.LockP()
	if err != nil {
		return dom.User{}, err
	}
	return u, nil
}

// HasUser reports user existence (used by the HTTP shim for validation).
//
// Stream 3 Phase 5: discharged. Pure delegate to the discharged
// `store.HasUser`. `LockP()` is unfolded so we can pass `s.st.LockP()`
// into the call, then refolded before return. Framing-only postcondition
// — the bool-result postcondition would require composing the Service-
// level `LockP()` with the inner `MemStore.LockP()` `unfolding`-clause
// from `store.HasUser`'s contract, which is the same nested-permission
// shape documented across the S3P4 store sub-PRs.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *Service) HasUser(handle string) (result bool) {
	// @ unfold s.LockP()
	result = s.st.HasUser(handle)
	// @ fold s.LockP()
	return result
}

// Follow records a follow edge. F4 rejects self-follow; F9 rejects unknown users.
//
// Stream 3 Phase 5: discharged. Composition-of-proofs sub-PR — F4 is
// dispatched by the discharged `dom.NewFollow` precondition (which
// guarantees `f.From != f.To` on the success path), and the framing
// permission for the store mutation is handed off to the discharged
// `store.PutFollow` which carries the `f.From != f.To` precondition.
// `LockP()` is unfolded to expose `acc(s.st.LockP())` for the inner
// PutFollow call, then refolded before each return. F4 (no self-follow)
// is enforced at construction time by `dom.NewFollow`'s contract; F9
// (no orphan-edge users) stays runtime-enforced by `store.PutFollow`'s
// internal user-existence guards (the framing-only store contract
// documented in S3P4 sub-PR 3).
//
// @ requires acc(s.LockP())
// @ requires isComparable(dom.ErrSelfFollow)
// @ ensures acc(s.LockP())
func (s *Service) Follow(from, to string) (err error) {
	if from == to {
		// F4 short-circuit: dom.NewFollow would return ErrSelfFollow on
		// this path; we mirror that here so the from != to fact is
		// available syntactically for the rest of the body. Identical
		// runtime behavior — Follow has no other side effects on the
		// self-follow path either way.
		_, derr := dom.NewFollow(from, to)
		return derr
	}
	f, derr := dom.NewFollow(from, to)
	if derr != nil {
		return derr
	}
	// from != to is in scope here, and dom.NewFollow's success-path
	// postcondition guarantees `f.From == from && f.To == to`, which
	// composes to `f.From != f.To` — exactly the precondition
	// store.PutFollow requires.
	// @ unfold s.LockP()
	err = s.st.PutFollow(f)
	// @ fold s.LockP()
	return err
}

// Unfollow removes a follow edge. Idempotent (F3): unknown users are NOT an
// error here per the conformance suite — DELETE on a non-existent edge is 204.
// We still validate the users exist so we don't silently swallow a typo.
//
// Stream 3 Phase 5 sub-PR 2: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded so the body can pass `s.st.LockP()` into three
// sequential discharged store-method calls (`HasUser` × 2 + `DeleteFollow`),
// each of which takes and returns `acc(s.st.LockP())` — the predicate
// roundtrips through every call so a single unfold/fold pair frames the
// whole composition. Refolded before each return path so the postcondition
// holds. Same explicit unlock-before-return pattern as the prior S3P5
// sub-PR (no `defer fold`).
//
// Framing-only postcondition: the F3 idempotency end-state assertion
// (`!(to in domain(s.st.follows[from]))`) is not expressible through
// `unfolding acc(s.LockP()) in unfolding acc(s.st.LockP()) in ...` for the
// same nested-permission reason documented across the S3P4 store sub-PRs
// (the inner edge-set permission is not held by `MemStore.LockP()`).
// Strengthening would require either (a) extending the predicate stack
// with per-edge-set permission, or (b) reshaping `s.follows`. Both are
// out of scope for the per-method discharge cadence. F3 idempotency
// stays runtime-enforced by the discharged `(*MemStore).DeleteFollow`'s
// `if ok` guard + `deleteFollowEdge` shim semantics.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *Service) Unfollow(from, to string) (err error) {
	// @ unfold s.LockP()
	if !s.st.HasUser(from) || !s.st.HasUser(to) {
		// @ fold s.LockP()
		return store.ErrUnknownUser
	}
	s.st.DeleteFollow(from, to)
	// @ fold s.LockP()
	return nil
}

// PostTweet posts a tweet. F6 rejects unknown authors; F7 stamps with the
// current clock; F8 issues a fresh strictly-monotonic ID.
//
// Stream 3 Phase 5 sub-PR 2: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded so the body can pass `s.st.LockP()` into the
// discharged store-method calls (`HasUser` + `PutTweet`) and access the
// trusted `s.idsTw.Next()` (no LockP requirement) and the interface-typed
// `s.clk` (via the `nowLogical` shim — see below). Refolded before each
// return path so the postcondition holds. Same explicit unlock-before-return
// pattern as `Unfollow` and the prior S3P5 sub-PR.
//
// The `s.clk.Now()` call is delegated to a tiny `// @ trusted` `nowLogical`
// shim (mirrors the `tickLogical` interface-dispatch shim from the prior
// S3P5 sub-PR). Gobra cannot fold `(*clock.Logical).LockP()` through the
// `clock.Clock` interface field, so threading `acc(c.LockP())` into the
// asserted concrete pointer's discharged `Now()` call is not expressible
// at the call site. Quarantining the dispatch into 4 lines of trusted code
// mirrors the inner-permission shim pattern from S3P4
// (`putFollowEdge` / `appendTweet` / `iterFollows`) and S3P5 sub-PR 1
// (`tickLogical`). The underlying `(*Logical).Now()` is ALREADY discharged
// in S3P1b — the shim is the framing wrapper that bridges the
// interface-LockP() gap only — no F7 obligation is delegated through it.
//
// Framing-only postcondition: the F6 author-existence assertion would
// require composing the Service-level `LockP()` with the inner
// `MemStore.LockP()` `unfolding`-clause from `(*MemStore).PutTweet`'s
// contract — the same nested-permission shape documented across the S3P4
// store sub-PRs. F6 stays runtime-enforced by the explicit
// `if !s.st.HasUser(author) { return ErrUnknownUser }` guard plus the
// matching `(*MemStore).PutTweet` author-existence guard (whose error-return
// code path is what Gobra verifies). F7 stays runtime-enforced by the
// discharged `(*Logical).Now()` (S3P1b) wrapped through `nowLogical`. F8
// stays runtime-enforced by the trusted `(*Generator).Next()` (which has no
// LockP requirement at the call site).
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *Service) PostTweet(author, text string) (t dom.Tweet, err error) {
	if text == "" {
		return dom.Tweet{}, ErrEmptyText
	}
	// @ unfold s.LockP()
	if !s.st.HasUser(author) {
		// @ fold s.LockP()
		return dom.Tweet{}, store.ErrUnknownUser
	}
	t = dom.Tweet{
		ID:        s.idsTw.Next(),
		Author:    author,
		Text:      text,
		CreatedAt: nowLogical(s.clk),
	}
	err = s.st.PutTweet(t)
	// @ fold s.LockP()
	if err != nil {
		return dom.Tweet{}, err
	}
	return t, nil
}

// HomeTimeline returns the current timeline page for `user`. F1 visibility +
// F2 ordering enforced by the store.
// @ trusted
func (s *Service) HomeTimeline(user string, limit int) []dom.Tweet {
	return s.st.HomeTimeline(user, limit)
}

// Tick advances the clock — used by the conformance harness between steps to
// produce the deterministic timestamps the spec encodes.
//
// Stream 3 Phase 5: discharged AT FRAMING LEVEL ONLY. The interface-typed
// `s.clk` field carries only field-existence permission in `LockP()` —
// Gobra cannot fold the concrete `*clock.Logical.LockP()` predicate
// through the `clock.Clock` interface, so the runtime type-assertion
// `s.clk.(*clock.Logical)` and the resulting `Tick()` call are
// quarantined into the tiny `// @ trusted` `tickLogical` shim helper
// (mirrors the inner-permission shim pattern used by S3P4's
// `putFollowEdge` / `appendTweet` / `iterFollows`). The shim's runtime
// semantic is the stdlib's: a non-`*Logical` concrete clock yields a
// no-op (the type assertion fails, the if-branch is skipped). The
// underlying `(*Logical).Tick()` is ALREADY discharged in S3P1b, so the
// shim is the framing wrapper that bridges the interface-LockP() gap
// only — no F7 obligation is delegated through it.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *Service) Tick() {
	// @ unfold s.LockP()
	tickLogical(s.clk)
	// @ fold s.LockP()
}

// tickLogical is the trusted interface-dispatch shim for `Service.Tick`.
// Performs the runtime type-assertion `clk.(*clock.Logical)` and calls
// `Tick()` on the concrete pointer. Trusted because Gobra cannot fold
// `(*clock.Logical).LockP()` through the `clock.Clock` interface field —
// the asserted concrete pointer's predicate state is not visible to the
// verifier through the interface dispatch, so threading
// `acc(t.LockP())` into `(*clock.Logical).Tick()` is not expressible
// here. Quarantining the dispatch into 3 lines of trusted code mirrors
// the inner-permission shim pattern used by S3P4's `putFollowEdge` /
// `appendTweet` / `iterFollows`. Runtime semantic is the stdlib's:
// non-`*Logical` concrete clock yields a no-op (assertion fails →
// if-branch skipped); `*Logical` yields a discharged `Tick()` call
// (the F7 proof was already landed in S3P1b).
//
// @ trusted
// @ decreases
func tickLogical(clk clock.Clock) {
	if t, ok := clk.(*clock.Logical); ok {
		t.Tick()
	}
}

// nowLogical is the trusted interface-dispatch shim for `Service.PostTweet`'s
// `s.clk.Now()` call (mirrors `tickLogical` above). Performs the runtime
// type-assertion `clk.(*clock.Logical)` and calls `Now()` on the concrete
// pointer. Trusted because Gobra cannot fold `(*clock.Logical).LockP()`
// through the `clock.Clock` interface field — the asserted concrete
// pointer's predicate state is not visible to the verifier through the
// interface dispatch, so threading `acc(c.LockP())` into
// `(*clock.Logical).Now()` is not expressible here. Quarantining the
// dispatch into 4 lines of trusted code mirrors the inner-permission shim
// pattern used by S3P4's `putFollowEdge` / `appendTweet` / `iterFollows`
// and the prior S3P5 sub-PR's `tickLogical`. Runtime semantic is the
// stdlib's: non-`*Logical` concrete clock falls through to `clk.Now()`
// (the interface dispatch resolves to the concrete implementation's
// `Now()`); `*Logical` yields a discharged `Now()` call (the F7 proof
// was already landed in S3P1b).
//
// @ trusted
// @ decreases
func nowLogical(clk clock.Clock) int64 {
	if c, ok := clk.(*clock.Logical); ok {
		return c.Now()
	}
	return clk.Now()
}

// Snapshot captures the full Service state — store + ID counters + clock
// — for the Stream 2 Phase 0 admin snapshot endpoint. TRUSTED: bypasses
// the verified write path; orchestrates the trusted Snapshot/Current/Now
// methods of the underlying primitives. Inventoried in TCB.md.
//
// @ trusted
func (s *Service) Snapshot() ServiceSnapshot {
	return ServiceSnapshot{
		Store:           s.st.Snapshot(),
		IDCounterUsers:  s.idsUsr.Current(),
		IDCounterTweets: s.idsTw.Current(),
		ClockNow:        s.clk.Now(),
	}
}

// LoadSnapshot atomically replaces the full Service state with the given
// snapshot. Mirror of Snapshot(). TRUSTED: full state replacement, no
// invariant re-check (the producer was a verified core). Inventoried in
// TCB.md.
//
// The clock is moved forward only — if snap.ClockNow is older than the
// current clock value, clock.SetNow logs a warning and leaves the clock
// alone (F7 forbids backwards motion).
//
// @ trusted
func (s *Service) LoadSnapshot(snap ServiceSnapshot) {
	s.st.Replace(snap.Store)
	s.idsUsr.SetCurrent(snap.IDCounterUsers)
	s.idsTw.SetCurrent(snap.IDCounterTweets)
	if l, ok := s.clk.(*clock.Logical); ok {
		l.SetNow(snap.ClockNow)
	}
}

// ServiceSnapshot is the Service-level view of a snapshot — store +
// generator counters + clock. Marshaled by the admin handler into the
// versioned JSON envelope.
type ServiceSnapshot struct {
	Store           store.StoreSnapshot
	IDCounterUsers  int64
	IDCounterTweets int64
	ClockNow        int64
}
