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


## Second occurrence, and what it changes

The anchors drifted **again** between the first repointing and the sweep — four
this time, not three, with `go/follow-precedence-flipped` newly broken by the
refinement lane's edits to `service.go`.

That is twice in one session, and it settles a question the first occurrence
left open: this is not an artefact of unusually invasive work. **Any change to
a verified implementation is likely to move at least one anchor**, because the
anchors sit exactly where the interesting logic is — validation order, the
timeline scan, the decode gate — which is also where proof work and contract
fixes land.

The repointing was made more robust rather than merely redone:

| mutant | old anchor | new anchor |
|---|---|---|
| `follow-precedence-flipped` | 9 lines spanning the whole guard | 2 lines |
| `timeline-scan-reversed` | the full loop header | 1 line |
| `timeline-tiebreak-by-id-asc` | 3 lines incl. a ghost annotation | 2 lines, no annotations |
| `unknown-json-fields-accepted` | 2 lines inside one gate | two 1–2 line edits, one per gate |

Two rules fell out of doing it:

**Anchor on the shortest text that is still unique.** The nine-line anchor
broke because a comment was inserted *inside* it; the two-line replacement
survives anything that does not touch those two statements. Uniqueness is
checked mechanically before writing, so shortening is safe to push until it
fails.

**Never anchor on a ghost annotation.** `// @ fold acc(s.LockP())` lines move
whenever proof work happens, which is precisely when the implementation is
being changed by someone who is not thinking about the mutation catalogue.
Anchor on executable statements only.

The `unknown-json-fields-accepted` case is the instructive one. Its single gate
became two after F010 — a raw-key case-sensitivity pass *and*
`DisallowUnknownFields` — so the one-line edit no longer expressed the defect
it names. Repointing it to a single site would have produced a mutant that
compiles, verifies, and silently under-injects: the raw-key check would still
reject unknown fields, so R0 would kill it for the wrong reason. It now carries
two edits, one per gate.

**A mutant can rot into a weaker version of itself without ever failing
`verify`.** Anchor validity and defect fidelity are different properties, and
only the first is checked mechanically.
