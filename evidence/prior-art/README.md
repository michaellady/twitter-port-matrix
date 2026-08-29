# Prior art — and a knowledge-transfer failure

These nine notes were written by the **WebSocket port project** (the larger
Java→Rust port this rig was built to calibrate), between 25 and 28 August 2026.
They are copied here unmodified except for one marker on `relates_to` metadata
pointing at internal policy files that are not published.

They are here because of what comparing them to `../findings/` shows.

## Several of this project's findings were already known

| this project | prior art | written |
|---|---|---|
| **F013** — negation canaries; injection cannot detect a vacuous proof | `canary-gates-need-runner-inversion` | **27 Aug** |
| **F015** — a property enforced twice cannot be measured by a single-point mutant | `enforcement-is-a-reachability-property` | **26 Aug** |
| **F008** — volume is not coverage; the alphabet bounds the reach | `corpus-invisible-boundaries-need-targeted-probes` | **28 Aug, 03:06** |
| **F010** — the reference machine inherited a host-language quirk | `reference-models-inherit-pinned-implementation-semantics` | **26 Aug** |
| **D8** — canonical byte encoding is part of the contract | `preregistration-is-an-executable-byte-contract` | 26 Aug |
| **F007/F012** — a documented blocker is a measurement with a timestamp | `dependency-gates-and-verification-toolchains-are-separate` | 28 Aug |

Every one of those predates the finding it corresponds to.

## The sharpest case

`corpus-invisible-boundaries-need-targeted-probes` was written at **03:06 on 28
August**. It contains this, verbatim:

> The same diagnostic applies after repairing an oracle or reference machine.
> If regeneration leaves the corpus byte-identical, downstream passing results
> have not observed the changed contract. The repair needs an input that
> reaches the changed decision.

At roughly **16:00 the same day**, this project fixed `S_obs` for F010,
regenerated the corpus, observed it was byte-identical, and had to work out
from first principles that all three corners could now be silently wrong while
passing 55/55.

The diagnostic existed. It was thirteen hours old. It did not reach the work.

## What that means

**This is a knowledge-transfer failure, not a knowledge gap.** The lessons were
found, written down, and correctly generalised — and then rediscovered from
scratch, expensively, in a project running on the same machine for the same
owner.

So the honest reading of `../TRANSFER-to-websocket-port.md` is not "here is
what we learned that you should apply." It is:

> You already know most of this. Here is the **measured** version, and here is
> where it was written down and still not applied.

That second half is worth more than the first. A methodology insight that
exists but is not reachable at the moment of the decision has the same value as
one that was never written — and this project is the proof, having paid full
price for four of them.

## What is genuinely new here

Not everything overlapped. These have no prior-art counterpart:

- **F016** — a verifier's headline count decomposed: 23 units → 11 empty, 11
  conditional on assumed axioms about drifted copies, **1 property actually
  proved**.
- **F014** — `"abc".equals("abc")` verifies as FALSE on JBMC 6.11.0; the
  ceiling is a tool defect, shared with the Java corner.
- **F017** — the same defect id needs different edit shapes per corner, and two
  properties sit on opposite sides of the TCB boundary in different corners.
- **F018** — a sequential reference machine cannot express concurrency, so no
  rung derived from it can score a concurrency defect at all.
- **`CALIBRATION.md`** — the numbers. The prior art establishes the *rules*;
  this project is the first to **price** them.
