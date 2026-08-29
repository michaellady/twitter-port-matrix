---
type: insight
domain: [engineering]
tags: [canary, gates, validation, polarity, evidence]
scope: global
source_session: T-20260827-175331-us-009-ac1-infra
created: 2026-08-27
confidence: high
relates_to:  # these targets are internal and not published here
  - personal/policies/hq-validation-evidence-binds-primary-inputs.md
  - workspace/insights/global/sandbox-profile-installation-is-not-enforcement-proof.md
---

# Canary gates need runner inversion

## Insight

A bad canary establishes that a detector can observe one known violation, but it
does not establish the gate's definition of success. A runner can still report
success after no violation is detected if it treats execution completion, an
empty finding set, or a default status as a pass.

Substituting the matched clean fixture through the identical runner tests that
second semantic boundary. The gate must fail because it caught nothing. Together
the bad-canary and clean-substitution results demonstrate both detection and the
requirement that a detection is necessary for the gate to pass.

## Context

Use this for enforcement, scanner, and policy canaries where an apparently
green test might otherwise mean only that the runner executed. The paired proof
distinguishes an operative control from a decorative negative fixture.

## Example

A dependency scanner accepts a deliberately unsafe crate as its bad canary and
reports the expected finding. Running the same gate with the clean crate in
that slot must return a failing `no qualifying finding` result, not `PASS`.
