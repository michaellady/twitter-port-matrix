# F018 — A safety guard turns a benign race into a dropped tweet and a 500

**Status:** confirmed by driving the Go corner over HTTP with branch counters
**Severity:** live defect, invisible to every rung
**Origin:** introduced by me, in the F005 fix

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
