# F041 — the R5 blocker was the lock, and the lock came out

**Status:** measured 2026-09-02; the blocker was reproduced verbatim first, then
removed for three of the five verify-enabled crates
**Class:** a structural claim in `ASSURANCE.md` that was true when written,
inherited without recheck, and is now false for most of the corner

## The claim under test

`ASSURANCE.md`, section "Why Rust cannot reach even R5-core":

> `abs_rust` cannot be given a body. Verus, verbatim:
> `std::sync::poison::rwlock::RwLock is not supported`. … **Until then every
> port with Rust at either end is capped below R5.**

F012 says the same thing and adds the fix in one sentence: *"make the verified
core a pure value type and move the lock into the trusted shim — the shape
`S_obs` itself has."* Nothing had done it, and nothing had re-tested the
blocker since it was written. F012's own closing line is the reason to look:
**"a documented blocker ages badly, and nothing re-tests it."**

## Step 1: the blocker is real, reproduced rather than quoted

`crates/store`'s `MemStore` was made structurally visible to Verus —
`ExMemStore`'s `#[verifier::external_body]` removed, `inner` made `pub`, and a
`spec fn` given a body that projects the field. `cargo-verus verus verify`,
its own words, all six errors:

```
error: `std::sync::poison::rwlock::impl&%23::deref` is not supported
error: `std::sync::poison::rwlock::RwLockReadGuard` is not supported
error: `std::sync::poison::PoisonError` is not supported
error: `std::sync::poison::rwlock::impl&%11::read` is not supported
error: `std::sync::poison::rwlock::RwLock` is not supported
error: cannot use type `store::Inner` which is ignored because it is either
       declared outside the verus! macro or it is marked as `external`.
```

`vstd 0.0.0-2026-04-20-1748` still ships no `std_specs/sync.rs` — checked
directly against the unpacked crate, whose `std_specs/` holds `alloc atomic
bits borrow btree clone cmp control_flow convert core default hash
manually_drop maybe_uninit mod num ops option range result slice smart_ptrs
vec vecdeque` and no `sync`. `vstd::rwlock::RwLock` still offers
`inv(&self, val)` and a handle-scoped `view()`, and no `spec fn value(&self)`.
Both halves of the documented blocker hold.

## Step 2: the state came out of the lock, and `abs_rust` got a body

The refactor is the one F012 named. In each crate the state is now a plain
owned value declared inside a top-level `verus! { … }` block, with `&mut self`
transitions carrying `ensures` clauses Verus discharges **against their own
bodies**; the lock moved out to a thin type that takes it and forwards.

| crate | before | after |
|---|---|---|
| `ids` | `Generator { LockState { Mutex<i64> } }`; F8 on `next_id_ensures`, an `external_body` fn with an `unimplemented!()` body that nothing calls | `Counter { value: i64 }` inside `verus!`; F8 on `Counter::next`, which `Generator::next_id` executes |
| `clock` | `Logical { LockState { Mutex<i64> } }`; F7 on twins over three `external_body` hooks | `Ts { value: i64 }` inside `verus!`; F7 on `Ts::tick` and `Ts::get` |
| `store` | `MemStore { RwLock<Inner> }`; `users_keys` / `follow_edges` / `author_tweet_count` all `external_body` and uninterpreted | `Inner` inside `verus!`; `abs_users`, `abs_follows`, `abs_tweets` have **bodies** |

Verus's own result lines, before and after, same tree, same driver:

```
before:  domain 9   store 6   ids 0   clock 2   service 4
         R4 PASSED: verification results:: 21 verified, 0 errors over 5 of 5 verify-enabled crate(s)

after:   domain 9   store 9   ids 5   clock 5   service 4
         R4 PASSED: verification results:: 32 verified, 0 errors over 5 of 5 verify-enabled crate(s)
```

`ids` went from **zero** obligations to five, and F8 is no longer "an assumed
postcondition, on a function that is never executed, stated over uninterpreted
symbols" (F012, F016, F024, `OBLIGATION.md` §6). `store` went from 6 to 9 while
**three twins were deleted**, so the nine are not the six plus three more; they
are a different nine, on the shipped `Inner`.

## Step 3: what of R5's obligation is now discharged

`ASSURANCE.md`'s R5 obligation is three clauses. On the store's three state
axes:

| obligation | status |
|---|---|
| `abs_L(init_L) == init_S` | **discharged**, all three axes, on `Inner::new` |
| `abs_L(step_L(s, r)) == step_S(abs_L(s), r)` | **discharged** for `put_user` (users axis), `put_follow` (follows axis), `put_tweet` (tweets axis) — each also asserting the two axes it must leave alone |
| `resp_L(s, r) == resp_S(abs_L(s), r)` | **not discharged.** One direction of every read is blocked, and not by the lock — see [F043](F043-the-abstraction-is-not-injective-and-vstd-will-not-say-it-is.md) |

That is 17 clauses, on shipped functions, in the vocabulary R5 is written in.
Before this they could not be *stated*.

## What this does NOT do, said plainly

**No R5 cell in `MATRIX.md` changes, and the two capped `go ↔ rust` cells stay
capped.** Three separate things are still missing and the first is the largest:

1. **There is no R5 rung for the Rust corner.** `tools/cmd/calibrate/rungs.go`
   hard-codes R5 as `Tool: "gobra"` with a Go-only `r5Files` list. A cell is a
   `calibrate` verdict over the mutant catalogue, not a set of clauses in a
   source file. Writing that rung is the next task, and it is not this one.
2. **The response axis is not discharged** (F043), so the refinement is
   half-stated even at the store layer.
3. **`crates/service` is still entirely twins.** Its state is three `Arc`-shared
   sub-stores plus a write mutex; the same lift applies and has not been done.

So the correction to `ASSURANCE.md` is precise: the sentence "every port with
Rust at either end is capped below R5" is still *true of the cells*, and its
stated *reason* is no longer true. Those are different claims and the file
conflated them.

## Cost, measured rather than estimated

Three crates lifted. `ids` and `clock` are ~130 lines of source each and took
one iteration; `store` took four, three of which were the two `assert forall`
set-extensionality proofs `put_user` and `put_follow` need. The whole Rust
tree's Verus run is **5.8s warm**, unchanged in order of magnitude.

By analogy for what is left: `crates/service` carries 4 twin blocks and 9
`external_body` shim clauses over three sub-stores. It is the same shape as
`store`, with one extra wrinkle `store` did not have — the write mutex spans
allocate-then-write (F018), so its transition is a composition of two lifted
cores rather than one. Expect it to cost more than `store` did, not less.

## The gate can now fail where it could not before

Standing rule 2. Before the lift, `crates/ids` was the corner's clearest dead
spot: F027 records that any mutant editing it left the twin untouched and the
crate reported `0 verified, 0 errors`, and F024 records that even adding
`false` to `next_id_ensures` gave `0 verified, 0 errors` — there was nothing
for a defect to break.

One character changed in the shipped body, `self.value + 1` to
`self.value + 2`:

```
Verus's own result lines:
  domain     verification results:: 9 verified, 0 errors
  store      verification results:: 9 verified, 0 errors
  ids        verification results:: 4 verified, 1 errors
  clock      (no result line: not reached by this run)
  service    (no result line: not reached by this run)

R4 FAILED: verification results:: 22 verified, 1 errors over 3 of 5 verify-enabled crate(s)   [3.5s]
```

The partial report is expected and is the driver saying so: a crate that fails
verification fails to compile, so `clock` and `service` downstream of `ids` are
never checked.

**This is also why F027's kill rate must be re-measured rather than carried
forward.** Its 1-in-14 was correct for a tree where `crates/domain` was the
only crate a mutant could break. Three more crates can now be broken.

## Behaviour is unchanged, and it was checked

`cargo test --workspace`: 17 test binaries, all `ok`, 0 failed.
`go run ./tools/cmd/replay -impl rust`: **R0 PASSED, 56/56 exact, 0 differ.**
The refactor moves bodies between functions and changes no control flow; the
one rewrite that is not a pure move is `put_tweet`'s `tweets.last()` becoming
an index of `tweets[len-1]`, which has the same value on the same inputs and is
covered by the corpus's F2 and F8 rows.
