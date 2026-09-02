# F028 — The refinement row is a copy of the proof row, and the canary is the only thing that separates them

**Status:** measured on the Go corner's full R4+R5 sweep, 18 mutants x 2 rungs
**Class:** a row of the kill table that carries no information the row above it
does not — and a case where the canary passes while the rate says nothing

> **CORRECTED IN PLACE 2026-09-02** by F038 and F039, which took the chance this
> finding says the catalogue never gave the two columns. **The open question is
> closed: the columns do separate**, by reach (F038) and by verdict (F039). The
> title still describes what the shipped 18-mutant sweep showed and every number
> below still holds of `tools/cmd/mutate/mutants.json` — what changes is that
> "0 of 18" is now known to be a fact about that catalogue rather than about the
> rungs. Corrections are marked inline.

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

> **CORRECTION (F038).** That zero is a property of
> `tools/cmd/mutate/mutants.json`, and it no longer describes the project's
> evidence. Two mutants confined to `internal/dom/dom.go` were written into the
> scratch catalogue `evidence/experiments/r4-r5-separation/manifest.json` and
> swept. Both came back `R4 killed` / `R5 unreached` — the cell this paragraph
> predicted:
>
> ```
> go/handle-alphabet-widened      R4 killed     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m15.2s]
> go/handle-alphabet-widened      R5 unreached  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m10.5s]
> go/text-control-chars-accepted  R4 killed     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m3.3s]
> go/text-control-chars-accepted  R5 unreached  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m3.4s]
> ```
>
> The scratch ids are kept out of the shipped catalogue on purpose, so the 18
> mutants and every denominator drawn from them are unchanged.

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

> **CORRECTION (F039).** "Every one" is still true of the nine, and the verdict
> half of the mechanism is no longer only an argument. `clock-now-off-by-one`,
> in the same scratch catalogue, breaks `(*clock.Logical).Now`'s postcondition —
> inside `internal/clock/clock.go`, which **is** in `r5Files`, on a member
> carrying no refinement clause. `R4 killed` / `R5 SURVIVED`, R5's
> `killed/reached` denominator `0/1`. The columns differ by verdict, not only by
> reach.

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
  **(F038: it did. Two of them, and the separation was immediate.)**
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
  **(F038/F039 moved it, in a scratch manifest rather than the shipped one, and
  both predicted separations appeared on the first attempt. That is evidence
  for this bullet, not against it: the three defects had to be written on
  purpose, aimed at the perimeter difference. Queue item 4 stands.)**
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
- `evidence/findings/F038-*.md`, `F039-*.md` — the separations that close this
  finding's open question
- `evidence/runs/calibration/dom-separation/` — their journal, results and console
