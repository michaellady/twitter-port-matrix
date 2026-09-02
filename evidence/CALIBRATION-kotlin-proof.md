# CALIBRATION — the Kotlin corner's bounded proof rung, R4, all 18 mutants

The third R4 column, and the first one that kills nothing. The rung was gated on
five mutants in `evidence/runs/calibration/kotlin-r4-gate/`; this is the full
sweep, and it is what the four `pending` cells in `evidence/MATRIX.md` were
waiting on.

```
go run ./tools/cmd/calibrate -impls kotlin -rungs R4 \
    -out evidence/runs/calibration/kotlin-proof -resume
```

Window `2026-09-02T20:05:49Z .. 2026-09-02T20:51:21Z`. Raw journal, results and
console in `evidence/runs/calibration/kotlin-proof/`.

Catalogue undrifted immediately before the sweep, per the standing note that a
drifted anchor injects nothing and every rung "kills" it (F011):

```
anchors: 18/18 match exactly one site
compile: 18/18 build clean

verify PASSED: every anchor matches one site; every mutant compiles
```

---

## Caveats — read these before the numbers

**1. This rung is BOUNDED, and the Go corner's is not.** Gobra discharges an
obligation for all inputs; JBMC searches for a counterexample within
`--unwind 30 --max-nondet-string-length 3`. A JBMC VERIFIED means *no
counterexample within the bound*, which is strictly weaker than what the other
two R4 drivers report. The column header says `R4 proof`; on this corner it is
`R4 bounded`, exactly as `ASSURANCE.md`'s ceiling table says.

**2. Eight of the fifteen obligations are in no denominator.** JBMC 6.11.0
cannot compare two strings (F014), cannot model `String.getBytes(Charset)`, and
exhausts memory on a nondeterministic `limit` over a four-entry log. The eight
obligations those defects block are excluded from the numerator **and** the
denominator, for the same reason F022 excludes a shim-only mutant from both: a
question the tool cannot answer must not be scored as a kill or as a survival.
The rung prints that exclusion in every verdict sentence it produces.

**3. The catalogue and the contracts share a parent.** Both derive from
`S_obs`. The standing caveat on every table in this repository.

**4. The box was NOT shared, and that is worth saying.** Nothing else ran on
this container for the whole window. Most cost figures in this repository are
not clean — `evidence/CALIBRATION-go-proof.md` records "the 1-minute load
average was 6–12 for most of the window" and inflates its wall column by an
unmeasured amount. These figures do not have that problem. They are still one
sample each, and F019 records that a verifier's wall time has a wide range even
on a quiet box.

---

## The kill table

```
rung             live  killed  survived  unreached  equiv  killed/reached     killed/live     wall
R4 proof           16       0        14          2      0       0/14 = 0%       0/16 = 0%    2085s
```

```
  R4 proof       killed/reached 0/14 = 0%   killed/live 0/16 = 0%
                 18 cell(s) measured, 14 in the killed/reached denominator, 4 excluded
                    2 unreached    live, but the verifier reads none of the files the
                                   mutant edits, so no obligation could have covered it
                                   (F022)
                    2 error        no verdict was read; a missing measurement, never a pass
```

`calibrate` flags its own row, and the flag is correct:

```
 1. R4 killed NOTHING (0/16 live mutants). A rung that never fires has not
    been shown to be able to fire; check it against a known-bad canary before
    reading this row as evidence about the mutants.
 2. R4: 2 cell(s) errored. Those are missing measurements, not passes.
```

**That check has been made, and it passes.** Standing rule 2 says a gate not
shown to fail is not evidence, and the injection canary in
`evidence/runs/calibration/kotlin-r4-canary-injection.log` is the demonstration
— the same instrument pointed at a deliberately broken tree:

```
o1a_oneCharAcceptSet               REFUTED    0 ok, 1 failed           22.6s     VERIFICATION FAILED
o1c_emptyAndBareSignRejected       REFUTED    2 ok, 1 failed           3.6s      VERIFICATION FAILED

R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s) (2 of 11 own assertion goals FAILURE): o1a_oneCharAcceptSet, o1c_emptyAndBareSignRejected; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [1m33.1s]
```

So `0/14` is a fact about the catalogue against these obligations, not a broken
rung. **But note which obligations fired.** `o1a` and `o1c` are `parseInt64`
obligations over `src/twitterport/dom/Dom.kt`, and **no mutant in the catalogue
edits `Dom.kt`.** The rung's only demonstrated firing path runs through a file
the catalogue never touches. That is written up as **F032**.

---

## Per mutant, with the verdicts verbatim

Reach is scored per F022: the input source for a proof rung is the contract, so
*unreached* means the verifier reads none of the files the mutant edits. On this
corner the verifier's perimeter is `src/twitterport/{dom,store,service}/`;
`src/twitterport/httpshim/` is trusted transport (F004).

| mutant | files edited | R4 | reach |
|---|---|---|---|
| `id-first-is-two` | `store/Store.kt` | SURVIVED | reached |
| `id-burned-on-reject` | `store/Store.kt`, `service/Service.kt` | **ERROR** | reached, not measured |
| `self-follow-guard-dropped` | `service/Service.kt` | SURVIVED | reached |
| `follow-precedence-flipped` | `service/Service.kt` | SURVIVED | reached |
| `timeline-scan-reversed` | `store/Store.kt` | SURVIVED | reached |
| `timeline-tiebreak-by-id-asc` | `store/Store.kt` | SURVIVED | reached |
| `follow-toggles` | `store/Store.kt` | SURVIVED | reached |
| `unfollow-rejects-missing-edge` | `service/Service.kt` | SURVIVED | reached |
| `orphan-author-accepted` | `service/Service.kt` | SURVIVED | reached |
| `created-at-frozen` | `store/Store.kt` | SURVIVED | reached |
| `tick-advances-by-two` | `store/Store.kt` | SURVIVED | reached |
| `tick-goes-backwards` | `store/Store.kt` | **ERROR** | reached, not measured |
| `next-cursor-always-emitted` | `store/Store.kt` | SURVIVED | reached |
| `next-cursor-is-first-id` | `store/Store.kt` | SURVIVED | reached |
| `cursor-inclusive` | `store/Store.kt` | SURVIVED | reached |
| `limit-off-by-one` | `store/Store.kt` | SURVIVED | reached |
| `unknown-json-fields-accepted` | `httpshim/Json.kt` | unreached | **trusted shim** |
| `repeated-query-param-accepted` | `httpshim/Server.kt` | unreached | **trusted shim** |

### The survivals

All fourteen produced the **same verdict line, character for character**:

```
R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator
```

with these wall times:

```
kotlin/id-first-is-two             survived   125.7s   [2m5.7s]
kotlin/self-follow-guard-dropped   survived   120.0s   [2m0s]
kotlin/follow-precedence-flipped   survived   122.2s   [2m2.2s]
kotlin/timeline-scan-reversed      survived   119.4s   [1m59.4s]
kotlin/timeline-tiebreak-by-id-asc survived   119.7s   [1m59.7s]
kotlin/follow-toggles              survived   117.6s   [1m57.5s]
kotlin/unfollow-rejects-missing-edge survived 120.3s   [2m0.3s]
kotlin/orphan-author-accepted      survived   111.4s   [1m51.3s]
kotlin/created-at-frozen           survived   108.5s   [1m48.5s]
kotlin/tick-advances-by-two        survived   125.6s   [2m5.6s]
kotlin/next-cursor-always-emitted  survived   121.5s   [2m1.5s]
kotlin/next-cursor-is-first-id     survived   121.3s   [2m1.3s]
kotlin/cursor-inclusive            survived   119.2s   [1m59.2s]
kotlin/limit-off-by-one            survived   123.1s   [2m3.1s]
```

Range 108.5 s to 125.7 s over fourteen identical-verdict runs on an unloaded
box — a 16% spread, which is the honest width of a single JBMC wall figure here.

### The two unreached

Gobra's counterpart cells look the same: the tool ran, passed, and the mutant is
outside its perimeter, so the pass is not a survival.

```
kotlin/unknown-json-fields-accepted   R4 PASSED   [2m3.5s]
    live (reached by corpus, seed=1, seed=2, seed=3, seed=4), but the verifier reads
    none of the files this mutant edits (src/twitterport/httpshim/Json.kt)
kotlin/repeated-query-param-accepted  R4 PASSED   [2m4.9s]
    live (reached by corpus, seed=1, seed=3), but the verifier reads none of the files
    this mutant edits (src/twitterport/httpshim/Server.kt)
```

### The two errors — two different mechanisms, neither a survival

**`kotlin/id-burned-on-reject` — the tree does not build (F031).**

```
jbmc produced no R4 verdict (exit 1). Nothing was measured:
    | /tmp/calibrate-mutant-962170801/tree/verification/Canaries.kt:73:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | /tmp/calibrate-mutant-962170801/tree/verification/Obligations.kt:179:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | /tmp/calibrate-mutant-962170801/tree/verification/Obligations.kt:195:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | /tmp/calibrate-mutant-962170801/tree/verification/Obligations.kt:208:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
```

The mutant splits id allocation out of `Store.createUser`, changing its arity.
`mutate verify` cleared it because the registry build compiles `src`; the rung
compiles `src` **and** `verification`. Written up as **F031**.

> **Superseded in part.** Five of those call sites were decoration and are gone
> (F048); the cell is still an ERROR, and the error now names only the two sites
> that are the property itself. See *"Re-measured after the obligation
> decoupling"* at the end of this file for the current text and the 18-cell
> comparison.

**`kotlin/tick-goes-backwards` — the vacuity audit refused to read the tree.**

```
jbmc produced no R4 verdict (exit 1). Nothing was measured:
    | decidable 7   VERIFIED 6   REFUTED 0   VACUOUS 1   UNDECIDED 0
    | blocked   8   (recorded JBMC 6.11.0 limits; in neither the numerator nor the denominator)
    |   ! c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted (VERIFIED); under vacuity a claim and its negation both verify, so o3b_createdAtNonDecreasing decides nothing (F013)
    |
    | R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read (c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted (VERIFIED); under vacuity a claim and its negation both verify, so o3b_createdAtNonDecreasing decides nothing (F013)); nothing was decided about this tree   [2m29.5s]
```

This is the gate's ERROR cell reproduced exactly, and it is the most interesting
cell in the run. Written up as **F032**.

---

## The vacuity audit ran, and F025 has not recurred

The audit is not a separate pass over this corner; it is a precondition of the
verdict. Three facts, each checkable in the tool rather than asserted here:

1. **An obligation with no canary cannot even start a run.** `cmdVerify` calls
   `c.unguarded()` before anything else and refuses outright:
   *"corner %s claims %d obligation(s) no negation canary guards (…); per F013
   this rung will not report a verdict over them"*. F025 was three of seven
   VERIFIED obligations that no canary named. That failure mode is now
   structurally unreachable on this corner, and this sweep is 18 more runs of
   evidence that the guard holds.
2. **The canary sweep runs on every tree that is not already refuted.**
   `verify.go` runs it whenever `refuted == 0` — which is all 16 PASSED trees
   here — and skips it only when a refutation has already decided the tree, in
   which case the deciding obligation is non-vacuous by construction.
3. **The verdict sentence carries the result.** `R4 PASSED` is emitted only
   when `rep.Verified == den` *after* the canary demotion in `decide()`, and it
   says so in words: **`every one refutable in this tree`**. That clause appears
   in all 16 passing cells above.

The one tree where the audit found something is `tick-goes-backwards`, and it
did exactly what it exists to do: refused to report a verdict rather than
report a false one.

**So: no obligation was VERIFIED in this sweep without a negation canary naming
it, and none was VERIFIED while its canary went unrefuted.** F025 is not
recurring here.

---

## The result that matters: two proof rungs disagree, on 8 of 12

`evidence/CALIBRATION-go-proof.md` records the Go corner's R4 over the same 18
mutant ids. Placing the two columns side by side — and comparing only the cells
where **both** rungs actually returned a kill-or-survive verdict:

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
| `id-burned-on-reject` | SURVIVED | ERROR | not comparable |
| `tick-goes-backwards` | **kill** | ERROR | not comparable |
| `next-cursor-always-emitted` | unreached | SURVIVED | not comparable |
| `next-cursor-is-first-id` | unreached | SURVIVED | not comparable |
| `unknown-json-fields-accepted` | unreached | unreached | both outside |
| `repeated-query-param-accepted` | unreached | unreached | both outside |

**Comparable: 12. Agree: 4. Disagree: 8.** Every one of the four agreements is a
SURVIVED/SURVIVED pair; the two rungs have never once agreed on a kill, because
this one has none.

F028 found R4 and R5 agreeing on 18 of 18 and was careful to say why that was
not corroboration: the same Gobra run read two ways. This is the opposite case —
two different verifiers, two different corners, two independently written
obligation sets — and they disagree on two thirds of what they can both see.

**What the disagreement is about, and what it is not about.** It is *not* two
tools disagreeing about whether a defect is a defect. R0 kills 18 of 18 on both
corners; both corners' behaviour changes; nobody disputes the bugs. The
disagreement is about **which properties somebody wrote down, and which of those
a tool can decide** — F027's shape, arriving on a third corner. Written up as
**F033**.

---

## Why nothing was killed

Not a mystery, and not a defect in the rung. The seven decidable obligations
cover three things:

| group | obligations | what they constrain | file |
|---|---|---|---|
| 1 | `o1a`, `o1b`, `o1c` | `parseInt64`'s accept set (D10) | `dom/Dom.kt` |
| 3 | `o3a`, `o3b`, `o3c` | ids increase, `createdAt` never falls (F005's premises) | `store/Store.kt` |
| 5 | `o5c` | syntax beats existence (D6) | `service/Service.kt` |

Against that, the catalogue: **no mutant edits `Dom.kt` at all**, so Group 1 —
the group the injection canary proved can fire — is unreachable by construction.
Group 5 is one obligation about one precedence rule. That leaves Group 3, three
obligations about log monotonicity, as the only live surface, and the two
mutants that actually perturb it are the corner's two ERROR cells.

The fourteen survivals split cleanly by cause, and the split is the result:

- **Six would have been caught by an obligation that exists and is BLOCKED.**
  `timeline-scan-reversed` and `timeline-tiebreak-by-id-asc` break
  `o4c_pageIsNewestFirst`; `limit-off-by-one` breaks `o4a_pageRespectsLimit`;
  `self-follow-guard-dropped` breaks `o5b_knownSelfFollowIsForbidden`;
  `next-cursor-always-emitted` and `next-cursor-is-first-id` break
  `o4b_cursorNullMeansExhausted`. Every one of those five obligations is blocked
  by F014 — three by `String.equals`, one by SAT exhaustion, one by `getBytes`.
- **Eight are F023's shape**: no obligation states the property at all.
  `follow-toggles` (idempotence), `orphan-author-accepted` (the author must
  exist), `created-at-frozen` and `tick-advances-by-two` (`o3b`/`o3c` pin
  *non-decreasing*, which a frozen or doubled clock still satisfies),
  `id-first-is-two` (F023's own worked case), `follow-precedence-flipped`,
  `unfollow-rejects-missing-edge`, and `cursor-inclusive` — the last being the
  cleanest instance, because the obligation set contains three pagination
  obligations and **not one of them ever passes a non-null cursor**, so the
  mutated `t.id >= cursor` branch is dead in all of them.

**The blocked obligations are not randomly distributed across the contract: six
of the fourteen survivals are exactly the ones a blocked obligation would have
caught.** F021 recorded the same concentration from the other direction — the
audit fails where the obligations are strongest.

## What this does and does not license

It does **not** license "the Kotlin port is worse". The same catalogue kills
18 of 18 at R0 on this corner, byte-exact, and the corner's behavioural columns
are identical to every other corner's.

It does **not** license "bounded model checking is worthless for this". The
rung's one demonstrated firing path proved `parseInt64`'s accept set over *all*
one- and two-character strings, and F014 records that it also proved F005's
monotonicity premise — the load-bearing one for the sort-free design. Those are
real results that no other rung in this repository produces.

It **does** license saying that on this corner, this catalogue and these
obligations, **R4 buys nothing that R0 does not already buy** — and that the
reason is a tool defect (F014) rather than anything about Kotlin or about the
port. `0/14` is the number; `8 of 15 obligations undecidable by
`String.equals`' being unsound` is what the number means.

---

# Re-measured after the obligation decoupling — 18 of 18, nothing moved

The five decorative `Store.createUser` calls F035 identified were removed from
`Obligations.kt` (`o4a`, `o4b`, `o4c`) and `Canaries.kt` (`c4`, `c5`), and two
were kept in `Refinement.kt` because they *are* R5 clause 2 (F048). That edits
the tree the rung compiles, so the whole sweep was re-run rather than reasoned
about:

```
go run ./tools/cmd/calibrate -impls kotlin -rungs R4 \
    -out evidence/runs/calibration/kotlin-proof-recovered -resume
```

**Every one of the 18 cells is unchanged** — same outcome, and for the sixteen
that produced one, the same verdict line character for character once the
trailing wall-time bracket is removed:

| | before (`kotlin-proof`) | after (`kotlin-proof-recovered`) |
|---|---|---|
| killed | 0 | 0 |
| survived | 14 | 14 |
| unreached | 2 | 2 |
| error | 2 | 2 |
| killed/reached | `0/14 = 0%` | `0/14 = 0%` |

That is the result the edits had to produce. The obligations were decoupled from
a signature; they were not allowed to change what the rung measures, and a moved
cell would have meant they had. `tick-goes-backwards`, the F032 vacuity ERROR,
reproduces byte for byte including its `VACUOUS 1` line.

## What did change: the error text on `id-burned-on-reject`

Before — four lines the compiler chose to print, across three files:

```
    | .../verification/Canaries.kt:73:22: error: no value passed for parameter 'id'.
    | .../verification/Obligations.kt:179:22: error: no value passed for parameter 'id'.
    | .../verification/Obligations.kt:195:22: error: no value passed for parameter 'id'.
    | .../verification/Obligations.kt:208:22: error: no value passed for parameter 'id'.
```

After — the two sites that carry R5 clause 2, and nothing else:

```
    | jbmc: kotlinc failed: exit status 1
    | .../verification/Refinement.kt:104:22: error: no value passed for parameter 'id'.
    |         s.createUser("a")
    |                      ^^^^
    | .../verification/Refinement.kt:197:22: error: no value passed for parameter 'id'.
```

The rung's own output now agrees with the F031 gate, which reports
`compile: 17/18 build clean` and names the same file twice. The cell is still a
missing measurement — but it is now held by exactly one obligation, and that
obligation is the one that cannot be moved without weakening it (F048).

## The cell is recoverable, and reclaiming it would not improve the rate

Measured, not assumed
(`evidence/runs/calibration/kotlin-r4-refinement-removed-probe.log`): the same
mutant tree with `verification/Refinement.kt` — the file the R4 rung does not
read — removed, and nothing else changed:

```
decidable 7   VERIFIED 7   REFUTED 0   VACUOUS 0   UNDECIDED 0

R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator
```

So the R4 verdict on this mutant is a **SURVIVAL**, and the row it would produce
is `0/15 = 0%`: the same zero over a denominator one larger. The obligation that
would have killed it is `o5d_rejectionBurnsNoId` — "a rejected registration burns
no id", exactly this defect — and it is one of the eight JBMC blocks ("SAT
checker ran out of memory"). The cell is missing for a build reason and would be
uninformative for a solver reason, which are two different failures that happen
to land on one mutant.

That probe is **not a cell**: its tree is not the tree `calibrate`'s guard
hashed. It answers what the rung would say, not what the rung said. F049 records
the shared-compile-unit mechanism and names the repair, which is not made here.

## Do not read this run's wall column

`kotlin-proof`'s cost figures were taken in one window on a box with nothing else
on it, and that is why they are quotable. This run was interrupted by two
container restarts and resumed from its journal, so its wall column was measured
on **three different machines** and separates by machine rather than by mutant:
container A's five survivals span 89.4–96.1 s and container C's eight span
107.4–117.7 s, with no overlap and identical verdicts throughout. The apparent
14% speed-up over `kotlin-proof` is an artefact of that, not an effect of the
edits — the three obligations the calls were removed from are BLOCKED and never
run. **F050** records it, with the boundaries recoverable from this branch's
committed journal history.

The verdicts in this run are sound and are what the comparison above rests on;
they do not depend on which machine produced them. The seconds do.
