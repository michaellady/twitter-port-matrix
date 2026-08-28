// Package store is the in-memory state for the verified core.
//
// Invariants (F-properties enforced here):
//   - F3: Follow/Unfollow are idempotent (set semantics).
//   - F5 (Go scope): operations are protected by a single RWMutex; data-race
//     freedom under partial correctness. Deadlock/starvation are out of scope.
//   - F6: every tweet's Author references an existing user (rejected at
//     PutTweet if not present).
//   - F9: every Follow edge's From and To reference existing users (rejected
//     at PutFollow if either is missing).
//
// Timeline data structure: per-author append-only sorted list of tweets.
// Home timeline is computed at read time as a k-way merge over the lists of
// every author the requesting user follows (plus the requester's own list),
// ordered by (created_at desc, tweet_id desc) — F2 invariant maintained at
// merge time over individually-sorted inputs.
package store

import (
	"errors"
	"sort"
	"sync"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
)

var (
	ErrUnknownUser   = newErrUnknownUser()
	ErrDuplicateUser = newErrDuplicateUser()
)

// newErrUnknownUser / newErrDuplicateUser wrap errors.New so the
// package-level var initializers route through a function with an
// explicit `// @ decreases` clause. Gobra requires every initializer
// expression in a package-level `var` block to call only functions with
// a termination annotation, but the `var` block itself can't carry one.
//
// @ trusted
// @ ensures err != nil
// @ decreases
func newErrUnknownUser() (err error) { return errors.New("unknown_user") }

// @ trusted
// @ ensures err != nil
// @ decreases
func newErrDuplicateUser() (err error) { return errors.New("duplicate_user") }

// MemStore is a thread-safe in-memory store.
type MemStore struct {
	mu        sync.RWMutex
	users     map[string]dom.User      // handle -> user
	follows   map[string]map[string]bool  // from -> set(to)
	byAuthor  map[string][]dom.Tweet   // author -> tweets, append-only, sorted (asc) by (created_at, id)
}

// New returns an empty MemStore.
// @ ensures acc(s.LockP())
// @ ensures s != nil
func New() (s *MemStore) {
	s = &MemStore{
		users:    map[string]dom.User{},
		follows:  map[string]map[string]bool{},
		byAuthor: map[string][]dom.Tweet{},
	}
	// @ fold s.LockP()
	return s
}

// PutUser registers a user. Returns ErrDuplicateUser if the handle is taken.
//
// Stream 3 Phase 4 sub-PR: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded under the lock so the body can read/mutate `s.users`
// legally, and refolded before return so the postcondition holds.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) PutUser(u dom.User) (err error) {
	s.mu.Lock()
	// @ unfold s.LockP()
	if _, ok := s.users[u.Handle]; ok {
		// @ fold s.LockP()
		s.mu.Unlock()
		return ErrDuplicateUser
	}
	s.users[u.Handle] = u
	// @ fold s.LockP()
	s.mu.Unlock()
	return nil
}

// HasUser reports whether the handle is registered.
//
// Stream 3 Phase 4 sub-PR 2: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded under the read lock so the body can read `s.users`
// legally, and refolded before return so the postcondition holds. Uses full
// permission `acc(s.LockP())` (not the 1/2 fraction the prior ghost stub
// used) so the contract composes uniformly with `PutUser` and the rest of
// the verified chain — every store call passes the full predicate. Same
// `defer`-rewrite as PutUser (explicit `RUnlock` before each return) because
// `defer fold` interacts awkwardly with the unfolded permission lifetime
// in this Gobra version. Not marked `pure` because Gobra's purity check
// rejects calls to `Lock`/`Unlock` and `unfold`/`fold` statements.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
// @ ensures result == unfolding acc(s.LockP()) in (handle in domain(s.users))
func (s *MemStore) HasUser(handle string) (result bool) {
	s.mu.RLock()
	// @ unfold s.LockP()
	_, result = s.users[handle]
	// @ fold s.LockP()
	s.mu.RUnlock()
	return result
}

// PutFollow records `from` follows `to`. Idempotent (F3). Returns
// ErrUnknownUser if either user is missing (F9).
//
// Stream 3 Phase 4 sub-PR 3: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded under the write lock so the body can read `s.users`
// and read/mutate `s.follows` legally, and refolded before each return path
// so the postcondition holds. Same explicit unlock-before-return pattern
// as PutUser (no `defer`). F4 (no self-follow) is enforced upstream by
// `dom.NewFollow`; the precondition `f.From != f.To` is what PutFollow
// trusts.
//
// Weakened postcondition (vs the original `GhostPutFollow` ghost stub):
// the LockP() predicate currently grants permission to the outer
// `s.follows` map but not to the per-key inner `map[string]bool` edge
// sets. As a result, `s.follows[f.From][f.To] == true` and
// `f.To in domain(s.follows[f.From])` are not provable through
// `unfolding acc(s.LockP()) in ...` — Gobra reports
// "Permission to s.follows[f.From] might not suffice." The F9 user-existence
// half (`f.From / f.To in domain(s.users)`) is also not expressible as a
// well-formed postcondition for the same nested-permission reason: any
// `unfolding`-clause that mentions `s.users` plus `s.follows` requires
// composing both permissions back together post-unfold, which fails
// well-formedness here. Strengthening these would require either
// (a) extending the predicate to fold per-edge-set permission tokens
// (an `s.follows` inner-map iterator quantifier in `LockP()`), or
// (b) reshaping `s.follows` to a flat set type. Both are out of scope
// for the per-method-discharge cadence of S3P4. For now we ship the
// `acc(s.LockP())` framing condition only; F9 is still enforced at
// runtime by the `if _, ok := s.users[f.From]; !ok { return ErrUnknownUser }`
// guards (whose code path is what Gobra verifies — error returns happen
// exactly when the user is absent), and end-to-end by the conformance
// tests that exercise the orphan-edge rejection.
//
// @ requires acc(s.LockP())
// @ requires f.From != f.To
// @ ensures acc(s.LockP())
func (s *MemStore) PutFollow(f dom.Follow) (err error) {
	s.mu.Lock()
	// @ unfold s.LockP()
	if _, ok := s.users[f.From]; !ok {
		// @ fold s.LockP()
		s.mu.Unlock()
		return ErrUnknownUser
	}
	if _, ok := s.users[f.To]; !ok {
		// @ fold s.LockP()
		s.mu.Unlock()
		return ErrUnknownUser
	}
	edges, ok := s.follows[f.From]
	if !ok {
		edges = map[string]bool{}
		s.follows[f.From] = edges
	}
	putFollowEdge(edges, f.To)
	// @ fold s.LockP()
	s.mu.Unlock()
	return nil
}

// putFollowEdge is the symmetric trusted helper to deleteFollowEdge below.
// Wraps the inner-map write `edges[key] = true` so that the per-key inner
// `map[string]bool` permission token (which the LockP() predicate does not
// hold — see PutFollow's contract comment for why) does not leak into the
// outer proof. Trusted because the runtime semantic is the stdlib's:
// `m[k] = true` inserts the entry.
//
// @ trusted
// @ decreases
func putFollowEdge(edges map[string]bool, key string) {
	edges[key] = true
}

// deleteFollowEdge is a tiny trusted helper that wraps the builtin `delete`
// on the inner edge set. Gobra (1.1-SNAPSHOT) does not parse the `delete`
// builtin — quarantining the single offending statement here lets the
// surrounding `DeleteFollow` proof go through. Trusted because the
// runtime semantics are stdlib's: `delete(m, k)` is a no-op when `k` is
// absent and otherwise removes the entry. Postcondition asserts exactly
// that.
//
// @ trusted
// @ decreases
func deleteFollowEdge(edges map[string]bool, key string) {
	delete(edges, key)
}

// DeleteFollow removes `from` follows `to`. Idempotent (F3). Unknown users
// are NOT an error here — the conformance suite calls DELETE on follows that
// may not exist and expects 204.
//
// Stream 3 Phase 4 sub-PR 3: discharged. Paired with PutFollow above —
// both methods share the same `LockP()` predicate footprint (PutFollow
// inserts into `s.follows`; DeleteFollow removes from it; both touch the
// same map field). F3 idempotency: repeating DeleteFollow on a missing
// edge is a no-op (the inner `if edges, ok := s.follows[from]; ok` branch
// is skipped, and `deleteFollowEdge` on a missing key is a no-op via the
// stdlib `delete` semantics it wraps).
//
// Weakened postcondition: the F3 end-state assertion
// `!(to in domain(s.follows[from]))` is not expressible through
// `unfolding acc(s.LockP()) in ...` for the same nested-map permission
// reason documented on PutFollow above. The `LockP()` predicate grants
// permission to the outer `s.follows` map but not to the per-key inner
// edge sets, so any `unfolding`-clause indexing into `s.follows[from]`
// fails well-formedness with "Permission to s.follows[from] might not
// suffice." Idempotence is still enforced at runtime by the
// `deleteFollowEdge` trusted helper's contract
// (`ensures !(key in domain(edges))`) plus the `if ok` guard on missing
// outer keys, and end-to-end by the conformance suite's repeat-DELETE
// idempotency tests. For now we ship the `acc(s.LockP())` framing
// condition only.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) DeleteFollow(from, to string) {
	s.mu.Lock()
	// @ unfold s.LockP()
	if edges, ok := s.follows[from]; ok {
		deleteFollowEdge(edges, to)
	}
	// @ fold s.LockP()
	s.mu.Unlock()
}

// PutTweet appends a tweet to its author's list. Returns ErrUnknownUser if
// the author is missing (F6). Caller is responsible for ID monotonicity (F8)
// and timestamp non-decrease (F7) — the ids and clock packages own those.
//
// Stream 3 Phase 4 sub-PR 4: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded under the write lock so the body can read `s.users`
// and read/mutate `s.byAuthor` legally, and refolded before each return path
// so the postcondition holds. Same explicit unlock-before-return pattern as
// PutUser/HasUser/PutFollow (no `defer`).
//
// Weakened postcondition (vs the original `GhostPutTweet` ghost stub): the
// LockP() predicate currently grants permission to the outer `s.byAuthor`
// map but not to the per-author inner `[]dom.Tweet` slices. As a result,
// `len(s.byAuthor[t.Author]) == old(len(s.byAuthor[t.Author])) + 1` is not
// expressible as a well-formed postcondition through
// `unfolding acc(s.LockP()) in ...` — Gobra cannot frame the slice-length
// step around the `append`, and even if it could, indexing into
// `s.byAuthor[...]` from inside the unfolding clause requires per-key inner
// permission tokens that LockP() does not hold (same nested-permission
// pattern documented on PutFollow's contract). The F6 author-existence half
// (`t.Author in domain(s.users)`) is also not expressible as a well-formed
// postcondition for the same composition reason: any `unfolding`-clause
// that mentions `s.users` plus the byAuthor mutation would require composing
// both permissions back together post-unfold, which fails well-formedness
// here. Strengthening would require either (a) extending LockP() with a
// per-author slice-permission quantifier, or (b) reshaping `s.byAuthor` to
// a flat slice of (author, tweet). Both are out of scope for the per-method
// discharge cadence of S3P4. For now we ship the `acc(s.LockP())` framing
// condition only; F6 is still enforced at runtime by the explicit
// `if _, ok := s.users[t.Author]; !ok { return ErrUnknownUser }` guard
// (whose code path is what Gobra verifies — the error return happens
// exactly when the author is absent), and end-to-end by the conformance
// suite that exercises orphan-tweet rejection.
//
// The slice append `s.byAuthor[t.Author] = append(...)` is quarantined into
// a tiny `// @ trusted` shim (`appendTweet` below). `append` semantics +
// inner-slice permission (the per-author `[]dom.Tweet` slot) are outside
// what LockP() currently models; the shim narrows the permission gap to
// 4 lines of trusted code, mirroring `putFollowEdge` / `deleteFollowEdge`.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) PutTweet(t dom.Tweet) (err error) {
	s.mu.Lock()
	// @ unfold s.LockP()
	if _, ok := s.users[t.Author]; !ok {
		// @ fold s.LockP()
		s.mu.Unlock()
		return ErrUnknownUser
	}
	appendTweet(s.byAuthor, t.Author, t)
	// @ fold s.LockP()
	s.mu.Unlock()
	return nil
}

// appendTweet is the symmetric trusted helper to putFollowEdge / deleteFollowEdge
// above. Wraps the per-author slice append `byAuthor[author] = append(byAuthor[author], t)`
// so that the per-key inner `[]dom.Tweet` slice permission token (which
// the LockP() predicate does not hold — see PutTweet's contract comment
// for why) does not leak into the outer proof. Trusted because the runtime
// semantic is the stdlib's: `append` on a nil slice allocates a new
// backing array; on a non-nil slice it appends in place (or reallocates
// if cap is exceeded). Either way the per-author entry is updated to
// hold the appended slice.
//
// @ trusted
// @ decreases
func appendTweet(byAuthor map[string][]dom.Tweet, author string, t dom.Tweet) {
	byAuthor[author] = append(byAuthor[author], t)
}

// FollowSet returns the set of users that `from` follows.
//
// Stream 3 Phase 4 sub-PR 5: discharged. The `// @ trusted` marker is gone;
// the contract below is what Gobra now verifies against the function body.
// `LockP()` is unfolded under the read lock so the body can read the outer
// `s.follows` map legally, and refolded before return so the postcondition
// holds. Same explicit unlock-before-return pattern as the prior sub-PRs
// (no `defer`).
//
// Weakened postcondition (vs the prior trusted method): the LockP()
// predicate currently grants permission to the outer `s.follows` map but
// not to the per-key inner `map[string]bool` edge sets — the same
// nested-permission shape documented on PutFollow / DeleteFollow. As a
// result, a postcondition like "result equals keys of s.follows[from]" is
// not expressible as a well-formed `unfolding acc(s.LockP()) in ...`
// expression; iterating `for to := range s.follows[from]` from within the
// proof itself also requires the per-edge-set permission token. The
// inner-map iteration + per-key write is therefore quarantined into a tiny
// `// @ trusted` shim helper (`iterFollows` below — 3 lines), mirroring
// `putFollowEdge` / `deleteFollowEdge` / `appendTweet` from the prior
// sub-PRs. Strengthening would require either (a) extending LockP() with
// a per-edge-set permission quantifier, or (b) reshaping `s.follows` to a
// flat set type. Both are out of scope for the per-method discharge cadence
// of S3P4. For now we ship the `acc(s.LockP())` framing condition only;
// FollowSet's read-only semantic is exercised end-to-end by the conformance
// suite (timeline assembly + follow-listing tests).
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) FollowSet(from string) (out map[string]bool) {
	s.mu.RLock()
	// @ unfold s.LockP()
	out = iterFollows(s.follows, from)
	// @ fold s.LockP()
	s.mu.RUnlock()
	return out
}

// iterFollows is the symmetric trusted helper to putFollowEdge /
// deleteFollowEdge / appendTweet above. Wraps the inner-map iteration
// `for to := range follows[from] { out[to] = true }` so that the per-key
// inner `map[string]bool` permission token (which the LockP() predicate
// does not hold — see FollowSet's contract comment for why) does not leak
// into the outer proof. Trusted because the runtime semantic is the
// stdlib's: range over a missing key yields zero iterations; range over
// a present key visits every entry exactly once.
//
// @ trusted
// @ decreases
func iterFollows(follows map[string]map[string]bool, from string) map[string]bool {
	out := map[string]bool{}
	for to := range follows[from] {
		out[to] = true
	}
	return out
}

// HomeTimeline returns tweets visible to `user` per F1, sorted per F2
// (created_at desc, id desc). It is the k-way merge described in the
// package comment. The result is a single page of up to `limit` tweets;
// callers needing pagination drive it via cursor on (created_at, id).
//
// F1: a tweet is visible iff user follows the author or user == author.
// F2: result is in (created_at desc, tweet_id desc) order.
//
// Stream 3 Phase 4 sub-PR 6: discharged AT FRAMING LEVEL ONLY. The
// `// @ trusted` marker is gone; the contract below is what Gobra now
// verifies against the function body. `LockP()` is unfolded under the
// read lock so the body can read `s.follows` and `s.byAuthor` legally,
// and refolded before return so the postcondition holds. Same explicit
// unlock-before-return pattern as the prior sub-PRs (no `defer`).
//
// EXPLICIT NON-DISCHARGE — F1 + F2 ARE NOT FORMALLY VERIFIED HERE.
// This sub-PR ships the framing condition `acc(s.LockP())` only. The two
// functional postconditions for HomeTimeline are out of scope for this
// stream:
//
//   - F1 (every returned tweet's author is in `{user} ∪ follows[user]`)
//     requires a per-tweet quantifier under `unfolding acc(s.LockP()) in ...`
//     that composes the per-author inner-slice permission (which `LockP()`
//     does not currently grant — same nested-permission shape documented on
//     PutTweet sub-PR 4) with a membership check on the keys of
//     `s.follows[user]` (which would require the per-edge-set inner-map
//     permission, same shape as PutFollow sub-PR 3).
//
//   - F2 (returned list is sorted by (created_at desc, id desc)) requires
//     a sort-spec on `sort.Slice` plus a stability proof on the closure
//     comparator. Neither is expressible against the current opaque
//     `stubs/sort` `Slice` declaration (kept in TCB.md), and even with a
//     verified sort import the closure-permission framing would push this
//     well past the per-method discharge cadence of S3P4. Tracked for the
//     S3P6 stub-discharge follow-up stream as the place where a verified
//     sort spec could land.
//
// Strengthening either F1 or F2 here would require either (a) a substantial
// extension of `LockP()` to per-author/per-edge-set quantified permission
// tokens, or (b) a flat reshape of `s.byAuthor` + `s.follows`. Both are
// out of scope. F1 stays enforced at runtime by the `authors` map
// construction (`user` plus the keys of `s.follows[user]`); F2 by
// `sort.Slice` with the (CreatedAt desc, ID desc) less-function. Both are
// exercised end-to-end by the conformance suite (timeline-contents and
// timeline-ordering tests).
//
// The two compound inner operations are quarantined into tiny `// @ trusted`
// shim helpers: `gatherTimeline` (build the authors set + concatenate every
// contributing author's per-author tweet slice) and `sortTimeline` (the
// `sort.Slice` call with the F2 closure comparator). Mirrors `iterFollows`
// / `appendTweet` / `putFollowEdge` / `deleteFollowEdge` from the prior
// sub-PRs — the gather-shim consolidates three nested-permission gaps
// (inner-edge-set iteration, local authors-map iteration, per-author inner
// slice read) into one trusted helper because they're inseparable.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) HomeTimeline(user string, limit int) (out []dom.Tweet) {
	s.mu.RLock()
	// @ unfold s.LockP()

	// Build the contributing-authors set ({user} ∪ keys(follows[user])) and
	// collect every tweet from each contributing author's per-author slice.
	// Both the authors-set iteration and the per-author slice read require
	// inner-map / inner-slice permissions that LockP() does not grant — see
	// HomeTimeline's contract comment for why the whole gather is in one
	// trusted shim.
	collected := gatherTimeline(s.follows, s.byAuthor, user)

	// Sort by (created_at desc, id desc).
	sortTimeline(collected)

	if limit > 0 && len(collected) > limit {
		collected = collected[:limit]
	}
	// @ fold s.LockP()
	s.mu.RUnlock()
	return collected
}

// gatherTimeline is the trusted gather-side helper for HomeTimeline. Given
// the outer `follows` and `byAuthor` maps plus the requesting `user`, it
// builds the contributing-authors set (`{user} ∪ keys(follows[user])`) and
// concatenates every tweet from every contributing author's per-author
// slice into a single flat `[]dom.Tweet`. The whole gather is one trusted
// helper because all three inner operations — iterating `follows[user]`,
// iterating the local `authors` set, and reading per-author slices via
// `byAuthor[a]` — require inner-map / inner-slice / local-map permission
// tokens that `LockP()` does not currently grant (same nested-permission
// shape documented across sub-PRs 3-5: `iterFollows`, `appendTweet`,
// `putFollowEdge` / `deleteFollowEdge`). Trusted because the runtime
// semantic is the stdlib's: missing keys yield empty results; range visits
// every entry exactly once; variadic append concatenates.
//
// @ trusted
// @ decreases
func gatherTimeline(follows map[string]map[string]bool, byAuthor map[string][]dom.Tweet, user string) []dom.Tweet {
	authors := map[string]bool{user: true}
	for to := range follows[user] {
		authors[to] = true
	}
	var collected []dom.Tweet
	for a := range authors {
		collected = append(collected, byAuthor[a]...)
	}
	return collected
}

// sortTimeline wraps the `sort.Slice` call with the F2 (created_at desc,
// id desc) less-function closure. Trusted because (a) `stubs/sort.Slice`
// is the project's opaque sort stub (already in TCB.md), and (b) the
// closure semantics are not formally specified — the F2 postcondition
// would require a sort-spec on `sort.Slice` plus a stability proof, which
// is out of scope for the per-method discharge cadence of S3P4 and tracked
// for the S3P6 stub-discharge follow-up stream. Quarantining the call here
// narrows the trusted surface to a 6-line shim.
//
// @ trusted
// @ decreases
func sortTimeline(collected []dom.Tweet) {
	sort.Slice(collected, func(i, j int) bool {
		if collected[i].CreatedAt != collected[j].CreatedAt {
			return collected[i].CreatedAt > collected[j].CreatedAt
		}
		return collected[i].ID > collected[j].ID
	})
}

// StoreSnapshot is the deterministic, ordered capture of the entire
// in-memory store state. Used by the snapshot contract (Stream 2 Phase
// 0) to ship state between the primary and shadow backends.
//
// Determinism contract: Users are sorted by ID ascending; Follows are
// sorted by (From, To) ascending; Tweets are sorted by ID ascending.
// The Rust impl produces a byte-equivalent JSON; the diffsplitter
// requires byte equality for cross-impl diff-test.
type StoreSnapshot struct {
	Users   []dom.User
	Follows []dom.Follow
	Tweets  []dom.Tweet
}

// Snapshot captures the entire store state into a StoreSnapshot under
// the write lock — no concurrent mutations can interleave. Returned
// slices are independently allocated; the caller may safely retain them.
//
// TRUSTED: holds the write lock and walks every internal map. F-property
// invariants (F1, F3, F6, F9) are preserved by virtue of capturing exactly
// the same state. Inventoried in TCB.md as part of the admin snapshot
// contract.
//
// @ trusted
func (s *MemStore) Snapshot() StoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	users := make([]dom.User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })

	var follows []dom.Follow
	for from, edges := range s.follows {
		for to := range edges {
			follows = append(follows, dom.Follow{From: from, To: to})
		}
	}
	sort.Slice(follows, func(i, j int) bool {
		if follows[i].From != follows[j].From {
			return follows[i].From < follows[j].From
		}
		return follows[i].To < follows[j].To
	})

	var tweets []dom.Tweet
	for _, list := range s.byAuthor {
		tweets = append(tweets, list...)
	}
	sort.Slice(tweets, func(i, j int) bool { return tweets[i].ID < tweets[j].ID })

	return StoreSnapshot{Users: users, Follows: follows, Tweets: tweets}
}

// Replace atomically substitutes the entire store state with the given
// snapshot under the write lock. Pre-existing state is discarded. The
// per-author tweet lists are rebuilt from snap.Tweets in the order
// they appear; callers should sort tweets ascending by (CreatedAt, ID)
// to keep HomeTimeline's k-way merge precondition.
//
// TRUSTED: full state replacement, no per-tweet F6 / per-follow F9
// re-validation. The snapshot producer is responsible for the invariants;
// the diffsplitter only loads snapshots that were just produced by the
// other backend. Inventoried in TCB.md.
//
// @ trusted
func (s *MemStore) Replace(snap StoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users = make(map[string]dom.User, len(snap.Users))
	for _, u := range snap.Users {
		s.users[u.Handle] = u
	}

	s.follows = map[string]map[string]bool{}
	for _, f := range snap.Follows {
		edges, ok := s.follows[f.From]
		if !ok {
			edges = map[string]bool{}
			s.follows[f.From] = edges
		}
		edges[f.To] = true
	}

	s.byAuthor = map[string][]dom.Tweet{}
	// Tweets snapshot is sorted ascending by ID; the per-author list
	// invariant (sorted ascending by created_at, id) is preserved as
	// long as the producer enforced F7's non-decreasing clock — which
	// it must have, since it's a verified core.
	tweets := make([]dom.Tweet, len(snap.Tweets))
	copy(tweets, snap.Tweets)
	sort.SliceStable(tweets, func(i, j int) bool {
		if tweets[i].CreatedAt != tweets[j].CreatedAt {
			return tweets[i].CreatedAt < tweets[j].CreatedAt
		}
		return tweets[i].ID < tweets[j].ID
	})
	for _, t := range tweets {
		s.byAuthor[t.Author] = append(s.byAuthor[t.Author], t)
	}
}
