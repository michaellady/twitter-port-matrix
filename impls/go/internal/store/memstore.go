// Package store is the in-memory state of the verified core.
//
// # The flat reshape (S_obs decision D9, finding F004)
//
// This package previously held two nested containers:
//
//	follows  map[string]map[string]bool   // from -> set(to)
//	byAuthor map[string][]dom.Tweet       // author -> per-author tweets
//
// Every operation that had to reach *inside* one of them needed inner-map or
// inner-slice permission that the LockP() predicate does not grant, so six
// operations were quarantined into `// @ trusted` shims: putFollowEdge,
// deleteFollowEdge, appendTweet, iterFollows, gatherTimeline and
// sortTimeline. Quarantining shrinks the trusted surface; it does not
// discharge anything.
//
// The upstream contract comment on HomeTimeline named the way out and then
// scoped it out: "Strengthening either F1 or F2 here would require either
// (a) a substantial extension of LockP() to per-author/per-edge-set
// quantified permission tokens, or (b) a flat reshape of s.byAuthor +
// s.follows."
//
// This file takes option (b). Both containers are now single-level:
//
//	follows map[dom.Follow]bool   // the edge IS the key
//	tweets  []dom.Tweet           // ONE append-ordered log, never sorted
//
// All six shims are gone, because the shape they were quarantining no longer
// exists. There is no inner map to reach into and no sort to specify.
//
// # Why the log is never sorted
//
// F2 asks for (created_at desc, id desc). Rather than sort and then owe a
// sort specification plus a stability proof, the order is made a consequence
// of the data structure:
//
//	MONOTONICITY LEMMA. For log positions i < j:
//	  tweets[i].ID < tweets[j].ID           ids are allocated monotonically
//	  tweets[i].CreatedAt <= tweets[j].CreatedAt   the clock never decreases
//
//	Therefore reverse iteration over the log IS descending lexicographic
//	(created_at, id): if the timestamps differ the later post has the larger
//	timestamp and comes first; if they tie the later post has the larger id
//	and comes first. Either way j precedes i.
//
// F2 is thus derived from an append-only invariant that PutTweet maintains
// locally, rather than proved about an opaque library sort. The two premises
// are the only obligations, and each is one line.
package store

import (
	"errors"
	"sort"
	"sync"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
)

// Error vocabulary. These are the wire codes from S_obs, not internal names:
// the observable contract fixes them, so the store speaks them directly
// rather than having the shim translate (a translation layer is somewhere a
// renaming can drift).
var (
	ErrUnknownUser = newErrUnknownUser()
	ErrHandleTaken = newErrHandleTaken()
	// ErrNonMonotonic rejects an append that would break the log invariant.
	ErrNonMonotonic = newErrNonMonotonic()
)

// @ trusted
// @ decreases
func newErrUnknownUser() (err error) { return errors.New("unknown_user") }

// @ trusted
// @ decreases
func newErrHandleTaken() (err error) { return errors.New("handle_taken") }

// @ trusted
// @ decreases
func newErrNonMonotonic() (err error) { return errors.New("non_monotonic_append") }

// MemStore holds all state. Both containers are single-level by design; see
// the package comment.
type MemStore struct {
	mu      sync.RWMutex
	users   map[string]dom.User // handle -> user
	follows map[dom.Follow]bool // the edge is the key; no nested map
	tweets  []dom.Tweet         // ONE append-ordered log; never sorted
}

// New returns an empty store.
//
// @ ensures acc(s.LockP())
// @ ensures s != nil
func New() (s *MemStore) {
	s = &MemStore{
		users:   map[string]dom.User{},
		follows: map[dom.Follow]bool{},
		tweets:  nil,
	}
	// @ fold s.LockP()
	return s
}

// PutUser registers a user, rejecting a duplicate handle.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) PutUser(u dom.User) (err error) {
	s.mu.Lock()
	// @ unfold acc(s.LockP())
	if _, ok := s.users[u.Handle]; ok {
		// @ fold acc(s.LockP())
		s.mu.Unlock()
		return ErrHandleTaken
	}
	s.users[u.Handle] = u
	// @ fold acc(s.LockP())
	s.mu.Unlock()
	return nil
}

// HasUser reports whether a handle is registered.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) HasUser(handle string) (result bool) {
	s.mu.RLock()
	// @ unfold acc(s.LockP())
	_, result = s.users[handle]
	// @ fold acc(s.LockP())
	s.mu.RUnlock()
	return result
}

// PutFollow adds an edge. Idempotent (F3): re-adding leaves the set
// unchanged. F9 holds by the two existence checks.
//
// The edge insert is now a plain single-level map write. The putFollowEdge
// trusted shim is gone.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) PutFollow(f dom.Follow) (err error) {
	s.mu.Lock()
	// @ unfold acc(s.LockP())
	if _, ok := s.users[f.From]; !ok {
		// @ fold acc(s.LockP())
		s.mu.Unlock()
		return ErrUnknownUser
	}
	if _, ok := s.users[f.To]; !ok {
		// @ fold acc(s.LockP())
		s.mu.Unlock()
		return ErrUnknownUser
	}
	s.follows[f] = true
	// @ fold acc(s.LockP())
	s.mu.Unlock()
	return nil
}

// DeleteFollow removes an edge. Idempotent (F3): deleting an absent edge is a
// no-op. The deleteFollowEdge trusted shim is gone.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) DeleteFollow(from, to string) {
	s.mu.Lock()
	// @ unfold acc(s.LockP())
	deleteFollowEdge(s.follows, dom.Follow{From: from, To: to})
	// @ fold acc(s.LockP())
	s.mu.Unlock()
}

// deleteFollowEdge quarantines the `delete` builtin.
//
// TRUSTED, AND THE FLAT RESHAPE DID NOT CHANGE THAT. Gobra 1.1-SNAPSHOT has
// no model for Go's `delete` builtin at all: it reports
// "got unknown identifier delete" on a SINGLE-LEVEL map exactly as it did on
// a nested one. This was measured directly -- `delete(m, k)` on a plain
// `map[dom.Follow]bool` is rejected with the same diagnostic.
//
// So this shim is NOT the nested-permission shim of the same name that the
// reshape retired. That one was quarantining `edges[key] = true`-style inner
// access. A flat map WRITE now verifies with no shim; a flat map DELETE still
// cannot be written at all. The blocker is a missing builtin in the verifier's
// front end, which no amount of reshaping the data can address.
//
// Trusted because the runtime semantic is the stdlib's: `delete(m, k)` removes
// the entry if present and is a no-op otherwise -- exactly what the
// postcondition states.
//
// @ trusted
// @ requires acc(m)
// @ ensures  acc(m)
// @ ensures  !(k in domain(m))
// @ decreases
func deleteFollowEdge(m map[dom.Follow]bool, k dom.Follow) {
	delete(m, k)
}

// Follows reports whether from follows to. A single-level lookup; this is the
// visibility test HomeTimeline uses per tweet.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) Follows(from, to string) (result bool) {
	s.mu.RLock()
	// @ unfold acc(s.LockP())
	result = s.follows[dom.Follow{From: from, To: to}]
	// @ fold acc(s.LockP())
	s.mu.RUnlock()
	return result
}

// PutTweet appends to the log.
//
// This maintains both premises of the monotonicity lemma: the caller
// allocates ids monotonically, and CreatedAt is read from a clock that never
// decreases. Append is the only mutation, so the invariant is local.
//
// F6 holds by the author-existence check. F8 holds by construction upstream
// in the id generator. The appendTweet trusted shim is gone -- a slice append
// on a single-level field needs no inner-slice permission.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) PutTweet(t dom.Tweet) (err error) {
	s.mu.Lock()
	// @ unfold acc(s.LockP())
	if _, ok := s.users[t.Author]; !ok {
		// @ fold acc(s.LockP())
		s.mu.Unlock()
		return ErrUnknownUser
	}
	// ENFORCE the monotonicity lemma's premises rather than assuming them.
	//
	// F2 is derived from the claim that the log is ordered by construction.
	// That claim rests on two facts about every append -- ids strictly
	// increase, and created_at never decreases -- and NOTHING previously
	// checked them. A caller appending out of order would silently produce a
	// mis-ordered timeline with no failing test and no failing proof, because
	// the premise lived only in a comment.
	//
	// The store's own test suite did exactly that: TestSnapshotIsSorted
	// appended ID 7 at ts 1 and then ID 3 at ts 0. Legitimate against the old
	// per-author map, illegal against a log.
	//
	// Checking it here turns the lemma's premise into an invariant the type
	// maintains, which is what makes F2 derivable instead of assumed.
	if n := len(s.tweets); n > 0 {
		last := s.tweets[n-1]
		if t.ID <= last.ID || t.CreatedAt < last.CreatedAt {
			// @ fold acc(s.LockP())
			s.mu.Unlock()
			return ErrNonMonotonic
		}
	}
	s.tweets = appendTweet(s.tweets, t)
	// @ fold acc(s.LockP())
	s.mu.Unlock()
	return nil
}

// appendTweet quarantines the `append` builtin.
//
// TRUSTED, AND THE FLAT RESHAPE DID NOT CHANGE THAT EITHER. Gobra models
// `append` with a DIFFERENT SIGNATURE from Go's: it wants a permission amount
// as the first argument (`append(p, slice, elems...)`), and rejects Go's
// two-argument form with
//
//	"append expects first argument of type perm followed by a slice ...
//	 but got []Tweet, Tweet".
//
// The permission-first form is not valid Go, and this file has to compile
// under `go build` as well as verify, so the call cannot be written in a way
// that satisfies both. Measured on a flat `[]int` field: same error.
//
// Like deleteFollowEdge above, this is not the shim the reshape retired. The
// old appendTweet was quarantining a per-author inner-slice permission gap
// (`byAuthor[author] = append(byAuthor[author], t)`); that gap is genuinely
// gone. What remains is purely the builtin's signature mismatch, which is a
// property of the tool, not of the data shape.
//
// Trusted because the runtime semantic is the stdlib's: `append` returns a
// slice one element longer with `t` last, reallocating if capacity is
// exceeded.
//
// The postcondition is a FULL functional spec of `append`, not just a length
// clause. It has to be: PutTweet must re-establish LockP()'s append-log
// invariant after the call, and that is only possible if the shim promises
// the existing elements are preserved in place and `t` lands last. A
// length-only postcondition would leave the invariant unprovable and would
// have forced F2 back into the trusted surface. This is the one place where
// widening a trusted shim's contract buys a proof rather than hiding one.
//
// @ trusted
// @ requires acc(xs)
// @ ensures  acc(res)
// @ ensures  len(res) == len(xs) + 1
// @ ensures  forall k int :: 0 <= k && k < len(xs) ==> res[k] == old(xs[k])
// @ ensures  res[len(xs)] == t
// @ decreases
func appendTweet(xs []dom.Tweet, t dom.Tweet) (res []dom.Tweet) {
	return append(xs, t)
}

// FollowSet returns the handles that from follows.
//
// Now a single-level range with a key filter. The iterFollows trusted shim is
// gone. Retained for the admin/snapshot path; the timeline no longer needs
// it, which is the point -- the timeline asks a membership question per
// tweet rather than materialising a set.
//
// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
func (s *MemStore) FollowSet(from string) (out map[string]bool) {
	out = map[string]bool{}
	s.mu.RLock()
	// @ unfold acc(s.LockP())
	iterFollows(s.follows, from, out)
	// @ fold acc(s.LockP())
	s.mu.RUnlock()
	return out
}

// iterFollows quarantines the map `range`.
//
// TRUSTED, AND THE FLAT RESHAPE DID NOT CHANGE THAT. Gobra 1.1-SNAPSHOT
// cannot verify a `range` over a map AT ALL. Measured directly: `for range m`
// over a flat `map[dom.Follow]bool` with FULL permission and an EMPTY BODY is
// still rejected with "Loop invariant is not well-formed", and supplying
// explicit `invariant acc(m)` changes the diagnostic to "Loop invariant might
// not be established" without fixing it. Half and full permission fail alike.
// An indexed `for i := 0; i < len(xs); i++` loop over a SLICE verifies fine,
// so this is specific to map iteration, not to loops.
//
// Again not the shim the reshape retired: the old iterFollows was
// quarantining iteration over an INNER `map[string]bool` reached through
// `s.follows[from]`. That inner level is gone. What is left is that the outer
// iteration itself was never verifiable, which flattening does not address.
//
// Trusted because the runtime semantic is the stdlib's: iterate every entry
// once, in unspecified order, and record the `To` end of each edge whose
// `From` end matches. Order does not escape -- the result is a set.
//
// @ trusted
// @ requires acc(m, 1/2)
// @ requires acc(out)
// @ ensures  acc(m, 1/2)
// @ ensures  acc(out)
// @ decreases
func iterFollows(m map[dom.Follow]bool, from string, out map[string]bool) {
	for e := range m {
		if e.From == from {
			out[e.To] = true
		}
	}
}

// HomeTimeline returns the page of tweets visible to user, newest first.
//
// F1 (visibility): a tweet is included exactly when its author is the user or
// the user follows its author. The test is per tweet and single-level.
//
// F2 (ordering): the result is descending (created_at, id) because the log is
// append-ordered and is walked backwards. See the monotonicity lemma in the
// package comment. NO SORT IS PERFORMED, so no sort specification is owed.
//
// D10 (pagination): cursor is exclusive -- only ids strictly below it are
// returned. cursor <= 0 means start from the newest. more reports whether a
// further visible tweet exists below the returned page, which is what lets
// the caller emit next_cursor: null to mean exactly "nothing remains".
//
// gatherTimeline and sortTimeline are both gone, and NEITHER COMES BACK.
// There is no sort here, so no sort specification is owed; F2 is the
// monotonicity lemma above applied to a backwards walk.
//
// NOTE ON THE BUFFER. The page is built by preallocating `limit` slots,
// index-assigning into them and returning `buf[:n]`, rather than by appending
// to a growing slice. That is a proof-shape choice, not a semantic one: the
// elements, their order and `more` are identical either way. Gobra cannot
// check Go's two-argument `append` (see appendTweet above), but it verifies
// index-assignment and reslicing directly -- so writing the loop this way
// keeps HomeTimeline, the F1/F2 carrier, entirely OUT of the trusted surface.
// Spending a preallocation to avoid a trusted shim on the one method the
// timeline properties live in is the right trade.
//
// F2 IS IN THE POSTCONDITION. The last `ensures` below is the ordering
// property, and Gobra discharges it against this loop. The derivation is the
// monotonicity lemma made mechanical:
//
//   - LockP() supplies "the log is ordered" (invariant 2 below re-states it
//     inside the loop, where the predicate is unfolded).
//   - Invariant 7 says everything collected so far came from a STRICTLY LATER
//     log position than the one being examined, expressed as a fact about ids
//     and timestamps rather than about positions.
//   - So when `buf[n] = t` runs, every earlier entry beats `t` on
//     (created_at, id), which is exactly invariant 6, the F2 relation.
//
// Nothing here is a sort and nothing here trusts one.
//
// @ requires acc(s.LockP())
// @ requires limit > 0
// @ ensures acc(s.LockP())
// @ ensures acc(out)
// @ ensures len(out) <= limit
// @ ensures forall a, b int :: 0 <= a && a < b && b < len(out) ==>
// @            (out[a].CreatedAt > out[b].CreatedAt ||
// @             (out[a].CreatedAt == out[b].CreatedAt && out[a].ID > out[b].ID))
func (s *MemStore) HomeTimeline(user string, limit int, cursor int64) (out []dom.Tweet, more bool) {
	buf := make([]dom.Tweet, limit)
	n := 0
	s.mu.RLock()
	// @ unfold acc(s.LockP())
	// @ invariant acc(&s.mu)
	// @ invariant acc(&s.users) && acc(s.users)
	// @ invariant acc(&s.follows) && acc(s.follows)
	// @ invariant acc(&s.tweets) && acc(s.tweets)
	// @ invariant forall p, q int :: 0 <= p && p < q && q < len(s.tweets) ==>
	// @              s.tweets[p].ID < s.tweets[q].ID && s.tweets[p].CreatedAt <= s.tweets[q].CreatedAt
	// @ invariant acc(buf)
	// @ invariant len(buf) == limit
	// @ invariant 0 <= n && n <= limit
	// @ invariant -1 <= i && i < len(s.tweets)
	// @ invariant forall a, b int :: 0 <= a && a < b && b < n ==>
	// @              (buf[a].CreatedAt > buf[b].CreatedAt ||
	// @               (buf[a].CreatedAt == buf[b].CreatedAt && buf[a].ID > buf[b].ID))
	// @ invariant i >= 0 ==> forall a int :: 0 <= a && a < n ==>
	// @              (buf[a].ID > s.tweets[i].ID && buf[a].CreatedAt >= s.tweets[i].CreatedAt)
	// @ decreases i + 1
	for i := len(s.tweets) - 1; i >= 0; i-- {
		t := s.tweets[i]
		if cursor > 0 && t.ID >= cursor {
			continue
		}
		if t.Author != user && !s.follows[dom.Follow{From: user, To: t.Author}] {
			continue
		}
		if n == limit {
			more = true
			break
		}
		buf[n] = t
		n++
	}
	// @ fold acc(s.LockP())
	s.mu.RUnlock()
	return buf[:n], more
}

// StoreSnapshot is the admin serialization format.
type StoreSnapshot struct {
	Users   []dom.User
	Follows []dom.Follow
	Tweets  []dom.Tweet
}

// Snapshot renders the state in a canonical order.
//
// Still trusted: it is admin-path serialization, not the observable API, and
// it sorts for stable output. Its sorts are a reporting concern and carry no
// F-property obligation -- unlike the timeline sort this reshape removed.
//
// @ trusted
// @ decreases
func (s *MemStore) Snapshot() StoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	users := make([]dom.User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })

	follows := make([]dom.Follow, 0, len(s.follows))
	for e := range s.follows {
		follows = append(follows, e)
	}
	sort.Slice(follows, func(i, j int) bool {
		if follows[i].From != follows[j].From {
			return follows[i].From < follows[j].From
		}
		return follows[i].To < follows[j].To
	})

	// The log is already in the canonical order. No sort needed.
	tweets := make([]dom.Tweet, len(s.tweets))
	copy(tweets, s.tweets)

	return StoreSnapshot{Users: users, Follows: follows, Tweets: tweets}
}

// Replace loads a snapshot.
//
// The incoming tweet order is normalised to (created_at asc, id asc) so the
// restored log satisfies the monotonicity lemma. This sort is a precondition
// repair on untrusted input, not part of any read path.
//
// LOAD-BEARING TRUST ASSUMPTION, RECORDED RATHER THAN PAPERED OVER. Now that
// LockP() carries the append-log invariant and F2 is proved from it, this
// method is the one way that invariant can be broken without Gobra noticing.
// It is `// @ trusted` and carries no LockP() contract, so the verifier never
// checks that the state it installs satisfies the invariant — and the sort it
// performs is NOT sufficient to establish it. Ordering by (created_at, id)
// leaves ids non-monotonic whenever the snapshot's ids disagree with its
// timestamps: the input [{id:5, ts:0}, {id:3, ts:1}] sorts to itself, and
// 5 < 3 is false, so `tweets[i].ID < tweets[j].ID` fails at i=0, j=1.
//
// Nothing in the observable API can reach this: `Replace` is admin-snapshot
// only, and every log the verified write path builds goes through PutTweet's
// checked guard. But F2's proof is conditional on snapshots being well-formed,
// and that condition lives here, in trusted code, not in the proof.
//
// @ trusted
// @ decreases
func (s *MemStore) Replace(snap StoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users = make(map[string]dom.User, len(snap.Users))
	for _, u := range snap.Users {
		s.users[u.Handle] = u
	}

	s.follows = make(map[dom.Follow]bool, len(snap.Follows))
	for _, f := range snap.Follows {
		s.follows[f] = true
	}

	tweets := make([]dom.Tweet, len(snap.Tweets))
	copy(tweets, snap.Tweets)
	sort.SliceStable(tweets, func(i, j int) bool {
		if tweets[i].CreatedAt != tweets[j].CreatedAt {
			return tweets[i].CreatedAt < tweets[j].CreatedAt
		}
		return tweets[i].ID < tweets[j].ID
	})
	s.tweets = tweets
}
