// Package sobs is S_obs: the deterministic, total reference machine over the
// observable Twitter-clone API.
//
// S_obs is the single source of truth for this repository. It is the machine
// that every implementation must refine (rung R5), the generator of the
// conformance corpus (R0) and differential traces (R1), and the thing the
// per-language verifier contracts are rendered from.
//
// Two properties are load-bearing and must never be weakened:
//
//	DETERMINISTIC  Step is a pure function. Same (state, request) always
//	               yields the same (response, state').
//	TOTAL          Every syntactically-representable request has a defined
//	               response. There is no "unspecified behaviour" hole through
//	               which two conforming implementations could legally differ.
//
// Together those give the equivalence theorem this repository exists to
// reach: if A refines S_obs and B refines S_obs, then A and B are
// observationally equivalent on every request sequence.
//
// NOTE: no implementation under impls/ may import this package. That rule is
// mechanically enforced by `matrixctl doctor` and exists to break correlated
// failure between the reference machine and the Go corner.
package sobs

// Request is a wire-level observable input. Body is raw JSON text; the empty
// string means "no body". Keeping the body as raw text (rather than a decoded
// map) is what makes totality meaningful: malformed JSON is representable and
// therefore must have a defined response.
type Request struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// Response is a wire-level observable output. Body is canonical JSON text (see
// encode.go); the empty string means "no body". Byte equality of Response is
// the observation relation used by every differential rung.
type Response struct {
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"`
}

// User is a registered account. IDs are allocated from 1, monotonically.
type User struct {
	Handle string
	ID     int64
}

// Tweet is a post. CreatedAt is the logical clock value at post time.
//
// The Text field has no counterpart in twitter.tla, whose TweetRec is
// [id, author, ts]. The projection to TLA+ drops it. See DECISIONS.md D1.
type Tweet struct {
	ID        int64
	Author    string
	Text      string
	CreatedAt int64
}

// Edge is a directed follow relation.
type Edge struct {
	From string
	To   string
}

// State is the abstract state. It is deliberately a value type: Step takes a
// State and returns a new one, never mutating its argument.
//
// The tweets field is an APPEND-ORDERED LOG and is never sorted. Timeline
// ordering is derived from reverse iteration over it -- see the monotonicity
// lemma in step.go. This is what removes the verified-sort proof obligation
// that blocks F1/F2 in both existing implementation repos.
type State struct {
	users        []User
	userByHandle map[string]User
	follows      map[Edge]struct{}
	tweets       []Tweet
	clock        int64
	nextUserID   int64
	nextTweetID  int64
}

// Init is the unique initial state. Corresponds to twitter.tla's Init.
func Init() State {
	return State{
		users:        nil,
		userByHandle: map[string]User{},
		follows:      map[Edge]struct{}{},
		tweets:       nil,
		clock:        0,
		nextUserID:   1,
		nextTweetID:  1,
	}
}

// clone produces an independent copy so Step can stay pure.
func (s State) clone() State {
	n := State{
		users:        make([]User, len(s.users)),
		userByHandle: make(map[string]User, len(s.userByHandle)),
		follows:      make(map[Edge]struct{}, len(s.follows)),
		tweets:       make([]Tweet, len(s.tweets)),
		clock:        s.clock,
		nextUserID:   s.nextUserID,
		nextTweetID:  s.nextTweetID,
	}
	copy(n.users, s.users)
	copy(n.tweets, s.tweets)
	for k, v := range s.userByHandle {
		n.userByHandle[k] = v
	}
	for k := range s.follows {
		n.follows[k] = struct{}{}
	}
	return n
}

// -- Read-only accessors, used by tracegen and tlclink. --

// Clock returns the current logical timestamp.
func (s State) Clock() int64 { return s.clock }

// Handles returns registered handles in registration order.
func (s State) Handles() []string {
	out := make([]string, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u.Handle)
	}
	return out
}

// TweetCount returns the number of posted tweets.
func (s State) TweetCount() int { return len(s.tweets) }

// Tweets returns a copy of the append-ordered tweet log.
func (s State) Tweets() []Tweet {
	out := make([]Tweet, len(s.tweets))
	copy(out, s.tweets)
	return out
}

// FollowEdges returns follow edges in a canonical (sorted) order. The order is
// for reproducible reporting only; no observable response depends on it.
func (s State) FollowEdges() []Edge {
	out := make([]Edge, 0, len(s.follows))
	for e := range s.follows {
		out = append(out, e)
	}
	sortEdges(out)
	return out
}
