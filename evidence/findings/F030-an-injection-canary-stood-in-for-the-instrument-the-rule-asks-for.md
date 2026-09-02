# F030 — an injection canary was standing in for the instrument the rule asks for

**Status:** found by reading the three R4 drivers against GOAL.md's own rule,
then closed by building the missing instrument and running it
**Class:** a gap in the rig, not a defect in any implementation — and the
second time the same substitution has been made in this project

## The rule, and what was actually satisfying it

GOAL.md's LOOP section says it in one sentence:

> **VERIFIED means refutable.** A rung's kill verdict on a proof-backed row
> counts only if the obligation that noticed the mutant was itself shown
> non-vacuous. `gobra canary` / `gobra reach` is the instrument; the
> equivalents for Verus and JBMC must exist before their rows are trusted.

Checked against the three drivers as they stood at `800edb2`:

| corner | driver | negation canary? |
|---|---|---|
| Go | `tools/cmd/gobra` | yes — `canary`, `reach`, `audit`, and a self-test that makes a live clause vacuous with `assume false` and requires the sweep to notice |
| Kotlin | `tools/cmd/jbmc` | yes — every obligation the rung claims carries at least one negation canary, and a VERIFIED with no canary naming it makes the whole run UNDECIDED |
| Rust | `tools/cmd/verus` | **no. Nothing.** `grep -n "vacu\|VACUOUS\|negat\|canary" tools/cmd/verus/*.go` returned nothing at all |

What the Rust rung had instead, quoted from its own loop-log entry, is
labelled as satisfying the rule:

> Canary (standing rule 2), the same materialised tree twice, one injected
> line apart — `false,` added to `Follow::new`'s `ensures` list

That is an **injection canary**: break the code, check the gate notices. It is
a real and necessary check — it is what caught the cargo-cache false green, and
that catch was worth the fire on its own — but it is not the check the rule
asks for, and F013 already says why in this repository's own words:

> an injection canary asks "if I break the code, does the gate notice". Put to
> a vacuous obligation that question is ill-posed — an obligation nothing
> reaches verifies over the broken code too, and over the infeasible point the
> injection created. Only a NEGATION canary can see it: under vacuity a claim
> AND its negation both verify, and nothing else produces that signature.

So the Rust R4 row — one kill in fourteen, thirteen survivals, quoted in
F027, in `MATRIX.md` and in PR #4 — was being reported under a rule it did not
meet, with a differently-shaped canary standing in the place of the missing
one. **F013 is a finding about exactly this substitution.** It has now been
made twice.

## What the instrument found

`tools/cmd/verus canary` is the missing instrument. For each `ensures` clause
on a shipped function it replaces the whole clause list with the **negated
antecedent** and asks Verus to prove it. Proved means the antecedent is
unsatisfiable and the obligation is discharged over nothing; refuted means the
antecedent is reachable and the obligation is load-bearing.

It self-tests before it reports anything, and the self-test is not decoration —
it is the only reason the sweep's zero can be believed:

```
self-test (does this sweep report VACUOUS when it should?) --
  probe crates/domain/src/lib.rs:131 from@ == to@ ==> result is Err
  with `requires false,` spliced in, every postcondition is vacuously provable,
  so the canary MUST come back VACUOUS
  -> VACUOUS [4s]
     domain: verification results:: 9 verified, 0 errors -- Verus PROVED the negated antecedent, so the clause's antecedent is unsatisfiable and the obligation is discharged over nothing
  self-test PASSED: the sweep reports VACUOUS when the obligation is unreachable
```

Baseline and sweep, Verus's own lines:

```
baseline (unmodified copy) --
  R4 PASSED: verification results:: 21 verified, 0 errors over 5 of 5 verify-enabled crate(s)   [2m17.5s]

sweep --
  [ 1/ 5] REFUTABLE      1s  crates/domain/src/lib.rs:131 from@ == to@ ==> result is Err
  [ 2/ 5] REFUTABLE      1s  crates/domain/src/lib.rs:132 from@ != to@ ==> result is Ok
  [ 3/ 5] REFUTABLE      1s  crates/domain/src/lib.rs:133 result is Ok ==> result->Ok_0.from@ == from@
  [ 4/ 5] REFUTABLE      1s  crates/domain/src/lib.rs:134 result is Ok ==> result->Ok_0.to@   == to@
  [ 5/ 5] REFUTABLE      1s  crates/domain/src/lib.rs:135 result is Ok ==> result->Ok_0.from@ != result->Ok_0.to@
           domain: verification results:: 8 verified, 1 errors -- Verus refuted the negated antecedent, so it is reachable

canary sweep: 5 clause(s)   REFUTABLE 5   VACUOUS 0   ILL-FORMED 0   TIMEOUT 0
```

**Five of five refutable, none vacuous.** The row survives its audit. The
baseline's 21 is the post-repair count F024 recorded (23 → 21), not a new
disagreement.

## The number that matters more than the zero

The sweep also counts what it is not sweeping, and that is the substantive
result:

```
  ensures blocks: 1 on shipped functions, 15 inside #[cfg(verus_only)] mod verus_proof
  clauses:        5 shipped, 57 twin
```

**Five of the Rust corner's sixty-two `ensures` clauses are on shipped
functions. Fifty-seven are on twins.** F027 established the shape of that split
by reading the code; this is the ratio, re-derived mechanically from the tree
by `extractClauses`, with a test that fails the day a clause moves out of a
twin.

So "the Rust R4 row is not measuring vacuous obligations" is a claim about
**8% of the corner's obligations**. The other 92% are not audited by this
instrument and are not going to be, because a negation canary on a twin
measures the twin: the clause is on a hand-written function over an
`external_body` shim, and refuting its negated antecedent says the *twin's*
antecedent is reachable, which is a fact about code that does not ship. The
`-twins` flag will sweep them on request; the default does not, and the report
says how many it left alone rather than quietly shrinking its denominator.

## What this does and does not license

- The one Rust R4 kill (`self-follow-guard-dropped`, `Follow::new`) is now
  backed by a negation canary on every clause of the contract that noticed it.
  It counts.
- The thirteen Rust R4 survivals are untouched by this. They survive for
  F027's structural reason — no clause on the shipped function to break — and
  a vacuity audit was never what was wrong with them.
- The Kotlin row was already compliant and is unaffected.
- **The general lesson is about the rig, not about Verus.** Two of three
  corners built the negation canary; the third built an injection canary,
  described it in the loop log with the words the rule uses, and no gate
  noticed the difference, because nothing mechanically checks that a corner's
  R4 row has a vacuity instrument behind it. The check that would have caught
  this is a test asserting every corner with an R4 driver has a canary
  subcommand — cheap, and not yet written.

## Reproduce

```
go run ./tools/cmd/verus canary -list          # the clauses and the canary each gets, no runs
go run ./tools/cmd/verus canary -budget 12m    # baseline, self-test, sweep
```

Full log: `evidence/runs/calibration/rust-canary/sweep.log`.
The sweep never writes to `impls/rust`; it copies the tree to a temp directory
and splices there, so a run killed between the splice and the restore cannot
leave the measured tree mutated.
