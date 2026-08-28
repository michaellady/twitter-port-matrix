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
- [ ] **1d** Same for Rust in `impls/rust/`
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

## STATE

**Phase:** 1
**Next step:** 1d(ii) — retarget the Rust implementation (vendored; R0 baseline 15/54)
**Last updated:** 2026-08-28

**Gates currently green:**
`matrixctl doctor` · `matrixctl spec check` (4/4)

**R0 status:** go **54/54 byte-exact** (was 7/54). Canary verified: reintroducing
the F003 defect fails exactly one step and reverting restores green.

**Blocked / waiting:** nothing

**Notes for the next iteration:**
- 1c is done. The store's `// @ trusted` markers went 10 -> 4; the six that
  went were putFollowEdge, deleteFollowEdge, appendTweet, iterFollows,
  gatherTimeline and sortTimeline, all of which existed only because of the
  nested map shapes. The four remaining are two error constructors and
  Snapshot/Replace.
- F005: enforcing the log invariant in PutTweet is what makes F2 genuinely
  derived. Carry the same enforcement into the Rust corner in 1d — the lemma
  has the same two premises there.
- F006: Go and Rust diverge from S_obs on EXACTLY the same 39 steps, and on
  30 of those they agree with each other. A Go<->Rust differential is blind to
  77% of the gap. Two substantive cross-impl divergences did show up:
  `POST /users {}` gives empty_handle vs invalid_json, and
  `GET /timeline?user=bob&user=alice` gives Go a 200 with five tweets where
  Rust returns 400. Neither contradicts twitter.tla.
- Rust retarget mirrors the Go one: flat containers + append log in `store`,
  ValidHandle/ValidText in `domain`, validation order + D4 + no id burn in
  `service`, strict JSON + canonical bytes + tick route + cursor + catch-all
  in `server`. Carry the F005 monotonic-append enforcement across.
- WATCH OUT in 1d: three `str.replace` patches on service.go silently no-oped
  because their anchors missed Gobra `// @ unfold` comment lines, and R0 still
  climbed to 44/54 on shim changes alone. Assert on EVERY source patch.
- Orchestration decided: loop drives 1c–1h and Phases 2–3; workflows are
  called by the loop at 1i and Phase 4. See the Orchestration section.
- `eclipse-temurin:21-jre` is now pulled and digest-pinned in
  `docker/pins.json`; TLC 2.19 launches inside it with the jar bind-mounted.
  The gates still run TLC on host JDK 17 because the sha256-pinned jar is the
  determinism lever, not the JVM. The container is available if a
  reproducibility question ever needs it — and since the two JVMs differ (21
  vs 17), a disagreement between them would itself be worth knowing.
- Docker already has `rust:1.95.0` and `crossbario/autobahn-testsuite` cached
  from the WebSocket project.
- Gobra's full image digest still needs resolving from the Go repo — the pin
  in `docker/pins.json` is truncated to `sha256:2ef080cc`.
