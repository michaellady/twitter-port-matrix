# F016 — "Verus: 23 verified, 0 errors" is one property proved, eleven conditional, eleven empty

**Status:** audited obligation by obligation, tree left byte-identical
**Effect:** the Rust corner's headline count overstates by roughly an order of magnitude

## The number, decomposed

Verus counts **units of work, not obligations.** Ten of the 23 carry no
`ensures` clause at all. The 43 ensures clauses that exist are spread over 12
units.

| | count | what it is |
|---|---|---|
| assert nothing | **11** | 4 compiler-derived `clone`s, 2 shipped fns with no `ensures` (`valid_handle`, `valid_text` — nothing ties them to D1/D6), 3 contentless wrappers, 1 spec fn with zero call sites anywhere, 1 unit Verus counts but will not name |
| proved about shipped code, unconditionally | **1** | `domain::Follow::new` — F4 |
| real content, but conditional | **11** | about hand-written twins, discharged by re-plumbing 15 *assumed* shim postconditions stated over 11 uninterpreted symbols |
| vacuous | **0** | — |

**Of F1–F9, exactly one property has a proof that is functional, non-vacuous,
about shipped code, reachable from the observable API, and not resting on a
project-local assumed axiom.** That is F4.

## The good news first: no vacuity

All 43 ensures clauses were negation-canaried individually — consequent negated
for implications, whole clause otherwise — and **all 43 were refuted.** The
five zero-clause bodies were probed with `assert(false)` and every one reported
`assertion failed`, i.e. reachable.

**F013's Kotlin failure mode does not replicate in Rust.** Verus is not
reporting VERIFIED over unreachable code. What it verifies, it verifies.

The problem is what it is verifying.

## `crates/ids` contributes nothing, and F8 depends on it

`next_id_ensures` is `external_body`, so its postcondition is **assumed, not
proved**. Nothing calls it. Production is `Generator::next_id`. `count(g)`
resolves to `lock_state_value(inner_state(g))`, both themselves
`external_body`.

Independently reproduced: adding `false` to its `ensures` list and re-running
gives **`0 verified, 0 errors`** — Verus does not check it at all. The audit
went further and showed a caller can then *prove* `false` from it.

**F8 in Rust is an assumed postcondition, strong enough to prove `false`, on a
function that is never executed, stated over uninterpreted symbols.**

## Four twins have drifted, and one is false of the shipped code

Per F012 the Verus contracts are on hand-written twins. Nothing mechanically
links a twin to its production function — the twins live in
`#[cfg(verus_only)] mod verus_proof`, so they do not exist in any shipped
build, and nothing outside the proof module references them.

**`service::create_user_ensures` is false.** Its guard is
`handle.as_str().is_empty()`; production is `!domain::valid_handle(handle)`,
which also rejects uppercase, over-length and punctuation. So the *verified*
clause

```
handle@.len() > 0 && !contains(handle@) ==> result is Ok
```

is false for `handle = "Alice"`. Shipped code returns `Err(InvalidHandle)`.

**The falsifying input is already in the corpus** — step 5,
`reject_uppercase_handle`, `POST /users {"handle":"Alice"}` → 400. A step this
corner passes, in a sweep reported as 56/56 byte-exact, while a verified
contract says the opposite.

**`service::follow_ensures` encodes the pre-D4 ordering** — literally the F003
defect, calling `Follow::new` before the existence checks. Giving the twin
production's real ordering still yields `5 verified, 0 errors`: **the contract
cannot distinguish the two orderings.** It is true, and blind to precisely the
thing 1c and 1d existed to fix.

The other two drifts are `home_timeline_ensures` at both layers: production
takes a cursor and returns `(Vec<Tweet>, bool)`; the twins take neither and
return the vector. D10 pagination is outside the proof entirely.

## F015 replicates on three properties — and not on the fourth

Store-level clauses describing branches the service layer returns before
reaching:

- `put_user_ensures` clause 1 — `Service::create_user` returns `HandleTaken` first
- `put_follow_ensures` clauses 1–2 (F9) — `Service::follow` returns `UnknownUser` first
- `put_tweet_ensures` clause 1 (F6) — `Service::post_tweet` returns `UnknownUser` first

`put_tweet_ensures`' monotonicity direction is reachable only through
`_admin/load-snapshot`, not from the corpus API surface.

**F4 is the exception, and it is why F4 survives the audit.** Rust's
`Service::follow` has *no* self-follow check of its own, so `Follow::new`'s
branch is live and observable. In Go the same property is guarded twice, which
is what made F015's mutant unobservable there. The property is better proved in
Rust *because it is defended less*.

## What has no obligation at all

`Service::post_tweet` — the composition carrying F6, F7 **and** F8 — has no
twin and no obligation. Neither do `tick`, `now`, `snapshot_state` or
`load_state`. F1 and F2 remain trusted inside `proof_home_timeline`. F7's
actual non-decreasing property is stated nowhere, and `Logical::set_now` can
rewind the clock with no obligation mentioning it.

## The rule

**A verifier's count is a count of work units, not of guarantees.** Before
quoting one, decompose it:

1. How many carry a functional postcondition at all?
2. Of those, how many are about the shipped symbol rather than a copy?
3. Of those, how many rest only on axioms outside the project?
4. Of those, how many describe branches the program can reach?

Here that sequence runs 23 → 12 → 1 → 1.

`ASSURANCE.md`'s ceiling of "R4, on hand-written copies" was the right *shape*.
The count attached to it was not, and this file is the correction.
