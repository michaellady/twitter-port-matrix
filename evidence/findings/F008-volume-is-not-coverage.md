# F008 — 100,000 requests found nothing; widening the alphabet found it at step 8

**Status:** confirmed on two implementations, fixed in both
**Found by:** a hand-built cross-check while standing up a third corner, not by any rung

## What happened

R1 had run 100,000 generated requests against each of Go and Rust with zero
mismatches. R0 was 54/54 byte-exact on both. R2's nine relations held on every
generated state. Three rungs, all green.

While building the Java corner, a 99-request hand-written cross-check against
`sobs.Step` — covering raw request targets, percent-escapes, integer accept
sets and UTF-8 boundaries — turned up **four divergences in the already-green
Go corner**:

| request | `S_obs` | Go |
|---|---|---|
| `POST /users?x=1` | 404 not_found | **201 Created** |
| `POST /%75sers` | 404 not_found | **201 Created** |
| `POST /tick` body `" {} "` | 400 malformed_request | **200 {"clock":1}** |
| `GET /timeline?user=bob&limit=%zz` | 400 malformed_request | **200, limit silently dropped** |

All four verified independently by driving the Go binary over a raw socket.

## Why every rung missed them

Not because the rungs are weak. Because **`tracegen` never emitted these
shapes.** It produced queries only on `/timeline`, never on a POST path; it
never percent-encoded a path segment; it never put whitespace around a body;
it never emitted a malformed percent-escape.

A differential rung compares two systems on the inputs it generates. Its
coverage is bounded by the **alphabet**, not by the count. 100,000 requests
drawn from an alphabet that omits a construct test that construct exactly zero
times, and the run reports a large reassuring number either way.

Twelve request shapes were added to the generator's hostile pool. R1 then
failed at **step 8 of the first trace** — 8 requests, against 100,000 before.

## The four root causes

Three are the standard library being helpful:

- `net/http`'s `ServeMux` matches on `r.URL.Path`, which is **percent-decoded**,
  so `/%75sers` reached the `/users` handler. And it ignores the query string
  entirely, so `?x=1` on a POST route was invisible.
- `axum` likewise routes on the path and ignores the query — the same defect,
  independently, in the other corner.
- `r.URL.Query()` **discards the error** that `url.ParseQuery` returns. A
  malformed escape silently dropped the parameter and the request was served
  with a default. This is the sharpest of the four: the failure is invisible at
  the call site by design.

One was mine. The hand-written `percent_decode` in the Rust shim decoded `%zz`
to the literal text and carried on, so `?limit=%zz` surfaced as `invalid_limit`
rather than `malformed_request` — a wrong answer that still looked like a
rejection, which is the hardest kind to notice.

## Java did not have the bug, and the reason is instructive

The Java corner hand-wrote HTTP/1.1 framing on a `ServerSocket` and got all
four right first time. Not from care — from **not having a framework to be
helped by**. `com.sun.net.httpserver` was rejected earlier for a related
reason: it parses the target with `java.net.URI` before any handler runs and
answers anything `URI` dislikes with its own 400 page, which is a hole in
totality that a `Filter` cannot close.

So the corner with the least infrastructure had the fewest divergences, because
every decision the others inherited it had to make explicitly.

## The transferable rule

**Volume is not coverage.** A differential campaign's reach is set by its
generator's alphabet, and no number of requests widens it. When a run reports
100,000 clean requests, the useful question is not "is that enough?" but
"which constructs can it not emit?"

This lands directly on the verified-java-to-rust-port work. Its Autobahn suite
and its differential lane against the pinned Java jar are both bounded the same
way. "0 FAILED across 247" and "100,000 requests, no mismatch" are the same
kind of number, and neither says anything about a construct the corpus does not
contain. The corpus's *alphabet* deserves an audit of its own, separate from
its pass rate.

## Fixes

Go: routing now compares `r.URL.EscapedPath()` and rejects a query on every
route except `/timeline`; the query is parsed with an explicit `url.ParseQuery`
error check; the tick body is compared exactly rather than trimmed.
Rust: `RawQuery` guards on all non-timeline routes; `percent_decode` returns
`None` on a malformed escape and the query is rejected.

All three corners: R0 54/54, R1 clean over 10,000 requests each with the
widened alphabet.

## Related

F006 measured that two sibling implementations agree on 77% of their shared
divergences from `S_obs`. This is the same failure one layer out: there, the
oracle was too weak; here, the **input distribution** is too narrow. An oracle
you cannot reach is no better than one you do not have.
