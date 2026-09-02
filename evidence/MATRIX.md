# The port matrix — twelve ordered corner pairs × the rungs

**This file is a skeleton, not a result.** Every cell below either carries a
number that already exists in `evidence/runs/` or in `evidence/CALIBRATION*.md`,
or says in words why it does not. Nothing here is estimated, interpolated, or
carried over from a similar cell. A cell with no measurement says so.

---

## Read this first — the same caveat that leads `evidence/CALIBRATION.md`

**The catalogue and the corpus are drawn from the same source.** Every mutant
injects a violation of something `S_obs` pins, and `generated/conformance.jsonl`
is generated *from* `S_obs` to exercise exactly those terms. A rung scoring 100%
here is therefore as much a statement about that alignment as about the rung's
power.

The table says: *against defects stated in the contract's own terms, this rung
catches this fraction of them.* It does not say the rung catches defects nobody
thought to specify. F008 and F009 are two recorded occasions when it
demonstrably did not, and both were closed by **adding inputs** — which is a
large part of why R0 looks as good as it does. A catalogue drawn from a
different source (production incidents, a fuzzer's crash corpus, real defect
history) would produce a different and more informative table. That is GOAL.md
queue item 4 and it is not done.

A second caveat, F006's, applies to the agreement *between* corners: one author
wrote all four implementations against one contract. Four corners agreeing is
closer to one measurement taken four times than to four independent
confirmations.

---

## What a cell is — settled 2026-09-02, and easy to misread

**A matrix cell is an ordered corner pair, B ← A, over the four implementations
that already exist.** The corners are Go, Rust, Java and Kotlin. A cell is
**corner B measured with corner A as the base it must agree with**. Twelve cells
= the twelve ordered pairs over four existing corners.

**No cell requires writing a new implementation.** A reader who assumes twelve
fresh ports — twelve new codebases, one per row — will misread every row in this
document. There are four codebases. The twelve rows are directions of claim over
those four, and `go ← rust` and `rust ← go` are different rows because the
weaker end of a port claim is not symmetric in what it caps.

Three consequences follow, and each is load-bearing:

1. **Every rung's oracle today is `S_obs`, not corner A.** `replay` (R0)
   compares against a corpus generated from `S_obs`; `diffrun` (R1) states it in
   its own package comment — *"The oracle is S_obs, NOT another implementation"*,
   for the reason F006 records; `proptest` (R2) checks relations that name
   neither corner; R4/R5 discharge obligations written against `S_obs`. So a
   cell's **number** is a property of B (and of A, separately), and A enters the
   cell in the two ways below. A rung whose oracle is corner A itself does not
   exist in this repository and is not planned.
2. **A cell is capped by its weaker end.** Per `ASSURANCE.md`'s port-claim
   table, "R5 on A only" licenses "A is correct; B is only as good as R0–R2" —
   the port itself is unproven. So a rung that does not exist for **either** end
   yields a capped cell, which is not a measurement and belongs to no
   denominator.
3. **Where both ends have the rung, the cell takes the weaker end's number.**
   Not B's alone: the pair's claim cannot be stronger than either end's evidence
   for it. On the four-corner run this rule changes nothing, because all four
   corners produced identical outcome vectors on all 18 defects — which is
   itself F017's rule 1 confirmed, and F006's warning about why the agreement is
   not four confirmations.

`calibrate` is per corner, not per pair. Filling this table therefore means
running the corners and applying the two rules above, and the provenance table
below records which run supplies each column.

---

## The table

Columns are the rungs; `ASSURANCE.md` defines each. Rows are the twelve ordered
pairs.

| B ← A | R0 corpus | R1 diff-fuzz | R2 property | R3 model | R4 proof | R5 refinement |
|---|---|---|---|---|---|---|
| go ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←rust † | cap←rust † |
| go ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java † | cap←java † |
| go ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←kotlin † | cap←kotlin † |
| rust ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←rust † | cap←rust † |
| rust ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←rust,java | cap←rust,java |
| rust ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←rust,kotlin | cap←rust,kotlin |
| java ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java † | cap←java † |
| java ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java,rust | cap←java,rust |
| java ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java,kotlin | cap←java,kotlin |
| kotlin ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←kotlin † | cap←kotlin † |
| kotlin ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←kotlin,rust | cap←kotlin,rust |
| kotlin ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←kotlin,java | cap←kotlin,java |

**Cell vocabulary.**

| cell | meaning |
|---|---|
| `k/r = p%` | measured. `k` mutants killed out of `r` **reached** (`reached = live − unreached`; equivalent mutants are outside `live` already). The denominator travels with the rate, per F008 |
| `cap←X` | **capped by X**: corner X has no rung of this kind that yields a kill verdict, so the pair has no cell. Not a measurement, in no denominator. `cap←X,Y` means neither end has it |
| `pending` | the rung exists at both ends and no run has produced this cell. **No cell is in this state today** |
| `n/a` | R3 is a claim about the TLA⁺ model and the `S_obs` link, not about either corner's code (`ASSURANCE.md`: *"Says nothing about code"*). `calibrate` has no R3 rung and is not getting one |
| `†` | one end of this pair is **Go**, the only corner with an R4/R5 rung — and its numbers so far are a 5-of-18 gate, not a sweep. The cell is capped by the other end regardless, so the mark changes no cell; it exists so a later fire does not mistake the gate for the sweep |

**Cell census.** 72 cells: **36 measured**, **24 capped**, **12 n/a** (the whole
R3 column), **0 pending**. Of the 24 capped, 12 carry `†` — the six rows with Go
at one end, at both proof rungs.

Every measured cell in a column is currently the same number, because all four
corners produced identical outcome vectors on all 18 defects (four-corner run,
and see F017 below). That is a finding about the corners, not a placeholder.

---

## Why the R4 and R5 columns are entirely capped

`ASSURANCE.md`'s per-corner ceiling table is the source, and no cell here may
contradict it:

| Corner | R4 | R5-core | Ceiling | Rung in `calibrate`? |
|---|---|---|---|---|
| Go | Gobra, 91 of 91 clauses refutable, 0 vacuous, 0 undecided | 30 of 42 clauses | R5-core, partial | **yes, both** (`rungs.go`, `Impls: ["go"]`) |
| Rust | Verus, **1 property** (F016) | no — `RwLock` has no vstd model | R4, one property (F4) | no |
| Java | not attempted | unknown | R3 | no |
| Kotlin | JBMC, bounded | no | R3 + bounded | no |

Only Go has an R4 or R5 rung that produces a kill verdict, and no ordered pair
has Go at both ends. So **every R4 and R5 cell is capped by at least one end**,
today, for a reason recorded in `ASSURANCE.md` rather than for want of running
something. Two of those reasons are structural rather than a matter of effort:

- **Rust cannot reach R5-core at all** until the verified core is lifted out of
  its `RwLock`. That is a refactor, not an annotation, so *every pair with Rust
  at either end is capped below R5* until it happens.
- **R5-wire is not reachable in any corner** (F012): the decode boundary is
  outside every verification perimeter by construction. The R5 column here is
  R5-core throughout.

The `†` rows: the Go end's own R4/R5 evidence exists, but it is a gate, not a
sweep — `evidence/runs/calibration/r45-gate/` covers **5 of 18** Go mutants and
reports R4 3/4 = 75% and R5 3/4 = 75% (killed/reached; 1 unreached in the
trusted shim). Five is a gate, not a rate, and R4 and R5 agreed on all five, so
it is not yet known whether the two rows discriminate at all. F022 bounds where
that sweep can land before it is run: 4 of 18 Go mutants edit only
`internal/httpshim`, which no obligation covers, so **R4's ceiling on the Go end
is 14 of 18 — a killed/reached denominator of 14, not 18 — before a single
obligation is written.**

---

## How each column gets filled

A later fire fills cells by running a command, not by deciding what a cell
means. Each invocation is run from the repository root.

| column | invocation | state |
|---|---|---|
| R0, R1, R2 | `go run ./tools/cmd/calibrate -impls go,rust,java,kotlin -rungs R0,R1,R2 -out evidence/runs/calibration/four-corner -resume` | **done**, 216 cells, window 2026-08-30T22:33:29Z .. 2026-08-31T03:30:42Z |
| R4 (Go end) | `go run ./tools/cmd/calibrate -impls go -rungs R4 -out evidence/runs/calibration/go-proof -resume` | gate only (2 mutants, `r4-gate`); the 18-mutant sweep is GOAL.md queue item 2 |
| R5 (Go end) | `go run ./tools/cmd/calibrate -impls go -rungs R4,R5 -out evidence/runs/calibration/go-proof -resume` | gate only (5 mutants, `r45-gate`) |
| R4 (Rust end) | *does not exist.* Needs the `cargo-verus` equivalent of `gobra verify`'s verdict line and budget, then an `R4` entry in `rungs.go` with `rust` in `Impls` — GOAL.md queue item 1 | blocked |
| R4 (Java, Kotlin ends) | *does not exist.* JBMC over bytecode, same shape; F014's string-equality defect bounds what it can claim | blocked |
| R3 | no invocation. `tlclink` checks the model and the `S_obs` link; it produces no per-corner kill verdict and no cell here | by design |

Defaults the four-corner run used, which any comparable run must match or
declare: `-r1-traces 20 -r1-steps 200 -r1-seed 1 -r2-rounds 6 -r2-setup 40
-r2-seed 1 -probe any -probe-traces 4 -probe-steps 120 -floor-samples 3
-rung-timeout 20m`. `mutate verify` must pass immediately before any sweep —
a drifted anchor injects nothing and every rung "kills" it (F011).

Reading a filled cell out of a run: `calibrate` reports each rung's rate with
its denominator already attached (`killed/reached`, plus a per-rung
`DENOMINATORS` block naming every excluded cell and why), and `results.json`
carries `reached`, `cells` and `excluded` per summary row. A cell of this table
is copied from there, never recomputed by hand.

---

## What the measured columns do and do not license

**R0 and R1 are at 100% and neither can be ranked from this table.** A rung that
kills everything has told you about the catalogue, not the rung. R1 added *zero*
kills over R0 across all four corners; that is a statement about the catalogue's
discriminating power, not a case for dropping the rung that found F008.

**R2 at 7/17 is the one non-degenerate number**, and its single unreached cell
is `follow-precedence-flipped` in every corner. R2 checks consistency, not
conformance, and most of this catalogue is conformance
(`evidence/CALIBRATION-four-corner.md` lists the ten survivors).

**F017 bounds the row-to-row comparison, and only for R4/R5.** Behavioural rungs
drive the observable API and saw the same defect in every corner — confirmed,
identical outcome vectors on all 18 defects. R4 and R5 are different: two
defects (`next-cursor-always-emitted`, `next-cursor-is-first-id`) sit in the
**verified core** in Java and Kotlin and in the **trusted shim** in Go and Rust,
so the same mutant id is inside one corner's proof perimeter and outside
another's. When the R4/R5 columns start carrying numbers, **those two defects'
cells are not comparable across rows**, and the coverage denominator is where
that shows up: they are `unreached` on one end and reachable on the other, so
the two ends' denominators are not the same 18.

**F023 warns against reading the columns as ordered.** `id-first-is-two` is
killed by R0 and R1 and survives R2, R4 *and* R5 on the Go corner. A rung
further right is not a superset of the rungs to its left, and a filled R5 column
will not dominate R2's.

---

## Attribution

Every rung that kills a mutant is credited with that kill; the sweep does not
stop at the first killing rung (`runRungs` in `tools/cmd/calibrate/sweep.go`,
locked by `TestEveryRungRunsAfterAKill`). So a row's rungs are **not** mutually
exclusive: the same mutant may appear in the kill count of R0, R1, R2, R4 and
R5, and the columns do not sum to anything. First-judge attribution would make
every column after R0 read as zero on this catalogue, since R0 kills 72 of 72.
