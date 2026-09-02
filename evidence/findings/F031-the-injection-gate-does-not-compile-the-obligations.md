# F031 — The anchor gate compiles the implementation, not the obligations, so a proof rung can be handed a tree it cannot build

**Status:** measured during the Kotlin corner's 18-mutant R4 sweep
**Class:** a gap in the F011 guard that is specific to proof rungs
**Effect:** one cell of 18 is a missing measurement, and the gate that exists to
catch exactly this kind of thing reported `ok` on it

## What happened

`mutate verify -impl kotlin` passed on all 18 mutants, immediately before the
sweep, exactly as F011 requires:

```
anchors: 18/18 match exactly one site
compile: 18/18 build clean

verify PASSED: every anchor matches one site; every mutant compiles
```

`kotlin/id-burned-on-reject` is one of the 18 it cleared:

```
  ok     kotlin/id-burned-on-reject             src/twitterport/store/Store.kt:55 src/twitterport/service/Service.kt:52 sha256:3b9b35823d684144  11.1s
```

The R4 rung was then handed that same tree — same hash, guard 5/5 — and could
not build it:

```
kotlin/id-burned-on-reject   3b9b35823d68  id-alloc
  guard    5/5 checks: kotlin@id-burned-on-reject -> /tmp/calibrate-mutant-962170801/tree (baseline kotlin is a different tree)
  setup    apply 0.0s (walk copy) + warm build 11.9s -- charged to no rung
  R4       ERROR      jbmc produced no R4 verdict (exit 1). Nothing was measured:
    | /tmp/calibrate-mutant-962170801/tree/verification/Canaries.kt:73:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | /tmp/calibrate-mutant-962170801/tree/verification/Obligations.kt:179:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | /tmp/calibrate-mutant-962170801/tree/verification/Obligations.kt:195:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | /tmp/calibrate-mutant-962170801/tree/verification/Obligations.kt:208:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
```

## Why the gate could not see it

The two builds are not the same build, and neither is wrong on its own terms.

| build | what it compiles | source |
|---|---|---|
| `mutate verify` / every empirical rung | `src` | `impls/registry.json` → `kotlin -nowarn -jvm-target 17 -include-runtime -d {bin}.jar src` |
| the R4 rung | `src` **and** `verification` | `SrcDirs: []string{"src", "verification"}` in `tools/cmd/jbmc/obligations.go` |

The mutant's own description says why the edit is shaped the way it is:

> *"Allocation is encapsulated inside `Store.createUser` in this corner, so the
> defect takes two edits: split allocation out of insertion, then call it above
> the check"*

Splitting allocation out changes `createUser`'s **arity** from
`createUser(handle: String)` to `createUser(handle: String, id: Long)`. The
shipped call site in `Service.kt` is updated by the mutant's second edit, so
`src` compiles. The four call sites in `verification/` are not, because the
mutant catalogue has no reason to know they exist.

So the mutant is a legal *implementation* and an illegal *obligation tree*, and
"every mutant compiles" was true of the only tree the gate was looking at.

## Why it is not a survival, and not a kill

`calibrate` records it as an **error cell** — a missing measurement, in neither
the numerator nor the denominator. That is the right call and it is the same
call `tools/cmd/jbmc` makes for UNDECIDED: a tree the tool never got to reason
about has told you nothing about the port. Scoring it as a survival would credit
the mutant with defeating a rung that never ran, which is F011's failure mode
with the sign flipped.

## The sharpest part

**Every broken call site is inside an obligation that is already BLOCKED and in
no denominator.** The four the compiler named are `c5_timelineIsOldestFirst`
(`Canaries.kt:73`) and `o4a_pageRespectsLimit`, `o4b_cursorNullMeansExhausted`,
`o4c_pageIsNewestFirst` (`Obligations.kt:179, 195, 208`). Those three are the
Group 4 obligations JBMC 6.11.0 cannot decide (F014: `o4b`/`o4c` by
`String.equals`, `o4a` by SAT exhaustion), and `c5` is `o4c`'s negation canary.

There is a fifth `Store.createUser` call site, `Canaries.kt:61` in
`c4_pageMayExceedLimit`, which the compiler did **not** name — `kotlinc` did not
report every instance of the same error, and this write-up quotes what it
printed rather than what it might have. `c4` guards `o4a`, so it is blocked too;
the claim above holds over all five sites and rests on reading the file, not on
the diagnostic list. Every other `createUser` call in `verification/` is
`Service.createUser`, a different method the mutant does not touch.

Not one of these could have contributed a kill or a survival.

The cell is lost to a compile error in code whose answers the rung had already
agreed to discard. `kotlinc` compiles a file or it does not; it has no notion of
which obligations the rung intends to score, so the exclusion that F014 bought
at the level of the *verdict* buys nothing at the level of the *build*.

## What would close it, and what would not

**Would close it:** teach the gate the rung's build. `mutate verify` knows the
corner and could compile whatever that corner's proof rung compiles, so a mutant
that breaks the obligation set fails the gate rather than the sweep — an
un-runnable cell caught in 11 s instead of discovered 12 minutes into a sweep.
That is a change to `tools/cmd/mutate` and to what the registry records, and it
is not made here.

**Would not close it:** editing `Obligations.kt` so it survives this mutant.
Rewriting an obligation to accommodate a held-out defect is the same move
standing rule 3 forbids on a base app and standing rule 8 forbids in general —
the ceiling gets written down, not worked around. The obligation set is written
against the *unmutated* contract, and a mutant that changes an API the
obligations call is telling you something real about how tightly the two are
coupled.

## The transferable form

A mutation catalogue is written against the shipped code. A proof rung reads
more than the shipped code: contracts, obligation entry points, canaries, ghost
files. **Any injection gate that validates only the shipped half will pass
mutants the proof rung cannot build**, and the failure surfaces as an error cell
in the middle of a long sweep rather than as a gate failure before it.

This is the fourth arrival of F007's shape: the cost lands somewhere other than
where the change was made.
