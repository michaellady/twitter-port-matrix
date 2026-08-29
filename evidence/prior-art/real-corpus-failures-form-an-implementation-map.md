---
type: insight
domain: [engineering]
tags: [corpus, compatibility, verification, prioritization]
scope: repo:verified-java-websocket-port
source_session: T-20260827-165705-wiring-landed-bd8e496-wiredcore
created: 2026-08-27
confidence: high
relates_to:  # these targets are internal and not published here
  - workspace/insights/global/reference-models-inherit-pinned-implementation-semantics.md
---

# Real corpus failures form an implementation map

## Insight

A compatibility corpus becomes a useful implementation map only after it runs
through the real candidate path. Once wired, each stable failure family names a
missing behavior rather than merely a harness limitation: unsupported decoded
bytes, unimplemented send paths, and observation-count divergences can each be
assigned to the story that owns that behavior.

The baseline must remain untuned and reproducible. That makes later score
movement interpretable: a named scenario changes because its corresponding
behavior landed, not because the evaluator or fixture was adjusted to improve
the number.

This also separates implementation velocity from verification confidence. A
chain of RFC-based changes, static reviews, or self-reported test totals may
look complete without ever traversing the live reference behavior. The first
authoritative corpus or differential run can therefore reveal both whether a
claimed pass is real and which apparent standards-correctness choices diverge
from the compatibility target.

## Context

Use this when incrementally porting a protocol implementation or adapter. A
small set of genuine passes and a larger typed failure breakdown are valuable
early evidence, provided the candidate, corpus, and evaluation path are the
ones that will be used for later comparisons.
