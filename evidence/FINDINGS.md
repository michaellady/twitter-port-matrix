# What fifteen findings have in common

An index, and an argument. The findings are in `evidence/findings/`; this file
is for the patterns that only appear when you read them together — which is
also the part that transfers to `verified-java-to-rust-port`.

---

## The index

| | finding | one line |
|---|---|---|
| F001 | self-fulfilling clock oracle | the conformance harness set the clock to the expected answer before asserting on it |
| F002 | tweet-id origin mismatch | model starts ids at 0, both implementations at 1; nothing compared them |
| F003 | unspecified precedence diverges | `follow(eve,eve)` — two implementations, two answers, both refine the model |
| F004 | sort-free design deletes an obligation | F2 becomes a consequence of the data structure, not a claim about a sort |
| F005 | derived properties need enforced premises | the monotonicity lemma had two premises and nothing checked either |
| F006 | siblings share a blind spot | Go and Rust agreed on 30 of their 39 shared divergences from `S_obs` |
| F007 | same change, different payoff | −6 trusted shims in Go, 0 in Rust — and the −6 was counted on unparseable code |
| F008 | volume is not coverage | 100,000 requests found nothing; widening the alphabet found it at step 8 |
| F009 | a rung cannot kill what it cannot reach | a shipped defect survived R0 because no input reached it |
| F010 | the reference machine inherited a Go-ism | `encoding/json` case-folding became cross-language contract |
| F011 | a drifted anchor inflates the kill table | an un-injected mutant is "killed" by every rung |
| F012 | R5 as written is unstatable | the decode boundary is outside every verification perimeter |
| F013 | six obligations verified over unreachable code | one undischargeable check made everything downstream vacuous |
| F014 | JBMC cannot compare strings | `"abc".equals("abc")` verifies as FALSE |
| F015 | redundant enforcement blinds mutation | F4 is guarded twice, so removing either guard tests nothing |

---

## Pattern 1 — a count that looks like evidence

Four findings are the same mistake wearing different clothes: **a number was
produced without the thing that would make it mean something.**

| | the count | why it meant nothing |
|---|---|---|
| F007 | 6 trusted shims deleted | grepped from a package Gobra could not parse |
| F011 | mutants killed | the anchors had drifted, so nothing was injected |
| F013 | 6 obligations VERIFIED | the code they described was unreachable |
| F015 | 1 obligation discharged | the branch it described is dead on every real path |

Each of these passes a plausibility check. Each is the kind of number that
appears in a status report. And each is produced by a measurement that never
touched the thing being measured.

**The rule.** Before quoting a count, ask what would have to be true for it to
be wrong, and check that instead. "133 obligations verified" is not a fact
about the code until you know the verifier could parse it, could reach the
obligations, and that the obligations describe live paths.

---

## Pattern 2 — the blind spot is never visible from inside the gate

| finding | what was too narrow | what found it |
|---|---|---|
| F001 | the oracle could not falsify a field | replaying the corpus against an independent machine |
| F006 | the oracle was a sibling | comparing both against something neither produced |
| F008 | the input alphabet | a hand-written cross-check while building a third corner |
| F009 | the corpus | injecting a defect and watching which rungs slept |
| F010 | the reference machine itself | two implementations disagreeing with each other |
| F013 | the reachable code | asserting the *negation* rather than injecting a bug |
| F015 | the mutation granularity | a probe refusing to score a mutant that changed nothing |

Not one was found by the gate reporting green. **Every single one was found by
stepping outside it** — and in five of seven cases, by something built for an
entirely different purpose.

**The rule.** A green gate tells you nothing about what it cannot see, and it
will never tell you. Budget for the outside check: a second oracle, a wider
alphabet, an injected defect, a negation. The cheapest of these is injecting
defects and observing which rungs sleep through them, which is the argument
for the calibration table being the deliverable rather than a by-product.

---

## Pattern 3 — falsifiability has two forms and they are not interchangeable

The project's founding discipline was "no gate is trusted until it has been
shown to fail," and F001 is what it was built for. It was not enough.

- An **injection canary** asks *if I break the code, does the gate notice?*
- A **negation canary** asks *if I assert the opposite, can the tool refute it?*

Against a vacuous proof the first is ill-posed — the broken statement is
downstream of the infeasible point too, so the verifier reports VERIFIED for
the broken version as well, and the canary passes having shown nothing. Only
the second exposes it, because a claim and its negation *both* verifying is the
unique signature of an unreachable obligation (F013).

**The rule.** Injection proves a gate can detect defects. Negation proves an
obligation is reachable. Every proved obligation needs both.

---

## Pattern 4 — the ceiling is set by tool coverage of the standard library

Every corner is limited, and not one is limited by its language.

| corner | limited by |
|---|---|
| Go | Gobra: no string indexing in the ghost language; no `delete` builtin; `range` over a map fails |
| Rust | Verus: no `vstd` model for `RwLock`; proofs on hand-written twins |
| Java | JBMC: `String.equals` is broken; OpenJML unprobed |
| Kotlin | JBMC, same defect; no deductive verifier exists |

Three of four are **standard-library** gaps — strings, locks, collections —
not language expressiveness. F007 predicted the shape and got the reason
wrong; F012 and F014 confirmed it twice more.

**The rule for the port work.** A restructuring justified by "it discharged an
obligation in the source language" carries no guarantee in the target. Re-cost
it against the target verifier's own gaps, and expect those gaps to be as
mundane as whether the tool parses `delete` or believes `"abc".equals("abc")`.

---

## Pattern 5 — a documented blocker ages badly and nothing re-tests it

Five recorded reasons-something-cannot-be-done were false on inspection
(F012): Go's F1 "cannot be expressed"; the service layer "cannot compose
`LockP()`"; the ids framing; Rust's "no `vstd::hash_set` model exists" (it
ships four); and `home_timeline` needing a sort spec, when there had been no
sort for weeks.

Two more were mine — F004's shim count, and the R5 claim in `ASSURANCE.md`.

Every one was written honestly, was true when written, and was then inherited
by every later reader as a fact about the world.

**The rule.** A blocker is a measurement with a timestamp. Re-run it before
building on it, and especially before citing it as a reason not to try.

---

## What this says to the WebSocket port

The specific transfer, stated plainly:

1. **Audit the corpus alphabet, separately from its pass rate.** "Autobahn
   7.3.2, 0 FAILED across 247" and "100,000 requests, no mismatch" are the same
   kind of number, and neither speaks to a construct the corpus cannot emit
   (F008).
2. **The Java oracle is structurally stronger than this project's was.** F006
   is a warning about *sibling* differential testing; the WebSocket port's
   oracle is the pinned jar itself. That advantage is real and worth stating —
   but it does not protect against F008 or F009, which are about inputs.
3. **Do not count `rust_identity_verified` rows.** F007, F011, F013 and F015
   are four different ways a verification count can be produced without
   verification happening. The blocked deductive rung there should be assessed
   by asking what the obligations *reach*, not how many there are.
4. **Re-test the recorded blockers before treating them as settled.** Five of
   five checked here were stale.
5. **Expect the ceiling to be a library gap.** If the Rust side is to carry
   deductive proof over state behind a lock, that is blocked today for a reason
   that has nothing to do with the port and will not be fixed by porting harder
   (F012).
