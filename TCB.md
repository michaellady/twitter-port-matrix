# Trusted computing base

What this repository trusts rather than verifies. A claim is only as strong as
this list.

## Trusted tools

| Component | Version | Why it is trusted |
|---|---|---|
| TLC | 2.19 (rev 5a47802), jar sha256 `936a2620...` | Model checker. A soundness bug would make R3 vacuous |
| Z3 | 4.12.5, bundled with Verus | SMT backend for the Rust proofs |
| Verus | 0.2026.04.24.f8e1704 | Deductive verifier for the Rust corner |
| Go toolchain | 1.25.5 | Compiles `S_obs` and the whole rig |
| JDK | 17.0.19 | Runs TLC |
| Gobra | ETH Zurich build, digest-pinned | Deductive verifier for the Go corner; R4 and R5 both read its verdict |
| JBMC | 6.11.0 (cbmc-6.11.0) | Bounded model checker for **both JVM corners'** R4. **Trusted only where it is sound**: `String.equals` and `String.getBytes(Charset)` are known-wrong on this build (F014), so the 8 obligations that depend on them are excluded from each corner's denominator rather than believed. The exclusion list is now measured on both corners and comes out identical (F034). The version is pinned because that list is a property of this build |
| javac | JDK 21, invoked with `--release 17` | Compiles the Java corner together with its obligations for JBMC. `--release` rather than `-target`, so the bytecode the checker reads does not depend on which JDK is first on PATH |
| OpenJML / KeY | not attempted | Listed in `ASSURANCE.md` as the Java corner's R4 route and never tried. The Java corner's R4 is bounded (JBMC), not deductive; nothing here rests on a deductive Java verifier |

Plus, for every corner, the usual floor: compiler, runtime, OS, CPU.

## Trusted artefacts in this repository

### `S_obs` itself

`spec/s_obs/step.go` is the contract. It is not verified by anything -- it *is*
the thing everything else is checked against. Three mitigations:

1. It is small enough to review exhaustively.
2. `tlclink` checks it refines `twitter.tla`, so a large class of errors in it
   would show up as an illegal transition.
3. It is exercised by a test suite asserting determinism, purity, totality, the
   monotonicity lemma, idempotence, and the pinned ambiguity decisions.

None of that makes it correct. It makes it checkable.

### The correlated-failure risk on the Go corner

`S_obs` is written in Go, and Go is one of the four target languages. Without a
guard, the Go implementation could pass differential rungs by construction
rather than by agreement.

Enforced mitigations:

- No implementation may import `S_obs`. Checked by `matrixctl doctor`.
- `S_obs` will be mutation-tested against the R3 link check.

Residual risk, stated plainly: the Go corner shares a language, a standard
library, and an author with the reference machine. Its differential agreement
is weaker evidence than the other three corners' by an amount this repository
cannot quantify. That asymmetry should be reported alongside any Go result.

### The abstraction function

`tools/cmd/tlclink/project.go` maps `S_obs` states onto the model's variables.
It is trusted. It is deliberately lossy in three recorded ways -- tweet text
(D1) and user ids (D2) are dropped, tweet ids are shifted by one (D11, see
finding F002). A wrong projection could make an illegal trace look legal.

The projection is not verified. It is short, and it is written down.

### The four generated `S_obs` renderings

Not yet built. When they exist, nothing will mechanically check that the Verus
spec, the Gobra predicate, the JML model and the Kotlin contract denote the
same machine. Generating them from one source reduces this to trusting
`specgen`, which is the R6 gap named in ASSURANCE.md.

## What is deliberately NOT trusted

- **No gate is decided by an exit code.** `tlclink` ignores TLC's exit status
  entirely and reads TLC's own words, because TLC exits nonzero both for the
  violation the check wants and for parse errors it does not.
- **No gate is trusted unless it has been shown to fail.** `spec check` runs a
  known-bad canary and treats a canary that passes as a hard failure.
- **No harness may write to implementation state.** Replay drives the system
  only through the observable API. This is the specific defect recorded in
  finding F001, where both existing harnesses set the clock to the expected
  answer before asking the question.

## The verified-core / trusted-shim boundary (added in step 1c)

Gobra's verification matrix is `[clock, ids, dom, store, service]`. It does
**not** include `httpshim`. The split follows that line exactly:

| Concern | Location | Status |
|---|---|---|
| Handle and text validity, validation order, error vocabulary, follow/unfollow semantics, the append-log invariant, timeline visibility and ordering, pagination, clock advance | `dom`, `store`, `service` | verified core |
| JSON strictness, canonical byte encoding, routing, status mapping | `httpshim` | trusted transport |

Putting contract semantics in the shim would produce a green R0 over code no
verifier ever reads, and R5 would then prove nothing about observable
behaviour. During step 1c three core patches silently failed to apply and R0
still climbed from 7/54 to 44/54 on shim changes alone — a live demonstration
that R0 alone cannot tell you *where* the behaviour lives.

### Trusted surface removed in step 1c

The store went from 10 `// @ trusted` markers to 4. Deleted: `putFollowEdge`,
`deleteFollowEdge`, `appendTweet`, `iterFollows`, `gatherTimeline`,
`sortTimeline` — all six existed solely because `follows` and `byAuthor` were
nested containers. Remaining: two error constructors, plus `Snapshot` and
`Replace`, which are admin-path serialization carrying no F-property
obligation.

This is a reduction, not a relocation. See findings F004 and F005 — F005 in
particular records why the reduction is only sound once the log invariant is
enforced at the mutation site.


## Corrections from the R5 assessment (S-13)

Two claims elsewhere in this file were overtaken by evidence.

**`Snapshot` and `Replace` "carry no F-property obligation."** False for
`Replace`, and it mattered: the pre-fix body would install
`[{ID:3 ts:1} {ID:5 ts:0}]` — a timeline in ascending timestamp order — through
the admin path, reachable rather than theoretical. `Replace` now carries a real
`LockP()` contract and its `// @ trusted` marker is gone. The fix was to trust
*less*: `sortLogByID` is trusted with no functional postcondition at all, so
Gobra havocs its output, and a **verified** `isMonotoneLog` check decides
whether the candidate is installed. A malformed snapshot loads as an empty log
instead of an F2 counterexample.

**The Rust corner's trusted surface is larger than `external_body` counts
show.** Those counts measure annotations on the *proof twins*, which are
separate functions from the shipped ones. The shipped code carries no contract
at all; the twins must be kept in step with it by hand, and one had drifted.
Any statement of the form "Verus verifies N obligations" should be read as "N
obligations about hand-written copies of the shipped code."

See `evidence/findings/F012`.

## Rust: the trusted boundary after the 2026-09-02 lock lift

`crates/ids`, `crates/clock` and `crates/store` now hold their state as plain
owned value types inside a top-level `verus! { … }` block, with the lock pushed
out (`evidence/findings/F041`). The sentence above is no longer true of those
three crates: 37 of the corner's 70 `ensures` clauses are on functions that
ship. It remains true of `crates/service`.

What that lift left trusted, exhaustively:

| row | what is trusted | why |
|---|---|---|
| `ids::LockState`, `ids::Generator` | `std::sync::Mutex<Counter>` provides mutual exclusion, and the critical section is one call to the verified `Counter::next` | `vstd 0.0.0-2026-04-20-1748` ships no `std_specs/sync.rs`; the types sit outside the `verus!` block so Verus does not read them |
| `clock::LockState`, `clock::Logical` | same, around `Ts::get` / `Ts::tick` / `Ts::set` | same |
| `store::MemStore` | `std::sync::RwLock<Inner>`, and that each method's critical section is one call to the corresponding `Inner` method | same |
| `Counter::next`'s `requires … < i64::MAX` | discharged by nobody; the shim owes it | Verus will not admit `+ 1` on an `i64` without it. Reaching it needs 9.2e18 calls |
| `Ts::tick`'s `requires … < i64::MAX` | same | same |
| `store::axiom_string_obeys_key_model` | `obeys_key_model::<String>()` | vstd proves it for primitives and `Box` thereof and **not** for `String`; its own `StringHashMap` / `StringHashSet` are `external_body` and assume exactly this |
| `store::axiom_string_pair_obeys_key_model` | `obeys_key_model::<(String, String)>()` | same, for the follow-edge key type |

Both axioms are `broadcast proof fn … { admit(); }`, named so they can be
grepped, and `tools/cmd/verus canary` now prints them under `excluded:` on
every run rather than counting them as shipped obligations
(`evidence/findings/F042`). **`crates/store`'s tweets axis needs neither**:
`Vec::push` is modelled by vstd outright.

Not trusted, and this is the change: `Counter::next`, `Ts::tick`, `Ts::get`,
`Inner::new`, `Inner::put_user`, `Inner::put_follow` and `Inner::put_tweet` all
have their postconditions discharged against their own bodies. The
`external_body` shims they used to go through — `next_id_ensures`,
`proof_lock_value`, `proof_lock_increment`, `lock_state_value`, `inner_state`,
`proof_users_insert`, `proof_follow_insert`, `proof_append_tweet`,
`proof_can_post_tweet`, `proof_log_breaks_monotonicity` — are deleted.
