# S_obs decisions

`twitter.tla` is a design-level model. It leaves questions open that any
executable implementation must answer, and two implementations answering them
differently would both still refine the model — which is exactly why refining
the model does not make two implementations equivalent.

`S_obs` closes every one of those questions. Each decision below is a place
where the model was silent and a choice had to be made. They are recorded, not
buried, because a reader checking whether an implementation refines `S_obs`
needs to know which constraints come from the model and which come from here.

---

## D1 — Tweets carry `text`; the TLA+ projection drops it

`twitter.tla` defines `TweetRec == [id: Nat, author: Users, ts: Nat]`. There is
no text field. The old conformance corpus nonetheless asserted
`"text":"hello world"`.

**Decision.** `S_obs` tweets carry `Text`. The projection used by `tlclink`
drops it before checking a trace against the model. Text is observable but
model-irrelevant: no F-property mentions it.

**Constraint.** 1–280 bytes, no control characters. Violations are
`invalid_text`.

## D2 — Users carry a numeric `id`; the TLA+ projection drops it

The model treats users as opaque handles drawn from the `Users` constant set.
The old corpus asserted `{"handle":"alice","id":1}`.

**Decision.** `S_obs` allocates user ids from 1, monotonically, in registration
order. The projection drops them. Handles remain the identity used everywhere
in the API; ids are returned but never accepted as input.

## D3 — The clock is client-driven via `POST /tick`

**This is the decision that matters most.** See `evidence/findings/F001`.

The model's `Tick` is a free action, unattached to any request. The observable
API had no way to invoke it, so the old corpus asserted `created_at: 1` in a
state no sequence of its own requests could reach, and both implementations'
conformance harnesses papered over it by writing directly to the clock before
each `POST /tweets`. The result was a `created_at` field with no falsifiable
check behind it in either repo.

**Decision.** `POST /tick` advances the clock by exactly 1 and returns
`{"clock":<n>}`. One request maps to exactly one TLA+ `Tick` step. The clock
starts at 0, matching the model's `Init`.

**Consequence.** Every `created_at` in a generated corpus is reachable by the
requests in that corpus. No harness may write to implementation state; replay
drives the system only through the observable API. `matrixctl doctor` enforces
this.

**Bound.** `S_obs`'s clock is unbounded. `twitter.tla` bounds it at
`MaxTimestamp`. `tlclink` only submits traces that stay within the model bound.

## D4 — For an unknown self-edge, `unknown_user` beats `self_follow_forbidden`

The model's `Follow(a,b)` requires `a \in knownUsers /\ b \in knownUsers /\
a # b`. TLA+ conjunction is unordered, so the model does not say which error
`follow(eve, eve)` produces when `eve` is unknown. Two implementations could
disagree and both refine the model.

**Decision.** Existence is checked before semantics. `follow(eve, eve)` with
`eve` unknown yields `unknown_user`. `follow(alice, alice)` with `alice` known
yields `self_follow_forbidden`.

## D5 — Self-unfollow of a known user is a legal no-op

`Unfollow(a,b)` requires `a, b \in knownUsers` but, unlike `Follow`, does
**not** require `a # b`. The old corpus never exercised it.

**Decision.** `DELETE /follow` with `from == to` on known users returns 204 and
changes nothing. It is not an error.

## D6 — Syntax is validated before existence, everywhere

Applies uniformly. `follow("EVE", "eve")` yields `invalid_handle`, not
`unknown_user`, because `EVE` is outside the handle alphabet.

Per-route order is pinned in `step.go` and is part of the contract.

## D7 — Strict request parsing

Unknown JSON fields, trailing content after the JSON value, unknown query
parameters, and repeated query parameters are all rejected with
`malformed_request`. Lenient parsing is a classic source of cross-language
divergence, so `S_obs` is strict.

Field names are matched **case-sensitively**, and that requires saying because
Go's `encoding/json` falls back to a case-insensitive match: under
`DisallowUnknownFields` alone, `{"Handle":"alice"}` counts as a *known* field
and is accepted. `S_obs` is written in Go and silently inherited that. Rust's
`serde` is case-sensitive and rejected the same body, so the two corners
disagreed in production while both passing every rung. See
`evidence/findings/F010`.

Numeric parameters (`limit`, `cursor`) use Go's `strconv.ParseInt` accept set,
which is narrower than "digits": `05` is accepted, `+5` is rejected, and
non-ASCII decimal digits such as `٥` are rejected. A port reaching for its
language's natural integer parser will get at least one of these wrong.

Length bounds are in **bytes**, not characters. `MaxHandleLen = 32` and
`MaxTextLen = 280` count UTF-8 bytes, so a port using UTF-16 code units —
Java and Kotlin's `String.length()` — accepts strings `S_obs` rejects.

Each of the three above is a decision `S_obs` delegated to a Go library rather
than one anybody wrote down. They are pinned here now; the remaining
delegations (`url.ParseQuery`'s error conditions, `net/url` percent-decoding)
are listed as open work in F010.

**Known limitation, stated rather than hidden.** Duplicate JSON keys resolve
last-wins. `tracegen` never emits duplicate keys, so no generated trace
depends on this.

## D8 — Canonical response encoding is part of the contract

Byte-identical replay across four languages needs the encoding pinned, not
left to each language's default JSON writer. Object key order is fixed per
response type and is **not** alphabetical; there is no insignificant
whitespace; every number is an integer; `null` is written literally. The rules
and the writers are in `encode.go`.

An implementation that produces a semantically equal but byte-different
response fails R0. That is intended: it is a real observable difference.

## D9 — Timeline is sort-free by construction

The tweet log is append-ordered and never sorted. Timelines are produced by
reverse iteration over it.

**Monotonicity lemma.** For log positions `i < j`: `tweets[i].ID <
tweets[j].ID` because ids are allocated monotonically, and
`tweets[i].CreatedAt <= tweets[j].CreatedAt` because the clock never decreases.
Therefore reverse log order is exactly descending lexicographic
`(created_at, id)` — if the timestamps differ the later post wins on
timestamp, and if they tie the later post wins on id.

**Consequence.** F2 (ordering) is a derived property of an insertion-ordered
structure. Discharging it needs no verified sort specification — which is the
obligation that currently blocks F1/F2 on `home_timeline` in *both* existing
implementation repos, in Gobra and in Verus alike.

## D10 — Pagination

`GET /timeline?user=<h>[&limit=<n>][&cursor=<n>]`. Default limit 50, maximum
100. The cursor is exclusive: a page returns visible tweets with `id <
cursor`. `next_cursor` is the id of the last tweet on the page when at least
one further visible tweet exists below it, and `null` otherwise — so `null`
means precisely "nothing remains."
