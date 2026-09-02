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
| go ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 1/14 = 7% ‡ | cap←rust † |
| go ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java † | cap←java † |
| go ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/14 = 0% ‡ | cap←kotlin † |
| rust ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 1/14 = 7% ‡ | cap←rust † |
| rust ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java | cap←rust,java |
| rust ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/14 = 0% ‡ | cap←rust,kotlin |
| java ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java † | cap←java † |
| java ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java | cap←java,rust |
| java ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java | cap←java,kotlin |
| kotlin ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/14 = 0% ‡ | cap←kotlin † |
| kotlin ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/14 = 0% ‡ | cap←kotlin,rust |
| kotlin ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | cap←java | cap←kotlin,java |

**Cell vocabulary.**

| cell | meaning |
|---|---|
| `k/r = p%` | measured. `k` mutants killed out of `r` **reached** (`reached = live − unreached`; equivalent mutants are outside `live` already). The denominator travels with the rate, per F008 |
| `cap←X` | **capped by X**: corner X has no rung of this kind that yields a kill verdict, so the pair has no cell. Not a measurement, in no denominator. `cap←X,Y` means neither end has it |
| `n/a` | R3 is a claim about the TLA⁺ model and the `S_obs` link, not about either corner's code (`ASSURANCE.md`: *"Says nothing about code"*). `calibrate` has no R3 rung and is not getting one |
| `†` | one end of this pair is **Go**, whose own R4/R5 evidence is now a completed 18-mutant sweep (9 killed, 5 survived, 4 unreached, 9/14 = 64% killed/reached, R4 and R5 agreeing on all 18 — F028). The cell is capped by the other end regardless, so the mark changes no cell; it records that the Go end's half of the pair is not what is missing |
| `‡` | **the two ends' numbers are not comparable in meaning**, so the weaker-end rule produces an arithmetically correct cell that a reader will misread. See "What the R4 cells actually say" below. Applies to **all six** R4 cells that carry a number, and F033 is the evidence that it has to: three corners now report an R4 rate, all three denominators are `14`, and the three `14`s are set by three unrelated things |

**Cell census.** 72 cells: **42 measured**, **18 capped**, **0 pending**,
**12 n/a** (the whole R3 column). Of the 18 capped, 6 carry `†`.

**Nothing on this table is pending any more.** The four cells that were —
`go ← kotlin`, `kotlin ← go`, `rust ← kotlin`, `kotlin ← rust` — all waited on
one run, the Kotlin corner's 18-mutant R4 sweep, and it has been made:
`evidence/CALIBRATION-kotlin-proof.md`, window
`2026-09-02T20:05:49Z .. 2026-09-02T20:51:21Z`. So the R4 column is now
**6 measured and 6 capped**, the 6 capped all by Java, which has no obligations
written at all. R5 is unchanged and still entirely capped: Gobra on Go is the
only R5 rung, and no ordered pair has Go at both ends.

Every remaining gap on this table is a *cap*, not a *to-do*. Filling any of the
18 requires writing an obligation set that does not exist (Java's R4/R5) or
lifting a verified core out of a lock (Rust's R5, F012), not running a
command.

Every measured cell in the **behavioural** columns (R0, R1, R2) is the same
number, because all four corners produced identical outcome vectors on all 18
defects (four-corner run, and see F017 below). That is a finding about the
corners, not a placeholder.

**The R4 column breaks that uniformity, and it is the first column to do so.**
Its six measured cells carry two distinct numbers — `1/14 = 7%` and
`0/14 = 0%` — against per-corner evidence of `9/14 = 64%` (Go), `1/14 = 7%`
(Rust) and `0/14 = 0%` (Kotlin). The spread is not a disagreement between the
corners about any defect: **R0 kills 18 of 18 on all three.** It is a difference
in where each corner's contracts were written and what each corner's tool can
decide. That is what the `‡` mark is for, and the section below is the whole
of it.

The two proof rungs have now also been compared cell by cell, which no two rungs
in this repository had been before. On the 12 mutants where Gobra and JBMC both
returned a kill-or-survive verdict, **they disagree on 8** (F033). F028 found
R4 and R5 agreeing on 18 of 18, but those were the same Gobra run read two ways;
this is two verifiers, two corners, two independently written obligation sets,
and two thirds of the overlap in dispute.

---

## What the R4 cells actually say — read this before quoting `1/14 = 7%` or `0/14 = 0%`

`go ← rust` and `rust ← go` both take **Rust's** number, and the four Kotlin
pairs all take **Kotlin's**, because the pair's claim cannot be stronger than
either end's evidence and those are the weaker ends: Go kills 9 of the 14
mutants its proof reaches, Rust 1 of its 14, Kotlin 0 of its 14. The arithmetic
is the rule this document already committed to. **What the arithmetic hides is
that the three 14s are not the same denominator** — they are not even the same
*kind* of denominator — and F027 and F033 are the findings about exactly that:

- **Go's 14** is set by the trusted transport shim. Four mutants edit only
  `internal/httpshim`, which no obligation covers (F022). The other 14 are
  inside the proof perimeter and 9 of them break a clause on a shipped
  function.
- **Rust's 14** is set by where the contracts were written. Thirteen of those 14
  mutants edit production code whose obligations live in a hand-written twin
  inside `#[cfg(verus_only)] mod verus_proof`, so the mutant leaves the twin
  untouched and verifying. Only `crates/domain` puts its `ensures` on the
  shipped function. Measured since F027 and quantified in F030: **5 of the Rust
  corner's 62 `ensures` clauses are on shipped functions; 57 are on twins.**

- **Kotlin's 14** is set by a tool defect plus two missing measurements, and it
  is the only one of the three that is not `live − unreached`. Two mutants are
  confined to `src/twitterport/httpshim/` and are unreached in the F022 sense.
  Two more are **error cells** — trees on which the rung produced no verdict at
  all, so nothing was measured: `id-burned-on-reject` changes
  `Store.createUser`'s arity and the obligation tree stops compiling (F031), and
  `tick-goes-backwards` makes the log's own invariant guard throw, which leaves
  the negation canary unreachable and the vacuity audit unable to read the tree
  (F032). Underneath both, **8 of the corner's 15 obligations are undecidable by
  JBMC 6.11.0** (F014) and in no denominator — and those 8 are not spread evenly
  over the contract, they are concentrated on exactly what this catalogue
  attacks: six of the fourteen survivals would have been caught by a blocked
  obligation.

So `1/14 = 7%` is a fact about the Rust corner's *proof layout* and
`0/14 = 0%` is a fact about **JBMC's inability to compare two strings**, not
about the quality of a port between Go and Kotlin. A reader who compares either
to Go's `9/14 = 64%` and concludes those implementations are worse has read the
wrong thing: the same defect catalogue kills 18 of 18 on all three corners at
R0, byte-exact. The `‡` exists to stop that reading, and it now sits on all six
numbered R4 cells rather than two.

All six cells are nonetheless real. Rust's one kill — `self-follow-guard-dropped`,
against `Follow::new` — is backed by a negation canary on every clause of the
contract that caught it, run and reported in F030: `REFUTABLE 5, VACUOUS 0`.
Before that instrument existed the cell would not have been allowed onto this
table at all.

**A zero needs the same instrument a kill does, and it has it.** `calibrate`
flags its own row — *"R4 killed NOTHING (0/16 live mutants). A rung that never
fires has not been shown to be able to fire"* — and standing rule 2 agrees. The
demonstration is `evidence/runs/calibration/kotlin-r4-canary-injection.log`, the
same instrument pointed at a deliberately broken tree, which reports
`R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s)`. The rung fires. What
F032 adds is the sharper reading: the two obligations it fired on are
`parseInt64` obligations over `src/twitterport/dom/Dom.kt`, and **no mutant in
the catalogue edits `Dom.kt`** — so the rung's demonstrated firing path and the
catalogue's reach do not intersect.

---

## Why the R5 column is entirely capped, and R4 no longer is

`ASSURANCE.md`'s per-corner ceiling table is the source, and no cell here may
contradict it:

| Corner | R4 | R5-core | Ceiling | Rung in `calibrate`? |
|---|---|---|---|---|
| Go | Gobra, 83 of 91 clauses refutable, 0 vacuous, 8 undecided | 26 of 42 clauses | R5-core, partial | **yes, both** (`rungs.go`, Gobra driver) |
| Rust | Verus, **1 property** (F016, F027); 5 of 62 clauses on shipped functions, all 5 refutable (F030) | no — `RwLock` has no vstd model | R4, one property (F4) | **yes, R4** (Verus driver) |
| Java | not attempted; `impls/java` has no obligation set at all | unknown | R3 | no |
| Kotlin | JBMC, 7 of 15 obligations decidable (F014 blocks 8); swept over all 18 mutants, `0/14 = 0%` | no | R3 + bounded | **yes, R4** (JBMC driver) |

Three corners now have an R4 rung that produces a kill verdict — Gobra on Go,
Verus on Rust, JBMC on Kotlin — so the R4 column is capped only where **Java**
is an end, which is six of the twelve pairs. Java's cap is not a matter of
effort spent on JBMC either: `impls/java` has no obligation set for a rung to
run, so the Kotlin corner's `Obligations.kt` has no Java twin.

**R5 is a different story and is still entirely capped.** Only Go has an R5
rung, and no ordered pair has Go at both ends, so every R5 cell is capped by at
least one end for a reason recorded in `ASSURANCE.md` rather than for want of
running something. Two of those reasons are structural rather than a matter of
effort:

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
| R4, R5 (Go end) | `go run ./tools/cmd/calibrate -impls go -rungs R4,R5 -out evidence/runs/calibration/go-proof -resume` | **done**, 18 mutants x 2 rungs, 36 cells, 2043s + 1059s. 9 killed / 5 survived / 4 unreached at both rungs, 0 disagreements (F028) |
| R4 (Rust end) | `go run ./tools/cmd/calibrate -impls rust -rungs R4 -out evidence/runs/calibration/rust-proof -resume` | **done**, all 14 covered mutants, 1 killed (F027). Vacuity audited: `go run ./tools/cmd/verus canary` reports REFUTABLE 5, VACUOUS 0 (F030) |
| R4 (Kotlin end) | `go run ./tools/cmd/calibrate -impls kotlin -rungs R4 -out evidence/runs/calibration/kotlin-proof -resume` | **done**, all 18 mutants, 2085 s, window `2026-09-02T20:05:49Z .. 2026-09-02T20:51:21Z`. 0 killed / 14 survived / 2 unreached / 2 error, `0/14 = 0%`. Vacuity audited on every tree that passed: the verdict line itself reads `every one refutable in this tree`. Write-up in `evidence/CALIBRATION-kotlin-proof.md`; F031, F032, F033 |
| R4 (Java end) | *does not exist, and not for want of a tool.* `impls/java` carries no obligation set, so there is nothing for a JBMC rung to run. Writing one is a Java-corner job, not a rung job | blocked |
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

**That prediction is now measured, and it held exactly.** The Kotlin R4 sweep
scores both `next-cursor-always-emitted` and `next-cursor-is-first-id` as
`SURVIVED` — reached, inside the verified core, in `store/Store.kt` — where the
Go sweep scores both `unreached` in `internal/httpshim`. So Go's outer reach is
14 of 18 and Kotlin's is 16 of 18, over the identical mutant ids. The two ends
arrive at a denominator of 14 by different subtraction: Go by removing 4 shim
mutants, Kotlin by removing 2 shim mutants and 2 error cells (F031, F032). Same
integer, different quantity — see F033.

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
