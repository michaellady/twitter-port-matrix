# F049 — The R4 cell is lost to an R5 obligation, and R4 never reads that file

**Status:** measured on the Kotlin corner after F048's decoupling
(`evidence/runs/calibration/kotlin-r4-refinement-removed-probe.log`)
**Class:** two rungs share one compile unit, so a coupling in either one's
obligations removes a cell from the *other's* denominator
**Effect:** the Kotlin R4 denominator is 14 rather than 15 for a reason that has
nothing to do with what R4 measures. Unlike F048's loss, this one is repairable.

## The two losses are not the same loss

F048 establishes that R5 clause 2 (`Refinement.kt`'s
`c02_createUserAddsExactlyThatHandle` and its canary `k02`) genuinely needs
`Store.createUser`, and that `id-burned-on-reject` must change that signature.
So the **R5** cell for that mutant is gone and stays gone.

The **R4** cell is a different question, and it has a different answer. R4's
obligations are the fifteen entry points in `Obligations.kt` and the canaries in
`Canaries.kt`. After F048's five removals, not one of them mentions
`Store.createUser`. R4 does not read `Refinement.kt` — `tools/cmd/jbmc/r5.go`
does, and `main.go` says so: `jbmc r5verify` "runs JBMC over
`verification/Refinement.kt`".

But both rungs declare the same source set:

```go
// tools/cmd/jbmc/obligations.go — the R4 corner
SrcDirs:  []string{"src", "verification"},
// tools/cmd/jbmc/r5.go — the R5 corner
SrcDirs: []string{"src", "verification"},
```

`kotlinc` compiles a directory or it does not. It has no notion of which
obligations the rung intends to score, so R5's clause 2 is in R4's build, and a
signature change that breaks clause 2 breaks R4's build with it.

## Measured, not argued

The counterfactual is cheap to run, so it was run rather than asserted. The
`id-burned-on-reject` tree, materialised by `mutate apply`, with **one file
removed — `verification/Refinement.kt`, the file R4 does not read** — and
nothing else changed:

```
        compile src + verification in 10.5s
o1a_oneCharAcceptSet               VERIFIED   1 ok, 0 failed           14.3s     VERIFICATION SUCCESSFUL
o1b_twoCharAcceptSet               VERIFIED   1 ok, 0 failed           14.6s     VERIFICATION SUCCESSFUL
o1c_emptyAndBareSignRejected       VERIFIED   3 ok, 0 failed           3.5s      VERIFICATION SUCCESSFUL
o3a_idsStrictlyIncrease            VERIFIED   1 ok, 0 failed           3.9s      VERIFICATION SUCCESSFUL
o3b_createdAtNonDecreasing         VERIFIED   2 ok, 0 failed           6.1s      VERIFICATION SUCCESSFUL
o3c_lemmaOverThreeAppends          VERIFIED   2 ok, 0 failed           10.9s     VERIFICATION SUCCESSFUL
o5c_syntaxBeatsExistence           VERIFIED   1 ok, 0 failed           5.1s      VERIFICATION SUCCESSFUL

decidable 7   VERIFIED 7   REFUTED 0   VACUOUS 0   UNDECIDED 0

R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator
```

All nine negation canaries were refuted in that tree, so the verdict is not
vacuous by F013's instrument, and the line is the **same line, character for
character**, that the other fourteen survivals produce.

Three things follow, and the third is the one that matters:

1. **The cell is recoverable.** R4 has a verdict available on this mutant.
2. **F048's five removals were sufficient for R4.** `Obligations.kt` and
   `Canaries.kt` compile against the mutant tree; only `Refinement.kt` does not.
3. **The recovered cell is a SURVIVAL, not a kill.** `id-burned-on-reject`
   allocates an id before the duplicate-handle check, and no decidable R4
   obligation sees it — `o5d_rejectionBurnsNoId` is the obligation that would,
   and it is one of the eight JBMC blocks ("SAT checker ran out of memory").
   So reclaiming this cell moves the Kotlin R4 row from `0/14 = 0%` to
   `0/15 = 0%`: **a larger denominator over the same zero.** F024's shape — the
   repair makes the number look no better, and that is what makes it a repair.

**This probe is not a cell and is not in any table.** The tree it ran on is not
the tree `calibrate`'s guard hashed — a file was deleted from it — so it answers
a counterfactual and nothing else. It says what the rung *would* report if its
build compiled what it reads.

## What would close it, and what would not

**Would close it:** give each rung the source set it actually reads. R4 compiles
`src` plus `Obligations.kt` and `Canaries.kt`; R5 compiles `src` plus
`Refinement.kt`. That is the F035 rule — *touch the smallest surface the
property needs* — applied one level up, to the rung rather than the obligation.
It is a change to `obligations.go`, `r5.go`, `implrun.Spec.VerifyBuild` (which is
one command per corner and would have to become one per rung) and to
`mutate verify`'s reporting, and **it is not made here**: it changes what the
F031 gate means for every JVM corner, and the Kotlin R5 sweep — the run that
would show whether narrowing R5's build moved anything — has never been made
(`MATRIX.md`: "rung exists, sweep not run").

**Would not close it:** dropping `Refinement.kt` from the gate's `verify_build`
while leaving the rungs sharing a build. That makes `mutate verify` green
without making any cell measurable, which is the gate lying rather than the gate
passing. The red gate is currently correct: on this tree, *an* obligation set
this corner ships does not compile.

**Also would not close it:** rewriting clause 2. See F048.

## The transferable form

Two rungs that read different obligations but share a compile unit are coupled
in exactly one direction that nobody designs and no verdict reports: **a
signature dependency in either one's obligations subtracts a cell from the
other's denominator.** The rung that loses the cell need never mention the
method, the file, or the property involved.

The general statement is F007's, arriving for the fifth time in this repository:
the cost lands somewhere other than where the change was made. The specific one
worth carrying: *a rung's denominator is a property of its build, not of its
obligations*, and those two are the same set only if somebody made them so.
