# F013 — Six obligations reported VERIFIED because the code was unreachable

**Status:** found and fixed while standing up the Kotlin corner
**Class:** vacuous proof — the deepest false green in this project so far

## What happened

`Store.appendTweet` used Kotlin's `log.lastOrNull()`. JBMC could not discharge
the erased checkcast the compiler emits for it, and **CBMC assumes a failed
check held for the remainder of the path.** Every statement after that point
became infeasible.

An obligation over unreachable code is vacuously true. All six store
obligations reported **VERIFIED**.

Not one of them had been checked.

## Why the usual defence missed it

This repository already treats an unfalsified gate as worthless — every rung
carries a canary, and `matrixctl` refuses a green it has not seen go red. That
discipline is what caught F001, and it is why R0 has a mutation sweep.

It was not enough here, and the reason is worth being precise about.

An **injection canary** asks: *if I break the code, does the gate notice?*
Against a vacuous proof that question is ill-posed — the broken statement is
downstream of the infeasible point too, so the verifier reports VERIFIED for
the broken version as well. The canary passes, having demonstrated nothing.

A **negation canary** asks a different question: *if I assert the opposite of
what I claim, is the verifier able to refute it?* Under vacuity it is not —
both a claim and its negation verify, which is the signature of an unreachable
obligation and cannot be produced any other way.

Only the negation canaries showed the six obligations were empty.

## The fix

`log.lastOrNull()` → `log[log.size - 1]`, which is what the Java corner already
does. No erased checkcast, so no undischargeable check, so the path stays
feasible and the obligations become real. The canaries are refutable again, and
R0/R1/R2 were re-run afterwards — the corner's numbers are all post-change.

## The rule

**"VERIFIED" answers "did anything contradict this?", not "is this true?"** An
obligation nothing can reach satisfies the first trivially.

So a verifier result needs two separate falsifiability checks, and they are not
interchangeable:

| check | question | catches |
|---|---|---|
| injection canary | break the code — does the gate go red? | a gate that cannot detect defects |
| **negation canary** | assert the opposite — can the verifier refute it? | an obligation nothing reaches |

Every proved obligation in this repository should carry the second. Right now
only the Kotlin corner does, because only the Kotlin corner had a verifier
quietly lie in a way that forced the question.

## Where else this could be hiding

Unaudited, and each worth checking:

- **Go/Gobra**, 133 verified obligations. Gobra's canaries so far are
  injection-style — flip F2 to ascending, delete the monotonicity guard. Both
  went red, which proves those specific obligations are live but says nothing
  about the other 131.
- **Rust/Verus**, 23 verified. The `assert(false)` check the refinement lane
  ran is closer to a negation canary and did produce `6 verified, 1 errors`,
  so at least that file is not wholly vacuous. It does not cover the rest.
  And per F012 these obligations are about hand-written twins regardless.
- **Java**, no deductive verifier attempted yet, so nothing to audit — but the
  same trap is waiting whenever JBMC is pointed at it, since Java has the same
  erased casts that caused this.

## Two smaller notes from the same lane

The Kotlin corner reached **56/56 R0, 4/4 canaries, R1 clean over 4,000
requests, 9/9 properties** — and independently hand-checked all ten F008/F010
trap shapes over a raw socket.

It also caught F010 landing mid-build: its first replay was 54/56 because it
had copied the Java corner's case-insensitive field matching, which was correct
against `S_obs` before F010 and wrong after. A contract change mid-flight is
visible to a corner being built against it, which is an argument for landing
contract changes as their own commit rather than folded into other work.
