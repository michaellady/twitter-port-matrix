# F032 — The guard that makes the premise true is what stops the proof rung from scoring it

**Status:** measured on the Kotlin corner's full 18-mutant R4 sweep; reproduces
the five-mutant gate's error cell exactly
**Class:** an interaction between defence-in-depth and vacuity auditing that
converts a kill into a missing measurement
**Effect:** the one mutant this corner's obligations were written to catch is
the one cell the corner cannot score

## The cell

```
kotlin/tick-goes-backwards   763477b8665d  clock
  R4       ERROR      jbmc produced no R4 verdict (exit 1). Nothing was measured:
    | decidable 7   VERIFIED 6   REFUTED 0   VACUOUS 1   UNDECIDED 0
    |   ! c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted (VERIFIED); under vacuity a claim and its negation both verify, so o3b_createdAtNonDecreasing decides nothing (F013)
    |
    | R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read (c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted (VERIFIED); under vacuity a claim and its negation both verify, so o3b_createdAtNonDecreasing decides nothing (F013)); nothing was decided about this tree   [2m29.5s]
```

The mutant is one character: `clockValue++` becomes `clockValue--` in
`src/twitterport/store/Store.kt`.

## Why this is the cell that should have been a kill

`o3b_createdAtNonDecreasing` is not an incidental obligation. It is **the**
obligation of this corner. F005 records that the sort-free timeline (D9) derives
its ordering from an insertion-ordered log rather than from a proved sort, and
that the derivation is sound only if two premises hold: ids strictly increase
and `createdAt` never falls. `Obligations.kt` says so in its own comment —
*"THE obligation of this corner"* — and `o3b` is the one that pins the clock
premise across both branches of a tick.

`tick-goes-backwards` breaks exactly that premise. If any mutant in this
catalogue should be killed by this corner's proof rung, it is this one.

## What actually happens

`Store.appendTweet` does not merely *have* the premise; it **enforces** it, at
the mutation site, and F005 is the finding that asked for precisely that:

```kotlin
if (t.createdAt < last.createdAt) {
    throw LogInvariantViolation(
        "append would break clock monotonicity: " +
            "last=${last.createdAt} new=${t.createdAt}"
    )
}
```

So on the mutant tree, the second `appendTweet` after a `tick()` **throws**. It
does not return a tweet with a smaller `createdAt`; that state is unreachable.
Follow the two runs:

- **`o3b_createdAtNonDecreasing`** asserts `t1.createdAt <= t2.createdAt`. On the
  `tickFirst = true` branch the second append throws before the assertion, so
  the assertion is not reached on that path; on the `tickFirst = false` branch
  the clock never moved and the assertion holds. JBMC reports every own goal
  SUCCESS. **VERIFIED.**
- **`c3_clockCanDecrease`**, its negation canary, asserts
  `t1.createdAt > t2.createdAt` with a `tick()` in between — a claim that is
  *true of the mutant's intent*. But its second append throws on the same path,
  so its assertion is unreachable too. JBMC reports SUCCESS. **VERIFIED, not
  refuted.**

Both the claim and its negation verify. That is the exact signature F013 defined
vacuity by, and `decide()` does what F013 says to do: demote `o3b` to VACUOUS
and report `R4 UNDECIDED` with no verdict. `calibrate` records an error cell.

**Every step is correct.** The enforcement is correct and F005 asked for it. The
canary is correct and F013 asked for it. The demotion is correct and F025 is why
it exists. The composition still loses the measurement.

## The shape of it

The vacuity audit asks *"is this claim reachable in THIS tree?"* — and it must,
because F013's whole point is that a claim nothing reaches verifies for free.
But a defect that a runtime guard converts into an **exception** makes the claim
unreachable in that tree for a reason that is not vacuity: the code did notice.

The audit cannot tell those two apart from goal counts alone. Both look like
"no own assertion goal was refuted, and the negation was not refuted either."

So:

> **Defence in depth is invisible to a vacuity-audited proof rung. The stronger
> the runtime enforcement of a property, the more likely a mutant that violates
> it lands as UNDECIDED rather than as a kill.**

## Relation to F015, which is the same collision seen empirically

F015 recorded that `go/self-follow-guard-dropped` became unscoreable because F4
was enforced in *two* places, so removing either changed nothing observable —
redundant enforcement blinding **mutation testing**. This is the same collision
one layer up: single, non-redundant enforcement blinding **the vacuity audit**.
F015 lost a cell to a rung seeing no difference; this loses a cell to a rung
refusing to read a difference it could not attribute.

Both are cases of a defence making a measurement impossible, and neither is an
argument for removing the defence.

## The second half: the rung's only firing path misses the catalogue entirely

The same sweep makes a related fact visible. `calibrate` flags a rung that kills
nothing, and standing rule 2 requires the demonstration that it *can* fire. It
exists, in `evidence/runs/calibration/kotlin-r4-canary-injection.log`:

```
o1a_oneCharAcceptSet               REFUTED    0 ok, 1 failed           22.6s     VERIFICATION FAILED
o1c_emptyAndBareSignRejected       REFUTED    2 ok, 1 failed           3.6s      VERIFICATION FAILED
```

Both are `parseInt64` obligations over `src/twitterport/dom/Dom.kt`. Derived
from the manifest: **no mutant in the catalogue edits `Dom.kt`.** All 18 Kotlin
mutants land in `store/Store.kt`, `service/Service.kt`, or `httpshim/`.

So the rung's only *demonstrated* firing path runs through a file the catalogue
never touches, and the only obligations over files the catalogue does touch are
Group 3 (three obligations, `Store.kt`) and `o5c` (one obligation,
`Service.kt`). Group 3 is the live surface, and both mutants that perturb it —
`tick-goes-backwards` here and `id-burned-on-reject` under F031 — are the
corner's two ERROR cells.

**The rung's demonstrated capability and the catalogue's reach do not
intersect.** That is a stronger statement than "it killed nothing", and it is
the one worth transferring: a canary that fires somewhere the mutants never go
proves the instrument works and says nothing about whether the measurement
could have come out differently.

## What would close it

Not weakening the guard. Two honest options, neither taken here:

1. **Give the audit a third answer.** An obligation whose canary is unreachable
   *because the implementation threw* is different from one that is unreachable
   because nothing constrains it. JBMC can be asked directly — an obligation
   asserting that `appendTweet` completes normally would separate the cases —
   and the rung could then report a kill on "the tree throws where the contract
   says it returns" rather than UNDECIDED.
2. **Write the obligation over the guard rather than around it.** `o3b` asks
   whether the log is monotone. It could instead ask whether `appendTweet`
   returns normally for every tick pattern, which is the property the guard
   actually implements and which the mutant plainly breaks.

Option 2 changes what the corner claims and would need to be argued on its
merits, not adopted because it improves a kill rate. Recorded, not worked
around, per standing rule 8.
