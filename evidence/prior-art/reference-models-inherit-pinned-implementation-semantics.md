---
type: insight
domain: [engineering]
tags: [reference-model, compatibility, implementation, protocol, validation]
scope: global
source_session: T-20260826-170721-us-005-review-block
created: 2026-08-26
updated: 2026-08-27
confidence: high
relates_to:  # these targets are internal and not published here
  - personal/policies/hq-validation-evidence-binds-primary-inputs.md
---

# Reference models inherit pinned implementation semantics

## Insight

A protocol standard defines a space of valid behavior, but a reference model
for a pinned library has a narrower job: it must reproduce the behavior the
library actually selects within that space. Library code can normalize values,
apply defaults, or preserve historical compatibility in ways that a
standards-first reconstruction will miss.

The reliable semantic source is therefore the pinned implementation and its
tests. Comparing each mirrored branch to that source prevents a model from
being theoretically defensible yet incompatible with the system it is meant to
check.

Those semantics include the paths that are easiest to mistake for defensive
implementation details: recursive-depth and entry limits, bounded line reads,
invalid-Unicode rejection, validation performed during execution, and
output-size replacement. They determine not only whether a request fails but
when it fails and what partial-result envelope a peer receives, so they are
part of a byte-compatible adapter's public behavior.

## Context

This matters for compatibility oracles, protocol translators, and differential
test models. For example, an empty close payload may have an RFC-level
interpretation that differs from the pinned library's normalization behavior.
The same applies to an oversized JSONL request: matching a final error code is
insufficient when the reference has already emitted results for earlier steps.
