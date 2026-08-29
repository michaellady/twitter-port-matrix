# What this POC says about `verified-java-to-rust-port`

The Twitter matrix was built to answer one question cheaply: **how much
assurance does each verification layer actually buy on a port, and at what
cost.** This is the answer, aimed at the WebSocket project specifically.

Read `evidence/CALIBRATION.md` for the numbers and `evidence/FINDINGS.md` for
the patterns. This file is only the transfer.

---

## Read this first: you already know most of it

Nine methodology notes written by that project between 25 and 28 August are
copied into `prior-art/`. Comparing them to `findings/` shows **four of this
project's findings were already written down there, before the finding**:

| this project | already recorded as | written |
|---|---|---|
| F013 negation canaries | `canary-gates-need-runner-inversion` | 27 Aug |
| F015 enforcement reachability | `enforcement-is-a-reachability-property` | 26 Aug |
| F008 alphabet bounds coverage | `corpus-invisible-boundaries-need-targeted-probes` | 28 Aug 03:06 |
| F010 the oracle inherits host semantics | `reference-models-inherit-pinned-implementation-semantics` | 26 Aug |

The last one to bite was the worst. That 03:06 note says, verbatim, that if
regeneration leaves the corpus byte-identical then downstream passing results
have not observed the changed contract. **Thirteen hours later this project hit
exactly that, and derived it again from scratch.**

So the useful framing of everything below is not *here is what we learned.* It
is **here is the measured version of what you already know, and here is where
it was written down and still not applied.** The second half is the more
valuable one: an insight that is not reachable at the moment of the decision
costs the same as one never written.

Which makes the first recommendation a process one, not a technical one:
**those insights need to be reachable from inside the work.** Nine notes in a
directory nobody opens mid-task is where this project's four rediscoveries came
from.

## Then, the good news, because it is real and specific

**Your oracle is structurally stronger than this project's was.**

F006 measured that two implementations built in parallel from one permissive
spec diverged from a deterministic contract on exactly the same 39 steps, and
**agreed with each other on 30 of them**. Sibling-vs-sibling differential
testing was blind to 77% of the gap — and no amount of traffic moves that
number, because the two systems genuinely agree.

The WebSocket port does not have that problem. Its oracle is the pinned
`Java-WebSocket 1.6.0` jar itself, driven through a dependency-free JSONL
adapter — the *original*, not a sibling. That is a category difference and it
should be stated as a strength rather than assumed.

It does **not** protect against anything in the next three sections, all of
which are about inputs and counting rather than about the oracle.

---

## 1. Audit the corpus alphabet, separately from its pass rate

`Autobahn 7.3.2 FAILED -> OK, 0 FAILED across 247` and `100,000 requests, no
mismatch` are the same kind of number. Neither says anything about a construct
the corpus cannot emit.

In this project, three rungs were green — R0 at 54/54 byte-exact, R1 over
100,000 differential requests, R2's nine relations holding — while **four live
divergences sat in the code**. A 99-request hand-written cross-check found
them. The generator had never emitted a query string on a POST path, a
percent-encoded path segment, whitespace around a body, or a malformed
percent-escape.

Adding twelve request shapes made R1 fail at **step 8 of the first trace**.

Volume is not coverage. **A differential campaign's reach is bounded by its
generator's alphabet, and no number of requests widens it.**

Concretely, for the WebSocket work: Autobahn's 247 cases are a fixed
conformance suite with a fixed alphabet. The useful question is not "is 247
enough" but **"which frame shapes, fragmentation patterns, close codes and
UTF-8 boundary conditions can this suite not express?"** — and then whether
the differential lane against the Java jar can express them either. That audit
does not exist yet in either project, and here it was worth four live defects.

---

## 2. Do not count `rust_identity_verified` rows

`assurance/formal/proof-targets.json` records
`RUST_IDENTITIES_NOT_YET_RESOLVER_VERIFIED`, with `rust_identity_verified=false`
on every row. When that work lands, the temptation will be to report the count
of rows that flipped to true.

**Four findings here are four different ways a verification count is produced
without verification happening:**

| | the count | why it meant nothing |
|---|---|---|
| F007 | 6 trusted shims deleted | grepped from a package the verifier could not parse |
| F011 | mutants killed | drifted anchors injected nothing |
| F013 | 6 obligations VERIFIED | the code they described was unreachable |
| F015 | 1 obligation discharged | the branch is dead on every real path |

Two of those four were mine, written into a report and believed for hours.

Each passes a plausibility check. Each is exactly the shape of a status-report
number. **Before quoting a count, ask what would have to be true for it to be
wrong, and check that instead.**

For a row to be load-bearing it must clear four questions, and only the first
is usually asked:

1. Did the verifier parse the file? (F007)
2. Can the verifier *reach* the obligation — does its negation get refuted?
   (F013)
3. Can the *program* reach the branch it describes? (F015)
4. Is the contract on the shipped symbol, or on a hand-written copy of it?
   (F012)

---

## 3. Your formal target may not be statable, and that is worth checking first

This is the finding most likely to save the most time.

`ASSURANCE.md` here claimed observational equivalence: *if A refines the
reference machine and B refines it, a port is correct with zero changes to the
base app.* **That claim was unsupported as written, and not for want of
effort.**

The obligation quantifies over `step(state, request)` where the request is a
**wire** object. The code that turns bytes into a core call sat outside every
verification perimeter *by design* — Gobra ran over `[clock ids dom store
service]` and not the HTTP shim; the Rust server crate had no verify key at
all. So the functions the obligation quantifies over were **not mentioned by
any contract anywhere**.

That is not a verifier limitation. It is the direct cost of the
verified-core / trusted-shim split — the same split that keeps semantics inside
code the verifier reads. The two goals are in tension and the document claimed
both.

**The parallel to check, before more effort goes in:** is the WebSocket port's
frame parser — the code that turns bytes off the socket into a typed frame —
inside the Kani/Verus perimeter, or is it trusted transport like `ws-driver`?
If it is outside, then a wire-level equivalence claim against the Java jar is
unstatable there for the same reason, and the reachable claim is the narrower
one over the decoded frame alphabet. Better to discover that now than after
the resolver work lands.

Also worth checking in the same pass: **are the Rust proofs on the shipped
functions, or on hand-written twins?** Here they were on twins, kept in sync by
hand, and one had already drifted into falsehood — claiming a two-outcome
postcondition for a function with three. `cargo test --workspace` had not
compiled for weeks, which is plausibly how it survived.

---

## 4. Re-test the recorded blockers before treating them as settled

Seven documented reasons-something-cannot-be-done were false on inspection.
Five were inherited from upstream, two were mine from earlier in the same
session:

- "F1 cannot be expressed" — it can; it is now proved.
- "The service layer cannot compose the store's lock predicate" — three lines.
- "No `vstd::hash_set` model exists" — it ships four.
- "`home_timeline` needs a sort spec" — there had been no sort for weeks.
- "The sort-free design deletes six trusted shims" — it deletes two.

Every one was honest when written, and was then inherited by every later
reader as a fact about the world.

**A blocker is a measurement with a timestamp.** The WebSocket project has
several — the `vstd::sync::Mutex` gap, the sort-spec gap, the
`RUST_IDENTITIES` state itself. Each deserves one re-run before it justifies a
design decision.

---

## 5. What the numbers actually say about rung value

From the kill table — 36 mutants, both corners, every cell guarded:

| rung | kill% | wall |
|---|---|---|
| conformance replay | 100% | 57s |
| differential fuzz | 100% | 1465s |
| property / metamorphic | 42% | 2495s |

Three readings, and the third is the one that transfers:

**The differential rung killed nothing the corpus rung missed, at 26× the
cost — and that is an argument for keeping it.** Its value was discovery: it
found four live divergences, and the fix moved that coverage into the corpus.
A discovery rung's steady state is quiet. Judged on a steady-state kill rate it
looks worthless, and dropping it on that basis removes the only mechanism that
finds the next gap. **Quiet and blind look identical from the kill column.**

**The property rung is worst and most expensive, and still worth keeping** —
it is the only rung that never consults the reference machine, so the only one
that survives the reference machine being wrong. F010 proved that is a live
risk: the reference machine here had silently inherited a Go standard-library
quirk and promoted it to cross-language contract.

**And the cost figures mostly are not measuring the rungs.** With the
per-launch floor measured and subtracted, the property rung's own cost went
*negative* — it is process startup. Compare rungs by launch count, not
seconds.

---

## The one caveat that limits everything above

**The mutants were built from the contract, and the corpus is generated from
the contract.** The 100% is partly a statement about that alignment.

A catalogue drawn from a different source — production incidents, a fuzzer's
crash corpus, or the real defect history of `Java-WebSocket` — would produce a
different and far more informative table. **The WebSocket project is in a
better position to do this than the Twitter matrix ever was**, because it has
a real library with a real bug history behind it.

That is the single highest-value follow-up, in either project.
