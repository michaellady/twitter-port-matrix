# F001 — The shared conformance oracle cannot falsify `created_at`

**Status:** confirmed, both implementations
**Found:** Phase 0, while replaying the legacy corpus through `S_obs`
**Severity:** the affected field has no R0 signal at all in either repo

## What was observed

Replaying the 18-step hand-written `conformance.jsonl` through `S_obs` diverges
on exactly 5 steps, and every one of them is the same single disagreement:

| step | field | corpus asserts | `S_obs` produces |
|---|---|---|---|
| 7  | `created_at` | 1 | 0 |
| 8  | `created_at` | 1 | 0 |
| 11 | `created_at` | 1 | 0 |
| 12 | `created_at` | 1 | 0 |
| 13 | `created_at` | 1 | 0 |

The other 13 steps match on status and on every field, including ordering
(step 13's clock-tie tiebreak), follow/unfollow idempotence, all four error
codes, and `next_cursor: null`.

So the corpus and the model disagree about exactly one thing: **the clock.**

## Why the corpus is unreachable as written

`twitter.tla` initialises `clock = 0` and advances it only via the free `Tick`
action. `internal/clock/clock.go` in the Go repo says plainly that `Logical`
"only advances when Tick() is called explicitly," and `New()` "returns a
Logical clock starting at zero."

No request in `conformance.jsonl` advances the clock. There is no tick
endpoint. Yet step 7 asserts `created_at: 1`.

The corpus therefore describes a state no sequence of its own requests can
reach.

## How both implementations pass it anyway

Both conformance harnesses read the answer out of the oracle and move the
system under test to match it, before asking the question.

Go — `internal/httpshim/conformance_test.go:49`:

```go
func setClockTo(c *clock.Logical, target int64) {
	for c.Now() < target {
		c.Tick()
	}
}
```

driven from the replay loop:

```go
if spec.Request.Method == http.MethodPost && spec.Request.Path == "/tweets" {
	if v, ok := spec.Expected.Body["created_at"]; ok {
		if f, ok := v.(float64); ok {
			setClockTo(c, int64(f))
		}
	}
}
```

Rust — `tests/conformance.rs:63,92`, structurally identical:

```rust
fn set_clock_to(clk: &Logical, target: i64) { ... }
// Drive the clock for PostTweet.
if let Some(ts) = body.get("created_at").and_then(|v| v.as_i64()) {
    set_clock_to(&clk, ts);
}
```

The Go file's own comment describes the design as intentional: "To keep the
test simple and the spec authoritative, we use a *driven* clock: before each
PostTweet step, we set the clock to the expected created_at by ticking
forward."

## Why this matters

The check is `assert(created_at == expected)` immediately after
`set_clock(expected)`. It passes for every possible clock rule. An
implementation that sets `created_at` to a constant, to a random value, or to
the tweet id would still pass this conformance step, because the harness
rewrites the clock to the expected value first.

`created_at` — and therefore F7, the non-decreasing-clock property, at the
observable layer — has **zero R0 signal** in either repo. The field looks
covered by an 18-step conformance suite and is in fact unfalsifiable by it.

This is the oracle-mutation failure mode from `maximize-verification` step 6
("Passing by mutating the oracle is the canonical cheat"), arriving from the
opposite direction: the harness mutates the system under test to agree with
the oracle rather than the reverse. The effect is the same — a green check
that cannot go red.

## Root cause

The clock-advance rule was never specified. `twitter.tla` leaves `Tick` free
and unattached to any request; the observable API has no way to invoke it. The
corpus needed a `created_at` value, an author picked 1, and the harnesses were
then written to make 1 happen.

Nothing here is a coding error. It is a specification hole that the artefact
set had no way to detect, because there was no deterministic total machine
over the observable API to check the corpus against.

## Fix carried in this repo

`S_obs` closes the hole three ways (see `spec/s_obs/DECISIONS.md` D3):

1. The clock becomes client-driven and observable: `POST /tick` maps 1:1 onto
   the TLA+ `Tick` action, so the clock can only advance by a request that
   appears in the trace.
2. The corpus is **generated** from `S_obs`, so it can only contain reachable
   states. An unreachable assertion is now unrepresentable.
3. No harness may write to implementation state. Replay drives the system
   exclusively through the observable API. `matrixctl doctor` enforces this.
