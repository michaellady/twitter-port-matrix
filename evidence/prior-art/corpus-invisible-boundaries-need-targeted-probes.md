---
type: insight
domain: [engineering]
tags: [corpus, differential-testing, state-machines, regression, probes]
scope: repo:verified-java-websocket-port
source_session: T-20260828-030630-differential-regression-lane-delivered
created: 2026-08-28
updated: 2026-08-28
confidence: high
relates_to:  # these targets are internal and not published here
  - workspace/insights/global/real-corpus-failures-form-an-implementation-map.md
  - workspace/insights/global/reference-models-inherit-pinned-implementation-semantics.md
---

# Corpus-invisible boundaries need targeted probes

## Insight

A complete-looking compatibility corpus can still omit a state boundary that
matters to the implementation. Differential agreement over every corpus case
then proves only the sampled surface, not that adjacent lifecycle guards are
equivalent.

Targeted probes earn their value by choosing states whose accounting or output
would change if a nearby guard were collapsed. In the observed closing-path
case, CLOSING and CLOSED accounted bytes differently while both implementations
agreed; that made a regression distinguishable even though the larger corpus
could not expose it.

The same diagnostic applies after repairing an oracle or reference machine. If
regeneration leaves the corpus byte-identical, downstream passing results have
not observed the changed contract. The repair needs an input that reaches the
new decision before the corpus can serve as regression evidence for it.

## Context

Use this after a broad corpus passes, especially around terminal states,
resource accounting, and error timing. Add a small differential probe when a
plausible refactor could preserve all existing cases while erasing a meaningful
boundary.
