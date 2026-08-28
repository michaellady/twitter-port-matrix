# F012 — R5 as written cannot be stated, and Rust cannot reach it at all

**Status:** confirmed against both verifiers running
**Effect:** `ASSURANCE.md`'s central claim was overstated and is now corrected

## The claim under test

> If A refines `S_obs` and B refines `S_obs`, and `S_obs` is deterministic and
> total, then A and B are observationally equivalent — so a port is correct
> with zero changes to the base app.

The obligation quantifies over `step_L(s, r)` where `r` is a **wire request**:
a `(method, path, body)` triple.

## Why it cannot be stated

The code that turns those bytes into a core call is **outside both
verification perimeters, by design**.

Gobra runs over `[clock ids dom store service]` — not `httpshim`.
`crates/server` has no `[package.metadata.verus] verify` key, and
`cargo-verus verus verify -p server` prints no `verification results::` line
at all.

So the functions R5 quantifies over are not mentioned by any contract in
either corner. This is not a verifier limitation. It is the direct cost of the
verified-core / trusted-shim split that `TCB.md` chose deliberately — the same
split that keeps validation semantics in code the verifier reads.

**The obligation splits in two:**

| | scope | statable? |
|---|---|---|
| **R5-wire** | `(method, path, body)` → response bytes | **No**, in either corner. The decode boundary is unverified by construction |
| **R5-core** | the decoded operation alphabet | **Yes** in Gobra; **no** in Verus, see below |

`ASSURANCE.md` claimed R5-wire and reported progress that was R5-core.

## Rust cannot reach even R5-core

`abs_rust` cannot be given a body. Verus, verbatim:

```
std::sync::poison::rwlock::RwLock is not supported
RwLockReadGuard is not supported
::read is not supported
PoisonError is not supported
<RwLockReadGuard as Deref>::deref is not supported
```

`vstd/std_specs/` ships no `sync.rs`. Switching to `vstd::rwlock::RwLock` does
not help: it offers `inv(&self, val)` and a handle-scoped `view()`, but no
`spec fn value(&self) -> V`, and there cannot be one — the value is only
meaningful while a guard is held.

**An abstraction function over state behind interior mutability is not
definable in any verifier without first lifting that state out of the lock.**

That is a real refactor, not an annotation: make the verified core a pure value
type and move the lock into the trusted shim — which is the shape `S_obs`
itself has. Until then, **every port with Rust at either end is capped below
R5, regardless of the other end.**

## And the Rust proofs are on twins, not on the shipped code

`MemStore::put_tweet` is at `crates/store/src/lib.rs:249`.
`put_tweet_ensures` — the function Verus actually verifies — is at line 820.
They are **separate functions**, and the contract is on the copy.

The twins are live (injecting `assert(false)` gives `6 verified, 1 errors`),
so they are not decoration. But they must be kept in sync with production by
hand, and one had already drifted into falsehood: `put_tweet_ensures` claimed
`author known ==> result is Ok`, while production also returns
`Err(NonMonotonic)`. Correcting it produced

```
error: postcondition not satisfied
766 | return Err(StoreError::NonMonotonic);   at this exit
```

So "Verus: 23 verified, 0 errors" meant 23 obligations about hand-written
copies, one of which disagreed with the code it stood for.

**Plausible cause of the drift going unnoticed:** `cargo test --workspace` had
not compiled since S-05 — stale `home_timeline` arity, stale `ServiceError`
variants, assertions naming the pre-`S_obs` error vocabulary. Now 94 passed.

## `crates/ids` verifies nothing, and F8 depends on it

`next_id_ensures` is `external_body`, so its postcondition is **assumed, not
proved**. Nothing calls it. The production path is `Generator::next_id`.
`count(g)` is defined over `inner_state` / `lock_state_value`, both themselves
`external_body`.

F8 in Rust is an assumed postcondition, on a never-executed function, stated
over uninterpreted symbols. It is load-bearing: F8 is exactly the premise that
would show `put_tweet`'s monotonicity guard never fires — the difference
between `S_obs`'s two-outcome `stepPostTweet` and the store's three.

Go's F8 was in the same state until this session; it is now proved on the real
`(*ids.Generator).Next`.

## What Go did gain

Gobra: 242 → **283** Viper members, 98 → **133** verified, verdict unchanged
("Gobra found no errors", 0 errors).

A concrete abstraction function — `AbsUsers()` really is `domain(s.users)`, not
an uninterpreted symbol — on three axes. Newly proved: `abs(init) = init_S` on
all three axes at both layers; state commutation for every mutating store
operation; **F1 visibility**, previously documented as not provable; D10
cursor; no-fabrication and no-loss on the timeline; and F8 on the real
generator.

Blocked, each with the capability named: Gobra has no string indexing in the
ghost language and `pure` funcs must be single expressions, so `ValidHandle`
cannot appear in a spec (kills the accept direction of four of six operations);
package-level `var` initializer postconditions are not threaded into method
bodies (kills reject-branch response clauses); existential postconditions are
not re-instantiated at call sites (no-fabrication and no-loss are proved at the
store and cannot be forwarded to the service).

## The `Replace` gap was live, and is fixed by trusting less

Recorded in F007 as a conditional premise. It was reachable: the pre-fix body
installs `[{ID:3 ts:1} {ID:5 ts:0}]` — a timeline in **ascending** timestamp
order — through the admin path.

The fix is not a better sort. `sortLogByID` is trusted with **no** functional
postcondition, so Gobra havocs its output, and a **verified** `isMonotoneLog`
check decides whether the candidate is installed. A malformed snapshot now
loads as an empty log rather than an F2 counterexample. `Replace` carries a
real `LockP()` contract and its `// @ trusted` marker is gone.

## Five documented claims were false

Corrected in place: Go's F1 "cannot be expressed"; the service layer "cannot
compose `LockP()` with the inner `MemStore.LockP()`" (a wildcard `unfolding`
does, in three lines); the ids "Phase 2b" framing; Rust's "no `vstd::hash_set`
model exists" (it ships `HashSetWithView`, `StringHashSet`, `ExHashSet`,
`ExHashMap`); and Rust's `home_timeline` "returns a `Vec` after a `sort_by`"
needing a sort spec — there has been no sort since S-05.

Every one was a *reason a thing could not be done*, written down and then
inherited without recheck. That is the pattern worth carrying: **a documented
blocker ages badly, and nothing re-tests it.**
