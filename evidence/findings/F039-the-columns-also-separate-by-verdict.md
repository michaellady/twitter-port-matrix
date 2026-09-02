# F039 — The R4 and R5 columns also separate by VERDICT, not only by reach

**Status:** measured, Go corner, 1 mutant x 2 rungs, inside the same scratch
experiment as F038
**Class:** the stronger of the two available separations of the proof and
refinement rows

## Why the reach separation was not enough

F038 gave R4 and R5 their first disagreement: two mutants confined to
`internal/dom/dom.go`, killed by R4, **unreached** by R5. The honest reading was
recorded there — the separation is by reach, and R5's reach is decided by
`r5Files`, a hand-maintained list of the four files carrying refinement clause
sites. A skeptic can say: of course a `dom.go`-only mutant is unreached; you
looked up a list and the list said no.

The stronger claim needs a defect that R5's perimeter **does** contain, that R5
then declines to kill. That is this finding.

## The construction

`clock-now-off-by-one`, added to
`evidence/experiments/r4-r5-separation/manifest.json` under a new `attribution`
family:

```
(*clock.Logical).Now returns l.now + 1
```

Chosen because of where it sits, not because of what it breaks:

- **`internal/clock/clock.go` is in `r5Files`.** R5's perimeter covers this file,
  so `covered` is true and the cell lands in R5's `killed/reached` denominator.
  There is no list to appeal to.
- **`clock.go` carries exactly one refinement clause site, and it is on
  `(*Logical).Tick`** (clause 36), not on `(*Logical).Now`. The mutant does not
  touch `Tick`.
- **`(*Logical).Now` carries a real functional postcondition** —
  `unfolding acc(l.LockP()) in result == l.now` — which the mutant makes false.
  So R4 has something to fail on and R5 has no clause on it.
- **It is observable.** Every tweet's `created_at` comes out one ahead.

Both gates, on the three-mutant manifest:

```
anchors: 3/3 match exactly one site
compile: 3/3 build clean

verify PASSED: every anchor matches one site; every mutant compiles
```
```
  witness   DIFFERS at request 1
            request   POST /tweets {"author":"alice","text":"hello"}
            original  201 {"id":1,"author":"alice","text":"hello","created_at":0}
            mutant    201 {"id":1,"author":"alice","text":"hello","created_at":1}
            S_obs     agrees with the original; the mutant diverges from the spec
  verdict   LIVE -- changes observable behaviour (witness)

probe PASSED: every mutant answers some request differently from the original
```

Both gates were shown refutable in F038, on the same manifest directory, by the
three canaries in `evidence/experiments/r4-r5-separation/canaries/`.

## The result

Verbatim from `evidence/runs/calibration/dom-separation/journal.jsonl`:

```
go/clock-now-off-by-one          R4 killed       64.3s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m4.3s]
go/clock-now-off-by-one          R5 survived     63.2s  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m3.2s]
```

The full three-mutant table:

```
rung             live  killed  survived  unreached  equiv  killed/reached     killed/live     wall
R4 proof            3       3         0          0      0      3/3 = 100%      3/3 = 100%     203s
R5 refinement       3       0         1          2      0        0/1 = 0%        0/3 = 0%     197s

mutant                             R4          R5
go/handle-alphabet-widened         kill        unreached
go/text-control-chars-accepted     kill        unreached
go/clock-now-off-by-one            kill        SURVIVED
```

R5's `killed/reached` denominator is no longer empty. It is `0/1 = 0%`: one
defect inside R5's own perimeter, which R5 saw and passed.

## The mechanism, from the tool's own mouth

`gobra r5verify` run by hand on the applied tree:

```
R5 sites: 30 of 42 clause(s) in clause-sites.json carry a Gobra postcondition; 47 site(s) located in this tree
Gobra has found 1 error(s)   [1m4.4s]
  internal/clock/clock.go:52 (*Logical).Now      not an R5 clause: unfolding acc(l.LockP()) in result == l.now
R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m4.4s]
```

Line 52 is `Now`'s own `ensures`. The error sits **on a clause line**, so the
attribution is taken by `clauseAt` — an exact span match on a clause parsed from
the mutant tree — and never reaches the member-span lookup. F040's span defect
is therefore not involved in this cell at all, and `clock.go` has no overlapping
member spans in any case.

So this is R5's attribution working exactly as designed and saying, correctly,
*a postcondition failed and it was not a refinement obligation*.

## What it establishes

**The two columns are not the same column, and the difference is not
bookkeeping.** One Gobra invocation, one failing obligation, two different
verdicts: R4 kills, R5 passes. No list was consulted to get there — the
distinction was computed from the failing clause's own text and location in the
tree under test.

**It is the exact shape F028 predicted and could not exhibit.** F028's argument
for keeping the R5 row was a canary in two directions plus the claim that the
rows *would* differ given the right defect. This is that defect, measured.

**It also shows what R5 costs you.** `clock-now-off-by-one` is a real,
observable, spec-violating defect — off-by-one timestamps on every tweet — and
R5 passes it. That is not a bug in R5; it is what "refinement obligation"
means. A reader who takes the R5 column as a strength ordering above R4 has it
backwards: R5 is a **narrower** question, and its 0/1 here is the price of the
narrowness. F028's rule stands and is sharpened: R4 and R5 are one invocation
read twice, and the R5 reading is strictly weaker.

**One cell is one cell.** `0/1 = 0%` is a single measurement and the tool prints
its denominator for exactly that reason (F008). This is an existence proof, not
a rate.

## What follows

- **The R5 row's meaning is now demonstrated in all three ways it can differ
  from R4**: unreached (F038), survived (this), and killed (F028's nine).
- **The `attribution` family is a template worth extending.** Any member inside
  a clause-carrying file that carries a functional postcondition and no clause
  site produces a cell of this shape. `internal/store/memstore.go` has
  `(*MemStore).Replace` and `isMonotoneLog`; `internal/ids/ids.go` has `New` and
  `NewAt`; `internal/service/service.go` has `(*Service).HasUser`, `Tick` and
  `Now`. Three trusted shims in the same position — `appendTweet`,
  `deleteFollowEdge`, `sortLogByID` — are **not** candidates: they are
  `// @ trusted`, so Gobra never checks their bodies and R4 would survive them
  too (F022's point, from the other side).
- **These ids stay out of `tools/cmd/mutate/mutants.json`.** Same reason as
  F038's two: the catalogue's ids are shared across four corners so a defect can
  be compared port-to-port, and a Go-only id would shift every published
  denominator.

## Where

- `evidence/experiments/r4-r5-separation/manifest.json` — the `attribution` family
- `evidence/runs/calibration/dom-separation/` — journal, results, report
- `spec/refinement/clause-sites.json` — 1 site in `clock.go`, on `(*Logical).Tick`
- `tools/cmd/gobra/r5rung.go` — `clauseAt`, the path this cell took
- `evidence/findings/F038-*.md` — the reach separation this strengthens
