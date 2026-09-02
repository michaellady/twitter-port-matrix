# F026 — The proof half of the matrix cannot have a single cell, and running the Go sweep will not change that

**Status:** derived while shaping `evidence/MATRIX.md`, from `ASSURANCE.md`'s
ceiling table and `tools/cmd/calibrate/rungs.go`.
**PARTLY SUPERSEDED the same evening — see the note below.**
**Class:** a structural property of the deliverable, not a defect

> ## Superseded for R4, still exact for R5
>
> This finding was derived in an isolated worktree at the same time two other
> worktrees were adding an R4 driver for Rust (Verus) and one for Kotlin
> (JBMC). It was true of the tree it was derived from and false of the tree it
> was merged into, within the hour. **The R4 column is no longer entirely
> capped:** three corners now have an R4 rung that yields a kill verdict, so R4
> is capped only where Java is an end — six of the twelve pairs — and
> `MATRIX.md` now records 2 measured, 4 pending, 6 capped for that column.
>
> **The finding's reasoning is not what failed.** The rule it derives — a cell
> is capped by its weaker end, and a rung absent at either end yields no cell —
> is the rule `MATRIX.md` still applies, and it is what produces the six Java
> caps. What changed is an input: how many corners have the rung. The
> finding's own headline claim, "running the Go sweep will not change that",
> was also correct and stayed correct; the Go sweep did not produce a cell.
> It was the Rust and Kotlin *rungs* that did.
>
> **Everything it says about R5 stands unchanged.** Gobra on Go is still the
> only R5 rung, no ordered pair has Go at both ends, and all twelve R5 cells
> are still capped.
>
> Kept rather than rewritten, because the shape of the error is worth more than
> a corrected copy: a finding about what is structurally impossible, derived
> from a snapshot, in a fan-out where three agents were changing that structure
> in parallel. A finding that quantifies over "every corner" needs to say which
> tree it read.

## What it says

A matrix cell is an ordered corner pair, **B ← A**, over the four existing
implementations, and a cell is **capped by its weaker end** — per
`ASSURANCE.md`'s port-claim table, "R5 on A only" licenses only *"A is correct;
B is only as good as R0-R2"*, which is not a port claim about B.

Exactly one corner has a rung that produces a proof verdict. `rungs.go` declares
both R4 and R5 with `Impls: []string{"go"}`, and `ASSURANCE.md` records why the
others do not have one: Verus has no `vstd` model for `RwLock` (F016, F012),
Java was not attempted, Kotlin's JBMC is bounded and carries F014's
string-equality defect.

**No ordered pair has Go at both ends.** Therefore **all 24 R4 and R5 cells of
the twelve-pair matrix are capped today** — 12 rows × 2 proof rungs — and not
one of them is capped for want of running something.

## Why it is worth recording

Because the obvious next action does not produce a cell.

The Go corner's full R4+R5 sweep — 18 mutants × 2 rungs, GOAL.md's queue item 2
narrowed to the one corner where both proof rungs exist — is real work worth
doing, and it will answer a real question (R4 and R5 agreed on all five mutants
of `r45-gate`, so it is not yet known whether the refinement row discriminates
at all). But it fills **one end** of six rows. It fills **zero cells** of the
matrix, because every one of those six rows is capped by its other end.

The first R4 cell of this matrix appears only when a **second** corner gets a
verifier rung — the `cargo-verus` verdict line and budget for Rust, or JBMC for
Kotlin and Java. Until then the proof half of the deliverable is a column of
recorded ceilings, which is a legitimate result (standing rule 8) but is not a
kill rate.

## What it does not say

It does not say the Go sweep should be skipped or deferred. A cell needs both
ends, so the Go end has to exist either way, and the sweep also settles whether
R4 and R5 are distinct rows or aliases — a question the five-mutant gate cannot
answer. It says only that "the matrix gains its first proof cell" is not among
that sweep's outcomes, and a fire that expects one will report the run as a
failure when it is not.

## The consequence for ordering

Two pieces of work are needed before any R4/R5 cell exists, and they are
independent of each other:

1. the Go end measured over the whole catalogue (a sweep), and
2. a second corner's verifier rung (new plumbing, queue item 1's remaining
   sub-steps).

Neither subsumes the other, and (2) is the one that gates the *cell*. The
current queue does (1) first, which is defensible — building a second verifier's
plumbing while the first corner has no rate is the wrong order — but the reason
to do it is the R4-vs-R5 discrimination question, not the matrix.

## Where the ceiling is already known before either happens

F022 bounds the Go end in advance: 4 of 18 Go mutants edit only
`internal/httpshim`, which no obligation covers, so the Go end's R4
killed/reached denominator is **14, not 18**, before an obligation is written.
The matrix carries that denominator in the cell rather than beside it, so a
reader cannot pick up "100%" without also picking up "of 14 of a possible 18".
