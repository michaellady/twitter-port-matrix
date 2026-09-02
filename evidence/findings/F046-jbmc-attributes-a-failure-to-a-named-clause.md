# F046 — JBMC can name which refinement clause failed, so R5 is not a Go-only rung

**Status:** found by running JBMC and reading its goal lines, then closed by
building the rung the answer licensed
**Class:** a capability of the rig that was assumed absent and had not been
measured

## The question

`MATRIX.md` records R5 as the weakest column in the matrix: all twelve cells
capped, because Gobra on Go was the only R5 rung and no ordered pair has Go at
both ends. Whether that is a ceiling or a backlog item turns on one empirical
question about a second corner, and for the Kotlin corner it is:

> Does JBMC's output let a failing obligation be attributed to a **named**
> refinement clause, the way `gobra r5verify` attributes a failing Gobra
> postcondition to a clause by line?

The Go rung's whole join rests on one property of Gobra's output — it reports a
failing postcondition at the postcondition's own line — so the question for
JBMC was whether anything of that shape exists in its.

## The answer, verbatim

It does, and it carries one field more. From a run of
`twitterport.verification.Refinement.c13_logPrefixNeverRewritten` on the
pristine tree (`evidence/runs/kotlin-r5-gate/08-raw-jbmc-goal-lines.txt`):

```
[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.1] line 122 assertion at file twitterport/verification/Refinement.kt line 122 function java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V bytecode-index 40: SUCCESS
[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.2] line 123 assertion at file twitterport/verification/Refinement.kt line 123 function java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V bytecode-index 62: SUCCESS
[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.3] line 124 assertion at file twitterport/verification/Refinement.kt line 124 function java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V bytecode-index 84: SUCCESS
```

Each `assert` in the source is one goal, and each goal names the **entry point**,
the **assertion index**, the **file** and the **line**. Gobra's join has the
line; JBMC's has the line and the enclosing obligation. So per-clause
attribution is not merely possible on this corner, it is finer than on the one
that already had it. On the mutant `tick-advances-by-two` the rung prints:

```
  Refinement.kt:133 c36_tickAdvancesByExactlyOne       R5 clause 36 FAILED: s.clock() == before + 1L

R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member); 2 clause obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [38s]
```

## The half of the question that was nearly the wrong answer

Attribution was the easy half. The harder half was whether any Kotlin
obligation **is** a refinement clause, and read against `Obligations.kt` — the
R4 set — the answer is **no**, in a way worth stating because it was almost
mistaken for a yes:

| R4 obligation | what it states | an S_obs refinement clause? |
|---|---|---|
| `o1a`, `o1b`, `o1c` | `parseInt64`'s accept set | **no.** The decoder, which `OBLIGATION.md` §2 puts outside both corners' perimeters. The Go corner's own `r5Files` excludes `internal/dom` for the same reason |
| `o3a`, `o3b`, `o3c` | appended ids increase, `createdAt` does not decrease | **no.** The *premise* of clause 14's F2 ordering, not the clause |
| `o5c` | `follow("EVE","eve")` is `invalid_handle` | **no.** A response-code precedence, which `obligations.json` does not enumerate as a clause on any corner |
| `o2*`, `o4*`, `o5a/b/d` | blocked by F014 | — |

So a rung pointed at `Obligations.kt` would have reported R5 numbers over a set
containing no refinement clause at all: R4 wearing R5's name, which is the exact
failure F028 already recorded once. What was actually missing on this corner was
not an attribution mechanism. It was **an abstraction function**.

## What made the rung possible: three of four abs axes are decidable

`OBLIGATION.md` §4 states `abs` per axis. Adding those projections to `Store`
and measuring each one against JBMC:

| axis | projection | JBMC |
|---|---|---|
| log | `absLogLen`, `absLogIdAt`, `absLogCreatedAtAt`, `absLogAuthorAt` | **decidable.** Claims verify, negations refuted |
| users | `absUserCount`, `absHasUser` | **decidable** — see the caveat below |
| clock | `clock()` | **decidable** |
| follows | `absFollows` | **undecidable** |

The follows axis fails with a signature worth distinguishing from vacuity:

```
[java::...R5Probe.p7_followsAxisCommutes:()V.assertion.2] line 75 ...: FAILURE
[java::...R5ProbeCanaries.n7_followsAxisDoesNotGain:()V.assertion.1] line 142 ...: FAILURE
```

The claim FAILS **and its negation FAILS**. That is not F013's vacuity — that
signature is both *verifying* — it is plain nondeterminism:
`HashSet.contains(Edge)` builds a fresh `Edge` whose reference test cannot
succeed, so it falls through to `Edge.equals` → `String.equals` → the F014
defect, and JBMC returns an unconstrained boolean. Either answer would be an
artefact, so clauses 7 and 9 are in **neither the numerator nor the
denominator** (F022's accounting), and the verdict sentence quotes the count.

The users axis is decidable for a reason that is a property of the ground
instances rather than of the clause: `HashMap.containsKey` reaches
`String.equals` only after `(k = e.key) == key` fails, and handles written as
literals are interned, so it never fails. Over a nondeterministic handle this
axis would be blocked exactly like the follows axis. That is recorded in
`clause-sites-kotlin.json` next to the sites, because a reader of the cell has
to reach it.

## What was built

- `impls/kotlin/src/twitterport/store/Store.kt` — the abs projections (F045
  records what they cost).
- `impls/kotlin/verification/Refinement.kt` — seven clause obligations and five
  negation canaries, kept apart from `Obligations.kt` so R4 and R5 cannot
  become the same row by accident.
- `spec/refinement/clause-sites-kotlin.json` — the join, **keyed on
  `obligations.json`'s clause numbers**, not a second numbering. "R5 clause 11"
  is the same sentence on both corners, which is the only thing that makes a
  `go <- kotlin` cell mean anything.
- `tools/cmd/jbmc r5verify` — the rung, shaped after `gobra r5verify`.
- the R5 driver for `kotlin` in `tools/cmd/calibrate/rungs.go`, added to the
  existing R5 entry rather than as a second one.

Clean run: **5 of 5 decidable clause obligations verified, covering R5 clauses
1, 2, 11, 13 and 36, every one refutable in this tree; 2 blocked, in no
denominator.** All five negation canaries refuted, so none of the five is
vacuous. The gate is `evidence/runs/kotlin-r5-gate/`.

## What this cell is NOT

The Go corner's cell. Every clause here is a **ground instance inside an
unwinding bound** — `c02` holds for the handle `"a"`, where Gobra's clause 2
holds for every handle. R5 on this corner also reaches one production file
(`Store.kt`) against R4's three, so a service-layer mutant is fair game for R4
and *unreached* by R5. And a mutant whose only refinement effect is on the
blocked follows axis reads as an R5 survival: `follow-toggles` is the worked
instance, kept in the gate directory rather than left to be discovered.

## What it changes about the R5 column

R5 is **not** structurally a Go-only rung. It was capped on this corner because
nobody had written an abstraction function for it, not because JBMC could not
read one. With `kotlin` on the R5 rung, the ordered pairs `go <- kotlin` and
`kotlin <- go` are no longer capped by the absence of a second R5 corner —
what they are capped by now is the weaker of their two ends, which is this
corner's bounded, ground-instance, three-axis version. That is a claim about
strength; the previous one was a claim about existence, and it was wrong.
