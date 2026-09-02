# F024 — The refinement row is a copy of the proof row, and the canary is the only thing that separates them

**Status:** measured on the Go corner's full R4+R5 sweep, 18 mutants x 2 rungs
**Class:** a row of the kill table that carries no information the row above it
does not — and a case where the canary passes while the rate says nothing

## The result

```
rung             live  killed  survived  unreached  equiv   kill%reach
R4 proof           18       9         5          4      0       64%
R5 refinement      18       9         5          4      0       64%
```

**R4 and R5 disagree on 0 of 18 mutants.** Same nine kills, same five
survivals, same four unreached. Cell for cell, the R5 column is the R4 column.

This was not knowable from the gate that put R5 in the table. That gate ran
five mutants, all five agreed, and the fire that ran it said so plainly and
asked for the rate. The rate is 18 for 18.

## Why this is not the construction

The obvious explanation — that R5 was defined as "R4, restated" — is wrong, and
the rungs were built specifically to avoid it. `rungs.go` says so:

> R5 is deliberately NOT credited with every R4 kill. A mutant that breaks some
> functional postcondition is killed by the proof; only one that breaks a clause
> carrying an `S_obs` refinement obligation is killed by the refinement layer.

And the canary demonstrates the separation, in both directions:

```
A. `// @ ensures false` on dom.ValidHandle (no R5 clause on that member)
     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m29.6s]
     R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause
B. `// @ ensures false` on clock.Tick (carries R5 clause 36)
     R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause
```

So the two rungs *can* answer differently. Nothing in the catalogue makes them.

## The mechanism, in two halves

**Half one — reach.** R4's perimeter is the five Gobra-verified packages.
R5's is the four files carrying a refinement clause site. Exactly one file
separates the two perimeters: `internal/dom/dom.go`, which carries obligations
but no refinement clause. A mutant confined to it would be *reached* by R4 and
*unreached* by R5 — the first cell where the columns differ.

No mutant in the catalogue is confined to `dom.go`. The only one that touches it
at all, `self-follow-guard-dropped`, also edits `service.go` and so is reached
by both. Re-derived from the manifest: **mutants where R4 reach ≠ R5 reach: 0.**

The four unreached cells are the four `internal/httpshim` mutants of F022, and
`httpshim` is outside *both* perimeters, so it separates nothing either.

**Half two — verdict.** Of the nine mutants R4 killed, every one failed an
obligation inside a member carrying a refinement clause:

| where the failure landed | count |
|---|---|
| on the refinement clause line itself | 4 |
| elsewhere in a member that carries one | 5 |

The five in the second row are the loop-invariant path in
`(*MemStore).HomeTimeline` — the machinery that discharges the refinement
postconditions rather than the postconditions themselves.

That split matters more than it looks. Attribution was widened from
clause-line-only to clause-*or*-member one fire earlier, after the clause-only
reading left cells `R5 UNDECIDED`. **Five of the nine agreements rest on that
widening.** Under the narrower rule the R5 row would not have disagreed with R4
on five mutants; it would have had *no verdict* on them. Either way the R5
column tells you nothing about those five that the R4 column did not.

## The rule

**A canary proves a rung can fail. It does not prove the rung distinguishes
anything.** Standing rule 2 gets a rung as far as refutable and no further. Two
rungs can both be non-vacuous, both be individually justified, both pass their
canaries in both directions — and still produce identical columns on every
mutant you own, because the cases that separate them are cases your catalogue
does not contain.

The gap between "this check can fail" and "this check tells me something the
cheaper check above it did not" is a second measurement, and it needs a rate,
not a gate. Five mutants agreeing is consistent with both a real rung and a
duplicated one; eighteen agreeing localises the question to the catalogue.

## What follows

- **Do not delete the R5 row.** It answers a different question, its canary
  works, and one `dom.go`-only mutant would separate the columns immediately.
- **Do not report R4 and R5 as two rungs' worth of assurance.** They are the
  same Gobra invocation read two ways. Nine kills reported twice is nine kills.
  (This also means their cost columns are not comparable — see the caveat in
  `evidence/CALIBRATION-go-proof.md`, where a single Z3 query took 1033 s under
  R4 and 62 s under R5 on the same tree, accounting for essentially the whole
  2043 s / 1059 s difference between the rows.)
- **The catalogue is the variable to move.** Queue item 4 — a second catalogue
  drawn from something other than the contract — is the test. A catalogue built
  from incident history has no reason to leave `dom.go` alone, and no reason to
  put every defect inside a member that a refinement clause happens to sit on.
- **Before adding the Rust/Verus and Kotlin/JBMC proof rungs**, note that this
  finding is about a *pair* of rungs on one corner. The same question — does
  this row ever differ from its neighbour — should be asked of each new row as a
  rate, not as a gate, before it is reported as a separate column.

## Where

- `evidence/CALIBRATION-go-proof.md` — the full table and every verdict line
- `evidence/runs/calibration/go-proof/` — journal, results, console
- `tools/cmd/calibrate/rungs.go` — `gobraVerified`, `r5Files`, and the comment
  explaining why R5 is not credited with every R4 kill
- `spec/refinement/clause-sites.json` — the sites `r5Files` is derived from
