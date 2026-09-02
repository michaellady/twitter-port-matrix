# GOAL — twitter_port_matrix

The durable objective and current state. Every loop iteration reads this
first, does the next unchecked step, updates the state block, and stops.

---

## The goal

Produce a **per-rung mutation-kill table** for cross-language ports, so the
question "how much assurance does each verification layer actually buy on a
port, and at what cost" has a number instead of an intuition.

```
rung          mutants  killed  survived  wall
R0 corpus          48      19        29     2s
R1 diff-fuzz       48      44         4    90s
R2 property        48      41         7    45s
R4 proof           48      46         2   380s
R5 refinement      48      48         0   380s
```

That table is the deliverable. Four verified Twitter clones and twelve ports
are the apparatus for producing it, not the point.

**Why it matters.** `verified-java-to-rust-port` has every empirical layer
working and its deductive rung blocked at
`RUST_IDENTITIES_NOT_YET_RESOLVER_VERIFIED`, with no way to know what closing
it is worth. This rig answers that at a scale where the whole ladder fits.

## Done criteria

The goal is met when all of these hold:

- [ ] Four implementations (Java, Kotlin, Rust, Go) each refine `S_obs` as far
      as their language's tooling allows, with the ceiling per corner recorded
      honestly rather than engineered around.
- [ ] All 12 ports have evidence at every rung their weaker end supports.
- [ ] `matrixctl calibrate` emits the kill table per port, per rung.
- [ ] Every rung has a known-bad canary proving it can fail.
- [ ] A transfer write-up mapping the calibration back to the WebSocket port.

## Orchestration

The loop drives; workflows are called *by* an iteration when the work becomes
fan-out-shaped. They compose rather than compete.

| Phase | Driver | Why |
|---|---|---|
| 1c–1h | loop | Sequential edit → `replay` → read → fix cycles on one small codebase. Parallel agents would need worktree isolation for less benefit than it costs |
| **1i** | **workflow** | 48 mutants × 5 rungs = independent chains. Pipeline, not barrier. Worktree isolation is genuinely correct here — mutants really do mutate files in parallel |
| 2, 3 | loop | Java and Kotlin corners are independent of each other but each is sequential internally |
| **4** | **workflow** | 12 ports, each an independent generate → verify → calibrate chain |

**The reason 1c–1h stay sequential is not speed, it is independence.** This
project exists to measure what happens when one agent writes both the code and
its checks. Fanning out implementation work and rung work into the same
orchestration rebuilds that defect. `S_obs` generates the oracle, `replay`
reports the gap, the implementation gets fixed — three roles that must not
quietly collapse into one.

**Outstanding, not yet done:** F001, F002 and F003 all rest on a single
reading — mine. F001 in particular claims a shipped conformance suite has no
signal behind a field, which deserves independent refutation before it is
relayed to the WebSocket project. A judge panel with distinct lenses is the
right instrument. Deferred by choice, not overlooked.

## Standing rules

1. **No gate is decided by an exit code.** Read the tool's own output.
2. **No gate is trusted until it has been shown to fail.** Every rung needs a
   canary. A check that cannot fail proves nothing — see finding F001.
3. **No changes to a base app to make a port provable.** The whole claim is
   that the base app's obligation was discharged against `S_obs`, not against
   the port. An adapter is allowed; a source change is not.
4. **No implementation imports `S_obs`.** Enforced by `matrixctl doctor`.
5. **Mutants are held out** from whoever writes the port.
6. **The rungs run here, not on a hosted runner.** No `.github/`, no CI: the
   kill table is produced by running the rungs and committing their own
   output, and a green badge from a runner nobody reads is exactly the kind of
   number Pattern 1 warns about. Pushing branches and opening draft PRs is how
   work lands; pushing to `main` is not. (This rule used to say "local only,
   nothing pushed"; the repository has been public with PRs since #1.)
7. **Record what the rig finds**, in `evidence/findings/`, including findings
   that are benign. F002 is benign and worth having.
8. Ceilings that cannot be reached get written down, not worked around.

---

## Phases

### Phase 0 — spec foundation ✅ COMPLETE

- [x] `S_obs`: deterministic, total, sort-free timeline
- [x] Ambiguities pinned in `spec/s_obs/DECISIONS.md` (D1–D10)
- [x] `corpusgen` — corpus generated, regeneration byte-stable
- [x] `tlclink` — `S_obs` refines `twitter.tla`, with a working canary
- [x] `matrixctl doctor` + `matrixctl spec check`
- [x] Findings F001, F002

### Phase 1 — Go↔Rust rig

- [x] **1a** Vendor the Go implementation into `impls/go/` as its own module
- [x] **1b** `replay` — drives an implementation over HTTP and byte-compares.
      R0 baseline recorded: 7/54 exact, 8 whitespace-only, 39 differ. See
      finding F003
- [x] **1c** Retarget Go to the `S_obs` contract. **R0 54/54 byte-exact**,
      canary verified. Six `// @ trusted` shims deleted from the store. Semantics go in the
      VERIFIED CORE (`dom`, `store`, `service`), wire format in the trusted
      shim — putting semantics in `httpshim` would green R0 over code no
      verifier reads, and R5 would prove nothing observable. See F004.
      Order: (i) append-log reshape of `store.HomeTimeline`, deleting the
      `sortTimeline` and `gatherTimeline` trusted shims; (ii) `POST /tick`
      route, which unblocks F2/F7/F8/D9 together; (iii) strict decoding
      (D7, 10 steps); (iv) error vocabulary (7 renamings); (v) drop the
      trailing newline from `writeJSON` (D8, 8 steps)
- [x] **1d** Same for Rust in `impls/rust/`. **R0 54/54 byte-exact**, canary
      verified. Same flat reshape; `external_body` shims retargeted in 1g.
- [ ] **1e** `tracegen` + `diffrun` — randomized differential traces. R1
- [ ] **1f** Shared property + metamorphic suite. R2
- [ ] **1g** `specgen` → Verus and Gobra contracts; both verifiers green. R4
- [ ] **1h** Refinement obligations against `S_obs`. R5.
      Expect the sort-free design to unblock F1/F2 on `home_timeline`
- [ ] **1i** `mutate` + `calibrate` — the first kill table, both directions

### Phase 2 — Java corner
- [ ] Host: Maven or Gradle. Docker: OpenJML, JBMC, digest-pinned
- [ ] Java implementation + JSONL adapter (the `java-oracle/` pattern)
- [ ] Ports Java↔Rust, Java↔Go

### Phase 3 — Kotlin corner
- [ ] Host: `kotlinc`. Docker: JBMC over bytecode
- [ ] Document the R5 ceiling failure plainly. It is a result

### Phase 4 — fill the matrix
- [ ] All 12 ports, full calibration report
- [ ] Transfer write-up back to the WebSocket project

---

## LOOP — how the matrix gets filled from here

The goal is pursued by a **scheduled routine**: every two hours a fresh session
starts, reads this file, does the next unchecked step, and stops. Nothing
carries over between fires except what is committed, so this section is the
whole contract.

### Definitions settled on 2026-09-02

- **A matrix cell is an ordered corner pair, B ← A.** The four corners are the
  implementations; a port is corner B measured with corner A as the base it
  must agree with. Twelve cells = twelve ordered pairs over existing code. No
  cell requires writing a new implementation.
- **The table's rows are the rungs, R0-R5.** `calibrate` runs R0-R2 today.
  R4 and R5 are entries in `tools/cmd/calibrate/rungs.go` that do not exist
  yet, and adding them is the first job (below). The Go half of R4/R5 -- the
  verdict-reading, the vacuity audit, the per-clause join -- is
  `tools/cmd/gobra`; Rust needs the same over `cargo-verus`, Kotlin over JBMC.
- **VERIFIED means refutable.** A rung's kill verdict on a proof-backed row
  counts only if the obligation that noticed the mutant was itself shown
  non-vacuous. `gobra canary` / `gobra reach` are the instrument; the
  equivalents for Verus and JBMC must exist before their rows are trusted.

### The queue, in order

1. **R4 and R5 as `calibrate` rungs.** One entry each in `rungs.go`, per
   corner that has a verifier, reading the verifier's own verdict line per
   standing rule 1, with a canary per standing rule 2. Start with Go/Gobra
   because the tooling exists; then Rust/Verus; then Kotlin/JBMC. A rung entry
   is done when `calibrate -rungs R4` produces a kill row for one corner and
   `mutate verify` still passes.
   - [x] **R4, Go/Gobra.** `calibrate -rungs R4` runs `gobra verify` over the
         mutant tree, reads its `R4 PASSED` / `R4 FAILED` line, and requires
         the exit code to agree. ~90 s per mutant, no server launched. A
         corner with no verifier for a rung gets a **capped** cell, which is
         in no denominator. Coverage is scored per mutant against the
         verification matrix (F022): a mutant confined to the trusted shim is
         *unreached*, not *survived*.
   - [x] **R5, Go/Gobra.** `gobra r5verify` runs the same Gobra invocation and
         attributes each failing obligation to a clause by line, joining
         against `spec/refinement/clause-sites.json`. A kill counts for R5
         only when the failure hits a refinement clause or the member whose
         contract carries one; a failure elsewhere kills R4 alone. Verified by
         canary in both directions, so the two rows are not aliases.
   - [ ] R4, Rust/Verus — needs the `cargo-verus` equivalent of
         `gobra verify`'s verdict line and budget first.
   - [ ] R4, Kotlin+Java/JBMC.
2. **Re-run the four-corner sweep** on the 72-mutant catalogue, all rungs
   that exist, attributing each kill to **every** rung that kills it rather
   than the first to run. `evidence/CALIBRATION-four-corner.md` is the R0-R2
   baseline to extend, not replace.
   - [x] **Per-judge attribution confirmed and locked.** The sweep already ran
         every rung against every mutant; `runRungs` now carries the reason and
         three tests fail against an injected first-judge `break`. Nothing about
         the sweep's shape needs deciding before it runs.
3. **The twelve cells.** For each ordered pair, run `calibrate` with A as the
   base and B as the subject, and shape `evidence/MATRIX.md` as the 12 x 6
   table GOAL.md opens with, with a **coverage denominator** beside each kill
   rate (F008). Cells whose weaker end caps the rung say so in the cell rather
   than leaving it blank.
   - [x] **The coverage denominator ships with every rate.** `calibrate` prints
         `killed/reached` as a fraction and states per rung what was excluded
         and why; `results.json` carries `cells`, `reached` and `excluded`.
   - [x] **`evidence/MATRIX.md` exists as a skeleton**: 36 measured cells, 24
         capped, 12 n/a (R3), 0 pending, no invented number, and the exact
         invocation that fills each column. **F024: all 24 R4/R5 cells are
         capped and the Go proof sweep fills none of them** -- the first proof
         cell needs a second corner's verifier rung, not another sweep.
4. **A second catalogue from a source other than the contract** -- incident
   history, a fuzzer corpus. Until this exists the 100% rows partly measure
   that catalogue and corpus share a parent.
5. Fix or delete the four drifted Verus twins (F012, F016).
6. S-10 `specgen`, S-24 the adversarial refutation pass.

### Related work to feed back into

`michaellady/mike-skills` PR #7 (branch `claude/port-jvm-to-rust-skills`) is a
twelve-skill family for behaviour-preserving JVM-to-Rust ports, much of it
distilled from this repository: `S_obs`/`DECISIONS.md` became `port-model`,
`tlclink --canary` the refinement link, `calibrate`'s exercised-mutant guard
and launch-floor subtraction `port-calibrate`, `ASSURANCE.md`'s ceilings
`port-ceiling`, and the findings the reason `port-learn` exists. It names
three things this matrix still lacks, and two of them belong in the queue:

- the R4/R5 rows of the kill table -- queue item 1;
- **a coverage denominator** (F008: a kill rate over mutants says nothing
  about inputs the rungs never reach) -- add to item 3's `MATRIX.md` shape;
- **per-judge mutation attribution** rather than first-judge -- when a
  mutant dies at several rungs, every rung that would have killed it gets
  the credit, not only the first to run. Add to item 2's re-run.

A fire that adopts or contradicts any of it feeds back through `port-learn`
with a PR against that family citing the finding. Not a directive for the
current step.

### One iteration

1. `git fetch origin main`. If an open draft PR exists for `claude/goal-loop`,
   check it out and merge `origin/main` into it; otherwise branch
   `claude/goal-loop` from `origin/main`. Never touch `main`.
2. `go run ./tools/cmd/matrixctl doctor`. If a rung's tooling is missing, the
   step that needs it is skipped for this fire and the skip is recorded in
   STATE with the doctor line that says why.
3. Take the **first unfinished item** in the queue above, or the first
   unchecked sub-step of it recorded in STATE. Do that one thing. Long runs
   checkpoint and resume; a fire that runs out of time leaves the checkpoint
   and says so.
4. Read the tool's own output for the verdict. Show the gate can fail.
5. Update this STATE block: what was done, the verdict line, what the next
   fire should do. Record any finding in `evidence/findings/`.
6. Commit with a message that says what was measured, push, open or update the
   draft PR. Stop.

A fire that finds an earlier fire's claim to be wrong corrects it in place and
records the correction, per F020. A fire that cannot finish its step does not
report it finished.

---

## STATE — session ended here

**Published:** https://github.com/michaellady/twitter-port-matrix (public)
**Stories:** 20 of 24 done · **Findings:** 24 · **Corners:** 4, all R0 56/56

### The deliverable exists

`evidence/CALIBRATION.md` — 36 mutants x 3 rungs, both original corners:
R0 100% @ 57s · R1 100% @ 1465s · R2 42% @ 2495s. Caveat leads the document:
the catalogue and the corpus both derive from `S_obs`, so the 100%s partly
measure that alignment.

`evidence/FINDINGS.md` — the through-line across all 21.
`evidence/TRANSFER-to-websocket-port.md` — the write-up this was built for.

### Where each corner stands

| corner | R0 | R1 | R2 | R4 | limited by |
|---|---|---|---|---|---|
| Go | 56/56 | clean | pass | Gobra green; 91 functional clauses, reachability audit clean (0 of 33 unreachable), negation sweep in PR #3. **R4 and R5 are both `calibrate` rungs since 2026-09-02**, ceiling 14 of 18 mutants (F022) | Gobra ghost-language limits; 5 HomeTimeline clauses undecidable within budget; the trusted shim is 4 of 18 mutants |
| Rust | 56/56 | clean | pass | Verus, **1 property** (F016) | `RwLock` has no vstd model |
| Java | 56/56 | clean | pass | not attempted | JBMC string equality (F014) |
| Kotlin | 56/56 | clean | pass | JBMC, 7 of 15 | same JBMC defect |

### Loop log

Fires append here, newest first. One line per fire: UTC time, what was done,
the verdict line, and what the next fire should do.

- **2026-09-02 17:45 (worker session `session_01ExaVft3sZPUuETJXKVZK1J`, branch
  `claude/loop-f-matrix-shape`)** — **The two prerequisites queue items 2 and 3
  both depend on, made before the 72-mutant sweep rather than after it.**
  1. **Attribution was already per-judge; nothing was changed to make it so.**
     `calibrateRunnable` runs every selected rung against every mutant with no
     short circuit, so a mutant killed at R0 and R2 is already in both rows'
     kill counts. What was missing was anything stopping that from being
     undone: the loop is now `runRungs` with the invocation injected, carrying
     the reason in its doc comment, and three tests lock it. Shown to fail
     (standing rule 2) by injecting `if c.Outcome == outcomeKilled { break }`:
     ```
     --- FAIL: TestEveryRungRunsAfterAKill          the sweep asked [R0]
     --- FAIL: TestResumedCellDoesNotSkipTheOtherRungs  asked [R1], want [R1 R2]
     --- FAIL: TestKillAtTwoRungsCreditsBothRows    R1: killed/reached = 0/0, want 0/1
     ```
     On this catalogue first-judge would report every rung after R0 as killing
     nothing, since R0 kills 72 of 72.
  2. **Every reported rate now carries its denominator.** `killed/reached` and
     `killed/live` are printed as fractions (`14/14 = 100%`, `7/17 = 41%`), the
     summary rows carry `cells`, `reached` and a per-outcome `excluded` list
     into `results.json`, and a `DENOMINATORS` section states per rung how many
     cells were excluded and why, in the excluded outcome's own words (F009 for
     an input gap, F022 for a proof rung's reach). Shown to fail by restoring
     the old rendering with the new fields in place — three tests fail, and the
     one that matters most is the zero-denominator case, where the old renderer
     printed a rate for a rung that reached nothing:
     ```
     before   R4 proof   2  0  0  2  0     0%     0%    0s
     after    R4 proof   2  0  0  2  0   0/0 = n/a   0/2 = 0%   0s
     ```
     `0%` is the same six characters for "reached nothing" and "saw everything
     and killed none". Smoke-tested end to end at `-impls go -rungs R0 -ids
     limit-off-by-one`: `R0 FAILED: 1 step(s) disagree with S_obs`, reported as
     `killed/reached 1/1 = 100%`, `0 excluded`.
  3. **`evidence/MATRIX.md` created as a skeleton** — 12 ordered pairs × R0-R5,
     72 cells: **36 measured** (R0/R1/R2 from `four-corner`, with denominators),
     **24 capped**, **12 n/a** (the R3 column: a claim about the model, not
     about code), **0 pending**. No number in it is invented; each is copied
     from `evidence/runs/calibration/four-corner/results.json` or from
     `ASSURANCE.md`'s ceiling table. It opens with the B ← A definition over the
     four EXISTING corners — no cell requires a new implementation — and with
     `CALIBRATION.md`'s caveat, and it records the exact `calibrate` invocation
     that fills each column.
  **F024 written**, and it changes what the next fire should expect: **all 24
  R4/R5 cells are capped**, because only Go has a proof rung and no ordered pair
  has Go at both ends. The Go R4+R5 sweep is still the right next step — it
  answers whether R4 and R5 discriminate at all, which five mutants cannot — but
  it fills **one end of six rows and zero cells of the matrix**. The first proof
  cell needs a *second* corner's verifier rung. A fire that runs the sweep
  expecting a matrix cell will report a success as a failure.
  `go build`, `go vet`, `go test ./tools/...` all clean; 9 new tests.
  **Next fire: the Go corner's full R4+R5 sweep**, unchanged from the entry
  below — `-impls go -rungs R4,R5 -out evidence/runs/calibration/go-proof
  -resume`, ~50 minutes, journalled per cell. Read its result as one end's rate,
  not as a matrix cell (F024).

- **2026-09-02 16:55 (worker session `session_01Mdy8cUZTbcq2fXuZ1BRi4X`, fire
  16:13)** — **Queue item 1, second sub-step: DONE. R5 is a `calibrate` rung on
  the Go corner.** `gobra r5verify` verifies one tree and attributes every
  failing obligation to a clause by line, joining against
  `clause-sites.json`; 47 of 47 recorded sites are located in a clean tree.
  Gate, `-impls go -rungs R4,R5` over five mutants:
  ```
  go/tick-goes-backwards      R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)
  go/timeline-scan-reversed   R5 FAILED: ... (0 on the clause itself, 1 elsewhere in its member)
  go/limit-off-by-one         R5 FAILED: ... (0 on the clause itself, 1 elsewhere in its member)
  go/id-first-is-two          R5 PASSED: Gobra has found 0 error(s); no refinement clause failed
  go/next-cursor-is-first-id  R5 PASSED: ...  -> unreached (trusted shim)
  R4 proof        live 5  killed 3  survived 1  unreached 1   kill%reach 75%
  R5 refinement   live 5  killed 3  survived 1  unreached 1   kill%reach 75%
  ```
  **A first attempt was wrong and has been corrected in place.** Attributing
  only to `ensures` lines left 2 of 5 R5 cells UNDECIDED: what fails on the
  memstore mutants is a **loop invariant** inside `(*MemStore).HomeTimeline`
  (`memstore.go:580 Loop invariant might not be established`), and those
  invariants are the machinery that proves the refinement postconditions —
  one is commented "R5 (no fabrication)" in the source. Attribution is now by
  clause *and* by member, which is the standard R4 already applies: a proof
  rung's kill means the tree can no longer be verified. Only 1 of the 3 kills
  lands on a clause line; the other 2 are the invariant path, so the
  clause-only reading would have decided a third of them.
  Canary (standing rule 2), in both directions, because R4 and R5 agreed on
  all five mutants and agreement alone cannot distinguish a rung from an
  alias:
  ```
  A. `// @ ensures false` on dom.ValidHandle (no R5 clause on that member)
       R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m29.6s]
       R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause
  B. `// @ ensures false` on clock.Tick (carries R5 clause 36)
       R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause
  ```
  **F023 written**, the substantive result of this fire: `id-first-is-two`
  shifts the id origin from 1 to 2, is live at request 0 of every input
  source, and is killed by R0 and R1 while surviving R2, R4 **and** R5. Every
  obligation on the generator is relational (`result >= 1`, counter advances
  by one) and the origin is stated only in English. The ladder is not ordered;
  `evidence/FINDINGS.md` gains Pattern 6 for it.
  `go build`, `go vet`, `go test ./tools/...` clean; 8 new tests including one
  that re-derives the R5 file list from `clause-sites.json` so it cannot go
  stale.
  **Next fire: the Go corner's full R4+R5 sweep**, all 18 mutants x 2 rungs,
  `-out evidence/runs/calibration/go-proof -resume` (it journals per cell, so
  a container restart resumes; budget roughly 50 minutes at ~85 s per cell).
  This is queue item 2 narrowed to the one corner where both proof rungs
  exist, and it answers the question this fire raised: **R4 and R5 agreed on
  all five mutants, so it is not yet known whether the refinement row
  discriminates at all.** Five is a gate, not a rate. Deliberately taken
  before queue item 1's remaining sub-steps (Rust/Verus, then Kotlin/JBMC):
  building a second verifier's plumbing while the first corner has no rate is
  the wrong order. Do not skip the `-resume` flag, and do not add the Verus
  rung in the same fire.
- **2026-09-02 14:30 (worker session `session_01Mdy8cUZTbcq2fXuZ1BRi4X`, fires
  12:27 and 14:18)** — **Queue item 1, first sub-step: DONE. R4 is a
  `calibrate` rung on the Go corner.** `gobra verify` gained a verdict
  sentence, a `-registry` so it resolves the same mutant tree the guard
  hashed, and a `-budget` (a run that exhausts it prints `R4 UNDECIDED` and no
  verdict, which `calibrate` records as an error cell, never a survival).
  `rungs.go` gained the R4 entry, a per-corner `Impls` field, and a `Covers`
  predicate. Gate, `-impls go,rust -rungs R4 -ids limit-off-by-one,next-cursor-is-first-id`:
  ```
  go/limit-off-by-one          R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m9.8s]   killed
  go/next-cursor-is-first-id   R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m53.7s]  unreached
  rust/next-cursor-is-first-id capped: no selected rung exists for corner rust
  R4 proof   live 2  killed 1  survived 0  unreached 1  equiv 0   kill%reach 100%  kill%live 50%
  ```
  Canary (standing rule 2), the same materialised tree twice, one injected
  line apart:
  ```
  1. untouched                        R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m28.4s]
  2. + `// @ ensures false` on Tick    R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m24.7s]  exit status 1
  ```
  Clean-tree baseline for comparison: `Gobra has found 0 error(s)  [59s]`,
  237 distinct Viper members. Catalogue unchanged and undrifted:
  `verify PASSED: every anchor matches one site; every mutant compiles`
  (72/72 anchors, 72/72 build clean). **F022 written:** 4 of 18 Go mutants edit only
  `internal/httpshim`, which no obligation covers, so R4's ceiling on this
  corner is 14 of 18 before any obligation is written — and scoring those four
  as survivors rather than unreached would have read 50% where the oracle is
  100%. `go build`, `go vet`, `go test ./tools/...` all pass; 6 new tests
  cover the verdict/exit-code disagreements, the corner split and the
  coverage predicate.
  **Next fire: queue item 1, second sub-step — the R5 entry for the Go
  corner.** Same shape as R4, but a kill counts for R5 only when a failing
  obligation's `file:line` is one of the sites
  `spec/refinement/clause-sites.json` records for an R5 clause; `gobra r5`
  already joins those three files and is the code to reuse. Gate:
  `calibrate -impls go -rungs R4,R5 -ids limit-off-by-one` shows the mutant
  killed at both rungs *if and only if* its failing site is an R5 site, and a
  mutant whose failing site is not an R5 site is killed at R4 only. Do not
  start the four-corner re-run (queue item 2) until R5 exists — it would have
  to be redone.
- **2026-09-02 11:01 (interactive session)** — Fire 3 (10:12, Fable, clone-first
  prompt, session `cse_01BjLpDECEYMroHsHGBgdfkx`) also pushed nothing: 17
  minutes, no loop-log line. Its session record shows no repository source
  attached, unlike a probe spawned interactively, so a routine-spawned fresh
  session cannot attach the repo even when told to. The fresh-session routine
  is disabled. The loop now fires into a **persistent worker session**
  (`session_01Mdy8cUZTbcq2fXuZ1BRi4X`) created with the repository as its
  source and `claude/goal-loop` as its outcome branch. Same cadence, same
  contract; the worker's context accumulates, so fires rely on this file, not
  memory. The next-fire entry below is unchanged and still the entry point.
- **2026-09-02 09:25 (interactive session, PR #3)** — Diagnosed the two
  silent fires. A session spawned into this environment by the routine has
  **no checkout of the repository**; a probe session reported `repo not
  cloned; need git access or repo pre-staging`. Both fires spent their budget
  in an empty container. The routine prompt now begins with an explicit
  `add_repo` + clone step and a hard stop if that fails. Confirmed at 09:32:
  a spawned session that calls `add_repo` first clones and pushes within
  75 seconds. The next-fire entry below is unchanged and still the entry
  point.
- **2026-09-02 06:34 (interactive session, PR #3)** — R4/R5 audit complete.
  Negation sweep `91 clauses: 83 refutable, 0 VACUOUS, 8 timed out`; R5
  `26 VERIFIED, 4 UNAUDITED, 12 UNATTEMPTED`; F019–F021 written; `GOAL.md`
  LOOP contract added; branch `claude/goal-loop` fast-forwarded to match.
  **Next fire: queue item 1, first sub-step** — add an `R4` entry to
  `tools/cmd/calibrate/rungs.go` for the Go corner that runs
  `go run ./tools/cmd/gobra verify` over the mutant tree, reads the
  `Gobra has found N error(s)` line for the verdict, and reports Pass/Fail the
  way the R0–R2 entries do. Gate: `calibrate -impls go -rungs R4 -ids <one
  mutant id>` produces a kill row, and an injected `// @ ensures false` in the
  mutant tree makes it FAIL (standing rule 2). Do not add R5 in the same fire.
- **2026-09-02 06:15 (routine fire 1, session `cse_013t8PuViUqGSYhhfdXwMjSN`)**
  — ran 13 minutes and pushed nothing; no STATE entry was written, so what it
  did is not recorded. That is the failure mode this log exists to prevent: a
  fire that ends without a line here did not happen as far as the next fire
  is concerned.

### Highest-priority outstanding work

1. ~~**Fix the concurrency defect (F018).**~~ **DONE.** `Service.wmu` now holds
   the allocate-then-write sequence in `PostTweet` and `CreateUser` atomically,
   so ids are appended in the order they are issued and the F005 guard is
   unreachable in fact. Two regression tests ship with it, both measured
   against the unfixed code first (20/20 and 28/40 detection). They are
   **standing checks, not a rung** — `S_obs` is sequential and cannot express
   the interleaving, so their oracle counts durable records instead of
   comparing against a reference machine. The Go corner has them; the other
   three do not, which is the next piece of this.
2. **Re-run the sweep across all four corners** — the catalogue is 72 mutants
   now. F017 bounds what the R4/R5 rows can compare.
3. **A second catalogue from a different source** than the contract —
   production incidents, a fuzzer corpus, real defect history. This is what
   would turn the table from an easy case into an informative one, and it is
   the highest-value follow-up in either project.
4. **Fix or delete the four drifted Verus twins.** One (`create_user_ensures`)
   is actively false of shipped code; another encodes the pre-D4 defect.
5. Remaining stories: S-10 specgen, S-18/S-21 the port matrix itself, S-24 the
   adversarial refutation pass.

### Things a future reader should not re-derive

- Gobra needs **no Docker** — it is a fat jar; `java -Xss128m -jar gobra.jar`,
  ~2x faster than the emulated amd64 image. `.devcontainer/` lifts it at build
  time.
- Gobra reports **one failing postcondition per method**; batch a negation
  audit and it silently under-reports.
- `mutate verify` must pass immediately before any sweep. Anchors drifted
  twice in one session; a drifted anchor injects nothing and every rung
  "kills" it.
- Verus **caches**; `touch` the crate source or a run prints nothing and looks
  like a pass.
- **The cloud environment now reaches all five rungs.** The Gobra jar is
  fetched by `cloud-setup.sh` (see `CLOUD.md` for the allowlist history), and
  Go R4 and R5 have been run there. Two things about running Gobra that were
  not written down anywhere: it needs a **GOPATH-shaped tree** (`-m <module>`
  does not resolve intra-module imports; `tools/cmd/gobra` lays one out), and
  a negated quantifier clause can wedge Z3 indefinitely, so every invocation
  needs a **time budget** and a run of any length needs a **checkpoint**.
- **The cloud container restarts without warning** -- twice in one night, once
  mid-sweep. Anything longer than a few minutes must checkpoint to disk and
  resume from it; `gobra canary` does, `calibrate` does. Do not wait on a
  process with `pgrep -f <pattern>`: it matches the shell whose command line
  contains the pattern, including the one doing the waiting.
- **A count from a verifier is a measurement with a range, not a constant.**
  The Go R4 "283 Viper members" was 236-238 across eight runs and 61%
  compiler-generated (F019). Record the unit that is about the contract -- the
  clause -- and record a range.
- The environment is pinned because several findings **are** properties of a
  specific build (F014 is a CBMC 6.11.0 defect; F012's blocker is a property
  of one vstd release).
