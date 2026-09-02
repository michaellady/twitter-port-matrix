# F018 — A safety guard turns a benign race into a dropped tweet and a 500

**Status:** FIXED, with a regression test that was shown to fail without the fix
**Severity:** was a live defect, invisible to every rung
**Origin:** introduced by me, in the F005 fix

> **Fix and gate, added after the audit below.** `Service` now carries a `wmu`
> mutex held across the whole allocate-then-write sequence in `PostTweet` and
> `CreateUser`, so ids are appended in the order they are issued and the
> `MemStore.PutTweet` guard is unreachable in fact rather than in a comment.
> Two regression tests were added and both were measured against the unfixed
> code before the fix landed — see "The gate that catches it" at the end. The
> audit below is left exactly as it was written.

## The defect

Under **1,280 concurrent `POST /tweets`**, the append-log monotonicity guard
fired **49 times.**

`ErrNonMonotonic` is not handled in `writeErrFromDomain`, so it falls through
to the `default` arm. The observable result is **HTTP 500 `internal_error`, and
the tweet is silently discarded.**

Two goroutines take ids 5 and 6 from the generator, then race to append. The
one holding 5 loses and is rejected — for being out of order relative to a
tweet that only exists because it lost the race.

`store.PutUser`'s handle-taken branch is reachable the same way (1 hit in
1,280).

## Why no rung can see it

`S_obs` is a sequential state machine. It has no concurrency notion at all, so
R0 replays a corpus, R1 replays traces, and R2 drives properties — **all
single-threaded, all against an oracle that cannot express the interleaving.**

It took compiling branch counters into a scratch copy and driving ~11,600
requests over real HTTP, including ~2,600 concurrent, to see it. Nothing in the
existing ladder does that.

## The comment that was wrong

`memstore.go` asserts the guard is unreachable:

> under the real composition, because ids come from a strictly-increasing
> generator (F8) and timestamps from a non-decreasing clock (F7)

Both premises are true. The reasoning omits concurrency: F8 guarantees ids are
*allocated* in increasing order, not that they are *appended* in that order.
Allocation happens outside the store lock.

Another instance of the F012 pattern — a documented claim that was honest when
written and inherited without recheck.

## I introduced this

F005 found that the monotonicity premise was unenforced and added the guard.
That was right: before it, an out-of-order append silently produced a
mis-ordered timeline.

But the fix converted a **silent wrong answer** into a **loud wrong answer**,
and both are wrong. The real defect is older and neither state exposed it: **id
allocation is outside the lock that protects the log it orders.**

The correct fix is to allocate and append atomically under one lock, which
removes the race rather than detecting it. The guard should then be
unreachable *in fact*, and its own comment finally true.

## What this says about the ladder

F008 and F009 were about inputs the generator could not produce. This is a
class the **oracle itself cannot express**: a deterministic sequential
reference machine has no vocabulary for interleaving, so no rung derived from
it can score a concurrency defect. Widening the alphabet does not help;
`S_obs` would have to gain a concurrency semantics, or a separate rung would
have to exist that does not consult it.

That is a real gap in `ASSURANCE.md`'s ladder and it is now recorded as one.

## Also from this audit: "133 verified" is about 43

The Gobra count, decomposed from `stats.json` over a clean baseline run of all
five packages (290 Viper members, 140 with a body and verified):

| what the member is | count |
|---|---|
| `$INIT` / `$IMPORTS` package glue | 35 |
| auto-generated termination proofs (from `decreases`) | 44 |
| `Defined…` interface-implements proofs | 9 |
| `LockP()` predicate declarations | 4 |
| error-interface boilerplate on 3 stateless structs | 9 |
| abstraction-function projections | 9 |
| **actual annotated functions** | **30** |

**92 of the 140 encode no user-authored claim at all.** The 30 real functions
carry roughly **52 functional `ensures` clauses**; 46 were shown refutable, 2
were inconclusive after >65 minutes, 4 untested, and 3 describe states the
observable API cannot reach.

**About 43 load-bearing clauses over 26 reachable functions.** Two-thirds of
the headline number is bookkeeping the tool emitted about itself.

`store.Follows` is worse than F015's shape: it carries a discharged R5
response-commutation obligation and has **zero callers anywhere in the
repository, including tests.** F015's `NewFollow` at least gets called.

## An instrument limitation worth knowing

**Gobra/Silicon reports at most one failing postcondition per method.**
Negating several clauses of one method in a single run yields one error and
silently hides the rest — so a batched negation audit under-reports, and
scores clauses as live that were never tested. Every clause needs its own run.

The instrument was validated before use: adding both
`ensures old(s.AbsLogLen()) < 0 ==> err == nil` and its negation to `PutTweet`
gave **0 errors for each** — the F013 vacuity signature, reproduced on demand,
confirming a refutation elsewhere is real signal.

---

# The gate that catches it

Added with the fix, because nothing in the ladder could have.

## Why it is not a rung

Every rung in `ASSURANCE.md` is derived from `S_obs`, and the deliverable of
this repository is a per-rung kill table. A concurrency check does not belong
in that table, because `S_obs` is a sequential state machine: R0 replays a
corpus, R1 replays traces, R2 drives properties, and R4/R5 quantify over
single-threaded `step_L`. None of them has a place to put an interleaving.
Adding a "concurrency rung" that scores mutants would put a row in the kill
table whose oracle is a different thing entirely, which is exactly the kind of
number `FINDINGS.md` Pattern 1 is about.

So this is a **standing implementation-level check**, alongside the race
detector, and it is named as such rather than promoted into the ladder.

## The oracle it uses instead

The reason a check is possible at all is that it does not consult `S_obs`:

> N concurrent accepted writes must produce N distinct durable records, and no
> accepted write may be reported as a server error.

That is checkable with only a counter, no reference machine — which is
precisely why it sees what every `S_obs`-derived rung is blind to.

## The two tests, and their measured falsifiability

Per standing rule 2, neither is trusted on the strength of passing. Both were
run against the unfixed code first.

| test | where | pre-fix result |
|---|---|---|
| `TestF018_ConcurrentPostTweetLosesNoTweet` | `internal/httpshim` | **detected in 20 of 20 runs**; 4–32 of 1,280 concurrent `POST /tweets` answered `500 internal_error`, and that many tweets never reached the log |
| `TestF018_ConcurrentCreateUserBurnsNoID` | `internal/service` | **detected in 28 of 40 single-trial runs**; ships at 10 trials, so it misses with probability ~6e-6 |

Both pass with the fix in place, over repeated runs.

Three things about their construction are worth keeping:

- **The tweet test asserts survival, not status codes.** "The guard never
  fires" would have passed against a fix that merely added `ErrNonMonotonic`
  to `writeErrFromDomain`'s table and returned a tidier code while still
  dropping the tweet. It asserts that every accepted request produced a
  distinctly-identified tweet that reads back.
- **The user test asserts the id set, not the error.** Losing the
  `CreateUser` race yields `409 handle_taken`, which is the *correct* answer;
  the observable divergence is the id the loser had already consumed, so the
  assertion is that N registrations consume exactly the ids `1..N`. `S_obs`
  allocates only on success and cannot produce a gap.
- **A barrier is load-bearing in the user test.** Without holding every worker
  until all of them are ready for the same handle, the goroutines start
  staggered and the contended window never opens: the same workload detected
  the burn in 1 trial out of 5 rather than 28 out of 40.

## What the race detector says, and why it is not enough

`go test -race` reports **no data race** on the unfixed code, before or after,
while both tests fail against it. That is correct behaviour, not a detector
gap: every individual step was properly locked. F018 is a **lost update across
correctly-locked components**, and only counting the results finds it. The
existing `race_test.go` hammers the same endpoints under `-race` and passed
throughout.

This is Pattern 2 again, one layer down: the race detector is a gate, and the
blind spot was not visible from inside it either.
