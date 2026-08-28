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
	delete(s.follows, dom.Follow{From: from, To: to})
	// @ fold acc(s.LockP())
	s.mu.Unlock()
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
	s.tweets = append(s.tweets, t)
	// @ fold acc(s.LockP())
	s.mu.Unlock()
	return nil
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
	for e := range s.follows {
		if e.From == from {
			out[e.To] = true
		}
	}
	// @ fold acc(s.LockP())
	s.mu.RUnlock()
	return out
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
// gatherTimeline and sortTimeline are both gone.
//
// @ requires acc(s.LockP())
// @ requires limit > 0
// @ ensures acc(s.LockP())
// @ ensures len(out) <= limit
func (s *MemStore) HomeTimeline(user string, limit int, cursor int64) (out []dom.Tweet, more bool) {
	out = make([]dom.Tweet, 0, limit)
	s.mu.RLock()
	// @ unfold acc(s.LockP())
	// @ invariant acc(s.LockP())
	// @ invariant len(out) <= limit
	for i := len(s.tweets) - 1; i >= 0; i-- {
		t := s.tweets[i]
		if cursor > 0 && t.ID >= cursor {
			continue
		}
		if t.Author != user && !s.follows[dom.Follow{From: user, To: t.Author}] {
			continue
		}
		if len(out) == limit {
			more = true
			break
		}
		out = append(out, t)
	}
	// @ fold acc(s.LockP())
	s.mu.RUnlock()
	return out, more
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
