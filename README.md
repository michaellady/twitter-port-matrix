# twitter_port_matrix

Four implementations of one small Twitter-clone API — **Go, Rust, Java,
Kotlin** — built against a single deterministic reference machine, then
deliberately broken 72 different ways to measure **how much assurance each
verification layer actually buys.**

The apparatus is not the point. The point is the numbers, and the eighteen
findings that came out of building it.

```
rung             live  killed  survived  unreached   kill%      wall
R0 corpus          36      36         0          0    100%       57s
R1 diff-fuzz       35      35         0          0    100%     1465s
R2 property        35      14        19          2     42%     2495s
```

Start with **[evidence/FINDINGS.md](evidence/FINDINGS.md)** — the through-line
across all eighteen — then
**[evidence/CALIBRATION.md](evidence/CALIBRATION.md)** for the table above and
what limits it.

## The five findings worth your time

**Three green rungs coexisted with four live defects.** R0 at 54/54
byte-exact, R1 over 100,000 differential requests, R2's nine relations holding
— and a 99-request hand-written cross-check found four real divergences. The
generator had simply never emitted a query string on a POST path. Adding twelve
shapes made R1 fail at step 8 of the first trace. **Volume is not coverage; a
differential campaign's reach is bounded by its alphabet.** ([F008](evidence/findings/F008-volume-is-not-coverage.md))

**A conformance suite that could not fail.** The upstream corpus asserted a
clock value no sequence of its own requests could reach — and both
implementations' harnesses resolved that by *setting the clock to the expected
answer before asserting on it*. The check was `assert(x == e)` immediately
after `set(e)`. It passes for every possible clock rule. ([F001](evidence/findings/F001-self-fulfilling-clock-oracle.md))

**Six obligations reported VERIFIED over unreachable code.** One
undischargeable erased checkcast made everything downstream infeasible. An
*injection* canary cannot detect this — the broken statement is downstream too.
Only asserting the **negation** exposes it: a claim and its negation both
verifying is the unique signature of a vacuous proof. ([F013](evidence/findings/F013-six-obligations-verified-because-nothing-reached-them.md))

**"23 verified, 0 errors" is one property proved.** Eleven of the 23 carried no
postcondition at all; eleven more are conditional on assumed axioms about
hand-written copies of the shipped code — and one of those copies was *false* of
the code it stands for, falsified by an input already in the corpus. ([F016](evidence/findings/F016-twenty-three-verified-means-one-property-proved.md))
The four drifted copies have since been fixed or deleted and the count is now
**21** — it went down, because two of them proved nothing at all. ([F024](evidence/findings/F024-a-count-that-goes-down-is-the-repair.md))

**`"abc".equals("abc")` verifies as FALSE** on JBMC 6.11.0 — reproduced in
plain Java. The Kotlin corner's ceiling turned out to be a tool defect, not a
language one, and the same wall blocks the Java corner. ([F014](evidence/findings/F014-jbmc-cannot-compare-strings.md))

## The pattern underneath

Four of the eighteen are the same mistake in different clothes: **a count
produced without the thing that would make it mean something.** Trusted shims
grepped from a package the verifier could not parse; mutants "killed" because
drifted anchors injected nothing; obligations verified over unreachable code; a
contract discharged about a dead branch.

And **not one of the eighteen was found by a gate reporting green.** Every one
came from stepping outside it.

## Running it

```bash
go run ./tools/cmd/matrixctl doctor
```

```bash
go run ./tools/cmd/matrixctl spec check
```

`.devcontainer/` pins every tool to the build the findings were produced on —
several findings *are* properties of a specific build. Nothing needs a Docker
daemon at runtime. See [CLOUD.md](CLOUD.md).

## Layout

| | |
|---|---|
| `spec/s_obs/` | the deterministic, total reference machine — `step.go` is the contract |
| `spec/s_obs/DECISIONS.md` | every question the TLA+ model left open, and how it was closed |
| `impls/` | the four corners |
| `tools/cmd/` | the rig: `replay` `diffrun` `proptest` `mutate` `calibrate` `tlclink` |
| `evidence/` | findings, calibration, raw runs |

## Provenance and honesty

Built as a scale model for a larger Java→Rust port, to price the verification
ladder before committing to it there —
[evidence/TRANSFER-to-websocket-port.md](evidence/TRANSFER-to-websocket-port.md)
is the write-up.

`spec/tla/twitter.tla` is vendored read-only at SHA `0b19aeb0` from
`michaellady/twitter-formal-spec`. The Go and Rust corners began as vendored
copies of `twitter-golang-formal-verification` and
`twitter-rust-formal-verification` and were then substantially rewritten; the
Java and Kotlin corners were written here.

Several findings correct claims made *earlier in this repository's own
history*, including two of my own. Those corrections are kept in place rather
than edited away — [F007](evidence/findings/F007-same-design-change-different-tcb-payoff.md)
preserves its original wrong number in a collapsed block, because how it was
wrong is more useful than the finding was.
