---
type: insight
domain: [engineering]
tags: [preregistration, protocol, wire-format, determinism, fixtures]
scope: global
source_session: T-20260826-170819-us-008-review-block
created: 2026-08-26
confidence: high
relates_to:  # these targets are internal and not published here
  - personal/policies/hq-preregistration-freezes-every-wire-byte.md
---

# Preregistration is an executable byte contract

## Insight

Preregistration is stronger than naming a protocol shape. It commits every
wire-visible choice required to reproduce the fixture, including values that
are generated inside a frame encoder. A masking flag without a frozen
masking-key derivation still permits multiple byte sequences, so it cannot
serve as a complete preregistered case.

Executable derivations close this gap because they let validation derive the
same bytes rather than merely inspect a compatible-looking message. Ordering,
keys, nonces, and padding all belong to the contract when they affect the
transmitted payload.

## Context

Use this for test vectors, protocol fixtures, reproducible benchmarks, and
security-sensitive exchanges whose evaluation depends on exact wire bytes.
