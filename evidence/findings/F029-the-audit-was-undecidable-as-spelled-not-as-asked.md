# F029 — the vacuity audit was undecidable as *spelled*, not as *asked*

**Status:** measured closing F021's open question on the Go corner
**Class:** a defect in the auditing instrument — not in the proof, not in the
solver, and not in the clauses it could not reach

## What F021 left open

F021 recorded eight clauses on `(*MemStore).HomeTimeline` that the negation
sweep could not decide — 722 s each, twice, no verdict — and drew the rule that
*auditing cost rises with clause strength, so the clauses most worth auditing
are the ones the auditor cannot decide.* It named three levers and said none
had been tried. All three have now been tried, in the order F021 gave them.

None of them is vacuous. All eight are REFUTABLE, each with a control run on
the same member, in the same shape, that came back VACUOUS.

| lever | canary shape | budget | result | wall |
|---|---|---|---|---|
| 1. `--parallelizeBranches` | default — all nine postconditions, one negated | 12 min | **no verdict** | 723 s |
| 2. a budget an order of magnitude larger | default | 45 min | **no verdict**, Gobra's own `did not terminate` | 2703 s |
| 3. a cheaper, targeted probe | one clause isolated; three of them respelled by hand | 12 min | **8 of 8 REFUTABLE, 0 vacuous** | 40–709 s each |

Raw output for all three is in `evidence/runs/gobra/homeline-levers-20260902.txt`.

## The number that reframes the problem

The store package, exactly as shipped, with all nine of `HomeTimeline`'s
functional postconditions present and true, verifies in **42 seconds**:

```
Verifying package .../internal/store - store [18:54:20]
Gobra found no errors
Gobra has found 0 error(s)
ELAPSED_SECONDS=42
```

That 42 s is the whole `internal/store` package — every member, every clause,
these nine included. Auditing *one* of the nine ran 722 s twice, 723 s again
with every core Gobra could use, and then 2703 s before Gobra gave up on
itself: sixty-four times the cost of proving the entire package, for no answer.

F021 read that gap as the price of clause strength. It is not, and the
corrected sweep says why: the *cheapest* clause on the member —
`s.AbsUsers() == old(s.AbsUsers())`, which refutes in 43–55 s on five other
`MemStore` methods and 19–27 s on four `Service` ones — was exactly as
undecidable as the strongest. Strength was never the variable.

Two things were.

## One: the canary carried eight clauses it was not asking about

The sweep substitutes one negation and leaves the member's other
postconditions in place, so every canary re-verifies the whole member. On a
method with nine quantified postconditions that means the solver is asked to
discharge eight goals it already knows *alongside* one it cannot, and the cost
is not the sum of the two.

`gobra canary -isolate` elides the sibling postconditions. It touches nothing
else — every `requires`, every `invariant` and both `acc(...)` postconditions
stay exactly as shipped, and the elision preserves the file's line count so
Gobra's reported positions still line up with the original source.

**Why that is still the same question.** A postcondition is a *goal*, never an
assumption: deleting one cannot change the path condition Gobra arrives at the
member's exit with. An `invariant` is different — it *is* assumed on the way
round the loop, so dropping one could make an exit state feasible that was not
— and isolation never touches them. Because the exit state is unchanged, "can
Gobra prove this negation there" is the same question, asked with less to prove
beside it.

And isolation cannot manufacture a REFUTABLE. Suppose the negation `¬P` were
provable from the exit's path condition *together with* the siblings, but not
from the path condition alone. The siblings are postconditions the clean run
verified, so they hold in every reachable exit state; `P` is also verified, so
it holds there too. `P` and `¬P` both holding means there is no reachable exit
state — and then `¬P` is provable from the path condition alone, so isolation
reports VACUOUS as well. The only direction isolation can move a verdict is
VACUOUS → VACUOUS.

## Two: the derived negation was spelled in a form the solver could not decide

Isolation alone was not enough, and the clause it failed on is the one that
makes the point. `gobra canary` derives the negation of

```
forall a int :: 0 <= a && a < len(out) ==> (out[a].Author == user || …)
```

as `forall a int :: !(0 <= a && a < len(out))` — correctly, because for a
quantified implication the vacuity question is whether the *range* is ever
non-empty. That assertion says exactly `len(out) == 0`. Gobra does not decide
it in 12 minutes. It decides `len(out) == 0` written out.

That is the probe F021 asked for in as many words: *"the reachability question
for these clauses reduces to 'can `out` be non-empty at exit', which is a single
assertion Gobra could be asked directly."* It is now a small, explicit table in
`tools/cmd/gobra/clauses.go` — three entries, each carrying the arithmetic that
makes the hand-written form the same assertion as the derived one, and a test
that fails if an entry stops binding to a real clause. `gobra r5` prints the
shape a verdict was reached in next to the verdict, so a status can never be
read apart from the run that produced it.

## The control, which is what makes any of this a canary

A canary reporting REFUTABLE has shown nothing until the same canary, in the
same shape, on the same member, reports VACUOUS when the exit really is
unreachable. The sweep has always established that once, on
`(*clock.Logical).Tick`. That is not enough here: the shape changed, and it
changed *on the member whose default shape did not terminate*. So
`gobra canary -control` runs every canary a second time against a copy of its
own member with `// @ assume false` as the first statement of the body, and
fails the sweep if any of them does not come back VACUOUS.

Nine of nine came back VACUOUS, in 23–44 s each — which is also the cheapest
measurement in this finding, and the one that says most. Gobra decides *"is
this member's exit unreachable"* on `HomeTimeline` in half a minute. It is only
the other question — *"given that it is reachable, can this negation be
proved"* — that runs away. The instrument was never short of the power to see
vacuity here. It was short of the ability to finish asking.

## What this does to the R5 table

R5 clauses 15–18 — F1 visibility, the D10 cursor bound, no-fabrication and
no-loss, the four `ASSURANCE.md` leaned on hardest — move from UNAUDITED to
VERIFIED, each on Gobra's own refutation of its negation:

```
15  (*MemStore).HomeTimeline
    internal/store/memstore.go:531:9 Postcondition might not hold.    [canary shape: isolated hand-written canary]
16  (*MemStore).HomeTimeline
    internal/store/memstore.go:534:9 Postcondition might not hold.    [canary shape: isolated]
17  (*MemStore).HomeTimeline
    internal/store/memstore.go:535:9 Postcondition might not hold.    [canary shape: isolated hand-written canary]
18  (*MemStore).HomeTimeline
    internal/store/memstore.go:537:9 Postcondition might not hold.    [canary shape: isolated]
```

The corner as a whole:

```
91 clauses: 91 refutable, 0 VACUOUS, 0 timed out, 0 ill-formed
audited 91 REFUTABLE verdicts: 91 backed by an error inside the clause's own
member, 0 backed only by an error elsewhere. (0 results were not REFUTABLE.)

42 clauses: 30 VERIFIED, 12 UNATTEMPTED
```

**This matters beyond four table rows.** F028 — the Go R4+R5 mutant sweep,
landing separately; these figures are its own report, not a re-measurement —
records that all nine of its R4 kills fall in a member carrying a refinement
clause: four on the clause line itself, and five elsewhere in the member, on
`HomeTimeline`'s loop-invariant path. So five of its nine R4/R5 agreements are
produced by attributing a kill to the *member* rather than to the clause line. That widening was resting on
four clauses nothing had audited. Had any of the four come back vacuous, those
five agreements would have been attributing kills to obligations that were
empty. They did not, so the widening stands — but it stood on an unchecked
assumption until now, and that is the sort of thing worth being able to say in
either direction before it is load-bearing.

## The two levers that did nothing, and why that is worth recording

A lever that does nothing is a measurement, and both of these close something
F021 could only hypothesise about.

**`--parallelizeBranches` genuinely parallelises and genuinely does not help.**
Two Z3 processes at ~90% CPU each, against one for every other run in this
work; the query still does not come back inside 12 minutes. Whatever this is,
it is not a shortage of cores.

**A 3.75× budget does not help either, and Gobra says so in its own words.**
This is the run F021 records as started twice and lost to container restarts
both times, so whether the proof "finishes at all under perturbation or
diverges" was genuinely unknown. It diverges:

```
The verification of package .../internal/store - store got terminated after 2700 seconds
The verification of member .../store.*MemStore.HomeTimeline(string, int, int64) did not terminate
The verification of 1 members timed out
Gobra has found 0 error(s)
```

Read the last two lines together. **Zero errors, on a run that verified
nothing** — and in a negation sweep, "no errors" is what VACUOUS looks like. A
driver that read the count without the wording above it would have scored this
run as *the clause is vacuous*, which is the worst available answer: the false
green of F013, arrived at by a different route, on the four clauses in the
corner where it would have mattered most. `tools/cmd/gobra` reads the wording,
which is why it reported no verdict instead.

**And the budget boundary is noisy, not just distant.** Clause
`memstore.go:527` — `len(out) <= limit`, the simplest clause on the member —
was run three times in the same isolated shape and gave REFUTABLE at 137 s,
TIMEOUT at 723 s, and REFUTABLE at 106 s. The three framing clauses, which are
the *same* assertion about three different abstraction axes, took 40 s, 138 s
and 709 s. Two refutations decide 527 and a run that produced no verdict does
not un-decide it — but a member whose median audit cost sits near the budget
will have its verdicts decided by noise. That is a second, independent argument
for isolation: it does not only make the median affordable, it moves it far
enough below the budget that the answer stops depending on the weather.

## The rule

F021's rule was **"an audit has a cost curve, and it is the same curve as the
thing being audited."** That is now measured, and it is wrong in a specific and
useful way. The audit's cost curve was not the proof's. It was the *instrument's*:

**A negation canary that re-verifies the whole member prices the audit at the
member's cost, not the clause's — and a negation that is derived rather than
written prices it at whatever the derivation happens to spell.** Neither is a
property of the obligation being audited. Both look exactly like one.

So when a vacuity audit cannot decide a clause, the first question is not "is
this clause too strong to audit" — it is **"am I asking it one question, and am
I asking it in the cheapest form of that question?"** On this member the answer
to both was no, and the eight clauses that looked like the limit of what a
deductive auditor can reach were nothing of the kind.

The corollary is the uncomfortable one. F021's undecided clauses were reported
honestly as UNAUDITED, which was right. But an instrument that fails on its
hardest targets and reports that failure as a property of the targets will
produce a coverage story that is exactly wrong at the top end — and the top end
is the part anyone reading an assurance case cares about.
