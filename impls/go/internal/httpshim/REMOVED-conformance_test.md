# Why `conformance_test.go` was not vendored

The upstream file drove the implementation's clock directly:

```go
func setClockTo(c *clock.Logical, target int64) {
	for c.Now() < target { c.Tick() }
}
```

called with the *expected* `created_at` before each `POST /tweets`, making the
subsequent assertion `assert(x == e)` immediately after `set(e)`.

See `evidence/findings/F001-self-fulfilling-clock-oracle.md`.

Conformance in this repository is `tools/cmd/replay`, which drives the
implementation only through the observable HTTP API and never touches its
internal state. Standing rule 2 in `GOAL.md`.
