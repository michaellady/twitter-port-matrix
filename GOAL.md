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

The goal was pursued by a **scheduled routine** — every two hours a fresh
session read this file, did the next unchecked step, and stopped. **That is no
longer how it works.** The routine is disabled and the remaining work runs as
an explicit parallel queue, below. Everything after "### Definitions" still
holds: the definitions, the standing rules, the one-iteration protocol and the
loop log are what any agent working this repository reads, whether it was
started by a schedule or by hand.

### The parallel queue — replaces the scheduled loop, 2026-09-02 20:00 UTC

**The two-hourly routine is DISABLED** (`trig_01RxsVmFWraT4T92EczcY3Y1`). One
fire doing one step every two hours was the wrong shape once the remaining work
turned out to be six mutually independent pieces. They now run **at the same
time, each in its own cloud session on its own container**, which also removes
the hazard that stopped a sweep earlier this evening: four cores shared between
two verifiers made a 723 s result against a 720 s budget, and a timeout caused
by load recorded as a solver result is a false finding.

**What is left is bounded, and it is mostly the two proof columns.** Of 72
cells: 36 behavioural cells are measured, 12 are `n/a` by design (all of R3),
and the remaining 24 are the R4 and R5 columns. R4 is 2 measured, 4 pending, 6
capped by Java. R5 is 12 capped, because Gobra on Go is the only R5 rung and no
ordered pair has Go at both ends.

| # | task | fills | branch | findings |
|---|---|---|---|---|
| 1 | Kotlin corner's 18-mutant R4 sweep | the 4 `pending` R4 cells | `claude/task-kotlin-r4-sweep` | F031–F033 |
| 2 | Java obligation set + R4 rung | the 6 R4 cells capped by Java | `claude/task-java-obligations` | F034–F037 |
| 3 | `dom.go` R4/R5 separation experiment | closes F028's open question | `claude/task-dom-separation` | F038–F040 |
| 4 | Lift the Rust core out of `RwLock` | 2 R5 cells, or a demonstrated block | `claude/task-rust-r5-rwlock` | F041–F043 |
| 5 | Kotlin R5 refinement rung feasibility | 2–4 R5 cells, or a demonstrated no | `claude/task-kotlin-r5` | F044–F046 |
| 6 | Second catalogue from a non-contract source | bounds every published kill rate | `claude/task-second-catalogue` | F047–F050 |
| 7 | Four-corner re-run, all rungs, per-judge | the whole table | *blocked on 1 and 2* | F051–F053 |
| 8 | Integrate, recompute the census, update the PR | — | `claude/goal-loop` | — |

Plus the HomeTimeline vacuity sweep (F029) running locally, which is why tasks
1–6 are in the cloud rather than on this box.

**Finding numbers are allocated in advance, per the table.** This is not
bureaucracy: the last fan-out produced a five-way collision on F024 that cost a
manual renumber across 30 references, and a renumbering regex then silently
renamed a row that was not meant to move. An agent that needs more than its
range stops at the last one and says so.

**Task 7 is deliberately blocked.** Re-running the four-corner sweep before the
Kotlin and Java R4 rows exist would regenerate the table missing exactly the
columns being added.

**Two tasks are allowed to come back "no".** Task 5 may find that JBMC cannot
attribute a failure to a named refinement obligation, and task 4 may find the
`RwLock` block is real. A demonstrated no is a better deliverable than a rung
reporting numbers it cannot justify, and it would establish that R5 is
structurally a Go-only rung on this rig — a sharper claim than "not built yet".

**Task 6 is the one that could invalidate the others.** Every mutant in the
catalogue is drawn from the contract, the R0 baseline or a finding this project
already made, and the rungs are built from the contract too. If the independent
catalogue's kill rates drop sharply at R4 and R5, every rate published here is
bounded by that, and it is the most important result the project could produce.


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
   - [x] **R4, Rust/Verus.** `verus verify` runs `cargo-verus` over the five
         verify-enabled crates of the mutant tree, reads Verus's own
         `verification results:: N verified, M errors` lines, and ends with
         one R4 verdict sentence. Verus caches through cargo, so the driver
         touches every `.rs` file in those crates first and refuses to call a
         run PASSED unless all five reported — an untouched re-run prints no
         result line at all and would otherwise read as a pass. **The row does
         not mean what the Go row means: F027.** Four of the five verified
         crates put their contracts on hand-written twins, so Verus kills 1 of
         the 14 covered mutants, and the one it kills is the one clause F016
         found on shipped code.
   - [x] **The vacuity instrument for Verus** (`tools/cmd/verus canary`), which
         this section's own definitions require before the Rust row is
         trusted and which did not exist when the row was first reported.
         `REFUTABLE 5, VACUOUS 0` on the shipped clauses, self-tested to
         return VACUOUS under an injected false precondition — **and 57 of the
         corner's 62 clauses are on twins, where a canary measures the twin.**
         F030. `TestEveryProofRungCornerHasAVacuityInstrument` is the gate that
         will catch the next corner that ships an R4 row without one.
   - [x] **R4, Kotlin/JBMC.** `calibrate -impls kotlin -rungs R4` runs
         `tools/cmd/jbmc verify` over the mutant tree's compiled bytecode and
         reads JBMC's own goal lines. Three outcomes per obligation, not two:
         an obligation JBMC cannot decide (F014) or cannot reach (F013) is in
         **neither** the numerator nor the denominator, the way F022 treats a
         shim-only mutant as unreached. Denominator 7 of 15; the other 8 are
         blocked by a recorded tool defect and counted separately in the
         verdict sentence. The F013 vacuity audit runs on **every tree
         judged**, which the Go corner cannot afford (F025).
         **Java is not done and is not blocked on JBMC**: `impls/java` has no
         obligation set at all, so there is nothing for the rung to run. A row
         over an empty denominator is worse than the capped cell a corner with
         no rung already gets, so no Java entry was registered.
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
         invocation that fills each column. **F026: all 24 R4/R5 cells are
         capped and the Go proof sweep fills none of them** -- the first proof
         cell needs a second corner's verifier rung, not another sweep.
4. **A second catalogue from a source other than the contract** -- incident
   history, a fuzzer corpus. Until this exists the 100% rows partly measure
   that catalogue and corpus share a parent.
5. [x] **Fix or delete the four drifted Verus twins (F012, F016).** Done
   2026-09-02 on `claude/loop-e-verus-twins`: two fixed, two deleted,
   Verus `23 -> 21 verified, 0 errors`. See `evidence/findings/F024`.
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

`evidence/FINDINGS.md` — the through-line across all 24.
`evidence/TRANSFER-to-websocket-port.md` — the write-up this was built for.

### Where each corner stands

| corner | R0 | R1 | R2 | R4 | limited by |
|---|---|---|---|---|---|
| Go | 56/56 | clean | pass | Gobra green; 91 functional clauses, reachability audit clean (0 of 33 unreachable), negation sweep in PR #3. **R4 and R5 are both `calibrate` rungs since 2026-09-02**, ceiling 14 of 18 mutants (F022) | Gobra ghost-language limits; 5 HomeTimeline clauses undecidable within budget; the trusted shim is 4 of 18 mutants |
| Rust | 56/56 | clean | pass | Verus **32 verified, 0 errors**. The lock lift (F041) moved F7, F8 and the store's three abstraction axes onto shipped functions: census **37 shipped / 20 twin / 13 assumed**, from 5 / 36 / 21, and **37 of 37 shipped clauses REFUTABLE, 0 VACUOUS** | `crates/service` is still all twins; `String`'s view is not known injective, which blocks R5's response axis (F043); there is no Verus R5 rung in `calibrate` |
| Java | 56/56 | clean | pass | not attempted — and now blocked on something more mundane than F014: `impls/java` has **no obligation set**. The Kotlin corner's `Obligations.kt` has no Java twin, so there is nothing for a JBMC rung to run | JBMC string equality (F014); plus no obligations written |
| Kotlin | 56/56 | clean | pass | JBMC, 7 of 15 decidable. **R4 is a `calibrate` rung since 2026-09-02** (`tools/cmd/jbmc verify`); the 8 blocked obligations are in neither numerator nor denominator, and the F013 vacuity audit re-runs on every tree judged. Coverage 16 of 18 mutants (2 are httpshim) | the JBMC defect sets the denominator, not the obligations (F014, F025) |
| Go | 56/56 | clean | pass | Gobra green; 91 functional clauses, reachability audit clean (0 of 33 unreachable), negation sweep in PR #3. **R4 and R5 are both `calibrate` rungs since 2026-09-02**, ceiling 14 of 18 mutants (F022). **Full sweep 2026-09-02: R4 9/14 reached, R5 9/14 reached, the two rows disagree on 0 of 18 (F028)** | Gobra ghost-language limits; 5 HomeTimeline clauses undecidable within budget; the trusted shim is 4 of 18 mutants |
| Rust | 56/56 | clean | pass | Verus, **1 property** (F016) | `RwLock` has no vstd model |
| Java | 56/56 | clean | pass | not attempted | JBMC string equality (F014) |
| Kotlin | 56/56 | clean | pass | JBMC, 7 of 15 | same JBMC defect |
### Loop log

Fires append here, newest first. One line per fire: UTC time, what was done,
the verdict line, and what the next fire should do.

- **2026-09-02 22:48 (fan-out task, branch `claude/task-kotlin-obligation-coupling`)**
  — **Five of the Kotlin corner's seven `Store.createUser` couplings are gone,
  the cell they were blamed for is still an ERROR, and the reason is now known
  exactly rather than suspected.** F031 predicted the repair would restore the
  cell. It does not, and that is the finding.

  Each of the seven sites was decided on what its property needs. Five needed
  nothing — `timelinePage` never reads `userByHandle`, so registering `"a"`
  changed no answer — and were deleted (`o4a`, `o4b`, `o4c`, `c4`, `c5`). Two
  are `Refinement.kt`'s `c02`/`k02`, which carry **R5 clause 2**, recorded in
  `spec/refinement/obligations.json` at `layer: "store"`, `op: "CreateUser"`,
  site `(*MemStore).PutUser`. Routing those through the service would turn the
  antecedent *fresh* into *valid and fresh* (the store accepts `"!"`, the
  service rejects it), break the clause-site keying that makes a `go <- kotlin`
  cell mean anything, and rewrite an obligation to accommodate a held-out
  defect. They stay, and say why in place.

  Gate (F031's own instrument), all 18:
  ```
  anchors: 18/18 match exactly one site
  compile: 17/18 build clean

  verify FAILED: 1 mutant(s): kotlin/id-burned-on-reject
           | verification/Refinement.kt:104:22: error: no value passed for parameter 'id'.
           | verification/Refinement.kt:197:22: error: no value passed for parameter 'id'.
  ```
  Before the removals the same command also named `Obligations.kt:195` and
  `:208`. Shown live rather than trusted: re-planting one deleted call in `o4a`
  made `verification/Obligations.kt:195` reappear, and 17 of 18 still build
  clean, so it is a gate and not a blanket refusal. **`mutate verify -impl
  kotlin` does not pass over all 18 and cannot be made to without weakening
  clause 2. That is the honest outcome, not a shortfall.**

  Sweep re-run because the edits change the compiled tree,
  `-out evidence/runs/calibration/kotlin-proof-recovered`: **all 18 cells
  unchanged** against `kotlin-proof/` — 0 killed / 14 survived / 2 unreached /
  2 error, `0/14 = 0%`, every verdict line character for character, and
  `tick-goes-backwards`'s F032 vacuity ERROR byte for byte. That is what step 2
  required: the couplings had to go without changing what the rung measures.

  **F048** — a contract *attached* to a method (Gobra's `// @ ensures` on
  `PutUser`) survives that method's signature change because the edit that
  changes the signature carries it; a contract that must *call* the method does
  not. F045's missing ghost mode, billed a second time and in cells.
  **F049** — the R4 cell is held by an obligation R4 never reads: R4 and R5
  declare the same `SrcDirs`, so clause 2 is in R4's build. Measured, not
  argued — delete `Refinement.kt` from the mutant tree and R4 returns
  `R4 PASSED`, a **survival**, so reclaiming the cell would read `0/15 = 0%`:
  the same zero over a bigger denominator (F024's shape). The obligation that
  would have killed it, `o5d_rejectionBurnsNoId`, is one of the eight JBMC
  blocks. Repair named — give each rung the source set it reads — and **not
  made**: it changes what the F031 gate means for every JVM corner, and the
  Kotlin R5 sweep that would show whether narrowing R5's build moves anything
  has never been run.
  **F050** — two container restarts interrupted the sweep; `-resume` produced
  one `results.json` whose wall column was measured on three machines and
  separates by machine (89.4–96.1 s vs 107.4–117.7 s, no overlap, identical
  verdicts). The tempting reading — the removals made the rung faster — is
  wrong: the three obligations edited are BLOCKED and never run. Do not quote
  that run's seconds; `kotlin-proof`'s remain the citable ones.

  `go build`, `go vet`, `go test ./tools/...` all pass.
  **Next fire: the Kotlin R5 sweep**, `-impls kotlin -rungs R5 -out
  evidence/runs/calibration/kotlin-refinement -resume`. It is the run the two
  `pending` R5 cells wait on, and it is now also the run that decides F049: R5
  will report its own ERROR on `id-burned-on-reject` for a reason that is
  genuinely R5's, and the pair of columns is what shows the R4 loss was
  borrowed. Do **not** narrow the rung builds before that sweep exists — there
  would be no before-picture to compare against.

- **2026-09-02 20:51 (fan-out task, branch `claude/task-kotlin-r4-sweep`)** —
  **The Kotlin corner's 18-mutant R4 sweep is done, the matrix has no `pending`
  cell left, and two proof rungs disagree for the first time.** F011 guard
  first, since a drifted anchor injects nothing and every rung "kills" it:
  ```
  anchors: 18/18 match exactly one site
  compile: 18/18 build clean

  verify PASSED: every anchor matches one site; every mutant compiles
  ```
  Then `calibrate -impls kotlin -rungs R4`, window
  `2026-09-02T20:05:49Z .. 2026-09-02T20:51:21Z`, 2085 s, **nothing else on the
  container** — which is worth saying because most cost figures here are not
  clean (`CALIBRATION-go-proof.md` records load average 6–12 through its window).
  ```
  rung             live  killed  survived  unreached  equiv  killed/reached     killed/live     wall
  R4 proof           16       0        14          2      0       0/14 = 0%       0/16 = 0%    2085s
  ```
  All fourteen survivals produced the same line, character for character:
  ```
  R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator
  ```
  **`every one refutable in this tree` is the vacuity audit, and it is a
  precondition of the verdict rather than a separate pass**: `cmdVerify` refuses
  to start if any obligation lacks a canary, the canary sweep runs on every tree
  nothing refuted (all 16 here), and `R4 PASSED` is emitted only after the
  demotion in `decide()`. No obligation was VERIFIED without a canary naming it —
  **F025 is not recurring.**

  **Two ERROR cells, two different mechanisms, neither a survival.**
  `id-burned-on-reject` → **F031**: the mutant changes `Store.createUser`'s
  arity, `mutate verify` cleared it because the registry build compiles `src`,
  and the rung compiles `src` *and* `verification`, where five call sites still
  pass one argument. `tick-goes-backwards` → **F032**: `appendTweet` *enforces*
  F005's monotonicity premise by throwing, so the negation canary is unreachable
  and the audit will not read the tree —
  ```
  ! c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted (VERIFIED); under vacuity a claim and its negation both verify, so o3b_createdAtNonDecreasing decides nothing (F013)
  R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read (...); nothing was decided about this tree   [2m29.5s]
  ```
  The guard F005 asked for is what costs the corner its one likely kill.

  **The result that matters — F033.** Against `CALIBRATION-go-proof.md` over the
  same 18 ids, on the 12 mutants where both rungs returned a kill-or-survive
  verdict: **agree 4, DISAGREE 8**, all four agreements SURVIVED/SURVIVED. F028
  found R4 and R5 agreeing 18 of 18, but that was one Gobra run read two ways;
  this is two verifiers, two corners, two obligation sets. Cause of the eight:
  four are properties that *are* written and F014 blocks (`o4c`, `o4a`, `o5b`),
  four are properties nothing states. The rung is not broken — the injection
  canary reports `R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s)` — but
  both obligations it fires on are over `dom/Dom.kt`, and **no mutant in the
  catalogue edits `Dom.kt`**, so its demonstrated capability and the catalogue's
  reach do not intersect.

  **Matrix:** the four `pending` cells (`go ← kotlin`, `kotlin ← go`,
  `rust ← kotlin`, `kotlin ← rust`) all take the weaker end's number, Kotlin's,
  `0/14 = 0%`, and all carry `‡`. Census **38 measured, 18 capped, 4 pending,
  12 n/a → 42 measured, 18 capped, 0 pending, 12 n/a**. Every remaining gap is a
  cap, not a to-do. Also corrected in place: F017's prediction that the two
  `next-cursor-*` defects sit in different perimeters per corner is now measured
  — `SURVIVED` on Kotlin, `unreached` on Go — so Go's outer reach is 14/18 and
  Kotlin's 16/18, and the three corners' three denominators of `14` are three
  unrelated quantities.

  **Next fire:** the R4 column is complete except where Java caps it, and Java
  caps it for want of an obligation set, not a tool. Either write
  `impls/java`'s obligations (six capped R4 cells, and F014 says JBMC cannot be
  the tool for them) or take queue item 4 — a catalogue drawn from something
  other than the contract, which is now the only way to learn whether these
  columns discriminate. F031's fix (teach `mutate verify` the rung's build) is
  small and would have saved a 12-minute discovery.
- **2026-09-02 21:05 (worker session `session_01ExaVft3sZPUuETJXKVZK1J`,
  branch `claude/loop-d-homeline-vacuity`)** — **F021's eight undecided clauses
  are decided. None is vacuous.** All three levers F021 named as untried were
  tried, in its order; the two it expected to work did nothing.
  ```
  1. --parallelizeBranches, default shape, 12 min  ->  no verdict,  723 s
  2. 45-minute budget,      default shape         ->  no verdict, 2703 s
  3. one clause at a time, three respelled by hand ->  8 of 8 REFUTABLE, 40-709 s
  ```
  Lever 2's own words, and the reason a driver must never read the count:
  ```
  The verification of package .../internal/store - store got terminated after 2700 seconds
  The verification of member .../store.*MemStore.HomeTimeline(string, int, int64) did not terminate
  The verification of 1 members timed out
  Gobra has found 0 error(s)
  ```
  Zero errors, on a run that verified nothing — and in a negation sweep "no
  errors" is what VACUOUS looks like. `tools/cmd/gobra` reads the wording.
  What worked was `gobra canary -isolate` (elide the member's other eight
  postconditions — sound, a postcondition is a goal and not an assumption, so
  the exit state is unchanged) plus a three-entry hand-canary table for
  negations the generator spells as `forall a int :: !(0 <= a && a <
  len(out))`, which Gobra will not decide, instead of the equal `len(out) ==
  0`, which it will. Corner-wide result:
  ```
  91 clauses: 91 refutable, 0 VACUOUS, 0 timed out, 0 ill-formed
  audited 91 REFUTABLE verdicts: 91 backed by an error inside the clause's own
  member, 0 backed only by an error elsewhere. (0 results were not REFUTABLE.)
  42 clauses: 30 VERIFIED, 12 UNATTEMPTED
  ```
  R5 clauses 15-18 (F1, D10, no-fabrication, no-loss) move UNAUDITED ->
  VERIFIED, each on Gobra's own line, e.g.
  `internal/store/memstore.go:531:9 Postcondition might not hold.`
  Standing rule 2 for the new shape: `gobra canary -control` re-runs every
  canary against a copy of its own member with `assume false` in the body and
  fails the sweep unless it comes back VACUOUS. Nine of nine did, in 23-44 s —
  so the probe that returned those REFUTABLEs is one shown to see vacuity on
  that member. Documented as CANARY I in
  `impls/go/internal/_broken/intentional_failure.gobra.txt`. F029 written;
  F021, ASSURANCE.md, MATRIX.md, OBLIGATION.md §8 and FINDINGS.md corrected.
  **Next fire:** the `reach` probe still returns no verdict on three members
  (`HomeTimeline`, `Replace`, `isMonotoneLog`) and now has an `-isolate` flag
  that has not been run over the corner; re-running it would close the last
  15 clauses that rung reports as unread.

- **2026-09-02 20:45 (fan-out task `claude/task-java-obligations`)** — **The
  Java corner has an obligation set, an R4 rung runs over it, and the R4 column
  has no capped cell left.** `impls/java/verification/twitterport/verification/`
  now carries `Obligations.java` and `Canaries.java`: the twin of the Kotlin
  corner's fifteen obligations, in the same five groups, over this corner's own
  classes. `tools/cmd/jbmc` drives both JVM corners (per-corner compiler,
  `javac --release 17` for Java) and `"java"` is registered in R4's
  `Drivers` alongside `"kotlin"` and `"rust"` — one rung ID, one column, four
  drivers.
  **1. F034 — the shared JBMC wall was an inference and is now a measurement.**
  F014 concluded from a `javac` repro that "this wall is shared with the Java
  corner", and `MATRIX.md` capped six R4 cells on the fact that
  `impls/java` had no obligation set to test it with. Every obligation and every
  canary was run BEFORE any `Blocked` reason was written down
  (`evidence/runs/calibration/java-obligation-probe/goal-lines.txt`). The result
  is identical, obligation for obligation: **7 decidable, 8 blocked, the same 8,
  the same three reasons, 11 own assertion goals on each corner.** Two blockers
  came out sharper than they were recorded: `getBytes` is unconstrained in
  contents as well as length (`assert "alice".getBytes(UTF_8)[0] == 'a'` and its
  negation are BOTH refuted), and the SAT wall is uncapped-kernel-verified at
  13.9 GB RSS, not the 11 GB the Kotlin entry records.
  **2. The gate, both ways, before the row.** Clean tree:
  ```
  R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion
  goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a
  recorded JBMC 6.11.0 limit (F014), in no denominator   [47.4s]
  ```
  One line of `Dom.parseInt64` broken so a bare sign parses as zero:
  ```
  R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s) (2 of 11 own assertion
  goals FAILURE): o1a_oneCharAcceptSet, o1c_emptyAndBareSignRejected; 8
  obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no
  denominator   [40.9s]
  ```
  And the vacuity instrument firing on a real catalogue mutant,
  `java/tick-goes-backwards`, where the enforced monotonicity guard throws and
  makes the claim and its negation both verify:
  ```
  R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read
  (c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted
  (VERIFIED); ...); nothing was decided about this tree   [48.6s]
  ```
  `calibrate` records that as an error cell, never a survival. Three outcomes
  demonstrated, not one.
  **3. F036 — the row is `0/15 = 0%`, and the zero decomposes.** Full sweep,
  `evidence/runs/calibration/java-proof/`, 18 mutants, 812s:
  `R4 proof  live 17  killed 0  survived 15  unreached 2  equiv 0  0/15 = 0%`.
  Nine survivals are obligations JBMC cannot read; three are properties **no
  obligation on either JVM corner states** (F3 idempotence, F6 author
  existence); three are relational obligations a non-relational mutant slips
  past (F023, now on its fourth corner); one is the vacuity above (F015 at the
  proof rung). The sharpest fact in it: **three of the seven decidable
  obligations are over `Dom.parseInt64`, and not one of the eighteen mutants
  edits `Dom.java`.** An origin obligation and a tick obligation would kill two
  of the survivals and are unblocked — deliberately not written, because the
  value of this set this week is that the two JVM corners are twins.
  **4. F035 — one decorative line costs a whole cell.** The obligation set is
  compiled against the tree under test, so it is coupled to every signature it
  *mentions*. `Obligations.kt` opens three timeline obligations with
  `s.createUser("a")`, which `Store.timelinePage` never consults, and
  `id-burned-on-reject` changes that signature: `kotlinc` reports five errors and
  the cell becomes an error cell. Reproduced. The Java twin drops the call from
  all six sites and **all 18 Java mutants then compile against the obligation
  set**. The Kotlin fix is five deletions and was deliberately NOT made here —
  its R4 sweep is running in another lane and changing the tree under a live
  journal is worse than the bug.
  **5. F037 — the vacuity gate is directional.** Both-verify is caught;
  both-refute is not, and both-refute is what a nondeterministic library model
  produces. `o2b_goodHandleIsValid` and `c14_goodHandleIsInvalid` are a strict
  pair and BOTH are refuted on the clean tree today. Delete their `Blocked`
  markers and the unmodified `impls/java` reports
  `R4 FAILED: JBMC refuted 2 of 9 decidable obligation(s) … o2a_emptyIsInvalid,
  o2b_goodHandleIsValid` — a false red with the verdict sentence and the exit
  code agreeing (`java-r4-gate/f037-false-red.log`). What stands between the
  rung and that today is a hand-maintained list, which is F030's shape in the
  direction nobody checked. The fix needs a `Strict bool` on the canary record —
  the witness canaries (`o1a`/`c10`) are *correctly* both-refuted — and it
  changes shared verdict logic, so it is written up and not landed.
  ```
  census   was  38 measured, 18 capped,  4 pending, 12 n/a
           now  42 measured, 12 capped,  6 pending, 12 n/a
  ```
  All 12 remaining capped cells are R5. Two `cap←java` cells became `pending`
  rather than measured: `java ← kotlin` and `kotlin ← java` wait on the Kotlin
  R4 sweep like the other four.
  Also corrected in place, both inside `MATRIX.md` and both wrong before this
  fire: the census said "Of the 18 capped, 6 carry †" when cell by cell there
  were **8**, and the `†` paragraph still said the Go end's R4/R5 evidence "is a
  gate, not a sweep — 5 of 18 … not yet known whether the two rows discriminate
  at all" while the provenance table three sections below recorded `go-proof` as
  a completed 18-mutant sweep with F028's answer to exactly that question.
  **Next fire:** run the Kotlin R4 sweep — it now fills six `pending` cells
  rather than four, and it is still the cheapest cell-filling move on the table.
  Fix `Obligations.kt`'s five `s.createUser("a")` calls first or lose
  `id-burned-on-reject` to F035. After that, F037's `Strict` canary flag is the
  one gate this repository is missing in a direction it has never looked.
- **2026-09-02 20:35 (fan-out task, branch `claude/task-rust-r5-rwlock`)** —
  **The R5 blocker was the lock, and the lock came out of three crates.**
  Rule 3 first: the documented blocker was *reproduced*, not re-quoted. Making
  `MemStore` structurally visible to Verus still gives all five errors
  `ASSURANCE.md` quotes, plus a sixth, and `vstd 0.0.0-2026-04-20-1748` still
  ships no `std_specs/sync.rs`
  (`evidence/runs/verus/rwlock-blocker-reproduction.txt`). Then the refactor
  F012 named in one sentence and nobody had done was done: `crates/ids`,
  `crates/clock` and `crates/store` now hold their state as plain owned value
  types inside a top-level `verus! { … }` block, with the lock pushed out to a
  thin type that forwards. **`abs_users`, `abs_follows` and `abs_tweets` have
  bodies.** Verus's own lines, before and after:
  ```
  before:  domain 9  store 6  ids 0  clock 2  service 4
           R4 PASSED: verification results:: 21 verified, 0 errors over 5 of 5 verify-enabled crate(s)
  after:   domain 9  store 9  ids 5  clock 5  service 4
           R4 PASSED: verification results:: 32 verified, 0 errors over 5 of 5 verify-enabled crate(s)   [5.8s]
  ```
  `crates/ids` went from **zero** obligations to five: F8 is on `Counter::next`,
  the transition `Generator::next_id` executes, not on `next_id_ensures`, an
  `external_body` function with an `unimplemented!()` body that nothing called.
  `store` went 6 → 9 *while three twins were deleted*, so they are a different
  nine. Of R5's three obligations, `abs(init) == init_S` is discharged on all
  three axes and state commutation on `put_user` / `put_follow` / `put_tweet` —
  17 clauses, on shipped functions, in R5's own vocabulary. **F041.**
  Rule 4, every clause moved onto a shipped function shown non-vacuous before
  it is claimed — `verus canary`, its own last lines:
  ```
  self-test ... -> VACUOUS [6s]   self-test PASSED
  baseline  R4 PASSED: verification results:: 32 verified, 0 errors over 5 of 5 verify-enabled crate(s)   [2m28.7s]
  canary sweep: 37 clause(s)   REFUTABLE 37   VACUOUS 0   ILL-FORMED 0   TIMEOUT 0
  ```
  **The instrument itself was wrong, and the tree that exercised it found out.**
  Two `broadcast proof fn … { admit(); }` axioms were entering the sweep as
  *shipped obligations*, under the name `fmt` from an unrelated `impl Display`
  sixty lines away, because `fnName` did not know Verus's `proof`/`spec`/
  `broadcast` modifiers. An admitted body proves every postcondition, so the
  canary would have printed VACUOUS as a tautology about `admit()` dressed as a
  finding — the F016 mistake rebuilt inside the instrument built to catch it.
  `splitBlocks` now returns four categories and prints ghost and assumed blocks
  by name on every run. **F042.** Re-measuring the pre-lift tree with the fixed
  classifier makes the comparison apples-to-apples: **5 shipped / 36 twin / 21
  assumed → 37 / 20 / 13.** So F030's "57 twins" was 36 twins plus 21 assumed
  clauses; F027 and F030 are corrected in place.
  **The next blocker down is named and is not the lock.** `vstd`'s
  `View for String` is `uninterp`, so nothing says `s@ == t@ ==> s == t`, and
  Verus refuses `result is Ok ==> !abs_users(old(self)).contains(u.handle@)`
  by name. Every read on this corner reduces to a membership test, so R5's
  response axis is blocked on that. The deleted twin stated that direction only
  because `proof_users_contains`, an `external_body` shim, assumed it — **a
  twin can look stronger than the shipped contract that replaces it, and
  deleting it lowers the count while raising the evidence. F043.**
  **The two `go ↔ rust` R5 cells did NOT change and MATRIX.md is unchanged in
  that column.** `tools/cmd/calibrate/rungs.go` hard-codes R5 as `Tool:
  "gobra"` with a Go-only file list, so a cell is not available without a Verus
  R5 rung, which is separate work and was not done here. What changed is the
  *reason* the cells are capped, and `ASSURANCE.md` had been conflating "the
  cells are capped" with "it is structurally impossible" in one sentence.
  Behaviour unchanged and checked: `cargo test --workspace` 17/17 ok,
  `replay -impl rust` **R0 PASSED, 56/56 exact, 0 differ**.
  `TestShippedClausesAreExactlyFollowNew` fired as designed and is replaced by
  `TestShippedClausesCoverTheLiftedCrates`, shown able to fail by reverting
  `crates/ids` to its pre-lift source.
  **Next fire:** (a) write the Verus R5 rung in `calibrate` — that is what
  turns 17 discharged clauses into a matrix cell; (b) re-run the Rust mutant
  sweep, since F027's 1-kill-in-14 was measured against a tree where only
  `domain` had shipped contracts and that is no longer true; (c) lift
  `crates/service`, the last twin holdout, expecting it to cost more than
  `store` did because its write mutex spans allocate-then-write (F018).
- **2026-09-02 20:15 (fan-out task, branch `claude/task-dom-separation`)** —
  **The R4 and R5 columns disagree for the first time, in both of the two ways
  they can.** F028 measured 18 mutants x 2 rungs and got zero disagreements,
  then said why: R4's perimeter is the five Gobra-verified packages, R5's is the
  four files carrying a refinement clause, the only file between them is
  `internal/dom/dom.go`, and no shipped mutant is confined to it. The columns had
  never been given the chance. `evidence/experiments/r4-r5-separation/` is a
  **scratch** catalogue that gives it — its ids stay out of
  `tools/cmd/mutate/mutants.json`, whose ids are shared across four corners so a
  defect can be compared port-to-port, and a Go-only id would shift every
  published denominator. Both gates passed and **all three of their arms were
  shown refutable first** (`canaries/`): `anchors: 0/1 match exactly one site`,
  `compile: 0/1 build clean`, `verdict NO OBSERVABLE CHANGE in 537 requests`.
  Worth recording that both `verify` canaries printed `exit status 1` into a
  pipeline whose own exit code was `0` — standing rule 1, in one line.
  **F038 — separation by REACH.** Two mutants confined to `dom.go`
  (`ValidHandle` accepts `A..z`; `ValidText` drops the control-character guard),
  each breaking a loop invariant that is the only place its property is proved,
  because Gobra rejects the string indexing a postcondition would need:
  ```
  go/handle-alphabet-widened      R4 killed     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m15.2s]
  go/handle-alphabet-widened      R5 unreached  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m10.5s]
  go/text-control-chars-accepted  R4 killed     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m3.3s]
  go/text-control-chars-accepted  R5 unreached  R5 PASSED: 1 failing obligation(s), none in a member carrying a refinement clause   [1m3.4s]
  ```
  The reach half is bookkeeping — `r5Files` is hand-maintained, so of course a
  `dom.go` mutant is outside it. **The content is that R4 actually kills:** the
  wider perimeter catches a defect that registers `"Alice"` and one that posts a
  NUL, and the narrower perimeter has no obligation for either.
  **F039 — separation by VERDICT, which is the stronger claim.**
  `clock-now-off-by-one` makes `(*clock.Logical).Now` return `l.now + 1`. It sits
  in `internal/clock/clock.go`, which **is** in `r5Files`, on a member carrying no
  refinement clause (`clock.go`'s one clause site is on `Tick`). `R4 killed` /
  **`R5 SURVIVED`**, and R5's `killed/reached` denominator stops being empty at
  `0/1`. `gobra r5verify` by hand:
  `internal/clock/clock.go:52 (*Logical).Now      not an R5 clause: unfolding acc(l.LockP()) in result == l.now`.
  No list was consulted to get there. What it costs is now visible too: a live,
  observable, spec-violating defect — every tweet's `created_at` one ahead — that
  the R5 row passes. R5 is the **narrower** row, never "R4 plus more".
  **F040 — found while reading Gobra's own error, and left unfixed on purpose.**
  `memberSpans` ends a member's span at the next line that is exactly `}`, so a
  one-line function body (`func (e invalidHandleError) Error() string { ... }`)
  swallows everything up to the next bare brace, and `memberAt` resolves the
  resulting overlap by **unordered map iteration**. That is why the F038 run
  printed `dom.go:206 (*invalidHandleError).Error` for an error inside
  `ValidHandle`. It flips no verdict recorded so far and the bound is derived,
  not assumed: the only overlaps in the four clause-carrying files are three
  clause-free error constructors in `memstore.go` 70–89, and `clock.go`, `ids.go`
  and `service.go` have none. But the condition making it harmless is an accident
  of source layout that nothing tests for, and one one-liner above a
  clause-carrying member would produce a **random false survival** in the R5
  column. `audit.go` is shared machinery five concurrent agents are measuring
  against, so this branch reports it with a proposed patch and a reproduction
  probe rather than changing the instrument mid-measurement.
  **Next fire:** apply F040's fix (clamp each member span to the next member's
  start; break ties narrowest-first) together with the assertion form of
  `evidence/experiments/r4-r5-separation/memberspans-overlap-probe_test.go.txt`,
  which fails on the current tree and is therefore the gate proving the fix
  landed. The `attribution` family of F039 is a template: any member in a
  clause-carrying file with a functional postcondition and no clause site gives
  another verdict-separating cell, and the three `// @ trusted` shims are not
  candidates because R4 would survive them too.

- **2026-09-02 19:45 (parent session `session_01ExaVft3sZPUuETJXKVZK1J`, branch
  `claude/goal-loop`)** — **The Rust corner had no vacuity instrument at all,
  and the R4 column is no longer entirely capped.** Two results, plus one
  experiment set up and one sweep started and deliberately stopped.
  **1. F030 — an injection canary was standing in for the instrument the rule
  asks for.** This section's own definitions say a proof-backed kill counts only
  if the obligation was shown non-vacuous, and that "the equivalents for Verus
  and JBMC must exist before their rows are trusted". Checked against the three
  drivers: Go has `canary`/`reach`/`audit`; Kotlin's JBMC rung carries a
  negation canary per obligation and goes UNDECIDED without one; **`grep -n
  "vacu\|VACUOUS\|negat\|canary" tools/cmd/verus/*.go` returned nothing.** What
  stood in its place was an *injection* canary — inject `false,` and check the
  gate notices — recorded in the 18:40 entry below with the words the rule uses.
  F013 is the finding about why that is a different check. So the Rust R4 row
  quoted in F027 and in PR #4 was reported under a rule it did not meet.
  `tools/cmd/verus canary` is now the instrument: for each `ensures` clause on a
  shipped function it replaces the clause list with the negated ANTECEDENT (for
  `A ==> B` the canary is `!A`; `!(A ==> B)` is `A && !B`, refutable exactly
  when the clause is vacuous, which scores every vacuous clause as live) and
  asks Verus to prove it. Its own output, verbatim:
  ```
  self-test (does this sweep report VACUOUS when it should?) --
    with `requires false,` spliced in ... -> VACUOUS [4s]
    self-test PASSED: the sweep reports VACUOUS when the obligation is unreachable

  baseline  R4 PASSED: verification results:: 21 verified, 0 errors over 5 of 5 verify-enabled crate(s)   [2m17.5s]
  canary sweep: 5 clause(s)   REFUTABLE 5   VACUOUS 0   ILL-FORMED 0   TIMEOUT 0
  ```
  The row survives its audit. The baseline's 21 is the post-repair count F024
  recorded, not a new disagreement. **The number that matters more than the zero
  is what it is not sweeping:** `ensures blocks: 1 on shipped functions, 15
  inside #[cfg(verus_only)] mod verus_proof` / `clauses: 5 shipped, 57 twin`.
  F027 established the shape of that split by reading the code; this derives the
  **ratio** mechanically, with a test that fails the day a clause moves out of a
  twin. "The Rust R4 row is not measuring vacuous obligations" is a claim about
  **8% of the corner's obligations**.
  Also adds the gate that would have caught this —
  `TestEveryProofRungCornerHasAVacuityInstrument` — and shows it failing with
  `canary.go` moved aside. **Its first version did not fail:** it looked for the
  bare word `VACUOUS`, and the driver's doc comment mentions vacuity in prose,
  so the test was satisfied by the tool *describing* the instrument it lacked —
  the same substitution the test exists to catch, reproduced inside the test
  within ten minutes. It now requires the quoted string literal, which is a
  verdict a tool can return rather than a claim it can make.
  **2. `evidence/MATRIX.md` was stale after the fan-out merge, and correcting it
  opens the R4 column.** It still said "R4 (Rust end) *does not exist*", "R4
  (Java, Kotlin ends) *does not exist*", "Only Go has an R4 or R5 rung", and it
  called the Go corner's evidence a 5-of-18 gate. Four false statements sitting
  in the tree that contradicted them. R4 caps were justified by "corner X has no
  rung of this kind that yields a kill verdict"; three corners have one now, so
  R4 is capped only where **Java** is an end — and Java's cap is that
  `impls/java` carries no obligation set for any rung to run, not anything about
  JBMC.
  ```
  census   was  36 measured, 24 capped,  0 pending, 12 n/a
           now  38 measured, 18 capped,  4 pending, 12 n/a
  ```
  The two new cells are `go ← rust` and `rust ← go`, both **1/14 = 7%**, taking
  Rust's number under this document's existing weaker-end rule (Go kills 9 of
  the 14 its proof reaches; Rust kills 1 of its 14). Both carry a new `‡` mark,
  and the mark matters more than the number: **the two 14s are not the same
  denominator.** Go's is set by the trusted shim (F022); Rust's by where its
  contracts were written (F027, quantified above). `1/14` is a fact about the
  Rust corner's proof layout, not about the quality of a Go/Rust port — the same
  catalogue kills 18 of 18 on both corners at R0. R4 is also now the first
  column whose measured cells are not all the same value, so the old "every
  measured cell in a column is the same number" line is narrowed to R0–R2.
  R5 is unchanged and still entirely capped.
  `ASSURANCE.md`'s R4 row updated too — licensed now that the canary has run,
  which is what the standing instruction was waiting on — and bounded: one
  property, F4, on one shipped function, five clauses each shown non-vacuous,
  and explicitly not the four crates whose obligations are on twins.
  **3. Set up but NOT run: two mutants confined to `internal/dom/dom.go`**
  (`evidence/experiments/r4-r5-separation/manifest.json`). F028 said R4 and R5
  agreed on all 18 cells and that the only file separating their perimeters is
  `internal/dom/dom.go`, which no catalogue mutant is confined to — the columns
  were never given a chance to disagree. These two break a loop invariant that
  is the only place its property is proved (`ValidHandle`'s alphabet,
  `ValidText`'s character range; neither is restatable in a postcondition
  because Gobra rejects string indexing). Deliberately a scratch manifest, not
  `mutants.json`: catalogue ids are shared across all four corners so the kill
  table can compare a defect port-to-port, and a Go-only id would break that
  symmetry and shift every published denominator to settle a question about one
  rung pair. Both gates green — `verify PASSED: every anchor matches one site;
  every mutant compiles` and `probe PASSED: every mutant answers some request
  differently from the original`, with witnesses (handle `Alice` registers 201
  where the original answers 400; text with a NUL posts 201 where the original
  answers 400). **Expected outcome: R4 KILLED, R5 UNREACHED** — `internal/dom/`
  is in `gobraVerified` and `internal/dom/dom.go` is not in `r5Files`. If R4
  instead SURVIVES, the extra perimeter buys nothing and that is the more
  interesting answer.
  **4. Started and deliberately stopped: the Kotlin corner's 18-mutant R4
  sweep**, which is what all four `pending` cells wait on. It got as far as
  `mutate verify` before I killed it. **Reason: it was corrupting a measurement
  in flight.** The box has 4 cores; the concurrent HomeTimeline vacuity sweep
  holds it at load 3.5–4.4 alone, and starting the Kotlin sweep took it to 9.4
  in two minutes. That sweep's clause 4 then reported `TIMEOUT 723s` against a
  720s budget — the thinnest possible margin, and exactly what a 4-core box at
  load 9 produces. A timeout caused by load recorded as a solver result would be
  a false F021. `calibrate -resume` loses nothing, so the sweep restarts from
  its journal.
  `go build`, `go vet`, `go test ./tools/...` clean after every commit.
  **Next fire, in this order.** (a) **Finish the Kotlin R4 sweep** —
  `go run ./tools/cmd/calibrate -impls kotlin -rungs R4 -out
  evidence/runs/calibration/kotlin-proof -resume`, ~18 x 2 min, and it fills
  four matrix cells, the cheapest cell-filling move on the table. Run it with
  nothing else on the box. (b) **Run the two `dom.go` mutants** at R4 and R5,
  `-manifest evidence/experiments/r4-r5-separation/manifest.json -impls go
  -rungs R4,R5`, ~3 min each, and close F028's open question. (c) Only then
  queue item 4. Do NOT quote any wall-clock cost measured while another sweep
  was running.
- **2026-09-02 18:49 (worker session `session_01ExaVft3sZPUuETJXKVZK1J`, fire
  17:30)** — **The Go corner's full deductive sweep: DONE. 18 mutants x R4,R5,
  36 cells, 52 minutes.** This is queue item 2 narrowed to the one corner where
  both proof rungs exist, and it answers the question the previous fire raised.
  Catalogue checked undrifted first, per F011:
  ```
  verify PASSED: every anchor matches one site; every mutant compiles
  ```
  ```
  rung             live  killed  survived  unreached  equiv   kill%reach  wall
  R4 proof           18       9         5          4      0        64%    2043s
  R5 refinement      18       9         5          4      0        64%    1059s
  ```
  **The interesting number is 0. R4 and R5 disagree on nothing — 0 of 18.**
  Same nine kills, same five survivals, same four unreached, cell for cell.
  Five mutants agreeing was a gate; eighteen agreeing is a result, and it is
  **F028**. It is not the construction: `rungs.go` deliberately withholds R4's
  kills from R5, and the previous fire's canary separates them in both
  directions. Two halves, both coming out the same way. *Reach* — the only
  file separating R4's perimeter (5 verified packages) from R5's (4
  clause-carrying files) is `internal/dom/dom.go`, and **no mutant is confined
  to it**; re-derived from the manifest, mutants where R4 reach != R5 reach: 0.
  *Verdict* — all 9 R4 kills landed inside a member carrying a refinement
  clause, 4 on the clause line itself and 5 elsewhere in the member. Those 5
  are the `HomeTimeline` loop-invariant path, so **5 of the 9 agreements rest
  on the clause-or-member widening the previous fire made**; under the
  clause-only reading they would have been `R5 UNDECIDED`, not a disagreement.
  Verdict lines, verbatim, two representative kills and one survival:
  ```
  go/tick-goes-backwards
     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [48.9s]
     R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [50.1s]
  go/limit-off-by-one
     R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [32.9s]
     R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [35.5s]
  go/id-first-is-two
     R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m26s]
     R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m13.9s]
  ```
  Coverage scored per F022: the 4 unreached are exactly the 4 `internal/httpshim`
  mutants F022 predicted, nothing moved into or out of that set, and R4's
  ceiling on this corner stays 14 of 18.
  **Caveat that leads `evidence/CALIBRATION-go-proof.md`:** R4 and R5 are the
  *same* Gobra invocation read two ways, so their agreement is not independent
  corroboration and **their cost rows are not comparable**. The 2043s vs 1059s
  gap is one cell — `go/orphan-author-accepted` took **1033.1s at R4 and 61.9s
  at R5 on the same tree with the same verdict**, 971s of the 984s difference.
  F019 said a verifier's obligation count is a measurement with a range; its
  wall time has a much wider one. Second caveat: the box was shared, load
  average 6-12 throughout, so all wall figures are inflated uniformly.
  `evidence/CALIBRATION-go-proof.md` written with every verdict line verbatim;
  raw journal, results and console in `evidence/runs/calibration/go-proof/`.
  F028 added to `evidence/FINDINGS.md`, which also gains a third form under
  Pattern 3: a canary is about one rung, a rate is about a row.
  `go build`, `go vet`, `go test ./tools/...` clean; no code changed.
  **Next fire: queue item 4, or queue item 1's Rust sub-step — and F028 says
  which.** The R5 row cannot be told apart from R4 by anything in this
  catalogue, so building a second verifier's plumbing (Rust/Verus) adds a
  third column that will be measured against the same catalogue that could not
  separate the first two. The higher-value move is **a mutant confined to
  `internal/dom/dom.go`** — one cell, ~3 minutes, and it either separates the
  R4 and R5 columns on the first try or shows the perimeters coincide in fact
  as well as on this catalogue. Do that before Rust. Do NOT delete the R5 row:
  the canary works and the rung answers a different question.
- **2026-09-02 18:40 (worker session `session_01ExaVft3sZPUuETJXKVZK1J`,
  branch `claude/loop-b-verus-rung`)** — **Queue item 1, third sub-step: DONE.
  R4 is a `calibrate` rung on the Rust corner.** `tools/cmd/verus` mirrors
  `tools/cmd/gobra`'s contract exactly: `-registry` so it resolves the tree
  calibrate's guard hashed, `-budget` whose exhaustion prints `R4 UNDECIDED`
  and no verdict, and a verdict sentence carrying Verus's own words.
  Gate, `-impls rust -rungs R4 -ids self-follow-guard-dropped,next-cursor-is-first-id`:
  ```
  rust/self-follow-guard-dropped  R4 FAILED: verification results:: 8 verified, 1 errors over 1 of 5 verify-enabled crate(s)   [83.5s]   killed
  rust/next-cursor-is-first-id    R4 PASSED: verification results:: 23 verified, 0 errors over 5 of 5 verify-enabled crate(s)  [104.5s]  unreached
  R4 proof   live 2  killed 1  survived 0  unreached 1  equiv 0   kill%reach 100%  kill%live 50%
  ```
  Canary (standing rule 2), the same materialised tree twice, one injected
  line apart — `false,` added to `Follow::new`'s `ensures` list:
  ```
  1. untouched                     R4 PASSED: verification results:: 23 verified, 0 errors over 5 of 5 verify-enabled crate(s)   [1m44.4s]  exit 0
  2. + `false,` on Follow::new     R4 FAILED: verification results:: 8 verified, 1 errors over 1 of 5 verify-enabled crate(s)    [1.1s]    exit 1
  3. reverted                      R4 PASSED: ... 23 verified, 0 errors over 5 of 5 ...                                          [4.1s]
  4. run again, tree unchanged     R4 PASSED: ... 23 verified, 0 errors over 5 of 5 ...                                          [4.3s]
  ```
  Run 4 is the **cargo-cache defence** and is part of the canary, not
  decoration: the same invocation without the driver's `touch` finishes in
  0.3s, prints `Finished dev profile` and NOT ONE `verification results::`
  line, and exits 0. Anything reading the error count scores that as a clean
  pass over a tree nothing looked at. The driver touches every `.rs` file in
  the verify-enabled crates and, as belt and braces, refuses a PASSED verdict
  unless all five crates reported — if the touch ever stops working the rung
  goes UNDECIDED rather than green.
  **F027 written, and it is the substantive result of this fire.** Every one
  of the 14 Rust mutants that edits a verify-enabled crate was run: **1 killed,
  13 PASSED with the obligation count unmoved at 23.** All 18 are live (R0
  kills 18/18, 0 equivalents). The reason is structural: `crates/domain` is the
  only verified crate whose `verus! { ... }` block encloses the shipped items,
  so `Follow::new` is the only production function carrying a clause. The other
  four put every obligation in `#[cfg(verus_only)] mod verus_proof` — separate
  hand-written functions (F012) over `external_body` shims (F016) — and a
  mutant editing production code there leaves the twin untouched and verifying.
  **So a Rust R4 kill means "a clause on the shipped function broke" only in
  `crates/domain`, and nowhere else; the twin-broke reading never arises,
  because no catalogue mutant is anchored inside a proof module.** Ceiling
  comparison: Go 14/18 = 78% (F022, set by the trusted shim), Rust 1/18 = 6%
  (set by the twin split). `calibrate` scores the 13 as *survived*, which
  overstates them — the contract never had a chance — and the file-level
  `Covers` predicate cannot see the difference, because on this corner the
  proof and the code it stands for are in the same file. Left at file
  granularity deliberately: reclassifying 13 survivors as unreached would hide
  the result.
  Plumbing note: `rungs.go` now allows several entries per rung ID (Gobra on
  go, Verus on rust); `splitRungs` caps an ID only when no entry applies to the
  corner and unions the entries' `Impls` for the cap message, and `reportRungs`
  collapses the entries to one table column — without it the first gate run
  printed the R4 column twice and doubled its aggregates.
  `mutate verify` unchanged and undrifted:
  `verify PASSED: every anchor matches one site; every mutant compiles`
  (72/72 build clean). `go build`, `go vet`, `go test ./tools/...` all pass;
  10 new tests, including the cache trap, the partial-run trap, the
  budget/UNDECIDED path, and one that re-derives the verify-enabled crate list
  from `impls/rust`'s manifests so the R4 denominator cannot go stale.
  **Next fire: queue item 1's last sub-step, R4 for Kotlin+Java/JBMC** — unless
  the Go R4+R5 sweep from the previous entry is still unfinished, which takes
  priority. Whichever runs, `evidence/MATRIX.md` must carry F027 beside the
  Rust R4 cell: an unqualified `R4 78%` next to `R4 7%` reads as a claim about
  the verifiers, and it is a claim about where two projects put their
  contracts.
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
  **F026 written**, and it changes what the next fire should expect: **all 24
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
  not as a matrix cell (F026).
- **2026-09-02 18:20 (worker session `session_01ExaVft3sZPUuETJXKVZK1J`, branch
  `claude/loop-c-jbmc-rung`)** — **Queue item 1, fourth sub-step: DONE. R4 is a
  `calibrate` rung on the Kotlin corner, driven by JBMC.** New tool
  `tools/cmd/jbmc` (`verify`, `list`), mirroring `tools/cmd/gobra`: `-registry`
  so it resolves the same mutant tree the guard hashed, `-budget` whose
  exhaustion prints `R4 UNDECIDED` and **no verdict** (an error cell, never a
  survival). `rungs.go` gained a `Drivers map[string]driver` so ONE R4 entry
  dispatches per corner — Gobra on go, JBMC on kotlin — rather than two rungs
  putting the same question in two columns. Clean tree, for the record:
  `R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own
  assertion goals FAILURE), every one refutable in this tree; 8 obligation(s)
  blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator [1m52.5s]`.
  Gate, `-impls kotlin -rungs R4` over five mutants
  (`evidence/runs/calibration/kotlin-r4-gate/`):
  ```
  kotlin/id-first-is-two              R4 PASSED: JBMC verified 7 of 7 ...   survived
  kotlin/timeline-scan-reversed       R4 PASSED: JBMC verified 7 of 7 ...   survived
  kotlin/created-at-frozen            R4 PASSED: JBMC verified 7 of 7 ...   survived
  kotlin/tick-goes-backwards          R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read   ERROR
  kotlin/unknown-json-fields-accepted R4 PASSED: JBMC verified 7 of 7 ...   unreached (httpshim)
  R4 proof   live 4  killed 0  survived 3  unreached 1  equiv 0   kill%reach 0%  kill%live 0%
  ```
  **Read that row honestly: the rung killed nothing over these five, and that
  is the result, not a defect in the harness.** The canary shows it can kill;
  the catalogue is where it cannot. Three separate reasons, and the row
  separates them: the timeline mutant survives because the obligations that
  cover the timeline are the ones F014 blocks; `tick-goes-backwards` is
  UNDECIDED because the store's own guard *throws*, which makes the assertion
  after it unreachable and the obligation **and its negation** both verify
  (F015 meeting F013); `id-first-is-two` survives because every id obligation
  is relational, which is F023 reproduced on a second corner with a different
  verifier. The httpshim mutant is `unreached`, F022's accounting one corner
  over.
  Canaries (standing rule 2), both directions, because an injection canary
  cannot see vacuity at all — the injected defect is downstream of the
  infeasible point too:
  ```
  A. injection: one line of Dom.parseInt64 so a bare sign parses as 0
       R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s) (2 of 11 own
       assertion goals FAILURE): o1a_oneCharAcceptSet,
       o1c_emptyAndBareSignRejected; 8 obligation(s) blocked ...  [1m33.1s] exit 1
  B. vacuity: Store.appendTweet reverted to log.lastOrNull(), the F013 defect
       decidable 7   VERIFIED 4   REFUTED 0   VACUOUS 3   UNDECIDED 0
       R4 UNDECIDED: 3 of 7 decidable obligation(s) could not be read
       (c2_idsDoNotIncrease guards o3a_idsStrictlyIncrease and was NOT refuted
       (VERIFIED) ...)  [1m37.5s] exit 1, and NO verdict line
  ```
  Canary B is the one that matters and it works: all three obligations report
  every own assertion goal SUCCESS, and the rung still refuses to call it a
  proof. Logs in `evidence/runs/calibration/kotlin-r4-canary-{injection,vacuity}.log`.
  **F025 written**, in two halves. (i) `Canaries.kt` had **no canary at all**
  for three of the seven claimed obligations, so F014's "7 VERIFIED" was four
  audited claims and three unexamined ones — the sweep was indexed by canary
  rather than by claim, so the one gate built to catch F013 could not see its
  own blind spot. `c10`/`c11`/`c12` added; all three refuted, so the number
  stands and is now earned. (ii) The audit's **price**, measured across
  corners: negating a bounded obligation costs what the obligation costs
  (3-7 s), negating a deductive one is strictly harder and sometimes does not
  terminate (F021) — so the weaker rung can afford the stronger audit, which
  is not the ordering the ladder suggests.
  `mutate verify` re-run: `verify PASSED: every anchor matches one site; every
  mutant compiles` (72/72 anchors, 72/72 build clean). `go build`, `go vet`,
  `go test ./tools/...` clean; 12 new tests covering verdict parsing, the
  budget/UNDECIDED path, blocked-obligation accounting, the demotion of an
  unaudited claim, the per-corner dispatch, and the exact sentences calibrate
  reads.
  **Next fire: the Kotlin corner's full R4 sweep**, all 18 mutants,
  `-out evidence/runs/calibration/kotlin-proof -resume` (~2 min per cell plus a
  22 s warm build; it journals per cell so a restart resumes). Five is a gate,
  not a rate, and the claim this fire could not finish is the one worth
  measuring: **whether the 7 decidable obligations kill anything at all in the
  catalogue.** If the answer is zero, say so — that is the sharpest number this
  corner can produce, and it prices fixing JBMC against writing more Kotlin
  obligations. Do not start the Java corner: it needs an `Obligations.java`
  first, which is a corner-build task, not a rung task.
- **2026-09-02 17:45 (worker session `session_01ExaVft3sZPUuETJXKVZK1J`, branch
  `claude/loop-e-verus-twins`)** — **Queue item 5: DONE. The four drifted Verus
  twins are fixed or deleted, and the count went DOWN.**
  ```
  before   ids 0   clock 2   domain 9   store 7   service 5     = 23 verified, 0 errors
  after    ids 0   clock 2   domain 9   store 6   service 4     = 21 verified, 0 errors
  ```
  Every twin was read against its production function, not taken from the
  findings on trust; F016's list of four is confirmed exactly.
  - `service::create_user_ensures` — **FIXED.** It guarded on
    `handle.as_str().is_empty()` where production guards on
    `!domain::valid_handle`, so its accept clause was **false of the shipped
    code** for `"Alice"` (corpus step 5) on a corner passing 56/56. Fixed
    rather than deleted because it can be made true without touching
    production: a `handle_valid` spec predicate pinned by a shim whose body
    IS `domain::valid_handle(h.as_str())`.
  - `service::follow_ensures` — **FIXED.** Body called `Follow::new` before the
    existence checks: the pre-D4 ordering, the F003 defect. Body now mirrors
    production, and two clauses name **which** error, so the ordering is
    visible to Verus for the first time — F016 had shown the old contract
    could not distinguish the two orderings at all.
  - `store::home_timeline_ensures`, `service::home_timeline_ensures` —
    **DELETED.** Zero `ensures` clauses each, over a drifted cursor-less second
    copy of the timeline walk. An obligation with no postcondition cannot be
    refuted; deleting it removes the count and no guarantee.
  **Verus caches** — a second run over an unchanged tree prints no
  `verification results::` line at all, which reads exactly like a pass. Every
  run above was preceded by `touch` on the crate sources.
  Canaries (standing rule 2), three of them:
  ```
  assert(false) in create_user_ensures    error: assertion failed          -> 3 verified, 1 errors
  pre-D4 body restored in follow_ensures  error: postcondition not satisfied (D6 and D4 clauses) -> 3 verified, 1 errors
  is_empty() guard restored in create_user_ensures  error: postcondition not satisfied -> 3 verified, 1 errors
  ```
  `cargo test --workspace` 94 passed / 0 failed.
  `go run ./tools/cmd/replay -impl rust`:
  `R0 PASSED: every step matches S_obs byte-for-byte` (56/56 exact).
  **Two corrections in place (F020 rule):** `OBLIGATION.md` §7 stated the
  `put_tweet_ensures` drift in the present tense when commit `4bc2706` had
  already fixed it in the same commit that wrote the sentence — the same
  claim-contradicts-its-own-commit shape F020 named. And `obligations.json`
  recorded R0 as 54/54 for both Go and Rust; the corpus is 56 steps and both
  corners re-measure at 56/56. New finding: **F028**.
  **Next fire: queue item 1's remaining sub-step — R4 as a `calibrate` rung on
  the Rust corner.** The verdict line and the cache-defeating `touch` are now
  both established here, which is what that sub-step said it was missing.
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
4. ~~**Fix or delete the four drifted Verus twins.**~~ **DONE 2026-09-02**
   (`claude/loop-e-verus-twins`, `evidence/findings/F024`). Two fixed
   (`create_user_ensures`, which was actively false of shipped code, and
   `follow_ensures`, which encoded the pre-D4 defect), two deleted (both
   `home_timeline_ensures` carried no `ensures` clause at all). Verus
   `23 -> 21 verified, 0 errors`. What is left in this corner is
   `ids::next_id_ensures`: an `external_body` function with an
   `unimplemented!()` body, contributing 0 verified units while F8 depends on
   it. That needs the counter lifted out of the `Mutex`, not a better
   contract. **The lift was done 2026-09-02**
   (`claude/task-rust-r5-rwlock`, `evidence/findings/F041`):
   `next_id_ensures` is deleted, F8 is on `Counter::next` — the transition
   `Generator::next_id` executes — and `crates/ids` reports 5 verified.
   `crates/clock` and `crates/store` got the same lift; `crates/service` is
   the last twin holdout.
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
