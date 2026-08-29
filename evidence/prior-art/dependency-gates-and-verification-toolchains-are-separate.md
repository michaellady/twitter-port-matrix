---
type: insight
domain: [engineering]
tags: [dependencies, verification, toolchain, reproducibility, formal-methods]
scope: global
source_session: T-20260828-023141-e3-final-round-delivered
created: 2026-08-28
confidence: high
relates_to:  # these targets are internal and not published here
  - personal/policies/hq-dependency-gates-separate-artifact-and-verification-scope.md
  - personal/policies/hq-preregistration-binds-resolved-toolchains.md
  - workspace/insights/global/toolchain-declarations-select-but-receipts-identify.md
---

# Dependency gates and verification toolchains are separate

## Insight

A zero-dependency requirement normally describes the composition of a released
artifact: which non-path libraries are linked into it. It does not, by itself,
describe the tools permitted to establish confidence in that artifact. Model
checkers, containerized conformance suites, and pinned reference oracles can
therefore be valid external verification vehicles without changing the
artifact's dependency claim.

That distinction does not make an actual-code verification run automatic. A
digest-qualified compiler or runtime pin remains an independent, executable
constraint. Separating the axes makes the honest conclusion precise: the
verification method is allowed, while a specific environment pin may still
block its execution.

## Context

Use this distinction when interpreting dependency gates for formal methods,
conformance suites, or oracle-backed testing, and when reporting why a valid
verification plan cannot yet run.
