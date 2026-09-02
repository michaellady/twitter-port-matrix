# F038 — The R4 and R5 columns finally disagree, and the disagreement is about reach

**Status:** measured, Go corner, 2 mutants x 2 rungs, 4 cells
**Class:** closes the question F028 left explicitly open
**Supersedes:** F028's "**mutants where R4 reach ≠ R5 reach: 0**" — that count was
correct for `tools/cmd/mutate/mutants.json` and is corrected in place there

## What F028 left open

F028 measured R4 and R5 over all 18 catalogue mutants and found **zero
disagreements**: same nine kills, same five survivals, same four unreached. It
then said why the agreement proved less than it looked like. R4's perimeter is
the five Gobra-verified packages; R5's is the four files carrying a refinement
clause site; exactly one file separates them, `internal/dom/dom.go`; and no
mutant in the catalogue is confined to that file. The columns had never been
given a chance to differ.

This is that chance, taken.

## The experiment

`evidence/experiments/r4-r5-separation/manifest.json` is a **scratch** catalogue,
deliberately not `tools/cmd/mutate/mutants.json`: those ids are shared across all
four corners so the kill table can compare a defect port-to-port, and a Go-only
id would shift every published denominator to settle a question about one rung
pair. It holds two mutants confined to `internal/dom/dom.go` and to nothing else:

- `handle-alphabet-widened` — `ValidHandle` accepts the byte range `A..z`, so
  `"Alice"` registers
- `text-control-chars-accepted` — `ValidText` drops the control-character
  guard, so a NUL in a tweet body posts

Both break a loop invariant that is **the only place its property is proved**.
Neither property can be restated in a postcondition: doing so needs a pure ghost
function that indexes a string, which Gobra rejects, and `dom.go`'s own comment
says so at the site. They are therefore obligations R4 can see and for which the
refinement clause list has no clause at all.

### Both gates, and both gates shown refutable first

```
anchors: 2/2 match exactly one site
compile: 2/2 build clean

verify PASSED: every anchor matches one site; every mutant compiles
```
```
live: 2/2   no observable change: 0

probe PASSED: every mutant answers some request differently from the original
```

A passing gate is worth nothing until it has been shown it can fail (standing
rule 2). Three canaries in
`evidence/experiments/r4-r5-separation/canaries/`, one per arm, each quoted from
the tool's own output:

| canary | arm | the tool's own words |
|---|---|---|
| `canary-anchor-never-matches` | verify / anchor | `anchors: 0/1 match exactly one site` … `verify FAILED: 1 mutant(s): go/canary-anchor-never-matches` |
| `canary-does-not-compile` | verify / compile | `compile: 0/1 build clean` … `verify FAILED: 1 mutant(s): go/canary-does-not-compile` |
| `canary-observationally-identical` | probe | `verdict   NO OBSERVABLE CHANGE in 537 requests` … `probe FAILED: [go/canary-observationally-identical]` |

Worth recording because it is the trap standing rule 1 names: both `verify`
canaries printed `exit status 1` into a pipeline whose own exit code was `0`.
The verdict was read from the tool's sentence, never from `$?`.

## The result

```
go run ./tools/cmd/calibrate -manifest evidence/experiments/r4-r5-separation/manifest.json \
    -impls go -rungs R4,R5 -out evidence/runs/calibration/dom-separation -resume
```

Window `2026-09-02T20:02:49Z .. 2026-09-02T20:07:30Z`. Every verdict line, verbatim
from `evidence/runs/calibration/dom-separation/journal.jsonl`:

```
go/handle-alphabet-widened      R4  killed      75.2s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m15.2s]
go/handle-alphabet-widened      R5  unreached   70.5s  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m10.5s]
go/text-control-chars-accepted  R4  killed      63.3s  R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m3.3s]
go/text-control-chars-accepted  R5  unreached   63.4s  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m3.4s]
```

**The columns differ on both cells.** This is the first R4/R5 disagreement in
the project.

The kill table at that window, with only these two mutants in the manifest:

```
rung             live  killed  survived  unreached  equiv  killed/reached     killed/live     wall
R4 proof            2       2         0          0      0      2/2 = 100%      2/2 = 100%     139s
R5 refinement       2       0         0          2      0       0/0 = n/a        0/2 = 0%     134s
```

**That two-row table is no longer what the run directory prints.** F039 added a
third mutant to the same scratch manifest and re-ran with `-resume`, which
reused these four cells unchanged (`journal holds 4 reusable cell(s) and 2
probe(s)`) and appended two more. `evidence/runs/calibration/dom-separation/report.txt`
now says:

```
rung             live  killed  survived  unreached  equiv  killed/reached     killed/live     wall
R4 proof            3       3         0          0      0      3/3 = 100%      3/3 = 100%     203s
R5 refinement       3       0         1          2      0        0/1 = 0%        0/3 = 0%     197s

mutant                             R4          R5
go/handle-alphabet-widened         kill        unreached
go/text-control-chars-accepted     kill        unreached
go/clock-now-off-by-one            kill        SURVIVED
```

The two cells this finding is about are the first two rows. The four verdict
lines quoted above are reproducible from the journal exactly as printed.

## What it does and does not establish

**The separation is by REACH, not by verdict.** `internal/dom/` is in
`gobraVerified`; `internal/dom/dom.go` is not in `r5Files`. A skeptic is right
to say that `r5Files` is a hand-maintained list, so of course a `dom.go`-only
mutant lands outside it. That half of the result is bookkeeping.

**The content is that R4 actually kills.** `R4 FAILED: Gobra has found 1
error(s)` on both. The wider perimeter is not decorative: it catches a real,
observable defect — one that registers `"Alice"`, and one that posts a NUL —
that the narrower perimeter has no obligation for. That is the claim F028 could
not make, and it is now measured rather than argued.

**A second mechanism, which does not consult `r5Files`, agrees.** `gobra
r5verify` never reads `r5Files`. It parses the clause spans and member spans out
of *the mutant tree under test*, joins Gobra's errors against
`spec/refinement/clause-sites.json`, and decides from that. Run by hand on the
first mutant's applied tree:

```
R5 sites: 30 of 42 clause(s) in clause-sites.json carry a Gobra postcondition; 47 site(s) located in this tree
Gobra has found 1 error(s)   [1m5.5s]
  internal/dom/dom.go:206 (*invalidHandleError).Error member carries no R5 clause: Loop invariant might not be preserved.
R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m5.5s]
```

Line 206 is invariant 3 of `ValidHandle` — the exact obligation the manifest
predicted would break, named by Gobra itself. So the file-level reach rule and
the per-error attribution engine reach the same answer by different routes.

They are **not fully independent**, and the record should say so: both descend
from `spec/refinement/clause-sites.json`, and `TestR5FilesMatchSites` keeps
`r5Files` derived from it. What the second route adds is that the answer is not
an artifact of *file granularity* — the attribution engine looked at the actual
failing obligation, in the actual mutant tree, and found no refinement clause on
it. If someone believes `dom.go` should carry a refinement clause, the finding
to write is about `clause-sites.json`, not about either consumer of it.

(The member name in that line, `(*invalidHandleError).Error`, is wrong — line
206 is inside `ValidHandle`. That is F040, and it changes no verdict here
because neither candidate member carries a refinement clause.)

**`r5Files` is not stale.** The third possible outcome — R5 KILLED, meaning
`dom.go` does carry a refinement clause and the map is out of date — did not
happen, and the direct check agrees: of the 47 clause sites in
`clause-sites.json`, 21 are in `service.go`, 22 in `memstore.go`, 3 in `ids.go`,
1 in `clock.go`, and **0 in `dom.go`**.

## The rate is 2 cells, and that is the whole of it

`R4 2/2 = 100%` is two cells. The tool's own report says the right thing about
it and it is repeated here rather than paraphrased:

> R4 killed EVERYTHING (2/2). Worth reading as a statement about the catalogue
> as much as the rung: a mutant set this rung finds trivial cannot distinguish
> it from a stronger one.

> R5 killed NOTHING (0/2 live mutants). A rung that never fires has not been
> shown to be able to fire; check it against a known-bad canary before reading
> this row as evidence about the mutants.

Both readings are correct and neither is a problem for this experiment, because
the experiment was built to make R5 *not* fire on defects it has no clause for.
F028's canary B (`ensures false` on `clock.Tick`, which carries R5 clause 36)
is the standing demonstration that the row can fire. This is not a rate for
either rung; it is an existence proof that the columns are not the same column.

## What follows

- **F028's open question is closed, and its zero is corrected in place.** "R4
  and R5 disagree on 0 of 18" remains true of the shipped catalogue and is now
  annotated with the reason it is a fact about the catalogue.
- **Do not merge the R5 row into R4.** F028 already argued this from the canary;
  it is now a measurement.
- **Do not report the pair as two rungs' worth of assurance either.** Nothing
  here changes that they are one Gobra invocation read twice. The two rows
  differ in *what they claim*, not in how much work was done.
- **The reach separation is the weaker of the two separations available.** The
  stronger one — a defect inside a clause-carrying file, on an obligation no
  refinement clause covers, so the columns differ by VERDICT — is F039, and it
  was constructed and measured in the same experiment. It answers the skeptic
  above outright: `clock.go` **is** in `r5Files`, so no list excuses the pass.

## Where

- `evidence/experiments/r4-r5-separation/manifest.json` — the scratch catalogue
- `evidence/experiments/r4-r5-separation/canaries/` — the three gate refutations
- `evidence/runs/calibration/dom-separation/` — journal, results, report
- `tools/cmd/calibrate/rungs.go` — `gobraVerified`, `r5Files`
- `tools/cmd/gobra/r5rung.go` — the attribution engine, which reads the tree, not the list
