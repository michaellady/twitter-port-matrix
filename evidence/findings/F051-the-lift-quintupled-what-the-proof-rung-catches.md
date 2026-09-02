# F051 — lifting the contracts off the twins took the Rust proof row from 1 kill to 5

**Status:** measured, full 18-mutant sweep on the lifted tree,
`evidence/runs/calibration/rust-proof-postlift/`
**Class:** the payoff of F041, quantified — and the first number in this project
that prices a *proof-engineering* decision rather than a tool

## The before and after

Same catalogue, same driver, same corner. The only thing that changed is where
the contracts live.

```
before (F027)   rung          live  killed  survived  unreached  killed/reached
                R4 proof        18       1        13          4     1/14 =  7%

after           rung          live  killed  survived  unreached  killed/reached
                R4 proof        18       5         9          4     5/14 = 36%
```

Four new kills, and **every one of them is in a crate the lift touched**:

| mutant | crate | before | after |
|---|---|---|---|
| `self-follow-guard-dropped` | `domain` | killed | killed |
| `follow-toggles` | `store` | survived | **killed** |
| `orphan-author-accepted` | `store` | survived | **killed** |
| `tick-advances-by-two` | `clock` | survived | **killed** |
| `tick-goes-backwards` | `clock` | survived | **killed** |

Verus's own lines, one of each:

```
R4 FAILED: verification results:: 17 verified, 1 errors over 5 of 5 verify-enabled crate(s)
R4 PASSED: verification results:: 32 verified, 0 errors over 5 of 5 verify-enabled crate(s)
```

`domain` was the only crate whose contracts were on shipped functions before,
and `self-follow-guard-dropped` was the only kill. That is not a coincidence and
F027 said so at the time: *"a Rust R4 kill means 'a clause on the shipped
function broke' only in `crates/domain`, and nowhere else."* The lift moved
`clock` and `store` into the same position, and the catalogue immediately found
four defects there that the proof had been blind to.

## What this prices

F012 named the repair in one sentence and nobody had done it. F041 did it and
was careful to claim nothing about cells, because a cell is a `calibrate`
verdict and no verdict had been run. This is that verdict.

**The twin structure was costing four of the fourteen reachable mutants — 29
percentage points of this corner's kill rate.** Before, a reader could believe
the Rust corner's 7% said something about Verus, or about Rust, or about how
hard the defects are. It said none of those things. It measured a decision about
where to write the contracts, and that decision was reversible in an evening.

This is the sharpest available answer to the question the whole rig exists to
ask. A verification layer's measured value is not a property of the verifier. It
is a property of how the obligations were attached to the code, and that is an
engineering choice that can be made badly and then repaired.

## What it does not say

- **No R5 cell moves**, for the reason F041 already gave: `calibrate` has no
  Verus R5 driver. Discharging an obligation and filling a cell remain different
  things.
- **`crates/ids` gained five obligations and still kills nothing.**
  `id-first-is-two` edits `crates/ids/src/lib.rs`, the crate that went from zero
  obligations to five, and it still comes back `survived?`. The contracts there
  constrain the counter's own transition (F8: ids strictly increase) and the
  mutant changes the *starting* value, which no clause names. More obligations
  is not the same as more coverage, and this is the cell that shows it.
- **The four unreached are unchanged** and are the `crates/server` mutants: the
  trusted transport shim, outside the verification perimeter by construction,
  in no denominator (F022).
- **Nine mutants still survive.** `crates/service` is the remaining twin holdout
  (F041), and five of the nine survivors edit it.

## Reproduce

```
go run ./tools/cmd/calibrate -impls rust -rungs R4 -out evidence/runs/calibration/rust-proof-postlift -resume
```

Journal, verdict lines and console in that directory. The run was interrupted by
a container restart at cell 5 and resumed from its journal, which is why the
wall figure (1942 s) covers only the cells this invocation ran.

Gates, before the sweep, both on the lifted tree:

```
verify PASSED: every anchor matches one site; every mutant compiles     (18/18)
probe  PASSED: every mutant answers some request differently from the original  (18/18 live, 0 equivalent)
R0     PASSED: every step matches S_obs byte-for-byte
```

The first two matter more than usual here. The lift changed roughly 600 lines
across three crates and **broke four of the eighteen mutant anchors**, which
`go test ./tools/...` does not check. Left unrepaired, those four defects would
have dropped silently out of every Rust rate at R0, R1, R2 and R4 — including
two of the four new kills this finding is about.
