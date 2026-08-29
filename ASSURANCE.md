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
| Go | yes | Gobra, **~43 load-bearing** | **partial** | no | **R5-core, partial** |
| Rust | yes | Verus, **1 property** | **no** | no | **R4, one property (F4)** |
| Java | yes | not attempted | unknown | no | **R3** |
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

### Why Rust's R4 is one property, not 23

Audited obligation by obligation (`evidence/findings/F016`). Verus counts
**units of work, not obligations**: 11 of the 23 carry no `ensures` clause at
all. Of the remainder, exactly **one** — `domain::Follow::new`, F4 — is
functional, non-vacuous, about shipped code, reachable from the API, and free
of project-local assumed axioms. Eleven have real content but are conditional
on 15 *assumed* `external_body` postconditions over 11 uninterpreted symbols,
and four of their twins have drifted from production.

**One of those drifted twins is false of the shipped code.**
`service::create_user_ensures` verifies `handle@.len() > 0 && !contains ==>
result is Ok`, which fails for `"Alice"` — an input already in the corpus at
step 5, on a corner passing 56/56.

`crates/ids` contributes zero obligations while F8 depends on it: adding
`false` to `next_id_ensures` still gives `0 verified, 0 errors`.

No vacuity was found — all 43 clauses were refuted by negation canary, so
F013's Kotlin mode does not replicate here.

### Why Rust's R4 is weaker than its count

The Verus contracts are on **hand-written twins**. `MemStore::put_tweet` is at
`crates/store/src/lib.rs:249`; `put_tweet_ensures`, the function Verus
verifies, is at 820. They are separate functions kept in step by hand, and one
had already drifted into falsehood. Read "23 verified, 0 errors" as *23
obligations about copies of the shipped code*.

`crates/ids` verifies **zero** obligations while F8 depends on it: assumed
postcondition, never-executed function, uninterpreted symbols.

See `evidence/findings/F012` for the full assessment.

**The operative limit is JBMC, not Kotlin.** `"abc".equals("abc")` verifies as
**FALSE** on JBMC 6.11.0 — reproduced in plain Java, so it is a tool defect and
not a Kotlin cost. It blocks every obligation reducing to string equality,
which is every timeline obligation. The same obligation over an *empty* log
verifies with 0 of 964 goals failing. Result: 7 of 15 obligations VERIFIED, 0
REFUTED, 8 BLOCKED, including F005's monotonicity premise among the verified.
**That wall is shared with the Java corner.** See `evidence/findings/F014`.

**Kotlin's ceiling is now measured rather than predicted.** JBMC (CBMC 6.11.0)
does discharge bounded obligations over compiled bytecode, so the corner
reaches R3 + bounded as forecast — but see `evidence/findings/F013`: six of
those obligations initially reported VERIFIED because an undischargeable
erased checkcast made the code after it infeasible. Only a *negation* canary
distinguished a real proof from a vacuous one.

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
| R4 | Not started. Phase 1 |
| R5-core | Go partial; Rust blocked on `RwLock` having no Verus model |
| R5-wire | Not reachable — the decode boundary is unverified by construction |
| R6 | Not reachable by design |

No claim above R3 is currently supported by anything in this repository, and
none should be made.
