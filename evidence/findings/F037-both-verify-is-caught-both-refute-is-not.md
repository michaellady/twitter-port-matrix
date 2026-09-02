# F037 — The vacuity gate catches "both verify" and is blind to "both refute"

**Status:** observed on the Java corner's clean tree; the hole is in shared
verdict logic, not in one corner
**Class:** an instrument that covers one of two symmetric failures, and a data
model that cannot express the other

## The two signatures

F013 named one: a claim and its negation **both VERIFY**. Nothing else produces
that, and what it means is that nothing reaches the claim. `tools/cmd/jbmc`
detects it — an unrefuted canary demotes the obligation it guards to VACUOUS
and the whole run to UNDECIDED, with no verdict for `calibrate` to read.

Writing the Java obligation set turned up its mirror image. A claim and its
negation **both REFUTE**:

```
assert  Dom.validHandle("alice")   -> FAILURE      (o2b_goodHandleIsValid)
assert !Dom.validHandle("alice")   -> FAILURE      (c14_goodHandleIsInvalid)

assert  Dom.validText("")          -> FAILURE      (c15_emptyTextIsValid)
assert !Dom.validText("")          -> FAILURE      (o2a_emptyIsInvalid, assertion.2)
```

Both are on the **clean tree**, today, with no mutant and no injected defect.
`o2b` and `c14` are a strict pair — the same call, the same argument, opposite
assertions — so exactly one of them can hold of any deterministic predicate.
Both failing means the predicate is nondeterministic, which here it is: F034
measures `"alice".getBytes(UTF_8)` as having neither a determined length nor
determined contents.

Both-verify says *nothing reaches the claim*. Both-refute says *the claim has
no truth value in this model*. Neither is a decision. The rung treats them
completely differently.

## What the rung does with each

| signature | what it means | what `jbmc verify` does |
|---|---|---|
| claim VERIFIED, negation VERIFIED | vacuous (F013) | demotes to VACUOUS, run UNDECIDED, no verdict, error cell |
| claim REFUTED, negation REFUTED | nondeterministic | **reports a KILL** |

The second row is not hypothetical arithmetic. `classifyOne` returns REFUTED as
soon as one own assertion goal is FAILURE; `decide()` tests for refutations
first — "A refutation is a kill and needs exactly one witness, so it is decided
first" — and `verify.go` then skips the canary sweep entirely:

```
negation canaries not run: 2 obligation(s) were refuted, which decides the tree on its own
```

A refutation decides the tree only if the refutation is a *decision*. Under
`getBytes`, it is not.

## Why nothing has gone wrong yet, and why that is not reassurance

`o2a` and `o2b` are marked `Blocked: getBytesReason`, so the rung never runs
them and no spurious kill can come from them. That is correct, and F014 already
states the reason in exactly these terms — "the defect produces a spurious
FAILURE, so an obligation blocked by it must not be counted as a KILL either".

But the `Blocked` column is a **hand-maintained declaration**, which is the
thing F030 is about: the Rust corner shipped an R4 row whose vacuity instrument
was declared rather than built, and no gate could tell. Here the same shape:
the protection against a spurious kill is a list somebody keeps up to date, and
there is no instrument that would notice if the list went stale — or if a
mutant introduced a `getBytes` call onto the path of an obligation that is
*not* on the list. The gate that exists protects a PASS. Nothing protects a
FAIL.

Remove the two `Blocked` markers and the clean Java tree reports **R4 FAILED**
with no defect in it at all. Run, not argued
(`evidence/runs/calibration/java-r4-gate/f037-false-red.log`) — the same
`impls/java` that passes 7 of 7, with `Blocked: getBytesReason` deleted from
the `o2a` and `o2b` entries and nothing else changed:

```
decidable 9   VERIFIED 7   REFUTED 2   VACUOUS 0   UNDECIDED 0

R4 FAILED: JBMC refuted 2 of 9 decidable obligation(s) (2 of 14 own assertion
goals FAILURE): o2a_emptyIsInvalid, o2b_goodHandleIsValid; 6 obligation(s)
blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [32.3s]
```

`calibrate` would read that as a kill: verdict sentence present, exit code 1,
the two halves agreeing. Every check the rung has would pass. That is the false
red the hand-maintained list is standing in front of.

## The instrument this needs, and why it cannot be written today

The obvious symmetric fix — after any refutation, run the refuted obligation's
negation canary; if that is also refuted, report UNDECIDED rather than a kill —
is **wrong as stated**, and the reason is worth more than the fix.

The canaries in this repository are two different instruments under one field:

- **Strict negations.** Same call, same arguments, opposite assertion:
  `o2b`/`c14`, `o5c`/`c9`, `c15`/`o2a`. For these, both-refuted is a
  contradiction and therefore a signal.
- **Witnesses.** A concrete instance that must be rejected, standing under a
  universally quantified claim: `o1a` quantifies over ALL one-character
  strings and `c10` fixes `parseInt64("x")`. These are not negations of each
  other, and both being refuted is ordinary and correct — it happens on the
  injection-canary tree in `java-r4-gate/canary-injection.log`, where `o1a` is
  refuted by the broken parser and `c10` is refuted because `"x"` still parses
  as nothing.

`obligation.Guards` records *which obligation* a canary names. It does not
record *which kind of canary it is*. So the check cannot be written against
today's data model without first splitting the field — and a check applied to
the witness canaries would turn the injection canary's own kill into an
UNDECIDED, which is a worse failure than the one it is fixing.

This is deliberately **not implemented here.** It changes verdict logic shared
with the Kotlin corner, whose R4 sweep is being run concurrently by another
lane, and a change that can turn a FAILED into an UNDECIDED must not land
underneath a sweep in progress. What it needs: a `Strict bool` on the canary
record, a canary sweep that runs the strict negations of refuted obligations
too, and a fourth outcome for the pair.

## The rule

**A falsifiability instrument is directional, and the direction is easy to
miss.** "Can the verifier refute the opposite?" protects a green. It says
nothing about a red. A checker that cannot decide a predicate produces a
*failing* goal, not a succeeding one, so the tool defect most likely to be
mistaken for a result is the one that looks like a kill — and a kill is the
outcome nobody re-examines, because it is the outcome everyone wants.

Ask of every rung: what would make this print FAILED when the tree is fine?
If the answer is "a list we keep up to date", that is F030 again, in the
direction nobody checked.
