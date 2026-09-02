# F021 — The vacuity audit fails exactly where the obligations are strongest

**Status:** measured while running the first negation-canary sweep over the Go corner.
**All eight undecided clauses were decided on 2026-09-02 and none is vacuous** —
see "What closed it" below, and
[F029](F029-the-audit-was-undecidable-as-spelled-not-as-asked.md), which
corrects the rule this finding drew from them.
**Class:** a limit of the audit, not of the proof — and one that lands on the
clauses most worth auditing

## The result

91 functional clauses, each negated in turn, 12-minute budget per canary:

```
91 clauses: 83 refutable, 0 VACUOUS, 8 timed out, 0 ill-formed
audited 83 REFUTABLE verdicts: 83 backed by an error inside the clause's
own member, 0 backed only by an error elsewhere.
```

**All eight timeouts are on one member, `(*MemStore).HomeTimeline`.** Nothing
elsewhere in the five packages came close: the other 83 canaries took between
10 s and 188 s, median 16 s. The eight took their full 722 s each, twice.

## It is the member, not the canary

The first reading — that the quantified canaries are hard for Z3 — is wrong,
and the corrected sweep shows why. Among the eight are three *framing* clauses:

```
memstore.go:542  !(s.AbsUsers()   == old(s.AbsUsers()))      TIMEOUT  722s
memstore.go:543  !(s.AbsFollows() == old(s.AbsFollows()))    TIMEOUT  722s
memstore.go:544  !(s.AbsLogLen()  == old(s.AbsLogLen()))     TIMEOUT  723s
```

The same three negations are refuted in 10–60 s on every other member that
carries them — `PutUser`, `HasUser`, `PutFollow`, `DeleteFollow`, `Follows`,
`PutTweet`. And `!(len(out) <= limit)` timed out too. The canary is trivial;
the method is not. **Any change to `HomeTimeline`'s contract sends its proof
past the budget**, and the per-member reachability probe agrees: `HomeTimeline`
was one of the three members whose `ensures false` probe did not return a
verdict. The other two are `Replace` and `isMonotoneLog`, the second of which
`Replace` calls. The reachability run reported `isMonotoneLog` as an *error*
after 128 s: Silicon threw an exception from inside `viper.silicon.rules.executor`
and Gobra then printed `did not terminate` for the member. Reproduced by hand
with a 600 s budget it did not throw, and ran out the clock instead. Either
way the member returned no verdict, and the way Gobra reports that is worth
quoting because it points the wrong way:

```
The verification of package .../internal/store - store got terminated after 600 seconds
The verification of member .../store.isMonotoneLog([]dom.Tweet) did not terminate
Gobra has found 0 error(s)
The verification of 1 members timed out
```

**A timed-out package reports zero errors.** Anything that reads the error
count without reading the lines above it scores a hung proof as a pass. The
tool reads Gobra's wording (`did not terminate`, `got terminated after`,
`members timed out`) and never the bare word "timeout", which also appears in
the stack trace Gobra prints for a malformed argument; on the first run the
exception trace reached the parser before the `did not terminate` line did,
and it failed safe by reporting no verdict at all.

One clause on the member did get through — F2's ordering clause, `:528`, was
refuted in the corrected run after timing out in the first. So the boundary is
not sharp; it is a proof that sits at the edge of what the solver finishes,
and a perturbation of any kind tips it over.

## Why this is the interesting case

`HomeTimeline` is where the store's strongest claims live: F1 visibility, F2
ordering, the D10 cursor bound, no-fabrication and no-loss — five of the
nineteen store clauses in R5, and the ones `ASSURANCE.md` has leaned on
hardest. Its body is the only loop in the store with a real invariant, and its
postconditions are the only ones with nested quantifiers and an existential.

That is not a coincidence. **The cost of proving a clause and the cost of
checking that the proof is not empty rise together.** A trivially-satisfied
postcondition on a straight-line method is cheap to prove and cheap to
negate; a load-bearing one on a loop with a completeness invariant is
expensive to prove and — because the canary re-verifies the whole method —
just as expensive to negate. So the audit reaches the clauses that least need
it and runs out at the ones that most do.

## What can and cannot be said

- Nothing here says these eight clauses are vacuous. Every other clause in
  the corner is refutable and every other member's exit is reachable; the
  Kotlin mechanism (F013) has no counterpart anywhere it could be checked.
- Nothing here says they are *not* vacuous either. A green package with the
  clause present is exactly the evidence F013 showed is compatible with an
  obligation nothing reaches. They are **unaudited**, and `gobra r5` reports
  them as a separate status rather than folding them into either neighbour.
- R5 clause 14 (F2) is verified; clauses 15–18 (F1, D10, no-fabrication,
  no-loss) are unaudited. The service-layer `HomeTimeline` clauses, which
  restate F1/F2/D10 one layer up, all refuted in under a minute.

*Superseded 2026-09-02: clauses 15–18 are now VERIFIED, on a refuted negation
with a VACUOUS control on the same member. Nothing above about the sweep as it
then stood is withdrawn — the clauses really were unaudited, and reporting them
as a third status rather than folding them into either neighbour is what made
it possible to go back and decide them.*

## What closed it — added 2026-09-02

All three levers named below were tried, in this order. Full measurement and
the corrected rule are in
[F029](F029-the-audit-was-undecidable-as-spelled-not-as-asked.md); this is the
part that belongs here, because this finding said the levers were untried and
that sentence is no longer true.

| lever | canary shape | budget | result | wall |
|---|---|---|---|---|
| **1.** `--parallelizeBranches` | default — all nine postconditions, one negated | 12 min | **no verdict** | 723 s |
| **2.** a budget an order of magnitude larger | default | 45 min | **no verdict**; Gobra reported `did not terminate` and, on the next line, `0 error(s)` | 2703 s |
| **3.** a cheaper, targeted probe | one clause isolated from its eight siblings; three of the nine respelled by hand | 12 min | **8 of 8 REFUTABLE, 0 VACUOUS** | 40–709 s each |

Raw output for all three, with the load conditions each was measured under, is
in `evidence/runs/gobra/homeline-levers-20260902.txt`.

**All eight clauses are REFUTABLE. None is vacuous.** Every one carries a
control run in the same shape, on the same member, with `assume false` at the
top of the body, and every control came back VACUOUS — so the probe that
returned those verdicts is one that can see vacuity here.

The three original bullets, and what became of each:

- **A cheaper probe for this member** — *this was the one that worked*, and
  more exactly than expected. The question really does reduce to "can `out` be
  non-empty at exit"; the sweep was already asking it, as
  `forall a int :: !(0 <= a && a < len(out))`, and Gobra will not decide that
  spelling. Written out as `len(out) == 0` it decides. The hand-written
  canaries are recorded as CANARY I in `impls/go/internal/_broken/` and as an
  explicit table in `tools/cmd/gobra/clauses.go`.
- **A budget an order of magnitude larger** — *tried, and it does not help*.
  The 25-minute retry that was lost twice to container restarts has now been
  run to completion at 45 minutes. See the table.
- **`--parallelizeBranches`** — *tried, and it does not help*. It genuinely
  parallelises (two Z3 processes at ~90% CPU each, against one), and the query
  still does not come back.

## The rule

**An audit has a cost curve, and it is the same curve as the thing being
audited.** Report the coverage of a vacuity sweep by *which* clauses it
reached, not by how many, because the ones it misses are not a random sample.

**The first sentence is wrong, and F029 measures why.** The second stands. The
clean package proves all nine of these postconditions in 42 s, so the audit was
never paying the proof's price: it was paying for re-verifying eight clauses it
was not asking about, in a spelling of the ninth that the solver could not
decide. Both of those are properties of the instrument. Neither is a property
of the obligation — and an instrument that fails on its hardest targets and
reports the failure as a fact about the targets gets the coverage story exactly
backwards at the top end.
