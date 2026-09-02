# Assurance ladder

Six rungs. Each licenses a strictly stronger claim and costs more. A port's
claim is capped by the **weaker** of its two ends.

| Rung | Method | Base-app changes | Licenses |
|---|---|---|---|
| R0 | Generated conformance replay | none | Agreement on the generated corpus |
| R1 | Differential trace agreement against a live base process | none (JSONL adapter only) | Agreement on N sampled traces, within the generator's distribution |
| R2 | Shared property + metamorphic suite | none | Named relations hold in both |
| R3 | TLC on `twitter.tla`, plus the `S_obs` link check | n/a | The model satisfies F1-F9 at bound; `S_obs` refines it. Says nothing about code |
| R4 | Per-language deductive proof of F1-F9 | none | Each implementation satisfies the same property *numbers*. **Not equivalence** |
| R5-core | Refinement to `S_obs` over the *decoded operation* alphabet | none | A and B agree on every core operation. **Not** wire equivalence |
| R5-wire | Refinement over `(method, path, body)` | none | A and B are observationally equivalent. **NOT REACHABLE — see below** |
| R6 | Machine-checked agreement across the four `S_obs` renderings | — | Not reachable. See below |

## What the ladder does not cover: concurrency

Every rung above is derived from `S_obs`, and `S_obs` is a **deterministic
sequential state machine**. It has no vocabulary for two requests being in
flight at once, so no rung built on it can score a concurrency defect — not by
widening the corpus (R0), the trace alphabet (R1) or the property set (R2), and
not by proving harder (R4/R5), whose obligations quantify over a single-threaded
`step_L`.

That is not a hypothesis. F018 was a live defect that dropped tweets behind an
HTTP 500 under concurrent load, on a corner passing R0 56/56, R1 clean, R2 pass
and R4 with ~43 load-bearing clauses. Every rung slept through it, and so did
`go test -race`, because the defect is a lost update across correctly-locked
components rather than a data race.

Closing this needs an oracle that is **not** `S_obs`. The one used here counts
instead of comparing — *N concurrent accepted writes must produce N distinct
durable records, and no accepted write may be reported as a server error* —
which is checkable without a reference machine at all. It lives as a standing
implementation-level check (`internal/httpshim/concurrency_test.go`,
`internal/service/concurrency_test.go` in the Go corner) and deliberately
**not** as a rung: the deliverable of this repository is a per-rung kill table,
and a row whose oracle is a different thing entirely would be exactly the kind
of number `evidence/FINDINGS.md` Pattern 1 warns about.

The Go corner has this check. The other three corners do not, and that is an
open gap rather than a claim that they are clean.

## R5's obligation

For each implementation L, an abstraction function `abs_L : State_L -> State_S`
with

```
abs_L(init_L)       = init_S
resp_L(s, r)        = resp_S(abs_L(s), r)          for all s, r
abs_L(step_L(s, r)) = step_S(abs_L(s), r)          for all s, r
```

Determinism and totality of `S_obs` make these compose by induction on trace
length, which is what yields equivalence by transitivity through the spec.

## Why R6 is not reachable, and what is done instead

No tool checks that a Verus `spec fn`, a Gobra ghost predicate, and a JML model
field denote the same machine. Four hand-written renderings of `S_obs` would be
four independent chances to say something subtly different, with nothing
comparing them.

Mitigation: **generate** all four renderings from one source. That collapses
the obligation from "four specs agree" to "`specgen` is correct" -- which is
reviewable, and which can be mutation-tested by injecting `specgen` mutants
that must break at least one implementation's proof.

The residual gap is real and is recorded in [TCB.md](TCB.md) rather than
argued away.

## Ceiling per corner

| Corner | R0-R3 | R4 | R5-core | R5-wire | Ceiling |
|---|---|---|---|---|---|
| Go | yes | Gobra, **91 of 91 clauses refutable, 0 vacuous, 0 undecided** | **30 of 42 clauses** | no | **R5-core, partial** |
| Rust | yes | Verus, **37 of 37 shipped clauses refutable, 0 vacuous**; R4 row `5/14 = 36%`, up from `1/14 = 7%` before the lift (F051) | **statable, partial: `abs` has a body; 17 clauses on 3 axes; no rung** | no | **R4 on 4 of 5 crates; R5-core has no rung** |
| Java | yes | JBMC, bounded, **7 of 15 obligations decidable** (F034) | no | no | **R3 + bounded (measured)** |
| Kotlin | yes | JBMC, bounded | **5 of 42 clauses, bounded** | no | **R5-core, bounded and partial (measured)** |

### Why R5-wire is not reachable, in either corner

R5 quantifies over `step_L(s, r)` where `r` is a **wire request**. The code
turning those bytes into a core call is outside both verification perimeters
*by design*: Gobra runs over `[clock ids dom store service]` and not
`httpshim`; `crates/server` has no `verify` key and `cargo-verus verus verify
-p server` emits no results line at all.

So the functions R5-wire quantifies over are not mentioned by any contract
anywhere. This is not a verifier limitation — it is the price of the
verified-core / trusted-shim split `TCB.md` chose, and that split is what keeps
validation semantics inside code the verifier reads. The two goals are in
direct tension, and **an earlier version of this file claimed both at once.**

### Why Rust's R5-core was blocked, and what the lock came out of

**This section asserted the opposite until 2026-09-02, and had never been
re-tested.** What it said:

> `abs_rust` cannot be given a body. Verus, verbatim:
> `std::sync::poison::rwlock::RwLock is not supported`. … An abstraction
> function over state behind interior mutability is not definable until that
> state is lifted out of the lock. That is a refactor, not an annotation …
> **Until then every port with Rust at either end is capped below R5.**

Both halves of the blocker were reproduced rather than re-quoted
([F041](evidence/findings/F041-the-r5-blocker-was-the-lock-not-the-property.md),
raw output in `evidence/runs/verus/rwlock-blocker-reproduction.txt`): making
`MemStore` structurally visible to Verus still gives all five "is not
supported" errors, `vstd 0.0.0-2026-04-20-1748` still ships no
`std_specs/sync.rs`, and `vstd::rwlock::RwLock` still has no
`spec fn value(&self)`.

**Then the refactor this file named was done.** `crates/ids`, `crates/clock`
and `crates/store` now hold their state as plain owned value types declared
inside a top-level `verus! { … }` block, with the lock pushed out to a thin
type that takes it and forwards — the verified-core / trusted-shim split
`internal/httpshim` has on the Go corner. `abs_users`, `abs_follows` and
`abs_tweets` have **bodies**.

What is discharged, on the shipped functions, in R5's own vocabulary:

| R5 obligation | Rust status |
|---|---|
| `abs_L(init_L) == init_S` | **discharged** on all three state axes |
| `abs_L(step_L(s, r)) == step_S(abs_L(s), r)` | **discharged** for `put_user`, `put_follow`, `put_tweet` — each also constraining the two axes it must leave alone |
| `resp_L(s, r) == resp_S(abs_L(s), r)` | **not discharged**, and not because of the lock — see below |

**The two `go ↔ rust` R5 cells in `MATRIX.md` do not change, and the reason
they do not is now a different reason.** Three things are still missing:

1. **There is no R5 rung for the Rust corner.** `tools/cmd/calibrate/rungs.go`
   hard-codes R5 as `Tool: "gobra"` with a Go-only file list. A matrix cell is
   a `calibrate` verdict over the mutant catalogue, not a count of clauses in
   a source file.
2. **The response axis is blocked one level below the lock.** `vstd`'s
   `View for String` is `uninterp`, so nothing says `s@ == t@ ==> s == t`.
   Every read on this corner reduces to a membership test, and the direction
   "the abstract set contains it, so the concrete map does" needs exactly that
   injectivity. Verus refuses the clause by name
   ([F043](evidence/findings/F043-the-abstraction-is-not-injective-and-vstd-will-not-say-it-is.md)).
   The deleted twin stated that direction only because an `external_body` shim
   assumed it.
3. **`crates/service` is still entirely twins.** Same lift applies; its write
   mutex spans allocate-then-write (F018), so its transition composes two
   lifted cores rather than one, and it will cost more than `store` did.

So: *"every port with Rust at either end is capped below R5"* is still true of
the cells. Its stated reason is not. Those are different claims and this file
conflated them for as long as the sentence stood.

### What Go's R4 number means, audited

The Go row is not a count of Viper members. `CLOUD.md` recorded 283 of those
and it neither reproduces nor decomposes into obligations — see
[F019](evidence/findings/F019-the-obligation-count-is-not-reproducible.md);
eight runs give 236-238, and 82 of the ~134 "verified" are compiler-generated
termination, import and initialisation proofs rather than anything anyone
wrote. The unit below is the `ensures` clause.

The contracts carry **146 `ensures` clauses**:

| | |
|---|---|
| functional obligations | 91 |
| permission framing (`acc(...)`) | 30 |
| assumed — the member is `// @ trusted` | 13 |
| assumed — the member is a body-less ghost declaration | 12 |

**25 of the 146 are never checked by Gobra at all.**

**The reachability probe is clean, and F013 does not replicate here.** Every
member carrying a checked clause was given `ensures false`; Gobra refutes it on
30 of 33, so those exits are reachable and the obligations on them are about
something. No member has an unreachable exit
(`evidence/runs/gobra/reachability.json`). Kotlin's six vacuous obligations have
no counterpart in the Go corner.

Three members returned no verdict from the `ensures false` probe — `(*MemStore).HomeTimeline` (11 clauses),
`(*MemStore).Replace` (2) and `isMonotoneLog` (2): two clean timeouts and,
for `isMonotoneLog`, a Silicon exception followed by `did not terminate`, with
a plain timeout on reproduction. Gobra reports all of these as `0 error(s)`,
which is why they are read from its wording rather than its count. The
per-clause sweep below has since decided all 11 of `HomeTimeline`'s, and its
`assume false` control establishes directly that the member's exit is
reachable, so the probe's silence there is a cost limit and not a gap in the
evidence (F029). The 4 clauses on `Replace` and `isMonotoneLog` are covered by
the per-clause sweep too. This probe on its own leaves 15 clauses unread.

**The per-clause negation sweep**, corrected key, 12-minute budget per canary:

```
91 clauses: 91 refutable, 0 VACUOUS, 0 timed out, 0 ill-formed
audited 91 REFUTABLE verdicts: 91 backed by an error inside the clause's
own member, 0 backed only by an error elsewhere.
```

The sweep first reported 83 refutable and **8 undecided, all on
`(*MemStore).HomeTimeline`** — F1, D10, no-fabrication and no-loss among them
([F021](evidence/findings/F021-the-audit-fails-where-the-obligations-are-strongest.md)).
Those eight are now decided and none is vacuous
([F029](evidence/findings/F029-the-audit-was-undecidable-as-spelled-not-as-asked.md)).
Neither of the two levers F021 expected to work did: `--parallelizeBranches`
still ran out the clock at 723 s, and a 45-minute budget ended with Gobra's own
`did not terminate` at 2703 s. What decided them was asking one question at a
time — the sweep's canary re-verified the whole member, so it was proving the
other eight postconditions alongside the one it could not — and, for three
clauses, spelling the negation as `len(out) == 0` rather than as the equivalent
`forall a int :: !(0 <= a && a < len(out))` the generator derives. Each of the
eight carries a control run on the same member with `assume false` in the body,
and all nine controls came back VACUOUS, so the probe that produced these
verdicts is one shown to detect vacuity here.
The first run of this sweep reported 86/5; that figure was produced by a
colliding checkpoint key and is withdrawn.

### Why Rust's R4 was one property, and what it is now

Audited obligation by obligation (`evidence/findings/F016`). Verus counts
**units of work, not obligations**. Of the 23 units that stood when F016 was
written, 11 carried no `ensures` clause at all, and exactly **one** —
`domain::Follow::new`, F4 — was functional, non-vacuous, about shipped code,
reachable from the API, and free of project-local assumed axioms. F024 then
disposed of four drifted twins and the count fell 23 → 21, which is the right
direction for a number nobody should read as a guarantee.

**Since 2026-09-02 the corner has more than one.** The lift (F041) moved F7,
F8 and the store's three abstraction axes onto shipped functions. Verus's own
lines:

```
domain 9   store 9   ids 5   clock 5   service 4
R4 PASSED: verification results:: 32 verified, 0 errors over 5 of 5 verify-enabled crate(s)
```

The clause census, from `verus canary`, which re-derives it from the tree:

| | before the lift | after |
|---|---|---|
| on shipped exec functions | 5 | **37** |
| on `verus_proof` twins | 36 | 20 |
| assumed (`external_body` / `admit()`) | 21 | 13 |

Both columns are measured with the **same** classifier; the pre-lift row is a
re-measurement of `origin/claude/goal-loop`, not the older figure. The
difference between the two twin columns and F030's "57" is
[F042](evidence/findings/F042-the-vacuity-instrument-counted-two-axioms-as-shipped-obligations.md):
the sweep had been folding assumed clauses in with twins, and a twin is checked
against a body while an assumed clause is not checked at all.

`crates/ids` no longer contributes zero. F8 is on `Counter::next`, the
transition `Generator::next_id` executes, and the crate reports 5 verified.

**All 37 shipped clauses are refutable and none is vacuous** —
`REFUTABLE 37, VACUOUS 0, ILL-FORMED 0, TIMEOUT 0`, with the sweep's self-test
returning VACUOUS first, so the zero is earned rather than assumed
(`evidence/runs/verus/canary-2026-09-02-after-lift.txt`).

### What is still on twins, and what a twin costs

The Verus contracts used to be almost entirely on **hand-written twins**:
`MemStore::put_tweet` at `crates/store/src/lib.rs:249` and `put_tweet_ensures`
at 820, separate functions kept in step by hand, one of which had already
drifted into falsehood (F012, F024). Twenty clauses are still in that state,
all in `crates/service` and in the three `crates/store` reads that were not
lifted.

Two things the lift settled about that arrangement, both worth carrying:

- **A twin can look stronger than the shipped contract that replaces it.**
  `store::verus_proof::put_user_ensures` stated both directions of the
  accept/reject condition. The shipped `Inner::put_user` states one. The twin
  could state the other because it called `proof_users_contains`, an
  `external_body` shim whose postcondition assumed the very injectivity vstd
  will not give
  ([F043](evidence/findings/F043-the-abstraction-is-not-injective-and-vstd-will-not-say-it-is.md)).
  Deleting it lowered the clause count and raised the evidence.
- **`crates/service` is the remaining holdout** and it is the expensive one.
  Its state is three `Arc`-shared sub-stores plus a write mutex that spans
  allocate-then-write (F018), so the lifted transition composes two cores
  rather than one.

See `evidence/findings/F012` for the full assessment and `F041` for what the
lift cost, measured.

**The operative limit is JBMC, not Kotlin.** `"abc".equals("abc")` verifies as
**FALSE** on JBMC 6.11.0 — reproduced in plain Java, so it is a tool defect and
not a Kotlin cost. It blocks every obligation reducing to string equality,
which is every timeline obligation. The same obligation over an *empty* log
verifies with 0 of 964 goals failing. Result: 7 of 15 obligations VERIFIED, 0
REFUTED, 8 BLOCKED, including F005's monotonicity premise among the verified.
**That wall is shared with the Java corner** — and that sentence was an
inference from a `javac` repro for two months, with nothing in Java ever run
against it, while it capped six R4 cells in `evidence/MATRIX.md`.
`impls/java/verification/` now carries the twin of the Kotlin obligation set and
the inference is a measurement: **the same 7 decidable, the same 8 blocked, the
same three reasons, obligation for obligation** (`evidence/findings/F034`). The
Java corner's R4 route was listed here as OpenJML / KeY; neither has been
attempted, and the route that *is* available is the one F014's repros were
written in. Two of the three blockers came out sharper on the measurement than
they were recorded: `getBytes` is unconstrained in contents as well as length
(`assert "alice".getBytes(UTF_8)[0] == 'a'` and its negation are BOTH refuted),
and the SAT wall is 13.9 GB resident before the kernel kills the process, not
11. See `evidence/findings/F014`, `F034`.

**What the Java bounded rung then kills is zero.** The complete 18-mutant sweep
(`evidence/runs/calibration/java-proof/`) is `0 killed / 15 survived / 2
unreached / 1 error` — `0/15 = 0%`. The rung is not broken: on a hand-broken
`parseInt64` it reports `R4 FAILED: JBMC refuted 2 of 7 decidable
obligation(s)`. The zero decomposes into 9 obligations JBMC cannot read, 3
properties no obligation on either JVM corner states, 3 relational obligations a
non-relational mutant slips past, and 1 mutant that makes its obligation vacuous
— `evidence/findings/F036`, which also records the sharpest fact in it: three of
the seven decidable obligations are over `Dom.parseInt64`, and not one of the
eighteen mutants edits `Dom.java`.

**Kotlin's ceiling is now measured rather than predicted.** JBMC (CBMC 6.11.0)
does discharge bounded obligations over compiled bytecode, so the corner
reaches R3 + bounded as forecast — but see `evidence/findings/F013`: six of
those obligations initially reported VERIFIED because an undischargeable
erased checkcast made the code after it infeasible. Only a *negation* canary
distinguished a real proof from a vacuous one.

**And "7 VERIFIED" was, at the time it was recorded, four audited claims and
three unexamined ones.** Three of the seven had no negation canary naming
them, and the sweep only ever checked that the canaries it had were refuted —
so a claim with no canary passed the gate built to catch exactly that
(`evidence/findings/F025`). Canaries have since been added for all three and
all three are refuted, so the number stands; what changed is that it is now
earned. Since 2026-09-02 the bounded rung is a `calibrate` rung
(`tools/cmd/jbmc verify`) and re-runs the whole audit **on every tree it
judges**, which the Go corner cannot afford to do — negating a bounded
obligation costs what the obligation costs, negating a deductive one does not
(F021).

**Kotlin has no mature deductive verifier.** The best available is compiling to
JVM bytecode and running JBMC for bounded model checking, plus kotest property
tests. So any port with Kotlin at either end cannot exceed bounded and
differential evidence, however strong the other end is.

**That is a limit on the STRENGTH of this corner's R5, not on its existence.**
The R5-core column above said `no` for this corner until it was measured rather
than predicted, and the prediction was wrong for a reason worth naming: the
obstacle was assumed to be JBMC's output — whether a failing goal can be traced
back to a named refinement obligation — and it is not. JBMC reports one goal per
`assert`, carrying the entry point, the assertion index, the file and the line,
which is a finer join than the one the Go rung is built on. What was actually
missing was an abstraction function, and three of its four axes turn out to be
decidable here: log, users and clock. The follows axis is not
(`HashSet.contains` reaches the F014 defect and JBMC answers FAILURE for the
claim *and* for its negation), so clauses 7 and 9 are in neither the numerator
nor the denominator. Five clauses — 1, 2, 11, 13 and 36 — are discharged and
all five are refutable in the tree they are discharged over.

Read the `5 of 42` against Go's `26 of 42` knowing it is a weaker 5: every
clause is a **ground instance inside an unwinding bound**, where Gobra's are
universally quantified. `evidence/findings/F046` states what the cell licenses
and `F045` states what it cost this corner's shipped class.

That asymmetry is a result the matrix is designed to surface, not a defect to
engineer around.

## Port-claim table

For a port B <- A with no changes to A:

| Evidence | Strongest honest claim |
|---|---|
| R0-R2 | "B agrees with A on every trace sampled." Statistical; the kill rate is the number |
| + R3 | "...and the model both target satisfies F1-F9 at bound k" |
| + R4 both ends | "...and both independently satisfy F1-F9." Still not equivalence |
| + R5 both ends | "**B and A are observationally equivalent**", modulo the TCB |
| R5 on A only | "A is correct; B is only as good as R0-R2." The port itself is unproven |

## A class the ladder cannot score at all

Every rung derives from `S_obs`, which is a **sequential** state machine with no
concurrency notion. So no rung can express an interleaving, and a concurrency
defect is invisible to all of them regardless of inputs.

This is not hypothetical: under 1,280 concurrent `POST /tweets` the Go corner
returns HTTP 500 and silently drops tweets, because id allocation sits outside
the lock protecting the log it orders. See `evidence/findings/F018`.

F008 and F009 were about inputs the generator could not produce, and both were
closed by widening the inputs. This one cannot be — `S_obs` would need a
concurrency semantics, or a rung would have to exist that does not consult it.

## Current status

| Rung | State |
|---|---|
| R0 | Corpus **generated** from `S_obs`, 54 steps, regeneration byte-stable. No implementation replays it yet |
| R1 | Not built. Phase 1 |
| R2 | Not built. Phase 1 |
| R3 | **PASSING.** TLC green on `twitter.tla` (8,989,719 distinct states, depth 20). `S_obs` link check green over 16 state-changing steps, and the known-bad canary is correctly rejected |
| R4 | **Go: green and audited.** All five packages `Gobra found no errors`, `0 error(s)`, reproduced eight times. 0 of 33 members unreachable; **91 of 91** functional clauses refutable, 0 vacuous, 0 undecided — the eight F021 could not decide were decided by asking one clause at a time, and none of them is vacuous (F029). **Rust: green, audited, and no longer 8% of the corner.** `verus canary` reports `REFUTABLE 37, VACUOUS 0, ILL-FORMED 0, TIMEOUT 0` over the shipped clauses, with a self-test that returns VACUOUS under an injected false precondition. The lift (F041) took the shipped/twin/assumed census from 5/36/21 to **37/20/13**, measured with the same classifier on both trees; the corner's remaining twins are in `crates/service` and three `crates/store` reads. `verification results:: 32 verified, 0 errors over 5 of 5 verify-enabled crate(s)`. Before F030 this corner had no vacuity instrument of any kind, and until F042 that instrument was counting two admitted axioms as shipped obligations. **Java: 7 of 15 obligations decidable, and the row kills nothing — 0 of 15 (F036).** **Kotlin: 7 of 15 decidable (F014 blocks 8).** |
| R5-core | **Go: 30 of 42 clauses VERIFIED, 0 UNAUDITED, 12 UNATTEMPTED, 0 FAILED, 0 VACUOUS** (`evidence/runs/gobra/r5-clause-status.txt`, derived by `gobra r5`). **Rust: statable since 2026-09-02 and partially discharged** — `abs_users` / `abs_follows` / `abs_tweets` have bodies, `abs(init) == init_S` and state commutation for `put_user` / `put_follow` / `put_tweet` are proved on shipped functions (17 clauses). Not a rung: `calibrate`'s R5 has no Verus driver, and the response axis is blocked on `String` view injectivity (F043). **Kotlin: 5 of 42 clauses, bounded ground instances, and it IS a rung (`jbmc r5verify`, F046)** |
| R5-wire | Not reachable — the decode boundary is unverified by construction |
| R6 | Not reachable by design |

**R4 and R5-core are now supported for the Go corner, and bounded.** What that
licenses precisely: on the decoded-operation alphabet, restricted to
syntactically valid arguments, 30 of the 42 R5 clauses hold with per-clause
evidence — Gobra's own refutation of each clause's negation, not a
package-level green. That includes F1, D10, no-fabrication and no-loss on the
store's `HomeTimeline`, which were unaudited for vacuity until F029 decided
them. Everything above R5-core, and every claim involving the Rust corner at
R5, remains unsupported.

**What the Rust corner's R4 now licenses, precisely.** Four crates' worth of
shipped contract, not one: F4 on `domain::Follow::new`, F7 on `clock::Ts`, F8
on `ids::Counter`, and the store's three abstraction axes on `store::Inner`.
All 37 clauses are shown non-vacuous by Verus refuting each negated antecedent.
It does **not** extend to `crates/service`, whose obligations are still on twins
over `external_body` shims with nothing mechanically tying a twin to the
function that ships.

**The mutant kill rate has not been re-measured against the lifted tree.** F027
recorded 1 kill in 14 because `crates/domain` was the only crate whose contract
sat on shipped code; that reason is now false for three more crates, so the
number should move. It is a `calibrate` sweep and it was not run here — the
figure in F027 stands as the last one actually measured, and is now known to be
measured against a tree that no longer exists.

`ASSURANCE.md` has twice asserted a ceiling it could not back. The numbers in
this file are now derived from `evidence/runs/gobra/` by
`go run ./tools/cmd/gobra r5`, so a claim here that drifts from the verifier
shows up as a join failure rather than as prose.
