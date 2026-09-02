# F033 — Two proof rungs disagree on 8 of the 12 defects both can see

**Status:** measured, Kotlin's full R4 sweep against the Go corner's
**Class:** the first disagreement between deductive/bounded rungs in this
project, and a bound on what the R4 column can be read to mean
**Effect:** the R4 column spans 64%, 7% and 0% over three corners that R0 kills
18 of 18 on

## The comparison

`evidence/CALIBRATION-go-proof.md` (Gobra, 18 mutants) against
`evidence/CALIBRATION-kotlin-proof.md` (JBMC 6.11.0, 18 mutants), over the same
18 mutant ids. Comparing only the cells where **both** rungs returned a
kill-or-survive verdict:

| mutant | go R4 (Gobra) | kotlin R4 (JBMC) | |
|---|---|---|---|
| `id-first-is-two` | SURVIVED | SURVIVED | agree |
| `follow-precedence-flipped` | SURVIVED | SURVIVED | agree |
| `unfollow-rejects-missing-edge` | SURVIVED | SURVIVED | agree |
| `created-at-frozen` | SURVIVED | SURVIVED | agree |
| `self-follow-guard-dropped` | **kill** | SURVIVED | **DISAGREE** |
| `timeline-scan-reversed` | **kill** | SURVIVED | **DISAGREE** |
| `timeline-tiebreak-by-id-asc` | **kill** | SURVIVED | **DISAGREE** |
| `follow-toggles` | **kill** | SURVIVED | **DISAGREE** |
| `orphan-author-accepted` | **kill** | SURVIVED | **DISAGREE** |
| `tick-advances-by-two` | **kill** | SURVIVED | **DISAGREE** |
| `cursor-inclusive` | **kill** | SURVIVED | **DISAGREE** |
| `limit-off-by-one` | **kill** | SURVIVED | **DISAGREE** |

**Comparable: 12. Agree: 4. Disagree: 8.** Six cells are not comparable — two
Kotlin ERROR cells (F031, F032), two mutants unreached on Go and reached on
Kotlin, two unreached on both.

All four agreements are SURVIVED/SURVIVED. The two rungs have never agreed on a
kill, because the Kotlin rung has none.

## Why this is not the same as F028

F028 recorded R4 and R5 agreeing on 18 of 18 on the Go corner, and was careful
about what that did and did not show: `gobra verify` and `gobra r5verify` issue
**the same verification of the same tree** and differ only in which question
they ask of the failures. Agreement there was close to arithmetic.

This is the opposite configuration in every respect: two different verifiers
(Gobra, JBMC), two different corners, two independently written obligation sets,
one deductive and one bounded. Nothing about the setup forces agreement, and
none appears.

The prior for this project was the behavioural columns, where all four corners
produced **identical outcome vectors on all 18 defects**. That uniformity is
what made `MATRIX.md` say every measured cell in R0/R1/R2 is the same number.
The R4 column breaks it, and this is how far it breaks: two thirds of what the
two rungs can both see.

## What the disagreement is about — and what it is not

**It is not two tools disagreeing about whether a defect is a defect.** R0 kills
18 of 18 on both corners, byte-exact. Both implementations' observable behaviour
changes under every one of these mutants. The bugs are not in dispute.

The disagreement is entirely about **which properties somebody wrote down, and
which of the written ones a tool can decide.** Split by cause, all eight
disagreements fall into two buckets:

| cause | mutants | the obligation that would have caught it | count |
|---|---|---|---|
| the property IS written on the Kotlin corner, and F014 blocks the obligation stating it | `timeline-scan-reversed`, `timeline-tiebreak-by-id-asc` | `o4c_pageIsNewestFirst` (blocked: `String.equals`) | 4 |
| " | `limit-off-by-one` | `o4a_pageRespectsLimit` (blocked: SAT exhaustion) | |
| " | `self-follow-guard-dropped` | `o5b_knownSelfFollowIsForbidden` (blocked: `getBytes`) | |
| no obligation states the property, blocked or not | `follow-toggles` (follow is idempotent), `orphan-author-accepted` (the author must exist), `tick-advances-by-two` (the clock advances by *exactly* one — `o3b`/`o3c` only pin non-decreasing), `cursor-inclusive` (every obligation calls `timelinePage` with a `null` cursor, so the mutated `t.id >= cursor` branch is dead in all of them) | — | 4 |

The first four are the sharper half. `o4a`, `o4c` and `o5b` exist, are written
against `S_obs`, and state exactly what those four mutants break. All three are
BLOCKED — two by `String.equals` being unsound in JBMC 6.11.0, one by SAT
exhaustion, one by the `getBytes` model (F014). **The eight blocked obligations
are not spread evenly over the contract; they are concentrated on precisely the
part of it this catalogue attacks.** F021 found the same concentration from the
other side: the vacuity audit fails where the obligations are strongest.

The second four are F023's shape — a rung cannot kill what no clause claims —
and `cursor-inclusive` is the cleanest instance: the obligation set does contain
three pagination obligations, and not one of them ever passes a non-null cursor,
so the mutated comparison is unreachable in every one.

## The three 14s, which are three different numbers

The R4 column now carries three measured corners, and by coincidence two of the
three denominators are literally the same integer:

| corner | rate | what sets the denominator |
|---|---|---|
| Go | 9/14 = 64% | 18 − 4 mutants confined to `internal/httpshim`, the trusted shim (F022) |
| Rust | 1/14 = 7% | 18 − 4 outside the perimeter; and 57 of 62 `ensures` clauses sit on hand-written twins, so 13 of the 14 leave a verifying twin untouched (F027, F030) |
| Kotlin | 0/14 = 0% | 18 − 2 in `httpshim` − **2 ERROR cells** (F031, F032); and 8 of 15 obligations are undecidable by a tool defect (F014) |

**Three corners, three 14s, three unrelated reasons.** Go's is a trust boundary
someone chose. Rust's is where the contracts got written. Kotlin's is two
missing measurements plus a tool defect, and it is the only one of the three
whose 14 is not `live − unreached` at all.

`MATRIX.md` already carries the `‡` mark for "the two ends' numbers are not
comparable in meaning". This finding is the evidence that the mark is needed on
every R4 cell, not just the two that had it: **an R4 cell's number is a fact
about one corner's obligation set and one tool's capabilities, and comparing two
of them ranks the obligation sets, not the ports.**

## What it means for the matrix

The weaker-end rule gives `go ← kotlin`, `kotlin ← go`, `rust ← kotlin` and
`kotlin ← rust` all the same cell, `0/14 = 0%`, because Kotlin is the weaker end
of all four pairs. The rule is right and the arithmetic is right, and a reader
who takes `0%` as "this port has no proof-level assurance" has read something
true; a reader who takes it as "Kotlin is the worst of the four corners" has
read something false — R0 kills 18 of 18 there too, and the 0% is a JBMC defect
wearing a port's clothes.

## The transferable form

Two verification rungs on two ports of one specification, both green on their
own terms, can disagree about two thirds of a shared defect catalogue **without
either being wrong**. A single "assurance level" per port cannot express that,
and neither can a column that reports a percentage without its denominator's
provenance.

For the WebSocket port specifically: `RUST_IDENTITIES_NOT_YET_RESOLVER_VERIFIED`
is a blocked deductive rung, and the tempting reading is that unblocking it buys
what the Java side's proof buys. This is a measured instance of that reading
failing. Before pricing the unblock, ask which obligations would exist
afterwards and which of those the tool can decide — the second question is what
took this corner from an expected 64%-shaped number to zero.
