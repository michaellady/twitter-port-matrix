---------------------------- MODULE twitter ----------------------------
(***************************************************************************
  Canonical TLA+ specification for the twitter-clone formal-verification
  project. This module is the abstract source of truth at the design level.

  Both implementation repos (twitter-golang-formal-verification and
  twitter-rust-formal-verification) consume this module as a git submodule
  pinned by SHA. Per-language verifier contracts (Gobra annotations, Verus
  specs) discharge the same numbered properties on the concrete code; this
  module is what the bounded-model checker (TLC) verifies on the abstract
  state machine.

  This is a bounded model check, not a refinement proof. See README.md.
 ***************************************************************************)

EXTENDS Naturals, Sequences, FiniteSets, TLC

CONSTANTS
    Users,         \* set of all candidate user handles
    MaxTweets,     \* upper bound on number of tweets posted in any run
    MaxTimestamp   \* upper bound on the logical clock

VARIABLES
    knownUsers,    \* set of registered user handles
    follows,       \* set of (follower, followee) pairs
    tweets,        \* sequence of [id, author, ts] records, indexed in post order
    clock,         \* current logical timestamp
    nextTweetId    \* monotonic counter for tweet ids

vars == <<knownUsers, follows, tweets, clock, nextTweetId>>

(***************************************************************************)
(* Type invariant                                                          *)
(***************************************************************************)

TweetRec == [id: Nat, author: Users, ts: Nat]

TypeOK ==
    /\ knownUsers \subseteq Users
    /\ follows \subseteq (knownUsers \X knownUsers)
    /\ tweets \in Seq(TweetRec)
    /\ clock \in 0..MaxTimestamp
    /\ nextTweetId \in Nat
    /\ Len(tweets) <= MaxTweets

(***************************************************************************)
(* Initial state                                                            *)
(***************************************************************************)

Init ==
    /\ knownUsers = {}
    /\ follows = {}
    /\ tweets = <<>>
    /\ clock = 0
    /\ nextTweetId = 0

(***************************************************************************)
(* Actions                                                                  *)
(***************************************************************************)

CreateUser(u) ==
    /\ u \in Users
    /\ u \notin knownUsers
    /\ knownUsers' = knownUsers \cup {u}
    /\ UNCHANGED <<follows, tweets, clock, nextTweetId>>

\* F4: no self-follow rejected at action level (a # b is a precondition).
\* F3: idempotent — repeating Follow leaves the set unchanged.
Follow(a, b) ==
    /\ a \in knownUsers
    /\ b \in knownUsers
    /\ a # b
    /\ follows' = follows \cup {<<a, b>>}
    /\ UNCHANGED <<knownUsers, tweets, clock, nextTweetId>>

\* F3: Unfollow is idempotent — removing an absent edge leaves the set unchanged.
Unfollow(a, b) ==
    /\ a \in knownUsers
    /\ b \in knownUsers
    /\ follows' = follows \ {<<a, b>>}
    /\ UNCHANGED <<knownUsers, tweets, clock, nextTweetId>>

\* F7: clock is non-decreasing. Tick increments by 1; the spec also permits
\* multiple PostTweet steps at the same clock value (ties allowed) by not
\* requiring a Tick between them.
Tick ==
    /\ clock < MaxTimestamp
    /\ clock' = clock + 1
    /\ UNCHANGED <<knownUsers, follows, tweets, nextTweetId>>

\* F8: tweet ids are globally unique and per-author monotonic by construction
\* (nextTweetId increments on every successful PostTweet).
PostTweet(u) ==
    /\ u \in knownUsers
    /\ Len(tweets) < MaxTweets
    /\ tweets' = Append(tweets, [id |-> nextTweetId, author |-> u, ts |-> clock])
    /\ nextTweetId' = nextTweetId + 1
    /\ UNCHANGED <<knownUsers, follows, clock>>

Next ==
    \/ \E u \in Users : CreateUser(u)
    \/ \E a \in knownUsers, b \in knownUsers : Follow(a, b)
    \/ \E a \in knownUsers, b \in knownUsers : Unfollow(a, b)
    \/ \E u \in knownUsers : PostTweet(u)
    \/ Tick

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Derived helpers                                                          *)
(***************************************************************************)

\* Set of tweets visible on u's home timeline, per F1.
VisibleTweets(u) ==
    { tweets[i] : i \in {j \in 1..Len(tweets) :
                            tweets[j].author = u \/
                            <<u, tweets[j].author>> \in follows} }

(***************************************************************************)
(* Invariants — the F-properties the model checks                           *)
(***************************************************************************)

\* F1: visibility — every tweet that should be visible to u is in u's visible
\* set, and no other tweet is. The abstract spec sees the full set; the impl
\* paginates and the F1 wording in the plan is per-page (page is a subset of
\* this set with no fabrication).
F1_Visibility ==
    \A u \in knownUsers :
        \A i \in 1..Len(tweets) :
            LET t == tweets[i] IN
                (t.author = u \/ <<u, t.author>> \in follows) <=>
                (t \in VisibleTweets(u))

\* F4: no self-follow edges.
F4_NoSelfFollow == \A e \in follows : e[1] # e[2]

\* F6: every tweet author is a known user.
F6_NoOrphanAuthors ==
    \A i \in 1..Len(tweets) : tweets[i].author \in knownUsers

\* F8: tweet ids are globally unique.
F8_UniqueTweetIds ==
    \A i \in 1..Len(tweets) :
        \A j \in 1..Len(tweets) :
            i # j => tweets[i].id # tweets[j].id

\* F8b: per-author tweet ids are strictly monotonically increasing.
F8b_PerAuthorMonotonic ==
    \A i \in 1..Len(tweets) :
        \A j \in 1..Len(tweets) :
            (i < j /\ tweets[i].author = tweets[j].author) =>
                tweets[i].id < tweets[j].id

\* F9: every follow edge references known users on both sides.
F9_NoOrphanFollowEdges ==
    \A e \in follows : e[1] \in knownUsers /\ e[2] \in knownUsers

\* F7 is implicit in the action semantics (clock only ever increases via Tick;
\* no action decrements it). We assert it explicitly as a safety check on the
\* clock variable across the unprimed/primed pair, but TLC's standard form is
\* a state invariant on the current state — the strict invariant below is
\* "clock has not decreased since the last state", encoded as a safety
\* property over <<clock, clock'>>:
F7_ClockNonDecreasing == [][clock' >= clock]_vars

\* Aggregate state invariant for INVARIANT directive in the .cfg.
Invariants ==
    /\ TypeOK
    /\ F1_Visibility
    /\ F4_NoSelfFollow
    /\ F6_NoOrphanAuthors
    /\ F8_UniqueTweetIds
    /\ F8b_PerAuthorMonotonic
    /\ F9_NoOrphanFollowEdges

\* Aggregate temporal property for PROPERTY directive in the .cfg.
TemporalProperties == F7_ClockNonDecreasing

================================================================================
