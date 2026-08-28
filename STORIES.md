# Story queue

Ordered backlog. Work top to bottom; no timers, no waiting. A story is done
when its gate passes and the gate has been shown capable of failing.

Status: `[ ]` queued · `[~]` in progress · `[x]` done

---

## Parallel lanes

Three agents run concurrently, in separate directories so they cannot collide.
The split is not arbitrary — it preserves the separation this repository exists
to measure:

| Agent | Story | Writes | Role |
|---|---|---|---|
| `gobra-r4` | S-12 | `impls/go/`, `docker/pins.json` | proves an existing implementation |
| `mutate-tool` | S-14 | `tools/cmd/mutate/` | builds the judge — may touch NO implementation |
| `java-corner` | S-16/17 | `impls/java/` | builds a subject — may touch NO tool |

No agent both writes an implementation and writes something that judges one.
That is the correlated-failure defect the calibration is designed to quantify,
and reproducing it inside the rig would invalidate the measurement.

---

## Phase 1 — Go ↔ Rust rig

- [x] **S-01** Vendor Go impl as its own module
- [x] **S-02** `replay` harness + R0 baseline
- [x] **S-03** Retarget Go to `S_obs` — R0 54/54, canary verified
- [x] **S-04** Vendor Rust impl; generalise harness launch config
- [x] **S-05** Retarget Rust to `S_obs` — R0 54/54, canary verified

- [x] **S-06** `replay --canary` as a first-class mode
      *Gate:* `matrixctl` asserts R0 can fail, without a manual inject-and-revert.
      Currently the canary is run by hand, which means the gate's own
      falsifiability is not itself gated.

- [x] **S-07** `tracegen` — randomized op sequences from the `S_obs` alphabet
      *Gate:* seeded and reproducible; covers every request shape including
      malformed ones; shrinks a failing trace to a minimal one.

- [x] **S-08** `diffrun` — replay one trace against N impls, diff responses (**R1**)
      *Gate:* 100k traces, zero unexplained mismatches, both corners.
      *Canary:* an injected defect must be caught and the trace shrunk.

- [x] **S-09** Property + metamorphic suite (**R2**)
      *Gate:* follow/unfollow idempotence, pagination partitions the visible
      set with no fabrication or loss, timeline monotonic under insertion,
      encode∘decode identity on the wire format.

- [ ] **S-10** `specgen` — render `S_obs` into per-language verifier contracts
      *Reordered after S-11/S-12:* the proof modules already carry hand-written
      contracts. Retargeting them first shows what shape the generator has to
      emit; generalising before that would be guesswork.
      *Gate:* all renderings generated from one source; a `specgen` mutant
      must break at least one implementation's proof.

- [x] **S-11** Retarget the Verus proof module (**R4**, Rust) — 23 verified,
      0 errors. Trusted-shim count unchanged; see finding F007.
      *Gate:* `cargo verus verify` green. The proofs still reference the old
      `HashMap<String, HashSet<String>>` / `by_author` shapes.
      *Expect:* several `external_body` shims become verifiable now the
      containers are flat — `proof_follow_set`, `proof_home_timeline`,
      `proof_append_tweet`, `proof_follow_insert`, `proof_follow_remove` —
      mirroring the six Go trusted shims deleted in S-03.

- [x] **S-12** Retarget the Gobra sidecars (**R4**, Go) — 242 obligations,
      0 errors, all 5 packages. F2 now proved. Revises F007.
      *Gate:* Gobra green per package. Needs the full image digest resolved;
      `docker/pins.json` currently truncates it to `sha256:2ef080cc`.

- [ ] **S-13** Refinement obligations against `S_obs` (**R5**)
      *Gate:* `abs_L` defined per language; init, response and step
      commutation discharged. This is the rung the whole repository exists
      to reach.

- [x] **S-14** `mutate` — 36 mutants (18 defects x 2 corners), data-driven
      catalogue. R0 sweep: 18/18 killed on Go. See F009.
      *Gate:* mutant families cover id allocation, self-follow guard, sort
      tiebreak, idempotence, orphan-author accept, clock regression,
      pagination fabrication.

- [ ] **S-15** `calibrate` — per-rung kill table, both port directions
      **This is the deliverable.** Fan-out story: run as a workflow.

## Phase 2 — Java corner
- [x] **S-16** Toolchain: no build tool installed — plain javac on JDK 17
- [x] **S-17** Java implementation — R0 54/54, 4/4 canaries, R1 clean
- [ ] **S-18** Java ↔ Rust and Java ↔ Go ports

## Phase 3 — Kotlin corner
- [ ] **S-19** Toolchain: `kotlinc` on host; JBMC over bytecode
- [ ] **S-20** Kotlin implementation; document the R5 ceiling failure plainly

## Phase 4 — fill the matrix
- [ ] **S-21** All 12 ports. Fan-out story: run as a workflow.
- [ ] **S-22** Full calibration report
- [ ] **S-23** Transfer write-up back to the WebSocket port

## Standing, not phase-bound
- [ ] **S-24** Adversarial refutation of F001–F006
      All six rest on a single reading — mine. F001 claims a shipped
      conformance suite has no signal behind a field, which deserves
      independent refutation before it is relayed to the WebSocket project.
