package sobs

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// Bounds on the observable surface. These are part of the contract: an
// implementation that accepts a 300-character tweet does not refine S_obs.
const (
	MaxHandleLen = 32
	MaxTextLen   = 280
	MinTextLen   = 1
	DefaultLimit = 50
	MaxLimit     = 100
)

// Error codes. Exactly this set; no implementation may invent another.
const (
	ErrMalformedRequest = "malformed_request"
	ErrInvalidHandle    = "invalid_handle"
	ErrInvalidText      = "invalid_text"
	ErrInvalidLimit     = "invalid_limit"
	ErrInvalidCursor    = "invalid_cursor"
	ErrUnknownUser      = "unknown_user"
	ErrSelfFollow       = "self_follow_forbidden"
	ErrHandleTaken      = "handle_taken"
	ErrNotFound         = "not_found"
)

// Step is the transition function of S_obs: pure, deterministic, and total.
//
// Every (State, Request) pair yields a defined (Response, State). Requests
// that are rejected leave the state unchanged -- rejection is observable in
// the response, never in the state.
func Step(s State, r Request) (Response, State) {
	path, query, hasQuery := strings.Cut(r.Path, "?")

	switch {
	case r.Method == "POST" && path == "/users" && !hasQuery:
		return stepCreateUser(s, r.Body)
	case r.Method == "POST" && path == "/follow" && !hasQuery:
		return stepFollow(s, r.Body)
	case r.Method == "DELETE" && path == "/follow" && !hasQuery:
		return stepUnfollow(s, r.Body)
	case r.Method == "POST" && path == "/tweets" && !hasQuery:
		return stepPostTweet(s, r.Body)
	case r.Method == "POST" && path == "/tick" && !hasQuery:
		return stepTick(s, r.Body)
	case r.Method == "GET" && path == "/timeline":
		return stepTimeline(s, query, hasQuery)
	default:
		return Response{Status: 404, Body: errorBody(ErrNotFound)}, s
	}
}

// --- request body decoding -------------------------------------------------

// decodeStrict parses exactly one JSON object into dst. Unknown fields are
// rejected and trailing content is rejected. Lenient parsing is a classic
// source of cross-implementation divergence, so S_obs is strict.
//
// Known limitation, documented rather than hidden: duplicate keys resolve
// last-wins (Go's decoder behaviour). tracegen never emits duplicate keys.
func decodeStrict(body string, dst any) bool {
	dec := json.NewDecoder(bytes.NewReader([]byte(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return false
	}
	// Reject trailing content.
	if _, err := dec.Token(); err != io.EOF {
		return false
	}
	return true
}

func malformed(s State) (Response, State) {
	return Response{Status: 400, Body: errorBody(ErrMalformedRequest)}, s
}

func reject(s State, code string) (Response, State) {
	status := 400
	if code == ErrHandleTaken {
		status = 409
	}
	return Response{Status: status, Body: errorBody(code)}, s
}

// --- validation ------------------------------------------------------------

// validHandle accepts 1..MaxHandleLen characters of [a-z0-9_].
// Deliberately narrow: a narrow alphabet is a narrow divergence surface.
func validHandle(h string) bool {
	if len(h) == 0 || len(h) > MaxHandleLen {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
		if !ok {
			return false
		}
	}
	return true
}

// validText accepts MinTextLen..MaxTextLen bytes, no control characters.
func validText(t string) bool {
	if len(t) < MinTextLen || len(t) > MaxTextLen {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] < 0x20 {
			return false
		}
	}
	return true
}

// --- transitions -----------------------------------------------------------

type createUserBody struct {
	Handle *string `json:"handle"`
}

// stepCreateUser: POST /users
//
// Validation order (pinned; twitter.tla leaves it open):
//  1. malformed body
//  2. handle syntax
//  3. handle already taken
func stepCreateUser(s State, body string) (Response, State) {
	var b createUserBody
	if !decodeStrict(body, &b) || b.Handle == nil {
		return malformed(s)
	}
	if !validHandle(*b.Handle) {
		return reject(s, ErrInvalidHandle)
	}
	if _, exists := s.userByHandle[*b.Handle]; exists {
		return reject(s, ErrHandleTaken)
	}
	n := s.clone()
	u := User{Handle: *b.Handle, ID: n.nextUserID}
	n.nextUserID++
	n.users = append(n.users, u)
	n.userByHandle[u.Handle] = u
	return Response{Status: 201, Body: userBody(u)}, n
}

type edgeBody struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

// stepFollow: POST /follow
//
// Validation order (pinned; twitter.tla's Follow is an unordered conjunction
// of a in knownUsers, b in knownUsers, a # b, so the model does NOT
// disambiguate follow(eve,eve) where eve is unknown. S_obs picks
// existence-before-semantics: unknown_user wins over self_follow_forbidden.
// See DECISIONS.md D4.):
//  1. malformed body
//  2. from syntax, then to syntax
//  3. from existence, then to existence
//  4. self-follow
//
// Follow is idempotent (F3): re-following an existing edge is 204 and leaves
// the follow set unchanged.
func stepFollow(s State, body string) (Response, State) {
	var b edgeBody
	if !decodeStrict(body, &b) || b.From == nil || b.To == nil {
		return malformed(s)
	}
	if !validHandle(*b.From) || !validHandle(*b.To) {
		return reject(s, ErrInvalidHandle)
	}
	if _, ok := s.userByHandle[*b.From]; !ok {
		return reject(s, ErrUnknownUser)
	}
	if _, ok := s.userByHandle[*b.To]; !ok {
		return reject(s, ErrUnknownUser)
	}
	if *b.From == *b.To {
		return reject(s, ErrSelfFollow)
	}
	n := s.clone()
	n.follows[Edge{From: *b.From, To: *b.To}] = struct{}{}
	return Response{Status: 204}, n
}

// stepUnfollow: DELETE /follow
//
// twitter.tla's Unfollow requires a,b in knownUsers but NOT a # b, so
// self-unfollow is legal and is a no-op. The corpus never covered this.
// See DECISIONS.md D5.
//
// Unfollow is idempotent (F3): removing an absent edge is 204.
func stepUnfollow(s State, body string) (Response, State) {
	var b edgeBody
	if !decodeStrict(body, &b) || b.From == nil || b.To == nil {
		return malformed(s)
	}
	if !validHandle(*b.From) || !validHandle(*b.To) {
		return reject(s, ErrInvalidHandle)
	}
	if _, ok := s.userByHandle[*b.From]; !ok {
		return reject(s, ErrUnknownUser)
	}
	if _, ok := s.userByHandle[*b.To]; !ok {
		return reject(s, ErrUnknownUser)
	}
	n := s.clone()
	delete(n.follows, Edge{From: *b.From, To: *b.To})
	return Response{Status: 204}, n
}

type postTweetBody struct {
	Author *string `json:"author"`
	Text   *string `json:"text"`
}

// stepPostTweet: POST /tweets
//
// Validation order (pinned): malformed, author syntax, text syntax, author
// existence. Syntax before existence throughout S_obs.
//
// F8 holds by construction: nextTweetID increments on every successful post,
// so ids are globally unique and strictly increasing in log order -- which is
// the monotonicity half of the timeline lemma below.
func stepPostTweet(s State, body string) (Response, State) {
	var b postTweetBody
	if !decodeStrict(body, &b) || b.Author == nil || b.Text == nil {
		return malformed(s)
	}
	if !validHandle(*b.Author) {
		return reject(s, ErrInvalidHandle)
	}
	if !validText(*b.Text) {
		return reject(s, ErrInvalidText)
	}
	if _, ok := s.userByHandle[*b.Author]; !ok {
		return reject(s, ErrUnknownUser)
	}
	n := s.clone()
	t := Tweet{ID: n.nextTweetID, Author: *b.Author, Text: *b.Text, CreatedAt: n.clock}
	n.nextTweetID++
	n.tweets = append(n.tweets, t)
	return Response{Status: 201, Body: tweetBody(t)}, n
}

// stepTick: POST /tick
//
// The clock-advance rule. twitter.tla has a free Tick action; the old
// conformance corpus showed created_at:1 with no request that could have
// caused it, leaving each implementation free to invent a rule. S_obs makes
// the advance explicit and client-driven: exactly one request maps 1:1 to
// exactly one TLA+ Tick step. See DECISIONS.md D3.
//
// The clock is unbounded here. twitter.tla bounds it at MaxTimestamp; tlclink
// only submits traces that stay within the model bound.
func stepTick(s State, body string) (Response, State) {
	if body != "" && body != "{}" {
		return malformed(s)
	}
	n := s.clone()
	n.clock++
	return Response{Status: 200, Body: clockBody(n.clock)}, n
}

// stepTimeline: GET /timeline?user=<h>[&limit=<n>][&cursor=<n>]
//
// THE SORT-FREE TIMELINE.
//
// Monotonicity lemma: for log positions i < j,
//
//	tweets[i].ID < tweets[j].ID          (ids allocated monotonically)
//	tweets[i].CreatedAt <= tweets[j].CreatedAt   (clock never decreases)
//
// Therefore reverse iteration over the append-ordered log yields exactly
// descending (CreatedAt, ID) lexicographic order:
//
//	if CreatedAt differs, the later post has the larger CreatedAt and comes
//	first; if CreatedAt ties, the later post has the larger ID and comes
//	first. Either way j precedes i.
//
// So F2 (ordering) is a DERIVED property of an insertion-ordered structure,
// and no verified sort specification is needed to discharge it. This is the
// obligation that blocks F1/F2 on home_timeline in both existing repos.
//
// Validation order (pinned): query syntax, user syntax, user existence,
// limit, cursor.
func stepTimeline(s State, query string, hasQuery bool) (Response, State) {
	if !hasQuery {
		return malformed(s)
	}
	vals, err := url.ParseQuery(query)
	if err != nil {
		return malformed(s)
	}
	// Unknown or repeated query parameters are rejected.
	for k, v := range vals {
		if k != "user" && k != "limit" && k != "cursor" {
			return malformed(s)
		}
		if len(v) != 1 {
			return malformed(s)
		}
	}
	user, ok := vals["user"]
	if !ok {
		return malformed(s)
	}
	if !validHandle(user[0]) {
		return reject(s, ErrInvalidHandle)
	}
	if _, ok := s.userByHandle[user[0]]; !ok {
		return reject(s, ErrUnknownUser)
	}

	limit := int64(DefaultLimit)
	if raw, ok := vals["limit"]; ok {
		n, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || n < 1 || n > MaxLimit {
			return reject(s, ErrInvalidLimit)
		}
		limit = n
	}

	var cursor *int64
	if raw, ok := vals["cursor"]; ok {
		n, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || n < 1 {
			return reject(s, ErrInvalidCursor)
		}
		cursor = &n
	}

	page := make([]Tweet, 0, limit)
	var next *int64
	u := user[0]
	for i := len(s.tweets) - 1; i >= 0; i-- {
		t := s.tweets[i]
		if cursor != nil && t.ID >= *cursor {
			continue
		}
		if t.Author != u {
			if _, follows := s.follows[Edge{From: u, To: t.Author}]; !follows {
				continue
			}
		}
		if int64(len(page)) == limit {
			// A further visible tweet exists below the page: emit a cursor.
			last := page[len(page)-1].ID
			next = &last
			break
		}
		page = append(page, t)
	}
	return Response{Status: 200, Body: timelineBody(page, next)}, s
}
