---
type: insight
domain: [engineering, operations]
tags: [enforcement, cli, reachability, controls, validation]
scope: global
source_session: T-20260826-170721-us-005-review-block
created: 2026-08-26
confidence: high
relates_to:  # these targets are internal and not published here
  - personal/policies/hq-enforcement-claims-bind-advertised-cli-path.md
  - core/policies/hq-policy-enforcement-claims-verify-wiring.md
---

# Enforcement is a reachability property

## Insight

An enforcement mechanism is more than a correct function or a stored policy
value. It exists for an operational path only when that path reaches the
mechanism before the protected action completes. Isolated tests can prove that
a budget recorder or latch behaves correctly while leaving the real CLI command
able to bypass it.

This makes control validation a reachability question: start at the command an
operator is instructed to run, follow calls to the recorder or rejection sink,
and observe the result. The strength of the operational claim must match that
evidence, not the presence of otherwise unused code.

For file-backed budget controls, reachability is not sufficient by itself: the
read, decision, and append must be one exclusive operating-system-level
transaction. Otherwise two CLI processes can both observe available budget.
Once the control rejects an invocation, the denial itself is evidence of the
boundary and must be persisted rather than lost through an early return.

## Context

Use this when reviewing resource budgets, authorization latches, policy gates,
and other safeguards that are described in runbooks or live procedures.
