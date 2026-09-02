# F024 — two of the four drifted twins had no postcondition, so the repair made the count go down

**Status:** every `*_ensures` twin in `impls/rust` read side by side with its
production function; four disposed of, gates re-run with cache defeated
**Effect:** Verus over the five verify-enabled crates goes **23 → 21 verified,
0 errors**, and one clause that was *false of the shipped code* is gone

## The number, before and after

`cargo-verus verus verify`, its own output, over the five crates carrying
`[package.metadata.verus] verify = true`. Verus **caches**: a second run over
an unchanged tree prints no `verification results::` line at all, which reads
exactly like a pass. Every run below was preceded by `touch` on the crate
sources.

Before (`clock 2 · ids 0 · domain 9 · store 7 · service 5`):

```
verification results:: 0 verified, 0 errors      ids
verification results:: 2 verified, 0 errors      clock
verification results:: 9 verified, 0 errors      domain
verification results:: 7 verified, 0 errors      store
verification results:: 5 verified, 0 errors      service
```

After (`clock 2 · ids 0 · domain 9 · store 6 · service 4`):

```
verification results:: 0 verified, 0 errors      ids
verification results:: 2 verified, 0 errors      clock
verification results:: 9 verified, 0 errors      domain
verification results:: 6 verified, 0 errors      store
verification results:: 4 verified, 0 errors      service
```

**The count fell, and that is the result.** Two of the four drifted twins
carried *zero* `ensures` clauses. Verus discharged the empty contract, counted
a unit of work, and reported it beside eleven units that say something. An
obligation with no postcondition cannot be refuted by a canary and cannot be
falsified by a mutant. Deleting it removes nothing but the number.

## The audit

Every `*_ensures` twin, read against the production function it stands for.
This is the whole table, not only the drifted rows — the ones that agree are
the control.

| twin | production | verdict |
|---|---|---|
| `clock::now_ensures` | `Logical::now` | agrees |
| `clock::tick_ensures` | `Logical::tick` | agrees |
| `ids::next_id_ensures` | `Generator::next_id` | **not a twin at all** — `external_body` with an `unimplemented!()` body. Contributes 0 verified units; its postcondition is assumed. See below |
| `store::put_user_ensures` | `MemStore::put_user` | agrees |
| `store::has_user_ensures` | `MemStore::has_user` | agrees |
| `store::put_follow_ensures` | `MemStore::put_follow` | agrees |
| `store::delete_follow_ensures` | `MemStore::delete_follow` | agrees |
| `store::put_tweet_ensures` | `MemStore::put_tweet` | **agrees — already repaired at S-13**, see the correction below |
| `store::follow_set_ensures` | `MemStore::follow_set` | signature agrees; **zero `ensures`**, so contentless but not drifted. Left in place |
| `store::home_timeline_ensures` | `MemStore::home_timeline` | **DRIFTED → deleted** |
| `service::has_user_ensures` | `Service::has_user` | agrees |
| `service::create_user_ensures` | `Service::create_user` | **DRIFTED, FALSE → fixed** |
| `service::follow_ensures` | `Service::follow` | **DRIFTED → fixed** |
| `service::unfollow_ensures` | `Service::unfollow` | agrees |
| `service::home_timeline_ensures` | `Service::home_timeline` | **DRIFTED → deleted** |

F016's list of four is confirmed exactly. Nothing was missed and nothing extra
was found.

## The one that was false

`service::create_user_ensures` guarded on

```rust
if handle.as_str().is_empty() { return Err(ServiceError::InvalidHandle); }
```

while the shipped `Service::create_user` guards on
`!domain::valid_handle(handle)`, which additionally rejects uppercase,
over-length and punctuation. The verified clause was therefore

```
handle@.len() > 0 && !contains(handle@) ==> result is Ok
```

false for `handle = "Alice"` — corpus step 5, `reject_uppercase_handle`,
`POST /users {"handle":"Alice"}` → 400, on a corner reported as 56/56
byte-exact.

**Fixed, not deleted, because it can be made true of the shipped function
without touching production** (GOAL.md standing rule 3). The repair introduces
an uninterpreted `spec fn handle_valid(h: Seq<char>) -> bool` and one
`external_body` shim whose **body is the production call itself**:

```rust
#[verifier::external_body]
pub fn proof_valid_handle(h: &String) -> (out: bool)
    ensures out == handle_valid(h@)
{ domain::valid_handle(h.as_str()) }
```

That is the point of the shape: a shim that *calls* the shipped predicate
cannot drift from it the way a shim that *restates* it can. The twin's body
now mirrors production's three steps (D6 syntax → duplicate check → allocate
id and put), and the accept clause reads
`handle_valid(handle@) && !contains(handle@) ==> result is Ok`.

`domain::valid_handle` still carries no `ensures` clause of its own, so
`handle_valid` remains uninterpreted. Giving it one would be an annotation on
a base app and is not claimed here.

## The one that encoded the defect the corpus exists to catch

`service::follow_ensures`' body was

```rust
let f = Follow::new(from, to)?;      // F4 first
proof_service_put_follow(s, f)
```

which is the **pre-D4 ordering** — literally the defect recorded in F003 and
removed by steps 1c/1d. It answers `self_follow_forbidden` for
`follow(eve, eve)` where `eve` is unregistered; `S_obs` and the shipped
`Service::follow` answer `unknown_user`.

F016 established that the *contract* could not tell the two orderings apart:
giving the twin production's ordering still verified. The reason is visible in
the clauses — every one had the shape `… ==> result is Err`, and **both**
orderings return some error.

So the repair is two changes, not one:

1. the body becomes production's control flow — D6 syntax, then D4 existence,
   then F4 semantics, then the store put;
2. two clauses name **which** error:

```
(!handle_valid(from@) || !handle_valid(to@))
    ==> result is Err && result->Err_0 is InvalidHandle,
(handle_valid(from@) && handle_valid(to@)
    && !(service_users_keys(old(s)).contains(from@)
         && service_users_keys(old(s)).contains(to@)))
    ==> result is Err && result->Err_0 is UnknownUser,
```

**The ordering is now visible to the verifier.** Re-inserting the old body
refutes both clauses (canary below). This is the first Verus obligation in the
repository that can distinguish the F003 defect from its fix.

## The two that were deleted

`store::home_timeline_ensures` and `service::home_timeline_ensures` were
identical in shape:

- **zero `ensures` clauses.** The whole body was one delegation to an
  `external_body` shim. Verus discharged `true` and counted a unit.
- **drifted signature.** Production `home_timeline` takes a `cursor: i64` and
  returns `(Vec<Tweet>, bool)`; both twins took neither and returned only the
  vector. The store-layer shim was a **second, divergent copy of the timeline
  walk** with no cursor logic in it at all; the service-layer shim called
  production with `cursor = 0` and threw the `more` flag away. D10 pagination
  was outside the proof entirely.

Nothing production calls either. The store-layer shim in particular was a
liability rather than a neutral: a reader who greps for the timeline algorithm
finds two implementations, one of which is verified-adjacent and wrong.

Deleting them loses no guarantee, because there was none. F1 and F2 in this
corner were, and remain, trusted (`OBLIGATION.md` blockers B4/B5 — `abs` is not
definable while the state sits behind `std::sync::RwLock`). The Go corner
proves both on the same algorithm; that is where the timeline evidence lives.

## Canaries — the gate shown to fail, three ways

Standing rule 2. All three preceded by `touch crates/service/src/lib.rs`.

**A. `assert(false)` in a twin that was kept** (`create_user_ensures`):

```
error: assertion failed
   --> crates/service/src/lib.rs:520:20
    |                    ^^^^^ assertion failed
verification results:: 3 verified, 1 errors
```

**B. the old pre-D4 body restored in `follow_ensures`** — the canary F016 said
the old contract could not run:

```
error: postcondition not satisfied
   --> crates/service/src/lib.rs:674:17
674 | /                 (!handle_valid(from@) || !handle_valid(to@))
675 | |                     ==> result is Err && result->Err_0 is InvalidHandle,
    | |____^ failed this postcondition
694 |                   Err(e) => return Err(match e {
    | |__________________- at this exit

error: postcondition not satisfied
   --> crates/service/src/lib.rs:681:17
681 | /                 (handle_valid(from@) && handle_valid(to@)
684 | |                     ==> result is Err && result->Err_0 is UnknownUser,
    | |____^ failed this postcondition
verification results:: 3 verified, 1 errors
```

**C. the old `is_empty()` guard restored in `create_user_ensures`** — the
falsehood itself, refuted:

```
error: postcondition not satisfied
   --> crates/service/src/lib.rs:516:17
516 |    handle_valid(handle@) && !service_users_keys(old(s)).contains(handle@) ==> result is Ok,
    |    ^^^^ failed this postcondition
521 |                 return Err(ServiceError::InvalidHandle);
    |                 --------------------------------------- at this exit
verification results:: 3 verified, 1 errors
```

## Corrections to earlier claims (F020 rule)

1. **`put_tweet_ensures` is not drifted and has not been since S-13.**
   `spec/refinement/OBLIGATION.md` §7 stated in the present tense that it
   claims `users_keys(old(s)).contains(t.author@) ==> result is Ok` while
   production has a third `NonMonotonic` branch. That was true when the
   sentence was drafted and false by the time it was committed: commit
   `4bc2706` both wrote it and landed `accepts_tweet`, which conjoins the
   monotonicity guard. §7 is rewritten. This is the *same* failure mode F020
   named — a claim and its own repair in one commit — recurring in the same
   file that records F012.

2. **`obligations.json` recorded R0 as 54/54 for both Go and Rust.** The corpus
   is 56 steps. Re-measured at S-14: both corners print
   `R0 result: 56/56 exact, 0 whitespace-only, 0 differ`. Corrected in place.

3. **F016's list of four drifted twins is exactly right.** Independently
   re-derived here from the current source without consulting the list first.

## What is still not proved, and is now easier to see

Deleting the two empty units does not change the F016 decomposition's answer —
`domain::Follow::new` is still the only obligation that is functional,
non-vacuous, about shipped code, reachable from the API, and free of
project-local axioms. But two things moved:

- `service::follow_ensures` is now **one shim away** from being a second one.
  Its D4 clause is about shipped behaviour and can be refuted; what keeps it
  conditional is that `handle_valid` and `service_users_keys` are uninterpreted.
- `ids::next_id_ensures` remains the sharpest problem in the corner and was
  deliberately left alone. It is not a drifted twin — it is an `external_body`
  function whose body is `unimplemented!()`, contributing **0** verified units
  while F8 depends on it. Adding `false` to its `ensures` still gives
  `0 verified, 0 errors`. Fixing that is a different job from this one: it
  needs the counter lifted out of the `Mutex`, not a better contract.

## The rule

**A repair to a verification count can be subtraction.** The instinct when a
proof is found to be weak is to strengthen it; the instinct when a count is
found to be misleading is to explain it. Neither was right for two of these
four. A postcondition-free obligation is not weak evidence, it is *no*
evidence, and the honest edit is to remove it and let the number fall.

The corollary is a test worth running on any headline count before quoting it:
**delete every unit that carries no postcondition and see what is left.** Here
that is 23 → 21 immediately, and F016's harder sequence takes it to 1.
