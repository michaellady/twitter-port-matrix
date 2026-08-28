# twitter_port_matrix

A 4x4 port matrix of maximally-verified Twitter clones -- Java, Kotlin, Rust,
Go -- built to answer one question with numbers rather than intuition:

> **How strongly can a port from language A to language B be proven correct,
> without changing the base app?**

This is a calibration rig, not a product. The output that justifies it is a
per-rung mutation-kill table: for each verification layer, what fraction of
injected semantic defects it catches and what it costs to run. That number
transfers to the real work -- the `verified-java-to-rust-port` project, whose
deductive rung is currently blocked and which is too large to find out cheaply
whether closing it is worth the cost.

**Local only.** No remote, no `.github/` directory, no CI. The four source
repositories under `~/dev/` are consumed read-only and are not modified.

## The claim this repository is built to reach

`S_obs` (`spec/s_obs/`) is a **deterministic, total** reference machine over
the observable API.

> If A refines `S_obs` and B refines `S_obs`, and `S_obs` is deterministic and
> total on the request alphabet, then A and B are observationally equivalent on
> every request sequence.

Port correctness then follows by transitivity **with zero changes to the base
app**, because the base app's obligation was discharged against `S_obs` and
never against the port.

Determinism and totality are both load-bearing. Totality is what removes the
hole through which two conforming implementations could legally differ on an
unspecified input.

## Layout

| Path | What it is |
|---|---|
| `spec/s_obs/` | The reference machine. `step.go` is the whole contract. |
| `spec/s_obs/DECISIONS.md` | Every question `twitter.tla` left open, and how it was closed. |
| `spec/tla/` | `twitter.tla` vendored read-only at SHA `0b19aeb0`, digest-checked. |
| `tools/cmd/` | The rig. Go binaries only. |
| `generated/` | Committed, regenerated, byte-compared in the gate. |
| `evidence/findings/` | What the rig found. |
| `impls/` | The four implementations. Phase 1 onward. |

## Running it

```bash
go run ./tools/cmd/matrixctl doctor
```

```bash
go run ./tools/cmd/matrixctl spec check
```

`doctor` checks the toolchain, verifies the vendored spec still matches its
pinned digest, and enforces that no implementation imports `S_obs`.

`spec check` is the Phase 0 gate. Four sub-gates: the corpus regenerates
byte-for-byte, TLC finds no violation of F1-F9, every `S_obs` transition is a
legal `twitter.tla` step, and -- critically -- a deliberately corrupted trace
is **rejected**. That last one is not optional. Without it the third gate could
be passing because it is incapable of failing.

Takes about 90 seconds, nearly all of it TLC exploring 9M states.

## What it found in Phase 0

Two disagreements between the model and the implementations, in a project
where four repositories, eighteen CI workflows, two deductive verifiers and a
model checker were all green.

- **[F001](evidence/findings/F001-self-fulfilling-clock-oracle.md)** -- the
  shared conformance oracle cannot falsify `created_at`. The corpus asserted a
  clock value no sequence of its own requests could reach, and both
  implementations' harnesses set the clock to the expected answer before asking
  the question. That field had no R0 signal in either repo.
- **[F002](evidence/findings/F002-tweet-id-origin-mismatch.md)** -- the model
  starts tweet ids at 0, both implementations start at 1. Harmless in itself;
  recorded because nothing in the existing artefact set could have told you
  either way.

Neither is a coding error. Both are the same structural gap: the model was
checked, the code was checked, the corpus was checked against the code, and
nothing checked the join.

See [ASSURANCE.md](ASSURANCE.md) for the rung ladder and current status, and
[TCB.md](TCB.md) for what is trusted rather than verified.
