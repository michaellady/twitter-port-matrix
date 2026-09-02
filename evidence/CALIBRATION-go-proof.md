# CALIBRATION — the Go corner's deductive rungs, R4 and R5, all 18 mutants

The first *rate* for the two proof rungs. The R5 entry was gated on five
mutants when it was added (2026-09-02 16:55) and agreed with R4 on all five,
which is a gate and not a measurement. This is the full sweep.

```
go run ./tools/cmd/calibrate -impls go -rungs R4,R5 \
    -out evidence/runs/calibration/go-proof -resume
```

Window `2026-09-02T17:56:28Z .. 2026-09-02T18:48:46Z`. Raw journal, results
and console in `evidence/runs/calibration/go-proof/`.

Catalogue undrifted immediately before the sweep, per the standing note that a
drifted anchor injects nothing and every rung "kills" it (F011):

```
anchors: 72/72 match exactly one site
compile: 72/72 build clean

verify PASSED: every anchor matches one site; every mutant compiles
```

---

## Caveats — read these before the numbers

**1. R4 and R5 are the same Gobra run, read two ways.** `gobra verify` and
`gobra r5verify` issue the same verification of the same tree; they differ only
in which question they ask of the failures. So the two rows agreeing is **not**
independent corroboration, and the two rows' costs are **not** comparable. The
report's cost block shows `R4 proof 2043s` against `R5 refinement 1059s`, a
2x gap that is entirely one cell:

```
go/orphan-author-accepted
   R4  killed       1033.1s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [17m13s]
   R5  killed         61.9s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [1m1.8s]
```

Same tree, same invocation, same verdict — 17 minutes against 1 minute. That
is Z3 nondeterminism, not a property of either rung. F019 recorded that a
verifier's obligation *count* is a measurement with a range; its *wall time*
has a much wider one. 2043 − 1059 = 984 s, and 1033 − 62 = 971 s of that is
this single query. **Do not read the cost row as R4 being twice R5.**

**2. The catalogue and the contracts share a parent.** Both derive from
`S_obs`. This is the standing caveat on every table in this repository and it
bounds what a 64% means here.

**3. The box was shared.** Another agent's compile jobs ran throughout; the
1-minute load average was 6–12 for most of the window. Wall figures are
inflated by an unmeasured amount, uniformly across cells.

---

## The kill table

```
rung             live  killed  survived  unreached  equiv   kill%   kill%     wall
                                                            reach    live
R4 proof           18       9         5          4      0     64%     50%    2043s
R5 refinement      18       9         5          4      0     64%     50%    1059s
```

## The interesting number: **R4 and R5 disagree on 0 of 18**

Not "few". Zero. Every mutant gets the same verdict from both rungs — the same
9 kills, the same 5 survivals, the same 4 unreached. The refinement row is, on
this catalogue, an exact duplicate of the proof row.

This is not a construction artifact. The rungs were deliberately built so R5 is
*not* credited with every R4 kill, and the previous fire's canary showed they
**can** separate:

```
`// @ ensures false` on dom.ValidHandle (no R5 clause on that member)
     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m29.6s]
     R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause
```

So the separating case exists. It just is not in the catalogue. Written up as
**F028**.

---

## Per mutant, with the verdict lines verbatim

Reach is scored per F022: the input source for a proof rung is the contract, so
*unreached* means the verifier reads none of the files the mutant edits. A
mutant confined to `internal/httpshim` — trusted transport, covered by no
obligation — is unreached, never survived.

| mutant | files edited | R4 | R5 | reach |
|---|---|---|---|---|
| `id-first-is-two` | `internal/ids/ids.go` | SURVIVED | SURVIVED | reached |
| `id-burned-on-reject` | `internal/service/service.go` | SURVIVED | SURVIVED | reached |
| `self-follow-guard-dropped` | `internal/dom/dom.go`, `internal/service/service.go` | kill | kill | reached |
| `follow-precedence-flipped` | `internal/service/service.go` | SURVIVED | SURVIVED | reached |
| `timeline-scan-reversed` | `internal/store/memstore.go` | kill | kill | reached |
| `timeline-tiebreak-by-id-asc` | `internal/store/memstore.go` | kill | kill | reached |
| `follow-toggles` | `internal/store/memstore.go` | kill | kill | reached |
| `unfollow-rejects-missing-edge` | `internal/service/service.go` | SURVIVED | SURVIVED | reached |
| `orphan-author-accepted` | `internal/service/service.go`, `internal/store/memstore.go` | kill | kill | reached |
| `created-at-frozen` | `internal/service/service.go` | SURVIVED | SURVIVED | reached |
| `tick-advances-by-two` | `internal/clock/clock.go` | kill | kill | reached |
| `tick-goes-backwards` | `internal/clock/clock.go` | kill | kill | reached |
| `next-cursor-always-emitted` | `internal/httpshim/shim.go` | unreached | unreached | **trusted shim** |
| `next-cursor-is-first-id` | `internal/httpshim/shim.go` | unreached | unreached | **trusted shim** |
| `cursor-inclusive` | `internal/store/memstore.go` | kill | kill | reached |
| `limit-off-by-one` | `internal/store/memstore.go` | kill | kill | reached |
| `unknown-json-fields-accepted` | `internal/httpshim/shim.go` | unreached | unreached | **trusted shim** |
| `repeated-query-param-accepted` | `internal/httpshim/shim.go` | unreached | unreached | **trusted shim** |

The four unreached are exactly the four F022 predicted, and nothing else moved
into or out of that set. R4's ceiling on this corner remains 14 of 18.

### The verdict lines, as the tools printed them

Kills:

```
go/self-follow-guard-dropped
   R4  killed         83.9s  R4 FAILED: Gobra has found 2 error(s) over 5 package(s)   [1m23.9s]
   R5  killed         88.9s  R5 FAILED: 1 of 2 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [1m28.9s]
go/timeline-scan-reversed
   R4  killed         57.4s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [57.4s]
   R5  killed         50.3s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [50.3s]
go/timeline-tiebreak-by-id-asc
   R4  killed         57.8s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [57.8s]
   R5  killed         59.6s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [59.5s]
go/follow-toggles
   R4  killed         37.1s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [37s]
   R5  killed         36.9s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [36.9s]
go/orphan-author-accepted
   R4  killed       1033.1s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [17m13s]
   R5  killed         61.9s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [1m1.8s]
go/tick-advances-by-two
   R4  killed         54.8s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [54.8s]
   R5  killed         52.0s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [52s]
go/tick-goes-backwards
   R4  killed         48.9s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [48.9s]
   R5  killed         50.1s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [50.1s]
go/cursor-inclusive
   R4  killed         51.8s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [51.7s]
   R5  killed         50.3s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [50.3s]
go/limit-off-by-one
   R4  killed         32.9s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [32.9s]
   R5  killed         35.5s  R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [35.5s]
```

Survivals — all five identical in shape:

```
go/id-first-is-two
   R4  survived       86.0s  R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m26s]
   R5  survived       73.9s  R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m13.9s]
go/id-burned-on-reject
   R4  survived       82.3s  R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m22.2s]
   R5  survived       85.2s  R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m25.2s]
go/follow-precedence-flipped
   R4  survived       78.4s  R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m18.4s]
   R5  survived       91.2s  R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m31.2s]
go/unfollow-rejects-missing-edge
   R4  survived       71.8s  R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m11.7s]
   R5  survived       70.1s  R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m10s]
go/created-at-frozen
   R4  survived       57.8s  R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [57.7s]
   R5  survived       51.4s  R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [51.4s]
```

Unreached — Gobra was run and passed, but the mutant is outside its perimeter,
so the pass is not a survival:

```
go/next-cursor-always-emitted     R4 PASSED / R5 PASSED   [50.8s / 50.0s]
go/next-cursor-is-first-id        R4 PASSED / R5 PASSED   [55.1s / 52.2s]
go/unknown-json-fields-accepted   R4 PASSED / R5 PASSED   [52.9s / 50.3s]
go/repeated-query-param-accepted  R4 PASSED / R5 PASSED   [50.8s / 49.5s]
```

---

## Why the two rows coincide

Two independent halves, and both come out the same way.

**Reach.** R4's perimeter is the five verified packages
(`clock`, `ids`, `dom`, `store`, `service`). R5's is the four *files* carrying a
refinement clause site — `clock/clock.go`, `ids/ids.go`, `service/service.go`,
`store/memstore.go`. The single file that separates them is `internal/dom/dom.go`:
it carries obligations but no refinement clause, so a mutant confined to it
would be reached by R4 and unreached by R5. **No mutant in the catalogue is
confined to `dom.go`.** The one mutant that touches it,
`self-follow-guard-dropped`, also edits `service.go`. Re-derived from the
manifest: *mutants where R4 reach ≠ R5 reach: 0.*

**Verdict.** For all 9 reached mutants that R4 killed, the failing obligation
landed inside a member carrying a refinement clause. Split:

| where the failure landed | count | mutants |
|---|---|---|
| on the refinement clause line itself | 4 | `self-follow-guard-dropped`, `orphan-author-accepted`, `tick-advances-by-two`, `tick-goes-backwards` |
| elsewhere in a member that carries one | 5 | `timeline-scan-reversed`, `timeline-tiebreak-by-id-asc`, `follow-toggles`, `cursor-inclusive`, `limit-off-by-one` |

The second row is the loop-invariant path inside `(*MemStore).HomeTimeline` —
the machinery that proves the refinement postconditions rather than the
postconditions themselves. The previous fire widened attribution from
clause-line to clause-*or*-member for exactly this reason, and corrected itself
in place. That correction is load-bearing here: **5 of the 9 agreements rest on
it.** Under the clause-line-only reading those five cells would have been
`R5 UNDECIDED` — error cells, not survivals — and the R5 row would have had no
rate at all rather than a different one.

**Survivals.** All five survive both rungs for one reason: the property the
mutant breaks is not stated in any clause, refinement or otherwise. F023 is the
worked case (`id-first-is-two`: the id origin is stated three times in English
and zero times in a clause). Nothing distinguishes a proof rung from a
refinement rung when neither has an obligation to violate.

## What this does and does not license

It does **not** license deleting the R5 row. R5 asks a different question, the
canary proves it answers differently, and a catalogue that contained a
`dom.go`-only mutant would separate the rows on the first cell.

It does license saying that **on this corner, this catalogue and these
contracts, the refinement row buys nothing over the proof row** — and that a
kill table with an R5 column that never differs from R4 is reporting one
measurement twice. Queue item 4 (a second catalogue from a source other than
the contract) is where that gets retested; a catalogue drawn from incident
history has no reason to avoid `dom.go`.
