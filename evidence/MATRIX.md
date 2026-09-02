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
| go ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/15 = 0% ‡ | cap←java † |
| go ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | pending | pending |
| rust ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 1/14 = 7% ‡ | cap←rust † |
| rust ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/15 = 0% ‡ | cap←rust,java |
| rust ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | pending | cap←rust |
| java ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/15 = 0% ‡ | cap←java † |
| java ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | 0/15 = 0% ‡ | cap←java,rust |
| java ← kotlin | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | pending | cap←java |
| kotlin ← go | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | pending | pending |
| kotlin ← rust | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | pending | cap←rust |
| kotlin ← java | 18/18 = 100% | 18/18 = 100% | 7/17 = 41% | n/a | pending | cap←java |

**Cell vocabulary.**

| cell | meaning |
|---|---|
| `k/r = p%` | measured. `k` mutants killed out of `r` **reached** (`reached = live − unreached`; equivalent mutants are outside `live` already). The denominator travels with the rate, per F008 |
| `cap←X` | **capped by X**: corner X has no rung of this kind that yields a kill verdict, so the pair has no cell. Not a measurement, in no denominator. `cap←X,Y` means neither end has it |
| `pending` | the rung exists at both ends and no run has produced this cell. **Eight cells are in this state**, waiting on two runs. Six wait on the Kotlin corner's **R4** sweep: its rung exists and is gated on five mutants, and the 18-mutant run has not been made. Two — the R5 pair `go ← kotlin` and `kotlin ← go` — wait on the Kotlin corner's **R5** sweep, gated on three mutants and one clean tree (`evidence/runs/kotlin-r5-gate/`). Two of the six arrived here from `cap←java` when the Java corner got an obligation set: a cap became a *pending*, which is a smaller improvement than a cap becoming a number and is still the honest cell |
| `n/a` | R3 is a claim about the TLA⁺ model and the `S_obs` link, not about either corner's code (`ASSURANCE.md`: *"Says nothing about code"*). `calibrate` has no R3 rung and is not getting one |
| `†` | one end of this pair is **Go**, whose own R4/R5 evidence is now a completed 18-mutant sweep (9 killed, 5 survived, 4 unreached, 9/14 = 64% killed/reached, R4 and R5 agreeing on all 18 — F028; that agreement is a fact about the shipped catalogue, and a scratch catalogue aimed at the perimeter difference separates the two columns on all three of its mutants — F038, F039). The cell is capped by the other end regardless, so the mark changes no cell; it records that the Go end's half of the pair is not what is missing |
| `‡` | **the two ends' numbers are not comparable in meaning**, so the weaker-end rule produces an arithmetically correct cell that a reader will misread. See "What the measured R4 cells actually say" below. Applies to all six R4 cells that carry a number today |

**Cell census.** 72 cells: **42 measured**, **10 capped**, **8 pending**,
**12 n/a** (the whole R3 column). Every capped cell is now in the R5 column.

Three separate pieces of work landed within an hour of each other and each
moved this census, so it is worth saying what each did rather than only where
it ended up.

- **The R4 column has no capped cell left.** It was entirely capped this
  morning. A Verus driver on the Rust corner and a JBMC driver on the Kotlin
  corner took it to 2 measured, 4 pending and 6 capped; writing the Java
  corner's obligation set (F034) took the last 6. Java's cap was
  "`impls/java` has no obligation set for a rung to run", which was a fact
  about this repository rather than about Java or about JBMC, and four of those
  six cells became measured while two became pending.
- **The R5 column is no longer entirely capped**, by two cells. The Kotlin
  corner now has an R5 rung (`jbmc r5verify`, F046), so `go ← kotlin` and
  `kotlin ← go` are pending rather than capped, and the four other R5 cells
  with Kotlin at one end are now capped by their *other* end alone.
- **The Rust R5 blocker was removed and no R5 cell moved** (F041). The verified
  core came out of its `RwLock` and the abstraction functions have bodies, but
  a cell is a `calibrate` verdict and there is no Verus R5 rung. That is the
  cleanest illustration on this page of the difference between an obligation
  being discharged and a cell being filled.

A correction while counting, and it went two ways. The census line before this
one said "Of the 18 capped, 6 carry `†`"; a merge of two branches proposed
**8**. Counted cell by cell in the table below the answer is **4** — `go ← rust`,
`go ← java`, `rust ← go` and `java ← go`, each in the R5 column only. The other
two pairs with Go at an end, `go ← kotlin` and `kotlin ← go`, carry no capped
cell at all any more: both their R4 and R5 cells are `pending`. Six was stale,
eight was wrong, and neither was arrived at by counting. **Every mark total on
this page is now re-derived from the table rather than carried forward**, which
is the only way a number in a summary line stays true through a merge.

Every measured cell in the **behavioural** columns (R0, R1, R2) is the same
number, because all four corners produced identical outcome vectors on all 18
defects (four-corner run, and see F017 below). That is a finding about the
corners, not a placeholder.

**The R4 column breaks that uniformity, and it is the first column to do so.**
Its six measured cells read `1/14 = 7%` and `0/15 = 0%` where Go's own R4
evidence is `9/14 = 64%`. The difference is not a disagreement between the
corners about any defect — R0 kills 18 of 18 on all four — it is a difference
in where each corner's contracts were written and in what each corner's
verifier can read. That is what the `‡` mark is for, and the section below is
the whole of it.

---

## What the measured R4 cells actually say — read this before quoting `1/14 = 7%` or `0/15 = 0%`

`go ← rust` and `rust ← go` both take **Rust's** number, because the pair's
claim cannot be stronger than either end's evidence and Rust is the weaker end:
Go kills 9 of the 14 mutants its proof reaches, Rust kills 1 of the 14 its proof
reaches. The arithmetic is the rule this document already committed to. **What
the arithmetic hides is that the two 14s are not the same denominator**, and
F027 is the finding about exactly that:

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

So `1/14 = 7%` is a fact about the Rust corner's *proof layout*, not about the
quality of a port between Go and Rust. A reader who compares it to Go's
`9/14 = 64%` and concludes the Rust implementation is worse has read the
wrong thing: the same defect catalogue kills 18 of 18 on both corners at R0.
The `‡` exists to stop that reading.

Both cells are nonetheless real. Rust's one kill — `self-follow-guard-dropped`,
against `Follow::new` — is backed by a negation canary on every clause of the
contract that caught it, run and reported in F030: `REFUTABLE 5, VACUOUS 0`.
Before that instrument existed the cell would not have been allowed onto this
table at all.

### And the four `0/15 = 0%` cells

The Java end is the weaker end of every pair it is in, so `go ← java`,
`rust ← java`, `java ← go` and `java ← rust` all take **Java's** number. It is
a zero, and a zero is the cell most likely to be misread on this table, so:

- **The rung fires.** `calibrate` itself prints "R4 killed NOTHING … check it
  against a known-bad canary before reading this row as evidence about the
  mutants", and it has been: one line of `Dom.parseInt64` broken so a bare sign
  parses as zero gives `R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s)
  … o1a_oneCharAcceptSet, o1c_emptyAndBareSignRejected`
  (`evidence/runs/calibration/java-r4-gate/canary-injection.log`).
- **The 15 is not the 14 above, and not for the same reason.** Java's
  denominator is 18 minus two shim-only mutants (F022's accounting, as on every
  corner) minus one cell that came back UNDECIDED — `tick-goes-backwards` makes
  `o3b` **and its negation** both verify, so the rung printed no verdict and
  `calibrate` recorded an error, never a survival.
- **The zero decomposes into four different zeroes**, and F036 is the whole of
  it: 9 survivals are obligations JBMC cannot read (F014), 3 are properties no
  obligation on either JVM corner states (F3 idempotence, F6 author existence),
  3 are relational obligations a non-relational mutant slips past (F023), and 1
  is the vacuity above (F015 at the proof rung).
- **Three of the seven decidable obligations are over `Dom.parseInt64`, and not
  one of the eighteen mutants edits `Dom.java`.** They are the corner's most
  convincing obligations — quantified over ALL one- and two-character strings —
  and they are aimed where the catalogue is not.

So `0/15 = 0%` is a fact about which obligations JBMC 6.11.0 can read and about
what the Kotlin-twin obligation set was written to answer (F005 and F014), not
about the Java port. The same eighteen defects are killed 18 of 18 by R0 on
this corner. This cell is a smaller, real number in place of a cap, which is
the trade `MATRIX.md` has argued for throughout: a capped cell says "never
asked", and this one says what was asked and what came back.

---

## Why the R5 column is mostly capped, and two of its cells no longer are

`ASSURANCE.md`'s per-corner ceiling table is the source, and no cell here may
contradict it:

| Corner | R4 | R5-core | Ceiling | Rung in `calibrate`? |
|---|---|---|---|---|
| Go | Gobra, 83 of 91 clauses refutable, 0 vacuous, 8 undecided | 26 of 42 clauses | R5-core, partial | **yes, both** (`rungs.go`, Gobra driver) |
| Rust | Verus, **37 of 37 shipped clauses refutable, 0 vacuous**; census 37 shipped / 20 twin / 13 assumed after the lift, from 5 / 36 / 21 (F041, F042) | **statable and partly discharged** — `abs` has bodies, `abs(init) == init_S` and commutation for 3 operations, 17 clauses; **no rung** | R4 on 4 of 5 crates; R5-core has no rung | **yes, R4** (Verus driver) |
| Java | JBMC, 7 of 15 obligations decidable (F014 blocks 8 — the same 8 as Kotlin, measured not assumed: F034) | no | R3 + bounded | **yes, R4** (JBMC driver) |
| Kotlin | JBMC, 7 of 15 obligations decidable (F014 blocks 8) | 5 of 42 clauses, bounded ground instances; 2 more blocked by F014 (F046) | R5-core, bounded and partial | **yes, both** (JBMC driver; `jbmc r5verify`) |

**All four** corners now have an R4 rung that produces a kill verdict — Gobra
on Go, Verus on Rust, JBMC on both JVM corners — so no R4 cell is capped any
more. Java was the last, and its cap was never a matter of effort spent on
JBMC: `impls/java` had no obligation set for a rung to run, so the Kotlin
corner's `Obligations.kt` had no Java twin. It has one now
(`impls/java/verification/`), and the twin measures what F014 could only infer:
the same 7 obligations decidable, the same 8 blocked, the same three reasons
(F034).

**R5 was a different story until the Kotlin corner got an R5 rung.** Go was
the only R5 rung and no ordered pair had Go at both ends, so every R5 cell was
capped. What was missing on the Kotlin corner turned out not to be an
attribution mechanism — JBMC's goal lines name the entry point, the assertion
index, the file *and* the line, which is finer than the Gobra join the Go rung
is built on — but an **abstraction function**. Three of its four axes are
decidable and the follows axis is not; F046 has the transcripts. So `go ←
kotlin` and `kotlin ← go` are now pending rather than capped, and the R5 column
is capped for 10 of 12 pairs rather than 12.

Two of the remaining reasons are structural rather than a matter of effort:

- **Rust's R5-core is no longer structurally blocked, and still has no rung.**
  The verified core was lifted out of its `RwLock` on 2026-09-02
  ([F041](findings/F041-the-r5-blocker-was-the-lock-not-the-property.md)):
  `abs_users` / `abs_follows` / `abs_tweets` have bodies, and `abs(init) ==
  init_S` plus state commutation for `put_user`, `put_follow` and `put_tweet`
  are discharged on shipped functions. **The cells do not change**, because a
  cell is a `calibrate` verdict and `rungs.go` hard-codes R5 as Gobra with a
  Go-only file list — and because the response axis is blocked one level below
  the lock, on `String`'s view not being known injective
  ([F043](findings/F043-the-abstraction-is-not-injective-and-vstd-will-not-say-it-is.md)).
  So *every pair with Rust at either end is still capped below R5*, for a
  different and smaller reason than before.
- **R5-wire is not reachable in any corner** (F012): the decode boundary is
  outside every verification perimeter by construction. The R5 column here is
  R5-core throughout.

The `†` rows: the Go end's own R4/R5 evidence is a **completed 18-mutant
sweep** — `evidence/runs/calibration/go-proof/`, 9 killed / 5 survived / 4
unreached at both rungs, `9/14 = 64%` killed/reached, and R4 and R5 disagreeing
on 0 of 18 (F028). (This paragraph said the opposite until now — "it is a gate,
not a sweep … 5 of 18 … it is not yet known whether the two rows discriminate
at all" — describing `r45-gate/` while the provenance table three sections
below already recorded `go-proof` as done. Two statements about the same
evidence in one document, and the older one was left standing. Corrected here;
the gate run still exists and is still the gate, it is just no longer the only
Go R4 evidence.) F022 bounds where that sweep could land before it was run: 4
of 18 Go mutants edit only `internal/httpshim`, which no obligation covers, so
**R4's ceiling on the Go end is 14 of 18 — a killed/reached denominator of 14,
not 18 — before a single obligation is written**, and the completed sweep lands
exactly on that denominator.

**Whether the two rows discriminate at all is now settled, and they do.** The
18-mutant sweep still shows 0 disagreements (F028), but that is a property of
`tools/cmd/mutate/mutants.json`, not of the rungs. A scratch catalogue built to
straddle the two perimeters — `evidence/experiments/r4-r5-separation/` — puts
the columns apart in both ways they can differ:

| scratch mutant | edits | R4 | R5 |
|---|---|---|---|
| `handle-alphabet-widened` | `internal/dom/dom.go` | kill | unreached |
| `text-control-chars-accepted` | `internal/dom/dom.go` | kill | unreached |
| `clock-now-off-by-one` | `internal/clock/clock.go` | kill | **SURVIVED** |

The first two separate by **reach**: `internal/dom/` is in `gobraVerified` and
`dom.go` is in no `r5Files` entry (F038). The third separates by **verdict**:
`clock.go` *is* in `r5Files`, and R5 reached the defect, read Gobra's own
failing postcondition, found no refinement clause on it and passed — R5's
`killed/reached` is `0/1` (F039). These three ids are deliberately **not** in
the shipped catalogue, so no denominator on this page moves. F022 bounds where
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
| R4/R5 separation (Go end, scratch catalogue) | `go run ./tools/cmd/calibrate -manifest evidence/experiments/r4-r5-separation/manifest.json -impls go -rungs R4,R5 -out evidence/runs/calibration/dom-separation -resume` | **done**, 3 mutants x 2 rungs, 6 cells, 203s + 197s. R4 3/3 killed; R5 2 unreached and 1 survived, `killed/reached 0/1`. Not part of any published denominator — its ids are outside `tools/cmd/mutate/mutants.json` on purpose (F038, F039) |
| R5 (Kotlin end) | `go run ./tools/cmd/calibrate -impls kotlin -rungs R5 -out evidence/runs/calibration/kotlin-refinement -resume` | **rung exists, sweep not run** — gate only, in `evidence/runs/kotlin-r5-gate/`: one clean tree and three mutants through `calibrate` end to end. This is what the two new `pending` R5 cells are waiting on |
| R4 (Rust end) | `go run ./tools/cmd/calibrate -impls rust -rungs R4 -out evidence/runs/calibration/rust-proof -resume` | **done**, all 14 covered mutants, 1 killed (F027). Vacuity audited: `go run ./tools/cmd/verus canary` reports REFUTABLE 5, VACUOUS 0 (F030) |
| R4 (Kotlin end) | `go run ./tools/cmd/calibrate -impls kotlin -rungs R4 -out evidence/runs/calibration/kotlin-proof -resume` | **rung exists, sweep not run** — gate only, 5 mutants in `kotlin-r4-gate`. This is what the four `pending` cells are waiting on, and it is the cheapest cell-filling move on this table |
| R4 (Java end) | `go run ./tools/cmd/calibrate -impls java -rungs R4 -out evidence/runs/calibration/java-proof -resume` | **done**, 18 mutants, window 2026-09-02T20:29:40Z .. 2026-09-02T20:44:36Z, 812s. 0 killed / 15 survived / 2 unreached / 1 error, `0/15 = 0%` (F036). Gate first: `java-r4-gate/canary-injection.log` is `R4 FAILED` on a hand-broken `parseInt64`, and `java-r4-gate/sweep` is a five-mutant `calibrate` run showing survivals and the vacuity error cell |
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

**F038 and F039 bound what the R5 column adds.** The two columns are one Gobra
invocation read twice, and the R5 reading is the strictly narrower question. On
the shipped catalogue that makes the R5 column carry nothing the R4 column does
not (F028); on defects aimed at the perimeter difference it makes R5 the weaker
row, not the stronger one — `clock-now-off-by-one` is a live, spec-violating,
R4-killed defect that R5 passes. Read the R5 column as *"a refinement obligation
is what broke"*, never as *"R4 plus more"*.

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
