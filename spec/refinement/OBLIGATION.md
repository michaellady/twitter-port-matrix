# R5 — the refinement obligation, decomposed

`ASSURANCE.md` states R5 in three lines. Those three lines are not directly
checkable by any verifier in this repository, and the reason is not that the
verifiers are weak. It is that the obligation quantifies over functions that
neither corner's verifier can see.

This file states the obligation precisely, splits it into the part that is out
of reach and the part that is not, and enumerates the clauses of the reachable
part so that "R5" stops being a single yes/no and becomes a list with a status
per line.

`obligations.json` carries the same table in machine-readable form.

---

## 1. The obligation as written

For each implementation `L`, an abstraction function `abs_L : State_L -> State_S`
with

```
abs_L(init_L)       = init_S
resp_L(s, r)        = resp_S(abs_L(s), r)          for all s, r
abs_L(step_L(s, r)) = step_S(abs_L(s), r)          for all s, r
```

`r` here is a `sobs.Request`: a `(method, path, body)` triple where `body` is
**raw JSON text**. That is deliberate — `spec/s_obs/types.go` says keeping the
body as raw text "is what makes totality meaningful: malformed JSON is
representable and therefore must have a defined response."

## 2. Why the obligation as written is out of reach in both corners

`resp_L` and `step_L`, at that request type, are the composition

```
wire request  ->  decode  ->  core operation  ->  encode  ->  wire response
```

and **the decode/encode ends are outside the verification perimeter in both
corners, by design**:

| Corner | Perimeter | Wire layer | In perimeter? |
|---|---|---|---|
| Go | Gobra `-directory` over `clock, ids, dom, store, service` (`docker/pins.json`) | `internal/httpshim` | **no** |
| Rust | crates carrying `[package.metadata.verus] verify = true` | `crates/server` | **no** — the manifest has no such key, and `cargo-verus verus verify -p server` prints no `verification results::` line |

`TCB.md` records the split as a deliberate choice, with a good argument behind
it: putting contract semantics in the shim would produce a green R0 over code
no verifier reads. The cost of that choice, which was not previously recorded,
is that **the R5 obligation as literally stated cannot even be typed**, because
`step_L` at the wire alphabet is not a function any contract in either corner
mentions.

Bringing the wire layer inside would require a **verified strict-JSON decoder,
a verified URL query parser, and a verified canonical JSON encoder** in each
verifier's specification language. Neither Verus nor Gobra ships any of these,
and Gobra additionally has no string indexing in its ghost language at all (see
§5), which rules out even stating what "a well-formed handle" is.

**This is a ceiling, not a backlog item.** It is recorded here rather than
worked around, per `GOAL.md` standing rule 8.

## 3. The obligation that is in reach: R5-core

Split the request alphabet at the perimeter boundary:

```
Request  --decode-->  Op  --core-->  CoreResult  --encode-->  Response
         (trusted)         (verified)            (trusted)
```

`Op` is the decoded operation alphabet, one constructor per `S_obs` route:

| Op | S_obs transition | Go core entry point |
|---|---|---|
| `CreateUser(handle)` | `stepCreateUser` | `(*Service).CreateUser` |
| `Follow(from, to)` | `stepFollow` | `(*Service).Follow` |
| `Unfollow(from, to)` | `stepUnfollow` | `(*Service).Unfollow` |
| `PostTweet(author, text)` | `stepPostTweet` | `(*Service).PostTweet` |
| `Tick` | `stepTick` | `(*Service).Tick` |
| `Timeline(user, limit, cursor)` | `stepTimeline` | `(*Service).HomeTimeline` |

**R5-core** is the same three lines with `Op` in place of `Request`. It is
strictly weaker than R5: it says two implementations agree on every sequence of
*decoded* operations, and says nothing about two implementations decoding the
same bytes into the same operation. R0/R1 cover the decoders empirically; R5
does not cover them at all.

## 4. abs, concretely

`abs` is written as one pure ghost projection per axis of the `S_obs` state,
not as a single function returning a record — Gobra cannot build a ghost struct
inside a `pure` function without also owning the fields, and the obligation is
stated per axis anyway.

| `S_obs` `State` field | abs axis | Go (`internal/store/memstore.gobra`) |
|---|---|---|
| `userByHandle` | registered handles | `(*MemStore).AbsUsers() set[string]` |
| `follows` | directed edges | `(*MemStore).AbsFollows() set[dom.Follow]` |
| `tweets` | append-ordered log | `(*MemStore).AbsLogLen()`, `(*MemStore).AbsLogAt(i)` |
| `clock` | logical time | `(*Logical).now`, exposed by `Now()`'s postcondition |
| `nextUserID`, `nextTweetID` | id counters | **not projected** — see §6 |
| `users` (registration order) | — | not projected; no response depends on it |

Every projection is **concrete**. `AbsUsers()` really is `domain(s.users)`. An
uninterpreted `abs` with axiomatised commutation clauses would verify trivially
and prove nothing; that distinction is the whole difference between stating this
obligation and assuming it.

The same four projections are composed one layer up onto `*Service` in
`internal/service/service.gobra`, so the obligation is readable at the boundary
a request actually crosses.

## 5. Named blockers

Each of these was measured against the pinned tools, not inferred.

**B1 — Gobra has no string indexing in the ghost language.**
`dom.ValidHandle` is a byte loop with early returns; Gobra `pure` functions must
be a single expression, so it cannot be `pure` and cannot appear in a
specification. `dom.ValidHandle`'s own postcondition already concedes this: it
exports only `result ==> len in range`, because the alphabet half "cannot be
restated over `h` in the postcondition, since that needs string indexing."
*Consequence:* every clause of R5-core whose S_obs counterpart branches on
handle or text syntax — which is the accept direction of four of the six
operations — is unstateable.

**B2 — Gobra does not thread a package-level `var`'s initializer postcondition
into method bodies.** `store.ErrHandleTaken` is initialised by
`newErrHandleTaken()`, which carries `// @ ensures err != nil`, yet
`assert ErrHandleTaken != nil` inside `PutUser` fails with
"Assertion ErrHandleTaken might not hold".
*Consequence:* the reject direction of the response commutation
(`... ==> err != nil`) is unstateable wherever the error is a sentinel `var`.
Worked around where it matters by keying the state commutation on `abs` and the
request instead of on `err` — which is also the literal shape of the obligation.

**B3 — Gobra does not re-instantiate an existentially-quantified postcondition
at a call site.** `(*MemStore).HomeTimeline`'s "no fabrication" and "no loss"
clauses are discharged; restating either verbatim on the line after the call in
`(*Service).HomeTimeline`, with the inner predicate still unfolded and in the
store's own vocabulary, fails with "Assert might fail". The usual remedy is a
ghost result carrying the witness, and it is unavailable: ghost parameters must
appear in the signature, and these files must also compile under `go build`.
*Consequence:* the two strongest timeline clauses are proved where the page is
built and are one layer short of where the request arrives.

**B4 — vstd has no model of `std::sync::RwLock` or `std::sync::Mutex`.**
`vstd-0.0.0-2026-04-20-1748/std_specs/` contains no `sync.rs`. Attempting to
give a Verus `spec fn` a body that projects the store's state yields, verbatim:
`std::sync::poison::rwlock::RwLock is not supported`,
`... RwLockReadGuard is not supported`, `... ::read is not supported`,
`std::sync::poison::PoisonError is not supported`, and
`... RwLockReadGuard as Deref>::deref is not supported`.
*Consequence:* `abs_rust` cannot be given a body. It can only be declared
`external_body`, i.e. uninterpreted, which makes every commutation clause about
it an axiom rather than a theorem.

**B5 — `vstd::rwlock::RwLock` exposes no value projection either.** Swapping to
vstd's own lock does not fix B4. `vstd::rwlock::RwLock<V, Pred>` offers
`pub open spec fn inv(&self, val: V) -> bool` — a predicate on candidate values
— and `ReadHandle::view()`, which requires *holding a read handle*. There is no
`spec fn value(&self) -> V` on the lock, and there cannot be: outside the
critical section a lock-protected value is not a function of the lock. **An
abstraction function over a state that lives behind interior mutability is not
definable, in any verifier, without first lifting that state out of the lock.**
`S_obs` itself has the right shape here — its `State` is a value and `Step` is
pure.

**B6 — Verus `external_body` proof modules are not linked to the production
functions they mirror.** See §7.

## 6. The id axis, and why it is not projected

`S_obs` allocates ids from 1 monotonically, and returns them in the response
bodies for `POST /users` and `POST /tweets`. So the id counters are part of
`abs` and the ids are part of `resp_S`. Neither corner can project them:

- **Rust `crates/ids`: `verification results:: 0 verified, 0 errors`.** All ten
  items in its `verus_proof` module are `external_body`. `count(g)` is defined
  as `lock_state_value(inner_state(g))` where both `lock_state_value` and
  `inner_state` are themselves `external_body` and therefore uninterpreted. The
  F8 contract lives on `next_id_ensures`, an `external_body` function that no
  production code calls. So F8 in Rust is an assumed postcondition, on a
  function that is never executed, stated over symbols with no definition. Zero
  obligations means zero obligations.
- **Go: F8 is proved.** `(*ids.Generator).Next` carries three functional
  postconditions and is not `// @ trusted`:

  ```
  // @ ensures result == old(unfolding acc(g.LockP()) in g.next)
  // @ ensures unfolding acc(g.LockP()) in g.next == result + 1
  // @ ensures result >= 1
  ```

  All three are refutable — Gobra rejects each negation at that member — and
  the member refutes `ensures false`, so its exit is reachable and they are not
  vacuous. `obligations.json` clause 20 has recorded this as `discharged` since
  S-13; `GhostNext` in `ids.gobra` is a leftover ghost declaration that nothing
  calls and that nothing now depends on.

**This paragraph previously said the opposite**, in the same commit that
discharged the obligation and recorded it as discharged in `obligations.json`.
The correction is [F020](../../evidence/findings/F020-the-prose-contradicted-its-own-commit.md);
it is left visible here rather than quietly rewritten, because the failure mode
— prose and data file stating the same fact with nothing comparing them — is
the finding.

So F8 is **not** symmetric across the corners: proved in Go, and in Rust an
assumed postcondition on a function that is never executed. That asymmetry is a
result, not a gap to close.

The Go half matters beyond F8: it is exactly the premise
`(*MemStore).PutTweet`'s accept condition needs. The store's guard rejects an
append whose id does not strictly exceed the last one, and discharging "that
guard never fires" is what would let the service layer prove that a post from a
known author is always accepted. See `AbsAcceptsTweet` in
`internal/store/memstore.gobra`. Note what F018 established about that guard,
though: ids being *allocated* in order is not ids being *appended* in order,
and the composition only holds because `Service.wmu` now spans allocate-then-
write. F8 alone does not discharge it, and `S_obs` has no vocabulary for the
part that does.

## 7. Rust: what `verus_proof` actually verifies

The Verus proofs in `impls/rust` are **not** on the production functions. Each
crate carries a `#[cfg(verus_only)] mod verus_proof` containing hand-written
`*_ensures` twins whose bodies restate the production control flow against
`external_body` shims. `MemStore::put_user` is plain Rust; `put_user_ensures` is
the thing Verus checks. Nothing mechanically relates the two.

That the module is live was confirmed: injecting `assert(false)` into
`store::verus_proof::has_user_ensures` yields
`error: assertion failed` and `verification results:: 6 verified, 1 errors`.
So Verus really is checking those twins.

**Because nothing mechanically relates a twin to its function, twins drift.**
Four had. S-14 (GOAL.md queue item 5) audited every twin against its
production function line by line and disposed of all four; the audit is
`evidence/findings/F024`. Two were repaired and two were deleted:

| twin | disposition |
|---|---|
| `service::create_user_ensures` | **fixed.** Guarded on `handle.as_str().is_empty()` where production guards on `!domain::valid_handle(handle)`, so its accept clause was *false* of the shipped function for `"Alice"` — corpus step 5. The guard is now a shim whose body is the production call |
| `service::follow_ensures` | **fixed.** Body called `Follow::new` before the existence checks — the pre-D4 ordering, i.e. the F003 defect. Body now mirrors production, and two clauses name *which* error, so the ordering is visible to the verifier for the first time |
| `store::home_timeline_ensures` | **deleted.** Zero `ensures` clauses, delegating to a second, cursor-less copy of the timeline walk |
| `service::home_timeline_ensures` | **deleted.** Same, one layer up; the shim passed `cursor = 0` and dropped the `more` flag |

`put_tweet_ensures` was the fifth drift and is **already repaired** — S-13
replaced its `users_keys(old(s)).contains(t.author@) ==> result is Ok` with
`accepts_tweet(old(s), t) ==> result is Ok`, conjoining the monotonicity guard
that production's third branch applies. An earlier revision of this section
stated that drift in the present tense after it had been fixed; that sentence
was wrong and is replaced by this one (F020).

The count over the five verify-enabled crates is now
**clock 2, ids 0, domain 9, store 6, service 4 — 21 verified, 0 errors**,
down from 23 because the two deletions removed two contentless units. Read
that number with F016's decomposition beside it: it counts units of work, not
guarantees, and what it counts is still copies rather than shipped functions.
Any refinement claim built on this shape would be a claim about the twins.

## 8. Status of R5-core, per clause

`D` = discharged by the verifier named, from its own output, **and** refutable:
Gobra rejects the clause's negation at that member, so the obligation is not
vacuous. `U` = the package verifies with the clause present but the negation was
not decided within a 6-minute budget, so vacuity is **unruled-out** and the
clause is unaudited rather than discharged. That distinction is F013's: a green
package is compatible with an obligation nothing reaches.

This table is now derived rather than asserted. `spec/refinement/clause-sites.json`
maps each clause to the `ensures` that carries it, and `go run ./tools/cmd/gobra r5`
prints the status together with the Gobra line that refuted each negation.
Current run: **30 VERIFIED, 0 UNAUDITED, 12 UNATTEMPTED, 0 FAILED, 0 VACUOUS**
(`evidence/runs/gobra/r5-clause-status.txt`). The four `U` rows this table
carried until 2026-09-02 were all on `HomeTimeline` and are now `D`: see F029
and the "four `U` rows" note below. An earlier run's 25/5 was produced by a
colliding checkpoint key and is withdrawn.
`—` = not stated, with the blocker named.

### Go — `internal/store` (Gobra, image `sha256:2ef080cc`)

| Operation | Clause | Status |
|---|---|---|
| `PutUser` | fresh handle ⇒ `AbsUsers' = AbsUsers ∪ {h}` | **D** |
| `PutUser` | taken handle ⇒ `AbsUsers' = AbsUsers` | **D** |
| `PutUser` | fresh handle ⇒ `err == nil` | **D** |
| `PutUser` | taken handle ⇒ `err != nil` | — B2 |
| `PutUser` | other axes framed | **D** |
| `HasUser` | `result == h ∈ AbsUsers` | **D** |
| `PutFollow` | both ends known ⇒ `AbsFollows' = AbsFollows ∪ {e}`, `err == nil` | **D** (F3, F9) |
| `PutFollow` | either end unknown ⇒ `AbsFollows' = AbsFollows` | **D** |
| `DeleteFollow` | `AbsFollows' = AbsFollows \ {e}`, unconditionally | **D** (F3) |
| `Follows` | `result == e ∈ AbsFollows` | **D** |
| `PutTweet` | `AbsAcceptsTweet(t)` ⇒ length +1, `t` last, `err == nil` | **D** (F6) |
| `PutTweet` | `¬AbsAcceptsTweet(t)` ⇒ length unchanged | **D** |
| `PutTweet` | prefix of the log unchanged (append-only) | **D** |
| `HomeTimeline` | descending `(created_at, id)` | **D** (F2) |
| `HomeTimeline` | every entry authored by `user` or followed by `user` | **D** (**F1**), F029 |
| `HomeTimeline` | `cursor > 0 ⇒ every id < cursor` | **D** (D10), F029 |
| `HomeTimeline` | no fabrication: every entry is some log entry | **D**, F029 |
| `HomeTimeline` | no loss: if `!more`, every visible entry under the cursor is on the page | **D**, F029 |
| `Replace` | installs a log satisfying the append-log invariant | **D**, not canaried — see below |

**The four `U` rows were all on `HomeTimeline`**, and so were three framing
negations that refute in seconds on every other member. F021 read that as the
method's proof sitting at the edge of the budget and concluded that auditing
cost rises with obligation strength — so the obligations most worth checking
for vacuity would be the ones the check cannot reach.

That is measured now and it was the wrong diagnosis
([F029](../../evidence/findings/F029-the-audit-was-undecidable-as-spelled-not-as-asked.md)).
The clean package proves all nine of these postconditions in 42 s; what cost
722 s and then 2703 s was the *canary's* shape, which re-verified the whole
member to ask about one clause, and the *spelling* of three derived negations
(`forall a int :: !(0 <= a && a < len(out))` rather than the equal and
decidable `len(out) == 0`). Asked one at a time, in a decidable spelling, all
four refute — 98 s to 637 s — and each carries a control run on the same member
with `assume false` in the body that comes back VACUOUS. Neither
`--parallelizeBranches` nor a 3.75x budget helped at all.

`Replace` carries no functional postcondition at all. What it discharges is the
`fold acc(s.LockP())` closing its body — `LockP()` carries the append-log
invariant, so the fold succeeds only if the installed log satisfies it. A
negation canary over an `ensures` cannot reach a fold obligation, so this row is
discharged but unaudited by a different route than the five above. Its canary
(CANARY H in `impls/go/internal/_broken/`) is a Go test, not a Gobra one.

### Go — `internal/service` (the boundary a request crosses)

| Operation | Clause | Status |
|---|---|---|
| `CreateUser` | at most `handle` is added; no other user appears | **D** |
| `CreateUser` | already-registered handle ⇒ `AbsUsers` unchanged | **D** |
| `CreateUser` | valid, fresh handle ⇒ added | — B1 |
| `Follow` | at most the named edge is added | **D** |
| `Follow` | `from == to` ⇒ `AbsFollows` unchanged | **D** (**F4 on abs**) |
| `Follow` | either endpoint unknown ⇒ `AbsFollows` unchanged | **D** (**F9 on abs**) |
| `Follow` | valid, known, distinct ⇒ edge added | — B1 |
| `Unfollow` | at most the named edge is removed | **D** |
| `Unfollow` | either endpoint unknown ⇒ `AbsFollows` unchanged | **D** |
| `PostTweet` | log grows by at most one | **D** |
| `PostTweet` | unknown author ⇒ log unchanged | **D** (**F6 on abs**) |
| `PostTweet` | the existing log prefix is never rewritten | **D** |
| `PostTweet` | valid, known author ⇒ appended | — B1, and §6 |
| `HomeTimeline` | ordering, visibility, cursor, framing | **D** (F1, F2, D10) |
| `HomeTimeline` | no fabrication, no loss | — B3 (proved one layer down) |
| `Tick` | `now' == now + 1` | **D** at `(*Logical).Tick`; not forwarded through the `nowLogical` / `tickLogical` interface shims, which stay `// @ trusted` |
| all | id counters | — §6 |

### Rust — `impls/rust` (Verus 0.2026.04.24.f8e1704)

| Clause | Status |
|---|---|
| `abs_rust` has a body | — B4, B5 |
| any commutation clause | — not stated; would be an axiom over an uninterpreted `abs`, per B4 |
| what the 21 verified obligations cover | the `verus_proof` twins, not the production functions — §7 |

### `init` commutation

`abs_L(init_L) = init_S` is **not discharged in either corner.** In Go the axes
are only reachable through `LockP()`, which `New()` folds without a functional
postcondition; stating `AbsUsers() == set[string]{}` on `(*MemStore).New` is a
small addition and is not claimed here because it has not been run. In Rust it
is blocked by B4 like everything else.

## 9. What this licenses

`ASSURANCE.md`'s R5 row currently reads:

> **A and B are observationally equivalent, on every request sequence**

Nothing in this repository supports that sentence today, and this file exists so
that it is not written down until something does. What is supported:

- On the Go corner, on the decoded-operation alphabet, restricted to
  syntactically valid arguments and excluding id allocation: a real abstraction
  function with concrete bodies, and the state commutation discharged on every
  axis for every mutating store operation, plus the full response commutation
  for the timeline.
- On the Rust corner: nothing at the refinement rung, and the R4 result is over
  hand-written twins rather than over the shipped code.

A port's claim is capped by the weaker of its two ends. With Rust at "no
abstraction function is definable", **every port with Rust at either end is
capped below R5 regardless of what the other end proves.**
