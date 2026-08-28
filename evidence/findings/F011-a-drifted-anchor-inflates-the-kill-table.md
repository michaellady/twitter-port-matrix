# F011 — A drifted mutant anchor is recorded as killed by every rung

**Status:** occurred three times in one session; detected each time by `mutate verify`
**Threatens:** the deliverable itself

## The mechanism

The mutation catalogue injects defects as **anchored source edits** — match this
exact text, replace it with that. When the source moves under an anchor, the
anchor no longer matches and **the mutant injects nothing**.

An un-injected mutant is byte-identical to the original. It therefore passes
nothing and fails nothing — every rung "kills" it, because there is no defect
to survive. It contributes a clean kill to every row.

So the failure mode is not a crash or a wrong number in one cell. It is a
**uniform upward bias across the entire table**, invisible in the output,
strongest exactly when the implementations are being actively improved — which
is precisely when calibration is most likely to be run.

## It is not hypothetical

Three occurrences in one working session:

| mutant | broken by |
|---|---|
| `go/timeline-tiebreak-by-id-asc` | the Gobra lane reshaping `HomeTimeline` to a preallocated buffer |
| `go/limit-off-by-one` | the same reshape |
| `go/timeline-scan-reversed` | the refinement lane's later edits to `memstore.go` |
| `go/unknown-json-fields-accepted` | **my own** F010 rewrite of `decodeStrict` |

The last one matters most: the person best placed to notice was the one who
broke it, and did not notice.

## Why it is caught

`mutate verify` checks that every anchor matches **exactly one** site and that
every mutant still compiles, and reports drift as a hard failure with the
reason stated plainly:

> A drifted anchor injects nothing. Until it is repointed, that mutant would be
> recorded as killed by every rung, inflating the whole table.

Uniqueness matters as much as existence. An anchor matching two sites would
inject into whichever the tool visits first, which is a different defect from
the one the catalogue describes.

## The rule

**`mutate verify` must pass immediately before any calibration sweep, and the
sweep must refuse to start otherwise.** A kill table produced over drifted
anchors is not partially right — it is systematically optimistic by an unknown
margin, and nothing in it says so.

More generally: any harness that couples to source *text* rather than to source
*structure* inherits this. The coupling buys a data-driven catalogue that needs
no code change to add a corner — a real benefit, demonstrated when the Java
corner appeared mid-session and required none — but it is paid for with a
staleness check that cannot be skipped.

## Consequence for scheduling

Calibration and implementation work **cannot run concurrently**. Three lanes
editing `impls/` while a sweep measured it would produce a table describing no
version of the code that ever existed. The sweep is a barrier: let the tree
settle, run `mutate verify`, then measure.

This is the one place in this project where the parallel-agent pattern does not
apply, and the reason is not conflict — the lanes were in disjoint directories
throughout — but that measurement requires a still subject.
