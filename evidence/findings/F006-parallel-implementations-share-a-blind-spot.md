# F006 — Two implementations built from one permissive spec share 30 of their 39 gaps

**Status:** measured, both implementations, before any retargeting
**Method:** R0 baselines replayed against `S_obs`, then compared to each other

## The measurement

| | exact | whitespace-only | differ |
|---|---|---|---|
| Go | 7/54 | 8 | **39** |
| Rust | 15/54 | 0 | **39** |

Both diverge from `S_obs` on **exactly the same 39 steps**. Identical step
sets; the symmetric difference is empty. The 8-vs-0 whitespace column is the
whole visible difference between them, and it is only that `json.Encoder`
appends a newline where `serde` does not.

Comparing the two implementations' *actual responses* on those 39 steps:

| | count | share |
|---|---|---|
| Both diverge from `S_obs` **and agree with each other** | **30** | 77% |
| Both diverge **and disagree with each other** | 9 | 23% |

## What that means

A differential test between these two implementations cannot see 30 of the 39
places where both disagree with a deterministic total contract. Not because
the test is weak, but because **agreement is the wrong oracle when both sides
inherited the same permission.** `twitter.tla` does not constrain query-string
handling, JSON strictness, error-code vocabulary, id origin, or clock advance,
so both implementations chose freely — and, having been built in parallel from
that one spec by the same author, chose alike.

The blind spot is not incidental. It is exactly the size and shape of the
shared spec's permissiveness.

This is the correlated-failure argument arriving at the implementation layer,
measured rather than asserted. `maximize-verification` states it as a rule:
"two tests that fail together give you one test's worth of assurance." Here it
is 30 steps' worth of agreement carrying one implementation's worth of
evidence.

## What the differential WOULD catch — and one that matters

Nine steps do differ between them. Five are the missing tick route showing
different 404 bodies, and two are empty-vs-JSON error bodies. Trivial.

Two are not.

**`reject_missing_handle_field`** — `POST /users` with body `{}`:

```
go   : 400 {"error":"empty_handle"}
rust : 400 {"error":"invalid_json"}
```

Same status, different error code, for the same request.

**`reject_timeline_repeated_param`** — `GET /timeline?user=bob&user=alice`:

```
go   : 200 {"tweets":[ ...five tweets... ],"next_cursor":null}
rust : 400 Failed to deserialize query string: duplicate field `user`
```

One serves a full timeline; the other refuses the request. A client can send
that URL today. Go's `r.URL.Query().Get("user")` silently takes the first
value; Rust's serde query deserializer rejects the duplicate field. Neither
contradicts `twitter.tla`, which says nothing about query strings. Both are
"correct" against the model. They are observably different programs.

## The honest reading

This does **not** say the diffsplitter is useless. It is the only component in
that family that compares the two implementations against each other at all,
and it would have had a chance at the two substantive divergences above.

It says something narrower and more useful: the ceiling on what
implementation-vs-implementation agreement can establish is set by the
spec they were both built from. Here that ceiling leaves 77% of the observable
gap unobservable — and no amount of traffic, sampling rate, or uptime moves it,
because the two systems genuinely agree.

Closing that gap needs an oracle neither implementation produced. That is what
`S_obs` is for, and it is the difference between rungs R1 and R5 in
`ASSURANCE.md`.

## Caveat on scope

The comparison above is over the 54-step generated corpus, driven through each
implementation's HTTP API. It is not a claim about the diffsplitter's runtime
behaviour under production traffic, which samples reads at 10% and JSON-diffs
bodies — whitespace-only differences would normalise away there, and write
responses are queued rather than diffed. The measurement is of the two
implementations; the implication for any differential between them follows
from that, not from reading its code.
