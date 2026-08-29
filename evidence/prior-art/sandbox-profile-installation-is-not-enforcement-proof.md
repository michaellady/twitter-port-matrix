---
type: insight
domain: [engineering, security]
tags: [sandbox, canary, enforcement, fail-closed, evidence]
scope: global
source_session: T-20260825-034032-us007-lifecycle-closeout
created: 2026-08-25
confidence: high
relates_to: []
---

# Sandbox profile installation is not enforcement proof

## Insight

A platform accepting or applying a sandbox profile proves only that the profile was syntactically accepted. It does not prove that the intended file, network, process, or resource boundary was enforced. Evidence must come from fixed canaries that attempt the exact forbidden action against disposable sentinel bytes and observe the expected denial.

Resource envelopes add a second prerequisite: the sandbox must actually
delegate the kernel primitives that impose them. A workload can run as uid 0
yet still lack the relevant capability, writable cgroup delegation, or namespace
operation. Read-only probes of those exact primitives establish whether a
resource design is enforceable before a live attempt; absent prerequisites are
a fail-closed platform constraint, not permission to quietly substitute a host
fallback or a weaker contract.

When a bounded canary attempt fails, keep the control typed blocked and remove unreachable mechanics that cannot meet the proof. Retaining a large implementation behind an unavailable promotion or enforcement gate creates a misleading pseudo-solution and increases future review surface without increasing assurance.

## Context

US-007 reached macOS `sandbox-exec` successfully, but both a coarse user-directory probe and an exact disposable sentinel remained readable. The live attempts were stopped, the unvalidated mechanics were removed, and only a small executable-digest-bound seam that fails before launch was retained.
