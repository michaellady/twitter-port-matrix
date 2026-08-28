# F007 — The same design change bought −2 shims in Go and 0 in Rust

> **Revised.** The first version of this file claimed −6 for Go and built an
> explanation on it. Both the number and the explanation were wrong. The
> original is preserved at the bottom, because how it was wrong is the more
> useful part.

**Status:** measured against both verifiers actually running
**Significance:** the first quantified per-language difference, and a lesson in
measuring a proxy instead of the thing

## The measurement

The flat reshape — nested containers replaced by an edge-keyed set and one
append-ordered log — was applied identically to both corners.

| Corner | Marker | Before | After | Delta |
|---|---|---|---|---|
| Go | `// @ trusted` | 25 | 27 | **+2** |
| Rust | `external_body` | 42 | 42 | **0** |

Gobra, run against the reshaped code: clock 21, ids 18, dom 34, store 64,
service 105 — **242 obligations, 0 errors**, read from Gobra's own output and
cross-checked against `stats.json`.

Go's trusted surface went **up**, not down.

## Why the original number was wrong

The earlier −6 was counted by grepping `// @ trusted` markers out of the Go
store after the reshape.

**Gobra could not parse that package at the time.** `dom` failed with four
errors, `store` died on them, and `LockP` still named the deleted `byAuthor`
field. The markers were counted on code no verifier could read, so the count
measured nothing. It was a proxy for trust, and the proxy had never been
validated against the tool it was standing in for.

Run the verifier, and three of the six "deleted" shims come straight back:

| shim | verdict | Gobra's own reason |
|---|---|---|
| `putFollowEdge` | gone | `s.follows[f] = true` verifies directly |
| `gatherTimeline` | gone | the indexed backward walk verifies |
| `sortTimeline` | gone | the operation no longer exists |
| `deleteFollowEdge` | **back** | `got unknown identifier delete` — no `delete` builtin, flat or nested |
| `appendTweet` | **back** | `append expects first argument of type perm…` — the permission-first form is not valid Go |
| `iterFollows` | **back** | `Loop invariant is not well-formed` — fails even with an empty body under full permission |

Only two of the six went for the reason the original explanation gave. The
other three are blocked on Gobra's coverage of Go *builtins* — `delete`,
`append`, `range` over a map — which flattening a container cannot touch.
Upstream's own `deleteFollowEdge` comment already said it "does not parse the
`delete` builtin," so the nesting story was never complete; it was just the
part that fit.

## What the reshape actually bought

Not a smaller trusted surface. A discharged obligation.

**F2 is now proved rather than assumed.** `LockP()` carries the D9 append-log
invariant as a quantifier; `PutTweet`'s monotonicity guard re-establishes it on
every append; `HomeTimeline` carries a real descending-`(created_at, id)`
postcondition, forwarded through `service`. No sort exists, so no sort
specification is owed.

That is the obligation both upstream repos recorded as out of scope — "a
sort-spec on `sort.Slice` plus a stability proof," "a `vstd::vec` sort spec or
a verified mergesort import." It is not discharged. It is **deleted**, because
the structure no longer creates it.

**F1 remains unproved.** It needs returned elements related back to log
positions, and that is stated in the sidecar rather than left implied by F2's
win.

## The deeper gap: a proof conditional on trusted code

`Replace` (snapshot load) is trusted, carries no `LockP` contract, and its
`(created_at, id)` sort does **not** establish the log invariant. Given
`[{id:5,ts:0},{id:3,ts:1}]` the sort is a no-op and the invariant is violated.

Unreachable from the observable API today, so no rung can see it. But F2's
proof is conditional on snapshot well-formedness, and that condition lives in
trusted code. A proof whose premise is established nowhere is a weaker claim
than its green check suggests — the same shape as F005, one layer up.

## Rust, for comparison

Zero shims removed, and for a genuinely different reason: `vstd` ships no model
for `std::collections::HashMap` / `HashSet` / `Vec` behind a
`std::sync::RwLock`. Flattening a container the verifier cannot see into at all
changes nothing. Rust's gain, like Go's, is that `proof_home_timeline` stopped
standing in for an ordering proof — narrower in what it asserts, unchanged in
count.

## What survives

The conclusion holds; the evidence for it did not. The assurance value of a
design change is not a property of the design — it depends on the target
verifier's coverage, and that coverage is about **library and builtin support**
at least as often as about the language.

For the Java→Rust work: a restructuring justified by "it discharged an
obligation in the source language" must be re-costed against the target
verifier's own gaps, and those gaps may be as mundane as whether the tool
parses `delete`.

## The lesson from being wrong

Counting `// @ trusted` markers is measuring a proxy. The proxy is only
meaningful if the verifier can read the file — and here it could not, which is
precisely why the markers were free to move without consequence.

**Do not count annotations as a stand-in for running the verifier.** It is the
same error as reading an exit code instead of the output, wearing different
clothes: a number that looks like evidence, produced without the tool that
would make it evidence.

<details>
<summary>Original claim, preserved</summary>

The first version reported Go `10 → 4 (−6)` against Rust's `21 → 21 (0)` and
explained the difference as Gobra being blocked on nested permission while
Verus is blocked on missing `vstd` models. The Rust half of that is right. The
Go half counted markers in a package Gobra could not parse, and the nesting
explanation covers two of the six shims rather than all six.

</details>
