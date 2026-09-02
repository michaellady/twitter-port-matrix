# F050 — A resumed `calibrate` run reports one wall column that was measured on several different machines

**Status:** measured on `evidence/runs/calibration/kotlin-proof-recovered/`,
which is the run this finding is about
**Class:** a cost-methodology trap created by `-resume`, the flag that exists so
a long sweep survives a container restart
**Effect:** the run's `wall_ms` column separates cleanly by *which container
measured it* and not by anything about the mutants. Nobody is misled yet,
because this finding is being written before the number is quoted.

## How it happened

The Kotlin R4 recovery sweep was interrupted by two container restarts. `-resume`
did its job: the per-cell journal was committed after each interruption, and the
run picked up where it stopped rather than re-measuring. Eighteen cells, three
containers, **one `results.json` with one wall column**.

The boundaries are not reconstructed from memory. They are in this branch's git
history, because the journal was committed at each interruption:

| container | cells | commit that records the boundary | survival wall times |
|---|---|---|---|
| A | `id-first-is-two` … `timeline-tiebreak-by-id-asc` | `7895ed3` | 89.4, 89.5, 91.1, 93.0, 96.1 |
| B | `follow-toggles` | `103ee21` | 67.7 |
| C | `unfollow-rejects-missing-edge` … `limit-off-by-one` | final `results.json` | 107.4, 112.3, 112.3, 113.5, 114.5, 114.7, 115.2, 117.7 |

`git show 7895ed3:evidence/runs/calibration/kotlin-proof-recovered/journal.jsonl`
reproduces the first row; `103ee21` the second.

**A and C do not overlap.** Every cell on container A is faster than every cell
on container C, by at least 11 seconds, and the fastest cell in the run is the
lone cell on container B. All fourteen produced the same verdict, so there is no
property of the mutants to explain the split — only the machine.

For scale, the original single-container run
(`evidence/runs/calibration/kotlin-proof/`) spanned 108.5–125.7 s across the same
fourteen mutants, which `CALIBRATION-kotlin-proof.md` calls "a 16% spread …
the honest width of a single JBMC wall figure here". Container A's spread is 7%
and sits entirely below that band; container C's sits inside it.

## The mistake this was one step away from

The obvious reading of the two runs side by side is that the obligation edits
made the rung faster: mean 119.7 s before, 102.5 s after, a 14% improvement, and
there is even a plausible mechanism — five `s.createUser("a")` calls removed from
the analysed obligations is five fewer `HashMap.put` calls for the SAT encoder to
unroll.

**That reading is wrong, and the data says so.** The three obligations the calls
were removed from (`o4a`, `o4b`, `o4c`) are all BLOCKED and are never run by this
rung; neither are the two canaries (`c4`, `c5`). The seven decidable obligations
that *are* run were not edited at all. There is no path from the edit to the
timing, and the split lands exactly on the restart boundaries rather than
anywhere near the edit.

The mechanism was available, the number agreed with it, and it was still not the
explanation. That is the whole trap: `-resume` makes a multi-machine measurement
look like a single-machine one, and a plausible story will be waiting to be told
about the difference.

## Why the tool is not wrong

`-resume` is correct and the journal is correct. `calibrate` records what it
measured, and a cell's wall time *was* what that cell took. Nothing is
misreported. What is missing is that `results.json` carries no marker for the
process boundary, so a reader — or a script computing a mean — cannot see that
the column is not one sample.

This is F019's shape one level out. F019 says a verifier's wall time has a wide
range on one box. This says the range across boxes is wider still and is
invisible in the artefact.

## The rule

**Costs from a resumed run are not a cost sample.** Quote its verdicts freely —
those do not depend on which machine produced them, which is exactly why the
verdict comparison in `CALIBRATION-kotlin-proof.md` is sound over this run. Do
not quote its seconds, do not average them, and do not compare them with a run
made in one window.

If a cost figure is what a sweep is for, the sweep has to finish in one window on
one box, and the write-up has to say that it did — which is what
`CALIBRATION-kotlin-proof.md` already says about the original run, and why that
run's numbers remain the ones to cite.

**A cheap thing that would help, not done here:** stamp each journalled cell with
a boot-scoped identifier, and have `calibrate` refuse to print a mean over cells
that do not share one. The exclusion would be visible in the artefact instead of
in a finding somebody has to have read.
