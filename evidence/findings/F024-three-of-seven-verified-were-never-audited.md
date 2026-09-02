# F024 — Three of the Kotlin corner's seven VERIFIED obligations had never been audited

**Status:** found while making the bounded proof rung a `calibrate` rung
**Class:** an audit gap in the corner that *discovered* vacuity — and a
measurement of what the audit costs on a bounded checker versus a deductive one

## The headline number was 4, not 7

F014 records the Kotlin corner's bounded rung as **7 VERIFIED, 0 REFUTED, 8
BLOCKED**, and every word of that is true. What it does not say — because
nothing in the run said it — is that only **four** of the seven had ever been
shown refutable.

`Canaries.kt` shipped nine negation canaries. Laid against the seven decidable
obligations they cover:

| obligation | negation canary | audited? |
|---|---|---|
| `o1c_emptyAndBareSignRejected` | `c1_bareSignIsANumber` | yes |
| `o3a_idsStrictlyIncrease` | `c2_idsDoNotIncrease`, `c7_storeIsReachable` | yes |
| `o3b_createdAtNonDecreasing` | `c3_clockCanDecrease` | yes |
| `o5c_syntaxBeatsExistence` | `c8_serviceIsReachable`, `c9_syntaxDoesNotBeatExistence` | yes |
| `o1a_oneCharAcceptSet` | — | **no** |
| `o1b_twoCharAcceptSet` | — | **no** |
| `o3c_lemmaOverThreeAppends` | — | **no** |

The other four canaries (`c4`, `c5`, `c6`) guard obligations that are BLOCKED,
so they carry no information either way.

The driver was not negligent about this. It checked, loudly, that every canary
it *had* was refuted, and it failed the run when one was not. What it never
asked was whether an obligation had a canary at all — so three claims passed
through the one gate in this repository built to catch exactly that, because
the gate was indexed by canary rather than by claim.

## Why it matters that this is the Kotlin corner

F013 is the finding that six Kotlin store obligations reported VERIFIED because
an undischargeable erased checkcast made everything downstream infeasible. It
is the deepest false green this project has recorded, and it was found *here*,
by *these* canaries. The corner that learned the lesson still had three claims
the lesson had never been applied to.

That is not irony, it is the ordinary shape of the failure: a negation canary is
written when an obligation is *suspected*, and the three unaudited ones —
`parseInt64` over all one- and two-character strings, and the monotonicity lemma
at the third append — were the ones nobody suspected. A gap that only appears
where nobody is looking cannot be closed by looking harder; it has to be closed
by a check that enumerates claims rather than canaries.

## What was actually true

Three canaries were added — `c10_nonDigitIsANumber`, `c11_signThenSignIsANumber`
and `c12_thirdAppendDoesNotIncrease` — and all three are refuted on the clean
tree. So the three claims were **true, and unearned**: no verdict changes, and
the corner's number is still 7. The finding is not that the corner was wrong.
It is that nothing in the run could have told the difference.

`c12` is worth a sentence on its own. `c2` negates the id lemma at the *second*
append; `c12` negates it at the *third*, which is where `o3c` actually reaches.
An undischargeable check appearing only at the third append would have left
`o3c` vacuous while `c2` stayed refutable — the F013 mechanism one iteration
further down the same loop.

## The cross-corner measurement: what the audit costs

The rung now runs the vacuity audit **on every tree it judges**, not once on the
clean corner. That is affordable here and is not affordable on the Go corner,
and the gap is large enough to be a design fact rather than a tuning detail.

| | Go / Gobra | Kotlin / JBMC |
|---|---|---|
| one obligation, verified | ~60–110 s for the whole tree | 3–13 s per obligation |
| its negation | often **does not terminate** — two canaries over `(*MemStore).HomeTimeline` ran 35 and 43 minutes at 2% CPU before being killed (F021) | 3–7 s, same order as the claim |
| audit per mutant tree | not attempted; F022 records that R4 "does not re-audit vacuity per mutant" | 9 canary runs, ~35 s, inside the same run as the 7 claims |

The reason is structural, not incidental. Negating a deductive obligation hands
the solver a *harder* query than the obligation itself — F021 measured exactly
that: vacuity-checking cost rises with clause strength, so the clauses most
worth auditing are the ones the auditor cannot decide. Negating a **bounded**
obligation hands the checker the same finite unrolling with one literal
flipped, so the negation costs what the claim costs.

**So the weaker rung can afford the stronger audit.** A bounded proof says less
per run than a deductive one, and it can say it about every tree; a deductive
proof says more and can only be audited once, on the clean tree, with the
audit's own coverage capped by what the solver will decide.

That is a real ordering result and it cuts against the intuition the rung
ladder encodes. R4-by-BMC is a weaker claim than R4-by-deduction, and it is a
**better measured** claim.

## What the rung does with all this

`tools/cmd/jbmc verify` reports three outcomes per obligation, not two:

- **VERIFIED** — JBMC's own assertion goals for this obligation are all
  SUCCESS *and* every negation canary naming it was refuted **in this tree**.
- **REFUTED** — at least one of its own assertion goals is FAILURE. One is
  enough to kill the tree.
- **BLOCKED or VACUOUS** — a recorded JBMC 6.11.0 defect (F014), no assertion
  goal of its own, an unrefuted canary, or a timeout. In neither the numerator
  nor the denominator, exactly as F022 puts a shim-only mutant in neither.

A tree where a claim goes vacuous is **UNDECIDED**: the tool prints no verdict
sentence at all and `calibrate` records an error cell, never a survival.

The last part is the one that needed a rung to notice. On the Go corner the
vacuity audit is prior to the table. Here it is *inside* it — because a mutant
can introduce the undischargeable construct itself, and then the obligation
that would have killed it reports SUCCESS having checked nothing. Scoring that
as a survival would read a hole in the contract where there is a hole in the
solver.

## The two canaries, verbatim

Both are the same instrument pointed at a deliberately broken copy of the
corner. Logs in `evidence/runs/calibration/`.

**Injection** — one line of `Dom.parseInt64` changed so a bare sign parses as
zero (`kotlin-r4-canary-injection.log`):

```
o1a_oneCharAcceptSet         REFUTED   0 ok, 1 failed   22.6s   VERIFICATION FAILED
o1c_emptyAndBareSignRejected REFUTED   2 ok, 1 failed    3.6s   VERIFICATION FAILED

R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s) (2 of 11 own assertion
goals FAILURE): o1a_oneCharAcceptSet, o1c_emptyAndBareSignRejected; 8
obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no
denominator   [1m33.1s]
```

**Vacuity** — `Store.appendTweet` reverted to `log.lastOrNull()`, the exact
F013 defect (`kotlin-r4-canary-vacuity.log`). This is the canary that matters,
because an injection canary cannot see vacuity at all: the injected defect is
downstream of the infeasible point too.

```
o3a_idsStrictlyIncrease      VERIFIED  1 ok, 0 failed    3.2s   VERIFICATION FAILED (library goals failed in twitterport.store.Store)
o3b_createdAtNonDecreasing   VERIFIED  2 ok, 0 failed    3.4s   VERIFICATION FAILED (...)
o3c_lemmaOverThreeAppends    VERIFIED  2 ok, 0 failed    3.3s   VERIFICATION FAILED (...)
...
c2_idsDoNotIncrease          VERIFIED  1 ok, 0 failed    3.5s   VERIFICATION FAILED (...)
c3_clockCanDecrease          VERIFIED  1 ok, 0 failed    3.5s   VERIFICATION FAILED (...)
c12_thirdAppendDoesNotIncrease VERIFIED 1 ok, 0 failed   3.5s   VERIFICATION FAILED (...)

decidable 7   VERIFIED 4   REFUTED 0   VACUOUS 3   UNDECIDED 0

R4 UNDECIDED: 3 of 7 decidable obligation(s) could not be read
(c2_idsDoNotIncrease guards o3a_idsStrictlyIncrease and was NOT refuted
(VERIFIED); under vacuity a claim and its negation both verify, so
o3a_idsStrictlyIncrease decides nothing (F013)); nothing was decided about
this tree   [1m37.5s]
```

Read the three obligation rows on their own and this tree is a clean proof:
every own assertion goal SUCCEEDS. Read the canary rows and the claim and its
negation both hold, which is impossible unless neither is reachable. The rung
prints **no verdict sentence at all** and `calibrate` records an error cell —
not a survival.

One incidental signal worth writing down: JBMC's own top-level line on those
runs is `VERIFICATION FAILED` while every one of the obligation's assertions
SUCCEEDS, because the undischargeable check is a non-assertion goal inside
`twitterport.store.Store`. **A failing non-assertion goal in the corner's own
class is a vacuity smell**, and it is visible one run earlier than the canary
sweep. It is reported in the rung's "library artefacts" column, but it is not
what the verdict is read from — a smell is not a decision procedure, and a
run can be vacuous without producing one.

## The other half: what the seven can actually kill

Making the rung a `calibrate` rung answers a question the F014 headline does
not: **the seven decidable obligations, run against the mutant catalogue, kill
almost nothing** — and the reasons are three distinct ones that a single "kill
rate" would blend into mush.

1. **F014 takes the families that matter.** The 8 blocked obligations are
   exactly the timeline (`o4a`–`o4c`) and validation-ordering (`o2*`, `o5a`,
   `o5b`, `o5d`) ones. That is the whole *ordering*, *pagination* and most of
   the *precedence* families of the catalogue — 8 of the 18 Kotlin mutants —
   left with no decidable obligation over them at all. They are not killed and
   they are not survivors of a weak contract: the obligation exists and the
   tool cannot read it.

2. **The premises are enforced, so breaking them yields vacuity, not
   refutation.** `Store.appendTweet` *throws* when an append would break id or
   clock monotonicity — which is F005's whole point, the premise enforced at
   the mutation site rather than assumed. So `tick-goes-backwards` does not
   refute `o3b`; it makes the second append throw, which makes the assertion
   after it unreachable, which makes `o3b` **and its negation** both verify.
   The rung reports UNDECIDED and `calibrate` records an error cell.

   This is F015 (redundant enforcement blinds mutation testing) arriving at
   the proof rung, and it is sharper here: the *same* design decision that
   makes the property true at runtime is what makes it unprovable-against on a
   mutant. A guard that throws converts every downstream obligation from a
   test of the property into a test of the guard.

3. **What is left is relational, so a constant slips through.** `o3a`/`o3c`
   say ids *increase*; nothing says where they start, so `id-first-is-two`
   passes — R4 PASSED, 7 of 7, and the mutant is live at request 0 of every
   input source. That is F023 reproduced on a second corner with a different
   verifier, which is worth having: it was not a Gobra artefact.

The rung is not thereby useless. It is *measured*, which is what the table is
for, and the measurement says the money is in fixing the tool (or replacing
it), not in writing more Kotlin obligations. An obligation written today over
the timeline would land in the blocked column.

## The rule

**An audit indexed by the checks you wrote cannot find the check you did not
write.** Enumerate the claims and ask each one what audits it; a claim that
answers "nothing" is not a claim yet.

And, for anyone pricing a verification layer: **the cost of proving something
and the cost of showing the proof is not vacuous are different costs, and they
move in opposite directions.** Budget the second one separately, or the rung
that is easiest to make green will be the one whose greens mean least.
