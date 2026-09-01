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
	"sync"

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
// safe for concurrent use.
//
// F018: the per-component locks are NOT sufficient on their own. Each of
// `clock.Logical`, `ids.Generator` and `store.MemStore` protects its own
// state, so every individual step is atomic -- and the compound operations
// this layer builds out of them were not. `PostTweet` took an id, read the
// clock, and appended, as three separately-atomic steps; two goroutines
// holding ids 5 and 6 could reach the append in the opposite order, and
// `MemStore.PutTweet`'s monotonicity guard rejected the one holding 5 for
// being out of order relative to a tweet that only existed because it lost
// the race. `ErrNonMonotonic` is not in `writeErrFromDomain`'s table, so the
// client saw HTTP 500 and the tweet was gone.
//
// `wmu` closes that gap: it makes the ALLOCATE-then-WRITE sequences atomic as
// a whole, which removes the interleaving rather than detecting it. It is
// held by exactly the two methods that allocate an id -- `CreateUser` and
// `PostTweet` -- and by nothing else, so reads and the two follow-edge
// mutators are unaffected. Lock order is always `wmu` before any component
// lock, and no component ever calls back into `Service`, so there is no
// cycle to deadlock on.
type Service struct {
	// wmu serialises the compound allocate-then-write operations. See the
	// type comment; F018 is the finding that made it necessary.
	wmu    sync.Mutex
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
// R5 (init commutation) at the service boundary.
//
// @ ensures acc(s.LockP())
// @ ensures s != nil
// @ ensures s.AbsUsers() == set[string]{}
// @ ensures s.AbsFollows() == set[dom.Follow]{}
// @ ensures s.AbsLogLen() == 0
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
// R5 at the service boundary (users axis). What is proved, and what is NOT,
// is spelled out because the difference is the whole content of the rung.
//
// PROVED: the other two axes of abs do not move; at most the requested handle
// is added and nothing else is (no fabrication of users); and a request for an
// already-registered handle leaves abs exactly where it was.
//
// NOT PROVED, and this is a hard ceiling rather than a missing lemma: the
// ACCEPT direction ("a syntactically valid, unregistered handle IS added").
// Stating it needs `dom.ValidHandle` inside a specification.
//
// MISSING CAPABILITY: Gobra 1.1-SNAPSHOT has no string indexing in the ghost
// language, so a handle-syntax predicate cannot be written as a `pure`
// function at all -- `dom.ValidHandle` is a byte loop with early returns, and
// Gobra `pure` functions must be a single expression. `dom.ValidHandle`'s own
// postcondition already records the consequence: it exports only the length
// half ("result ==> len in range"), because the alphabet half "cannot be
// restated over `h` in the postcondition, since that needs string indexing".
// S_obs's Step branches on handle and text syntax on every route, so the
// refinement obligation is statable only on the sub-alphabet where syntax is
// already settled.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
// @ ensures s.AbsFollows() == old(s.AbsFollows())
// @ ensures s.AbsLogLen() == old(s.AbsLogLen())
// @ ensures s.AbsUsers() == old(s.AbsUsers()) ||
// @            s.AbsUsers() == old(s.AbsUsers()) union set[string]{handle}
// @ ensures (handle in old(s.AbsUsers())) ==> s.AbsUsers() == old(s.AbsUsers())
func (s *Service) CreateUser(handle string) (u dom.User, err error) {
	// Syntax before existence (D6). The bound and the alphabet are part of
	// the observable contract, so they live here in the verified core rather
	// than in the HTTP shim, which Gobra does not verify.
	if !dom.ValidHandle(handle) {
		return dom.User{}, dom.ErrInvalidHandle
	}
	// F018. The existence check, the id allocation and the write are one
	// atomic operation, not three. See the `Service` type comment.
	s.wmu.Lock()
	defer s.wmu.Unlock()
	// @ unfold s.LockP()
	// Reject a duplicate BEFORE consuming an id. The previous order allocated
	// first and rejected second, so every rejected registration burned a user
	// id -- visible in the R0 baseline as {"handle":"Alice","id":5} after four
	// rejections. S_obs allocates only on success.
	//
	// Until F018 this check could still lose the race to PutUser under
	// concurrent registration of the same handle, and the loser had already
	// consumed an id by the time it found out. Uniqueness was never at risk --
	// PutUser is authoritative -- but the id gap it left IS observable on the
	// wire, and `S_obs`, which allocates only on success, cannot produce one.
	// `wmu` removes the window; `internal/service/concurrency_test.go` is the
	// regression test.
	if s.st.HasUser(handle) {
		// @ fold s.LockP()
		return dom.User{}, store.ErrHandleTaken
	}
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
// R5 at the service boundary (follows axis). F4 and F9 stop being separate
// property claims here and become clauses about abs: a self-follow and a
// follow naming an unregistered endpoint both leave the abstract state exactly
// where it was, and no request can add any edge other than the one it named.
//
// The accept direction is again out of reach, for the reason recorded on
// CreateUser: it is guarded by `dom.ValidHandle`, which Gobra cannot express.
//
// @ requires acc(s.LockP())
// @ requires isComparable(dom.ErrSelfFollow)
// @ ensures acc(s.LockP())
// @ ensures s.AbsUsers() == old(s.AbsUsers())
// @ ensures s.AbsLogLen() == old(s.AbsLogLen())
// @ ensures s.AbsFollows() == old(s.AbsFollows()) ||
// @            s.AbsFollows() == old(s.AbsFollows()) union set[dom.Follow]{dom.Follow{From: from, To: to}}
// @ ensures from == to ==> s.AbsFollows() == old(s.AbsFollows())
// @ ensures !(from in old(s.AbsUsers())) ==> s.AbsFollows() == old(s.AbsFollows())
// @ ensures !(to in old(s.AbsUsers())) ==> s.AbsFollows() == old(s.AbsFollows())
func (s *Service) Follow(from, to string) (err error) {
	// S_obs decision D4: EXISTENCE IS CHECKED BEFORE SEMANTICS.
	//
	// twitter.tla's Follow is an unordered conjunction
	// (a in knownUsers /\ b in knownUsers /\ a # b), so the model does not
	// say which error follow(eve, eve) yields when eve is unknown. This code
	// previously tested from == to first and answered self_follow_forbidden;
	// S_obs answers unknown_user. Both refine the model, TLC accepts either,
	// and either would prove F4 under Gobra -- yet one request tells them
	// apart. See evidence/findings/F003.
	if !dom.ValidHandle(from) || !dom.ValidHandle(to) {
		return dom.ErrInvalidHandle
	}
	// @ unfold s.LockP()
	if !s.st.HasUser(from) || !s.st.HasUser(to) {
		// @ fold s.LockP()
		return store.ErrUnknownUser
	}
	// @ fold s.LockP()
	// D4 order is unchanged: existence above, self-follow here. The test is
	// written out rather than being read back off `dom.NewFollow`'s error,
	// because concluding `from != to` from `derr == nil` needs
	// `dom.ErrSelfFollow != nil`, and Gobra does not carry a package-level
	// `var`'s initializer postcondition into a method body (blocker B2 in
	// spec/refinement/OBLIGATION.md). Same value returned on the same input;
	// `dom.NewFollow` still runs and still owns F4.
	if from == to {
		return dom.ErrSelfFollow
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
// R5 at the service boundary (follows axis). Same shape as Follow: the
// removal direction is guarded by `dom.ValidHandle` and so is out of reach,
// but "this request removes at most the edge it named, and an unknown endpoint
// removes nothing" is proved.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
// @ ensures s.AbsUsers() == old(s.AbsUsers())
// @ ensures s.AbsLogLen() == old(s.AbsLogLen())
// @ ensures s.AbsFollows() == old(s.AbsFollows()) ||
// @            s.AbsFollows() == old(s.AbsFollows()) setminus set[dom.Follow]{dom.Follow{From: from, To: to}}
// @ ensures !(from in old(s.AbsUsers())) ==> s.AbsFollows() == old(s.AbsFollows())
// @ ensures !(to in old(s.AbsUsers())) ==> s.AbsFollows() == old(s.AbsFollows())
func (s *Service) Unfollow(from, to string) (err error) {
	// @ unfold s.LockP()
	// Syntax before existence (D6). Self-unfollow of a known user is a legal
	// no-op: twitter.tla's Unfollow requires a,b in knownUsers but, unlike
	// Follow, does NOT require a # b. See S_obs decision D5.
	if !dom.ValidHandle(from) || !dom.ValidHandle(to) {
		// @ fold s.LockP()
		return dom.ErrInvalidHandle
	}
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
// R5 at the service boundary (log axis). The append-only clause is the one
// worth reading: nothing already in the log can be rewritten by a post. That
// is the premise the timeline's ordering proof rests on, stated where a
// request can reach it. F6 (no orphan tweets) becomes the clause that an
// unregistered author leaves the log length alone.
//
// The accept direction is out of reach for two separate reasons here: the
// `dom.ValidHandle` / `dom.ValidText` guards, and the monotonicity guard
// inside `(*MemStore).PutTweet`, which needs F8 (ids strictly increase) and F7
// (the clock never goes backwards) to be discharged through
// `(*Generator).Next` and the `nowLogical` interface shim -- both of which are
// still `// @ trusted`. See the note below on the composition obligation.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
// @ ensures s.AbsUsers() == old(s.AbsUsers())
// @ ensures s.AbsFollows() == old(s.AbsFollows())
// @ ensures s.AbsLogLen() == old(s.AbsLogLen()) ||
// @            s.AbsLogLen() == old(s.AbsLogLen()) + 1
// @ ensures !(author in old(s.AbsUsers())) ==> s.AbsLogLen() == old(s.AbsLogLen())
// @ ensures forall k int :: 0 <= k && k < old(s.AbsLogLen()) ==>
// @            s.AbsLogAt(k) == old(s.AbsLogAt(k))
func (s *Service) PostTweet(author, text string) (t dom.Tweet, err error) {
	// Syntax before existence, uniformly (D6).
	if !dom.ValidHandle(author) {
		return dom.Tweet{}, dom.ErrInvalidHandle
	}
	if !dom.ValidText(text) {
		return dom.Tweet{}, dom.ErrInvalidText
	}
	// F018. Taking the id, reading the clock and appending are one atomic
	// operation, not three. Both premises of the store's monotonicity lemma
	// are about the ORDER OF APPENDS, and holding `wmu` across all three steps
	// is what makes the order they are allocated in the order they land in.
	// See the `Service` type comment; `internal/httpshim/concurrency_test.go`
	// is the regression test.
	s.wmu.Lock()
	defer s.wmu.Unlock()
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

// HomeTimeline returns one page of the timeline for `user`, newest first.
//
// F1 (visibility) and F2 (ordering) are discharged in the store by the
// append-log reshape rather than delegated to a trusted sort -- see the
// monotonicity lemma in package store. This method is now a plain forwarding
// call with a real contract, so the `// @ trusted` marker is gone.
//
// cursor is exclusive: only ids strictly below it are returned. cursor <= 0
// starts from the newest. `more` reports whether a further visible tweet
// exists below the page, which is what lets the caller distinguish
// "next_cursor: null means nothing remains" from "the page happened to fill".
//
// The `unfold`/`fold` pair is what hands `acc(s.st.LockP())` to the store
// method and takes it back; without it Gobra reports "Permission to s.st
// might not suffice" at the call. The result-permission and length clauses
// are forwarded verbatim from store.HomeTimeline's contract, so this really
// is a pass-through at the level of the proof and not only of the code.
//
// The F2 ordering clause is forwarded verbatim from store.HomeTimeline, so
// the property survives the layer boundary instead of being re-asserted on
// trust. This is the first time the service layer exports F2 as a proved
// postcondition rather than a comment.
//
// @ requires acc(s.LockP())
// @ requires limit > 0
// @ ensures acc(s.LockP())
// @ ensures acc(out)
// @ ensures len(out) <= limit
// R5 at the service boundary (timeline response commutation). Every clause
// below is forwarded verbatim from the store's contract, so the property
// survives the layer boundary as a proof rather than as a comment. Together
// they are the response half of the obligation for GET /timeline: the page is
// ordered (F2), visible (F1), under the cursor (D10), fabricates nothing, and
// -- when the walk reached the bottom of the log -- loses nothing.
//
// @ ensures forall a, b int :: 0 <= a && a < b && b < len(out) ==>
// @            (out[a].CreatedAt > out[b].CreatedAt ||
// @             (out[a].CreatedAt == out[b].CreatedAt && out[a].ID > out[b].ID))
// @ ensures forall a int :: 0 <= a && a < len(out) ==>
// @            (out[a].Author == user ||
// @             (dom.Follow{From: user, To: out[a].Author} in s.AbsFollows()))
// @ ensures cursor > 0 ==> forall a int :: 0 <= a && a < len(out) ==> out[a].ID < cursor
// NOT FORWARDED, and the reason is a tool gap rather than a false claim: the
// two clauses of the store contract that are existentially quantified -- "no
// fabrication" (every returned tweet is some log entry) and "no loss" (every
// visible entry under the cursor is on the page, when the walk reached the
// bottom). Both are DISCHARGED at `(*MemStore).HomeTimeline`. Neither survives
// the call.
//
// MISSING CAPABILITY: Gobra 1.1-SNAPSHOT does not re-instantiate an
// existentially-quantified postcondition at a call site. Restating the store's
// own clause verbatim on the line after the call -- with the inner predicate
// still unfolded, in the store's own vocabulary -- fails with "Assert might
// fail". The usual remedy is to export the witness as a ghost result, and that
// is not available here: ghost parameters must appear in the signature, and
// this file has to compile under plain `go build`.
//
// So the timeline's response commutation is proved where the page is BUILT and
// is one layer short of where the request ARRIVES.
// @ ensures s.AbsUsers() == old(s.AbsUsers())
// @ ensures s.AbsFollows() == old(s.AbsFollows())
// @ ensures s.AbsLogLen() == old(s.AbsLogLen())
func (s *Service) HomeTimeline(user string, limit int, cursor int64) (out []dom.Tweet, more bool) {
	// @ unfold s.LockP()
	out, more = s.st.HomeTimeline(user, limit, cursor)
	// @ fold s.LockP()
	return out, more
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

// Now returns the current logical timestamp without advancing it.
//
// Exported so the tick route can report the clock it just advanced. Read-only:
// there is deliberately no exported way to SET the clock, because that is the
// capability that made the shared conformance corpus unfalsifiable on
// created_at (finding F001).
//
// Same framing point as HomeTimeline above: `s.clk` is reachable only after
// unfolding LockP(), so the read is bracketed rather than returned directly.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *Service) Now() (result int64) {
	// @ unfold s.LockP()
	result = nowLogical(s.clk)
	// @ fold s.LockP()
	return result
}
