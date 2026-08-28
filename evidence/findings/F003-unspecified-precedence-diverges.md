# F003 — Two implementations can refine the model and still disagree observably

**Status:** confirmed against the vendored Go implementation
**Significance:** this is the repository's central thesis, made concrete

## The claim under test

`A ⊑ S ∧ B ⊑ S` does not give `A ≡ B` when `S` is nondeterministic or silent.
Both existing repos' READMEs concede the point in the abstract — "two sources
of evidence pointing at the same property numbers, not a refinement proof."

Phase 1's first R0 run turned it into a specific, reproducible disagreement.

## The disagreement

`twitter.tla`'s Follow action is an unordered conjunction:

```tla
Follow(a, b) ==
    /\ a \in knownUsers
    /\ b \in knownUsers
    /\ a # b
    ...
```

TLA+ conjunction has no evaluation order, so the model does not say which
error `follow(eve, eve)` produces when `eve` is unknown. Both answers refine
it.

`S_obs` pins existence-before-semantics (decision D4): `unknown_user`.

The Go implementation picked the other one:

```
DIFF  reject_self_follow_unknown_is_unknown_user
      request  POST /follow {"from":"eve","to":"eve"}
      expected 400 {"error":"unknown_user"}
      got      400 {"error":"self_follow_forbidden"}
```

Neither is wrong. Both satisfy F4 and F9. TLC would accept either. Gobra and
Verus would each happily prove F4 of either. And a client can tell them apart
with one request.

That is the whole argument for R5 in a single line of output: proving the same
property *numbers* on both sides leaves observable behaviour free to differ.
Only refinement to a machine that is deterministic **and total** closes it,
and totality is what makes this case have an answer at all.

## Full R0 baseline, before any retargeting

`evidence/runs/r0/go-before-retarget.txt`

```
R0 result: 7/54 exact, 8 whitespace-only, 39 differ
```

By kind — the shape of the gap matters more than its size:

|  n | kind |
|---|---|
| 14 | body differs |
|  6 | status 200, want 400 |
|  5 | route absent |
|  3 | status 201, want 400 |
|  8 | error-code vocabulary (7 distinct renamings) |
|  1 | error-shape mismatch |
|  1 | method not allowed |
|  1 | status 409, want 400 |

By property and decision:

```
* D7  0/10   strict parsing -- the impl accepts unknown fields and trailing content
* D3  0/3    no tick route: the clock is not reachable through the API at all
* F7  0/4    downstream of D3
* F8  0/5    downstream of D3
* F2  0/3    downstream of D3
* D9  0/2    downstream of D3
* D6  0/3    syntax-before-existence not applied; "Alice" is accepted as a handle
* D10 1/7    pagination absent
* D4  1/2    THIS finding
  F3  3/3    idempotence already correct
  F9  3/3    orphan-edge rejection already correct
  F4  1/1    self-follow rejection already correct
  D5  1/1    self-unfollow no-op already correct
```

## Reading the baseline honestly

39 of 54 is not a quality judgement on the Go implementation. It is the
distance between a contract that was *permissive and partial* and one that is
*deterministic and total*, measured for the first time.

Most of it collapses into three roots:

1. **No tick route (D3).** One missing endpoint invalidates F2, F7, F8 and D9
   at once, because every `created_at` in the corpus becomes unreachable. This
   is the same root cause as finding F001, seen from the implementation side.
2. **Lenient parsing (D7).** Ten steps. Every one is a place where two
   implementations could accept different inputs and both look correct.
3. **Error-code vocabulary.** Seven renamings — `duplicate_user` vs
   `handle_taken`, `empty_handle` vs `invalid_handle`, `invalid_json` vs
   `malformed_request`, and so on. Purely a naming question, and entirely
   unconstrained by the model.

The eight whitespace-only differences are a trailing newline. Under D8 that is
still a real observable difference and still fails R0, but it is filed
separately so it does not get counted as a semantic gap.

## What this changes

Nothing about the plan; it confirms it. Step 1c retargets the Go
implementation onto the `S_obs` contract, and this file is the before-picture
that makes the after-picture meaningful.
