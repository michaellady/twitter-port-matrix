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
6. **Local only.** No remote, no `.github/`, no CI, nothing pushed. The four
   source repos under `~/dev/` are read-only inputs.
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

## STATE — session ended here

**Published:** https://github.com/michaellady/twitter-port-matrix (public)
**Stories:** 20 of 24 done · **Findings:** 18 · **Corners:** 4, all R0 56/56

### The deliverable exists

`evidence/CALIBRATION.md` — 36 mutants x 3 rungs, both original corners:
R0 100% @ 57s · R1 100% @ 1465s · R2 42% @ 2495s. Caveat leads the document:
the catalogue and the corpus both derive from `S_obs`, so the 100%s partly
measure that alignment.

`evidence/FINDINGS.md` — the through-line across all 18.
`evidence/TRANSFER-to-websocket-port.md` — the write-up this was built for.

### Where each corner stands

| corner | R0 | R1 | R2 | R4 | limited by |
|---|---|---|---|---|---|
| Go | 56/56 | clean | pass | Gobra, ~43 load-bearing clauses | Gobra ghost-language limits |
| Rust | 56/56 | clean | pass | Verus, **1 property** (F016) | `RwLock` has no vstd model |
| Java | 56/56 | clean | pass | not attempted | JBMC string equality (F014) |
| Kotlin | 56/56 | clean | pass | JBMC, 7 of 15 | same JBMC defect |

### Highest-priority outstanding work

1. **Fix the concurrency defect (F018).** Live, in the Go corner, now public.
   Id allocation sits outside the store lock; under load the F005 guard turns a
   race into HTTP 500 and a silently dropped tweet. Fix is to allocate and
   append atomically. **I introduced this.**
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
- The environment is pinned because several findings **are** properties of a
  specific build (F014 is a CBMC 6.11.0 defect; F012's blocker is a property
  of one vstd release).
