# F010 — The reference machine encoded a Go-ism, and it became the contract

**Status:** confirmed live on two corners, fixed in three
**Severity:** the highest-value finding so far, because it compromised the oracle itself

## The observation

`POST /users` with body `{"Handle":"zed"}` — capital H — against the two
deployed corners, both of which were passing R0 at 55/55 and had survived
100,000+ differential requests each:

```
go     201 {"handle":"zed","id":1}
rust   400 {"error":"malformed_request"}
```

A live cross-implementation divergence, on a one-line request, in code that
three rungs called green.

## Why `S_obs` sided with Go

D7 says unknown JSON fields are rejected. `S_obs` implements that with Go's
`encoding/json` and `DisallowUnknownFields`.

**That is not strictness.** `encoding/json` falls back to a case-INSENSITIVE
match when no exact match exists, so `Handle` resolves to the `handle` field
and is therefore a *known* field. `DisallowUnknownFields` never fires.

Rust's `serde` with `deny_unknown_fields` is case-sensitive and rejects it.
Java's corner deliberately emulated the Go behaviour, having checked `S_obs`
and found it accepted the body — the agent flagged the written-vs-executable
contract disagreement in its report rather than silently matching.

So both implementations were correct against their reading of D7, and they
disagreed observably.

## The real defect: the arbiter is written in one of the languages it arbitrates

`TCB.md` has warned about this since the first commit:

> the Go corner shares a language, a standard library, and an author with the
> reference machine. Its differential agreement is weaker evidence than the
> other three corners' by an amount this repository cannot quantify.

That was written as a caution about the *Go corner*. It turned out to be a
caution about `S_obs` itself. A decoding behaviour nobody chose, belonging to
one language's standard library, was promoted to cross-language contract — and
the corner sharing that library satisfied it for free while every other corner
had to emulate a quirk to conform.

The written contract said one thing; the executable contract did another; and
because the executable one wins by construction, D7's stated intent was
quietly overridden by a library default.

This is the same shape as F001 — a contract asserting something no request
could establish — except one level up. There the *corpus* had drifted from the
model; here the *reference machine* drifted from its own written specification.

## Why no rung caught it

R0 could not: no corpus step used a case-variant field name. R1 could not:
`tracegen` never emitted one. R2 could not: the properties are about
relations between requests, not about which requests are accepted.

Same shape as F008 and F009. The oracle can only be as good as the inputs that
reach it — and here the oracle was itself wrong, which no amount of input
would have revealed, only cross-checking two implementations against each
other.

**F006 said sibling-vs-sibling agreement is weak evidence. This is the
converse: sibling DISAGREEMENT is strong evidence, and it is the only signal
that can catch a wrong oracle.** Neither direction is sufficient alone.

## The fix

`S_obs` now matches field names case-sensitively, via an explicit pass over the
raw keys before decoding — because Go's decoder cannot be configured to do it.
This is the doc winning over the library: D7 said unknown fields are rejected,
`Handle` is an unknown field, and now it is rejected.

Consequently **Rust was right all along**, and Go and Java both had to change.
That ordering matters: the corner that shared the reference machine's language
was the one carrying the defect.

Also added, because the fix was invisible without them: a corpus step
(`reject_case_variant_field`) and three generator shapes emitting case-variant
field names.

All three corners: R0 56/56 byte-exact, R1 clean over 5,000 requests each.

## What this changes about the project

The oracle needs a check that is not the oracle. `S_obs` is trusted (TCB.md
says so), and this is the first demonstration that trusting it has a real
cost — not a hypothetical one.

Two cheap defences, neither yet in place:

1. **Cross-implementation disagreement should be a first-class signal**, not
   just a step on the way to comparing each against `S_obs`. When two corners
   disagree, exactly one of three things is true: A is wrong, B is wrong, or
   the contract is under-specified. All three are worth knowing, and only the
   comparison surfaces the third.
2. **Every place `S_obs` delegates a decision to a Go library is a candidate
   Go-ism.** `encoding/json` case folding, `strconv.ParseInt`'s accept set,
   `url.ParseQuery`'s error conditions, `net/url` percent-decoding. Each is a
   contract term nobody wrote down. They should be enumerated and either
   pinned deliberately or reimplemented explicitly.

Item 2 is not speculative: `strconv.ParseInt` is already load-bearing. `S_obs`
accepts `limit=05` and rejects `limit=+5` and `limit=٥` — all three inherited
from Go, none stated in D10, all of which a Java or Kotlin port would get
wrong by writing the obvious thing.
