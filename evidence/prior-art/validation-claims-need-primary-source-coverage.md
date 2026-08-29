---
type: insight
domain: [engineering]
tags: [validation, evidence, source-correspondence, coverage, review]
scope: global
source_session: T-20260826-152436-owner-decisions-executed-project
created: 2026-08-26
updated: 2026-08-28
confidence: high
relates_to:  # these targets are internal and not published here
  - personal/policies/hq-validation-evidence-binds-primary-inputs.md
  - workspace/insights/global/evidence-before-promotion-bind-actual-inputs.md
---

# Validation claims need primary-source coverage

## Insight

Validation becomes tautological when it reads only an artifact produced by the
same workflow it is meant to check. It can confirm that an oracle is internally
well formed without establishing that the oracle still corresponds to the
source tree or that its claimed surfaces are complete.

A meaningful correspondence check creates an independent path from qualified
primary bytes through reproduction to comparison. Completeness requires the
same discipline at member level: derived resolution must cover every relevant
surface, rather than inheriting a default success state from the presence of a
row or document.

Where the acceptance surface is declaration-defined, the coverage set must
come from a systematic walk of the declarations and their public members. An
interpretation table is a useful presentation layer, but cannot establish that
the table contains every binding the declaration contract requires.

The most reliable implementation makes that walk the sole enumeration: it
materializes the test fixtures and the declared coverage result from the same
members. Separate hand-written lists create two truths that can drift together
plausibly—a test can still pass and a result record can still look complete—so
the shared enumeration turns the observed members and cardinality into the
evidence itself.

Likewise, a digest proves the identity of a committed artifact, not its
provenance. Verification has to invoke the generator from the qualified inputs
and byte-compare its output; otherwise a stale or hand-edited artifact can
remain internally consistent and digest-bound.

A digest declaration in one retained artifact about another has an additional
two-sided requirement: the verifier must reopen both the declaration and the
counterpart, then compare the recorded and freshly derived values. Refreshing
the declaration has to compute its value from the current counterpart in that
same operation. A fixture that can be recorded without re-pinning demonstrates
only that metadata can be written, not that the binding is being maintained.

That is why an additional byte pin is not automatically an independent
defense. If the sanctioned regeneration path can refresh both the artifact and
its pin from the same mistaken interpretation, it merely launders the error
into a newly consistent state. Independence comes from re-deriving the claimed
semantic value from the cited primary source or run and comparing that result
through a separate verification path.

The same distinction applies to a record that claims a live execution outcome.
Schema acceptance proves only that the record can be parsed. A meaningful
reconciler has to connect its counters to each other, resolve its digests to the
retained artifacts, and show that its status could only arise after the stated
gates passed. Without that path, the record is a well-formed assertion rather
than verification evidence.

Status-dependent result fields need the same two-layer protection. A schema
that permits a counterexample digest beside a non-detection status leaves a
second, independent assertion channel: the status can look harmless while the
result field carries an outcome it has not earned. Conditional schema pairings
make invalid combinations unrepresentable, while a typed semantic validator
keeps the rule effective when values are assembled or transformed in code.

Runtime attestation has the same end-to-end identity requirement. If each
transcript line names a jar digest, protocol version, or comparable runtime
identity, the evaluator must compare those values with the qualified execution
closure on every accepted line. A valid scenario digest does not authenticate
an unchecked runtime field, so checking it only once—or not at all—leaves a
forgeable gap in an otherwise well-formed transcript.

## Context

Use this for migration maps, generated dossiers, compliance inventories, and
other evidence artifacts whose acceptance language says that source facts are
represented or that all touched surfaces are resolved. A missing source root is
an explicit blocked condition, not a reason to make the qualification skippable.

This distinction matters most when reviewers can inspect the original
declarations directly: the validation must independently reach the same
member-level conclusion and reproduce any committed derivative.

The status/value pairing matters wherever a record uses lifecycle labels to
control promotion, remediation, or audit conclusions. Both layers are needed:
shape checks prevent accidental construction and semantic checks protect the
meaning at the validation boundary.

The same discipline applies when a receipt points to a test. A cited test is
evidence only for the behavior its setup and assertions can distinguish. For
example, an accepted-send assertion followed by a queue-capacity check may
still pass when the receiver is alive, so it does not demonstrate a typed
receiver-drop refusal. Reading the test body and running an inverted-lifecycle
decoy separates genuine behavioral coverage from a plausible but irrelevant
nearby assertion.
