# Why `tests/conformance.rs` was not vendored

The upstream file drove the implementation's clock to the expected answer
before asking the question:

```rust
fn set_clock_to(clk: &Logical, target: i64) { ... }
// Drive the clock for PostTweet.
if let Some(ts) = body.get("created_at").and_then(|v| v.as_i64()) {
    set_clock_to(&clk, ts);
}
```

making the following assertion `assert(x == e)` immediately after `set(e)`.
Structurally identical to the Go harness. See
`evidence/findings/F001-self-fulfilling-clock-oracle.md`.

Conformance in this repository is `tools/cmd/replay`, which drives the
implementation only through the observable HTTP API and never touches its
internal state. GOAL.md standing rule 2.
