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
| Go | yes | Gobra, **83 of 91 clauses refutable, 0 vacuous, 8 undecided** | **26 of 42 clauses** | no | **R5-core, partial** |
| Rust | yes | Verus, **1 property** | **no** | no | **R4, one property (F4)** |
| Java | yes | JBMC, bounded, **7 of 15 obligations decidable** (F034) | no | no | **R3 + bounded (measured)** |
| Kotlin | yes | JBMC, bounded | no | no | **R3 + bounded (measured)** |

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

### Why Rust cannot reach even R5-core

`abs_rust` cannot be given a body. Verus, verbatim:
`std::sync::poison::rwlock::RwLock is not supported`. `vstd` ships no
`sync.rs`, and `vstd::rwlock::RwLock` does not help — it has no
`spec fn value(&self) -> V`, and cannot, since the value is only meaningful
while a guard is held.

An abstraction function over state behind interior mutability is not definable
until that state is lifted out of the lock. That is a refactor, not an
annotation: make the verified core a pure value type and move the lock into the
trusted shim — the shape `S_obs` itself has. **Until then every port with Rust
at either end is capped below R5.**

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

Three members returned no verdict — `(*MemStore).HomeTimeline` (11 clauses),
`(*MemStore).Replace` (2) and `isMonotoneLog` (2): two clean timeouts and,
for `isMonotoneLog`, a Silicon exception followed by `did not terminate`, with
a plain timeout on reproduction. Gobra reports all of these as `0 error(s)`,
which is why they are read from its wording rather than its count. Those 15 clauses are **unaudited, not
verified**.

**The per-clause negation sweep**, corrected key, 12-minute budget per canary:

```
91 clauses: 83 refutable, 0 VACUOUS, 8 timed out, 0 ill-formed
audited 83 REFUTABLE verdicts: 83 backed by an error inside the clause's
own member, 0 backed only by an error elsewhere.
```

All eight undecided clauses are on `(*MemStore).HomeTimeline`, and three of
them are trivial framing negations that refute in seconds on every other
member — so it is the method's proof that is at the edge of the budget, not
the canaries. Those eight, F1 / D10 / no-fabrication / no-loss among them, are
**unaudited, not verified** —
[F021](evidence/findings/F021-the-audit-fails-where-the-obligations-are-strongest.md).
The first run of this sweep reported 86/5; that figure was produced by a
colliding checkpoint key and is withdrawn.

### Why Rust's R4 is one property, not 21

Audited obligation by obligation (`evidence/findings/F016`). Verus counts
**units of work, not obligations**. Of the 23 units that stood when F016 was
written, 11 carried no `ensures` clause at all. Exactly **one** —
`domain::Follow::new`, F4 — is functional, non-vacuous, about shipped code,
reachable from the API, and free of project-local assumed axioms. That is
still true at 21.

**S-14 disposed of the four drifted twins** (`evidence/findings/F024`).
`service::create_user_ensures` was *false* of the shipped code — it verified
`handle@.len() > 0 && !contains ==> result is Ok`, which fails for `"Alice"`,
an input already in the corpus at step 5 on a corner passing 56/56. It now
guards on the predicate production actually applies. `service::follow_ensures`
had the pre-D4 ordering in its body (the F003 defect); its body now mirrors
production and two of its clauses name *which* error, so the ordering is
checkable rather than invisible. The two `home_timeline_ensures` twins carried
**zero** `ensures` clauses over a drifted, cursor-less copy of the timeline
walk, and were **deleted** — an obligation with no postcondition cannot be
refuted, so it was count and not evidence.

The count is now **21 verified, 0 errors** (clock 2, ids 0, domain 9, store 6,
service 4). It went *down* by deleting two things that proved nothing, which
is the right direction for a number nobody should read as a guarantee.

`crates/ids` still contributes zero obligations while F8 depends on it: adding
`false` to `next_id_ensures` gives `0 verified, 0 errors`.

No vacuity was found — all clauses were refuted by negation canary, so F013's
Kotlin mode does not replicate here.

### Why Rust's R4 is weaker than its count

The Verus contracts are on **hand-written twins**. `MemStore::put_tweet` is at
`crates/store/src/lib.rs:249`; `put_tweet_ensures`, the function Verus
verifies, is at 820. They are separate functions kept in step by hand, and one
had already drifted into falsehood. Read "21 verified, 0 errors" as *21
obligations about copies of the shipped code*. S-14 audited every twin against
its production function and fixed or deleted the four that had drifted
(`evidence/findings/F024`); the twins are in step with production as of that
audit, and nothing mechanical keeps them there.

`crates/ids` verifies **zero** obligations while F8 depends on it: assumed
postcondition, never-executed function, uninterpreted symbols.

See `evidence/findings/F012` for the full assessment.

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
| R4 | **Go: green and audited.** All five packages `Gobra found no errors`, `0 error(s)`, reproduced eight times. 0 of 33 members unreachable; 83 of 91 functional clauses refutable, 0 vacuous, 8 undecided (all on `HomeTimeline`, F021). **Rust: green, and now audited too — but over 8% of the corner.** `verus canary` (F030) reports `REFUTABLE 5, VACUOUS 0, ILL-FORMED 0, TIMEOUT 0` on the shipped clauses, with a self-test that returns VACUOUS under an injected false precondition. That covers the 5 `ensures` clauses on shipped functions; the corner's other **57 clauses are on hand-written twins** inside `#[cfg(verus_only)] mod verus_proof` (F012, F016, F027), where a canary would measure the twin. Before F030 this corner had no vacuity instrument of any kind |
| R5-core | **Go: 26 of 42 clauses VERIFIED, 4 UNAUDITED, 12 UNATTEMPTED, 0 FAILED, 0 VACUOUS** (`evidence/runs/gobra/r5-clause-status.txt`, derived by `gobra r5`). Rust blocked on `RwLock` having no Verus model |
| R5-wire | Not reachable — the decode boundary is unverified by construction |
| R6 | Not reachable by design |

**R4 and R5-core are now supported for the Go corner, and bounded.** What that
licenses precisely: on the decoded-operation alphabet, restricted to
syntactically valid arguments, 26 of the 42 R5 clauses hold with per-clause
evidence — Gobra's own refutation of each clause's negation, not a
package-level green. Four more (F1, D10, no-fabrication, no-loss on the store's
`HomeTimeline`) are unaudited for vacuity. Everything above R5-core, and every
claim involving the Rust corner at R5, remains unsupported.

**What the Rust corner's R4 now licenses, precisely.** One property, F4, on one
shipped function: `Follow::new` rejects `from == to`, and the five `ensures`
clauses stating it are each shown non-vacuous by Verus refuting the negated
antecedent. That is a real, audited R4 claim and it is the whole of it. It does
NOT extend to the four other verify-enabled crates: their obligations are on
twins over `external_body` shims, nothing mechanically ties a twin to the
function that ships, and 13 of the 14 catalogue mutants that reach this corner's
proof perimeter leave the twin untouched and verifying.

`ASSURANCE.md` has twice asserted a ceiling it could not back. The numbers in
this file are now derived from `evidence/runs/gobra/` by
`go run ./tools/cmd/gobra r5`, so a claim here that drifts from the verifier
shows up as a join failure rather than as prose.
