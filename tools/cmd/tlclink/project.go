package main

import (
	"fmt"
	"sort"
	"strings"

	sobs "github.com/michaellady/twitter-port-matrix/spec/s_obs"
)

// tlaState is an S_obs state projected onto twitter.tla's five variables.
//
// The projection is the abstraction function that makes S_obs a refinement of
// the model. It is deliberately lossy in three documented ways:
//
//	D1  Tweet text has no counterpart in TweetRec and is dropped.
//	D2  User ids have no counterpart and are dropped; handles carry identity.
//	D11 Tweet ids are shifted by one. twitter.tla starts nextTweetId at 0, so
//	    its first tweet has id 0; S_obs and both conformance corpora start at
//	    1. The abstraction function subtracts 1. A refinement mapping need not
//	    be the identity, but the mismatch is real and is recorded rather than
//	    silently absorbed -- see evidence/findings/F002.
type tlaState struct {
	KnownUsers  []string
	Follows     [][2]string
	Tweets      []tlaTweet
	Clock       int64
	NextTweetID int64
}

type tlaTweet struct {
	ID     int64
	Author string
	TS     int64
}

// project maps an S_obs state onto the model's variables.
func project(s sobs.State) tlaState {
	handles := append([]string(nil), s.Handles()...)
	sort.Strings(handles)

	var follows [][2]string
	for _, e := range s.FollowEdges() {
		follows = append(follows, [2]string{e.From, e.To})
	}

	var tweets []tlaTweet
	for _, t := range s.Tweets() {
		tweets = append(tweets, tlaTweet{ID: t.ID - 1, Author: t.Author, TS: t.CreatedAt})
	}

	// nextTweetId is the id the NEXT tweet would receive, in model numbering.
	return tlaState{
		KnownUsers:  handles,
		Follows:     follows,
		Tweets:      tweets,
		Clock:       s.Clock(),
		NextTweetID: int64(len(tweets)),
	}
}

func (t tlaState) key() string { return t.record() }

// record renders the state as a TLA+ record literal.
func (t tlaState) record() string {
	var b strings.Builder
	b.WriteString("[knownUsers |-> {")
	for i, h := range t.KnownUsers {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", h)
	}
	b.WriteString("}, follows |-> {")
	for i, e := range t.Follows {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "<<%q, %q>>", e[0], e[1])
	}
	b.WriteString("}, tweets |-> <<")
	for i, tw := range t.Tweets {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "[id |-> %d, author |-> %q, ts |-> %d]", tw.ID, tw.Author, tw.TS)
	}
	fmt.Fprintf(&b, ">>, clock |-> %d, nextTweetId |-> %d]", t.Clock, t.NextTweetID)
	return b.String()
}
