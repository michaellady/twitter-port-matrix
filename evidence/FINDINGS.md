# What these findings have in common

An index, and an argument. The findings are in `evidence/findings/`; this file
is for the patterns that only appear when you read them together — which is
also the part that transfers to `verified-java-to-rust-port`.

---

## The index

| | finding | one line |
|---|---|---|
| F001 | self-fulfilling clock oracle | the conformance harness set the clock to the expected answer before asserting on it |
| F002 | tweet-id origin mismatch | model starts ids at 0, both implementations at 1; nothing compared them |
| F003 | unspecified precedence diverges | `follow(eve,eve)` — two implementations, two answers, both refine the model |
| F004 | sort-free design deletes an obligation | F2 becomes a consequence of the data structure, not a claim about a sort |
| F005 | derived properties need enforced premises | the monotonicity lemma had two premises and nothing checked either |
| F006 | siblings share a blind spot | Go and Rust agreed on 30 of their 39 shared divergences from `S_obs` |
| F007 | same change, different payoff | −6 trusted shims in Go, 0 in Rust — and the −6 was counted on unparseable code |
| F008 | volume is not coverage | 100,000 requests found nothing; widening the alphabet found it at step 8 |
| F009 | a rung cannot kill what it cannot reach | a shipped defect survived R0 because no input reached it |
| F010 | the reference machine inherited a Go-ism | `encoding/json` case-folding became cross-language contract |
| F011 | a drifted anchor inflates the kill table | an un-injected mutant is "killed" by every rung |
| F012 | R5 as written is unstatable | the decode boundary is outside every verification perimeter |
| F013 | six obligations verified over unreachable code | one undischargeable check made everything downstream vacuous |
| F014 | JBMC cannot compare strings | `"abc".equals("abc")` verifies as FALSE |
| F015 | redundant enforcement blinds mutation | F4 is guarded twice, so removing either guard tests nothing |
| F016 | 23 verified means one property proved | 11 of 23 units carry no `ensures` at all; one real, non-vacuous, undelegated clause |
| F017 | the same defect is not the same defect | a mutant catalogue does not transfer across corners as cleanly as its name suggests |
| F018 | the oracle itself cannot express the bug | a sequential reference machine has no vocabulary for interleaving, so no rung derived from it can score a concurrency defect |
| F019 | the obligation count is not reproducible | 283 recorded, 236-238 measured across eight runs, and 61% of what it counts is compiler-generated scaffolding |
| F020 | the prose contradicted its own commit | one commit discharged F8 in Go, recorded it as discharged, and said in the file beside it that F8 was unproved |
| F021 | the audit fails where the obligations are strongest | vacuity-checking cost appeared to rise with clause strength, so the five clauses most worth auditing were the five the auditor could not decide — corrected by F029 |
| F022 | the proof rung's denominator is set by the trusted shim | 4 of 18 Go mutants edit code no obligation covers, so R4's ceiling is 78% before a clause is written |
| F023 | the strongest rungs cannot see a constant | an id origin shifted by one is killed by R0 and R1, and survives R2, R4 and R5 |
| F024 | a count that goes down is the repair | two of the four drifted Verus twins carried no `ensures` at all, so deleting them took 23 verified to 21 and lost nothing |
| F025 | three of seven VERIFIED were never audited | the canary sweep was indexed by canary, not by claim, so three obligations with no canary at all passed the one gate built to catch that — and the audit that would have caught it costs 3 s on a bounded checker and does not terminate on a deductive one |
| F026 | the proof half of the matrix has no cell | one corner has a proof rung and no ordered pair has that corner at both ends, so all 24 R4/R5 cells are capped and the Go sweep fills none of them — **R4 half superseded within the hour; the R5 half stands** |
| F027 | the Rust proof row measures the twins | 1 of 14 covered Rust mutants is killed by Verus, because four of the five verified crates put their contracts on hand-written copies |
| F028 | the refinement row is a copy of the proof row | R4 and R5 disagree on 0 of 18 mutants; the canary separates them, the catalogue never does |
| F029 | the audit was undecidable as spelled, not as asked | the eight clauses F021 could not decide are all refutable; a solver flag and a 3.75x budget changed nothing, and asking one clause at a time in a decidable spelling changed everything |
| F030 | an injection canary stood in for the instrument the rule asks for | the Rust R4 row shipped with no vacuity instrument at all; built one, and 5 of 5 shipped clauses are refutable — but 57 of the corner's 62 clauses are on twins |
| F031 | the anchor gate compiles the implementation, not the obligations | a mutant that changes an API arity passes `mutate verify` and then stops the proof rung's tree from building; an error cell, not a survival |
| F032 | the guard that makes the premise true stops the rung from scoring it | the log's own invariant check throws, so the negation canary is unreachable and the vacuity audit turns the corner's one likely kill into UNDECIDED |
| F033 | two proof rungs disagree on 8 of the 12 defects both can see | Gobra kills 9 of 14 where JBMC kills 0 of 14; the R4 column's three denominators are all `14` and all three are set by unrelated things |
| F034 | the shared JBMC wall was an inference until Java had obligations | F014 concluded the wall is shared from a `javac` repro; measured on `impls/java` it is identical — same 7 decidable, same 8 blocked, same three reasons |
| F035 | one decorative line in an obligation file costs a whole cell | the obligation set is compiled against the mutant, so it is coupled to every signature it mentions; `s.createUser("a")` in a timeline obligation that never needed it turns `id-burned-on-reject` into an error cell |
| F036 | the fourth proof row kills nothing, and the zero decomposes | Java R4 is 0/15, and the 15 split into 9 blocked by a tool defect, 3 with no obligation at all, 3 relational, 1 vacuous — three of the seven working obligations are over a file no mutant edits |
| F037 | the vacuity gate catches "both verify" and is blind to "both refute" | a claim and its negation both REFUTED means the predicate is nondeterministic, and the rung reports that as a KILL; only a hand-maintained list prevents a false red on a clean tree |
| F038 | the proof and refinement columns finally disagree | two mutants confined to `internal/dom/dom.go`, the one file between R4's perimeter and R5's: killed by R4, unreached by R5. F028's zero disagreements was a fact about the catalogue |
| F039 | the columns also separate by verdict, not only by reach | `clock-now-off-by-one` sits inside a file R5's perimeter *does* contain; R5 reached it, read Gobra's failing postcondition, found no refinement clause on it and passed |
| F040 | the attribution engine can name the wrong member, at random | a member's end was found by scanning for a bare `}`, which a one-line method never produces, so one span swallowed the next function; `memberAt` then ranges over a map and Go randomizes that order. Fixed, with the overlap gate the finding asked for |
| F041 | the R5 blocker was the lock, and the lock came out | `ASSURANCE.md` had asserted for months that Rust cannot reach R5 because `RwLock` has no vstd model. Reproduced verbatim, then removed for three crates: 21 → 32 obligations, `ids` from 0 to 5, three twins deleted. **No R5 cell moved** — there is no Verus R5 rung |
| F042 | the vacuity instrument counted two trusted axioms as shipped obligations | under a third function's name. The instrument built in F030 was itself over-counting; its shipped/twin ratio was wrong before anyone read it |
| F043 | the next blocker down is `String` view injectivity | vstd will not say the abstraction is injective, so the response axis of the refinement stays undischarged — and the twin had looked stronger only because it trusted more |
| F044 | the R5 rung's documented undecided cases were not the ones it implements | the prose and the code disagreed about when the rung declines to answer |
| F045 | the Kotlin abstraction function is shipped code, and the Go one is not | Go's projections are ghost and `go build` erases them; Kotlin has no ghost mode, so its `abs` methods are in the compiled artefact. A widening of that corner's surface the Go corner does not pay |
| F046 | JBMC can name which refinement clause failed, so R5 is not a Go-only rung | the feasibility question answered by measurement rather than argument; `go ← kotlin` and `kotlin ← go` stopped being capped |
| F047 | a second catalogue, and what independence actually cost | 43 mutants from 12 defects across three non-contract sources. The independence claim is about the *generator*, not the effect: every live mutant necessarily deviates from `S_obs`, so only the procedure that chose it can be independent |
| F051 | lifting the contracts off the twins quintupled the Rust proof row | 1/14 = 7% became 5/14 = 36% on the same catalogue and driver; all four new kills are in the two crates the lift touched, and the twin structure was costing 29 points of kill rate |

---

## Pattern 0 — the oracle bounds the ladder, not the rungs

F018 is the one finding that is not about a rung being too narrow. It is about
the **reference machine** being too narrow, which is a different and worse
thing: a rung can be widened, and an oracle that cannot express a class of
behaviour makes every rung derived from it blind to that class at once.

`S_obs` is deterministic and sequential. Under concurrent load the Go corner
dropped tweets behind an HTTP 500 while passing R0 56/56, R1 clean, R2, and R4
with ~43 load-bearing clauses — and `go test -race` was silent too, because the
defect was a lost update across correctly-locked components rather than a data
race.

**The rule.** Before reading a kill table, ask what the oracle can *say*. Every
rung in it inherits that vocabulary, and no amount of climbing escapes it. The
check that eventually caught F018 works precisely because it consults no
reference machine: it counts durable records against accepted writes.

---

## Pattern 1 — a count that looks like evidence

Five findings are the same mistake wearing different clothes: **a number was
produced without the thing that would make it mean something.**

| | the count | why it meant nothing |
|---|---|---|
| F007 | 6 trusted shims deleted | grepped from a package Gobra could not parse |
| F011 | mutants killed | the anchors had drifted, so nothing was injected |
| F013 | 6 obligations VERIFIED | the code they described was unreachable |
| F015 | 1 obligation discharged | the branch it described is dead on every real path |
| F025 | 7 obligations VERIFIED | three of the seven had no negation canary, so nothing had ever tried to refute them |

Each of these passes a plausibility check. Each is the kind of number that
appears in a status report. And each is produced by a measurement that never
touched the thing being measured.

**The rule.** Before quoting a count, ask what would have to be true for it to
be wrong, and check that instead. "133 obligations verified" is not a fact
about the code until you know the verifier could parse it, could reach the
obligations, and that the obligations describe live paths.

---

## Pattern 2 — the blind spot is never visible from inside the gate

| finding | what was too narrow | what found it |
|---|---|---|
| F001 | the oracle could not falsify a field | replaying the corpus against an independent machine |
| F006 | the oracle was a sibling | comparing both against something neither produced |
| F008 | the input alphabet | a hand-written cross-check while building a third corner |
| F009 | the corpus | injecting a defect and watching which rungs slept |
| F010 | the reference machine itself | two implementations disagreeing with each other |
| F013 | the reachable code | asserting the *negation* rather than injecting a bug |
| F015 | the mutation granularity | a probe refusing to score a mutant that changed nothing |

Not one was found by the gate reporting green. **Every single one was found by
stepping outside it** — and in five of seven cases, by something built for an
entirely different purpose.

**The rule.** A green gate tells you nothing about what it cannot see, and it
will never tell you. Budget for the outside check: a second oracle, a wider
alphabet, an injected defect, a negation. The cheapest of these is injecting
defects and observing which rungs sleep through them, which is the argument
for the calibration table being the deliverable rather than a by-product.

---

### F022 extends this to the proof rung

F008 and F009 are about inputs a rung's *corpus* never emits. A proof has no
corpus, so the same question becomes: what does the verifier *read*? On the Go
corner the answer is five of six packages — the trusted shim is excluded by
design (F004), and 4 of 18 mutants live entirely inside it. Scored without
that distinction, R4 reads as passing live defects; scored with it, those four
are unreached and R4's oracle is undiminished. Both readings are in the table,
and the ceiling — 14 of 18, 78% — is a property of where the perimeter was
drawn, not of how good the obligations are.

The transferable form: **every rung has a reach, and for a deductive rung the
reach is the verification matrix.** A kill rate quoted without it is quoted
over a denominator nobody chose deliberately.

### F025 turns Pattern 2 back on the check itself

Every entry in Pattern 2's table is a gate that could not see its own blind
spot. F025 is the same shape one level up: the **negation-canary sweep** — the
instrument built to catch F013, the deepest false green here — could not see
its own blind spot either. It verified that every canary it had was refuted,
and never asked which claims had no canary. Three of the Kotlin corner's seven
VERIFIED obligations had none, so "7 VERIFIED" was four audited claims and
three unexamined ones.

The fix is not a better canary; it is changing what the loop iterates over.
Enumerate the **claims** and ask each what audits it. A check indexed by the
things you remembered to write can only ever confirm that you wrote them.

F025 also prices the audit, which nothing here had done: negating a bounded
obligation costs what the obligation costs (3–7 s), while negating a deductive
one is strictly harder than the obligation and sometimes does not terminate at
all (F021). So the vacuity audit can run on **every tree** a bounded rung
judges and only on the **clean tree** for a deductive one — and R4 on the Go
corner accordingly does not re-audit per mutant while R4 on the Kotlin corner
does. The weaker rung carries the better-measured claim, which is not the
ordering the ladder suggests.

## Pattern 3 — falsifiability has two forms and they are not interchangeable

The project's founding discipline was "no gate is trusted until it has been
shown to fail," and F001 is what it was built for. It was not enough.

- An **injection canary** asks *if I break the code, does the gate notice?*
- A **negation canary** asks *if I assert the opposite, can the tool refute it?*

Against a vacuous proof the first is ill-posed — the broken statement is
downstream of the infeasible point too, so the verifier reports VERIFIED for
the broken version as well, and the canary passes having shown nothing. Only
the second exposes it, because a claim and its negation *both* verifying is the
unique signature of an unreachable obligation (F013).

**The rule.** Injection proves a gate can detect defects. Negation proves an
obligation is reachable. Every proved obligation needs both.

**Applied to the Go corner, F013's mode does not replicate:** 30 of 33 members
refute `ensures false`, and **91 of 91** functional clauses refute their own
negation, with 0 vacuous.

Getting to 91 took two findings. The sweep first returned 83, with eight
undecided — all on one member, the one carrying the store's strongest clauses
(F021). That needed a third state alongside "refutable" and "vacuous":
**unaudited**, meaning the package is green and nothing rules out the
obligation being empty. Folding that into either neighbour is how a verified
count goes wrong — in the F016 direction if you round it up, and in the
opposite direction if you round it down. Keeping it separate is what made the
eight findable later.

And they were findable: none was vacuous, and none was beyond the solver
(F029). The audit had been re-verifying the whole member to ask about one
clause, and putting three of its questions in a spelling the solver would not
decide. **So "unaudited" is a statement about the instrument, and it should be
read as one.** F021 read it as a statement about the clauses — that the
strongest obligations are intrinsically the hardest to audit — and that was
wrong in a way that would have quietly capped the corner's coverage story at
exactly the clauses anyone cares about.

**And a third form, from F028: a canary is about one rung, not about a row.**
Injection and negation both ask whether *this* check can fail. Neither asks
whether it fails on anything the cheaper check above it missed. R4 and R5 on
the Go corner each pass their canaries in both directions and are each
non-vacuous — and then agree on all 18 mutants, because the one file that
separates their perimeters is the one file no mutant touches. A gate answers
"can this fail"; only a rate answers "does this ever differ".

---

## Pattern 4 — the ceiling is set by tool coverage of the standard library

Every corner is limited, and not one is limited by its language.

| corner | limited by |
|---|---|
| Go | Gobra: no string indexing in the ghost language; no `delete` builtin; `range` over a map fails |
| Rust | Verus: no `vstd` model for `RwLock`; proofs on hand-written twins |
| Java | JBMC: `String.equals` is broken; OpenJML unprobed |
| Kotlin | JBMC, same defect; no deductive verifier exists |

Three of four are **standard-library** gaps — strings, locks, collections —
not language expressiveness. F007 predicted the shape and got the reason
wrong; F012 and F014 confirmed it twice more.

**The rule for the port work.** A restructuring justified by "it discharged an
obligation in the source language" carries no guarantee in the target. Re-cost
it against the target verifier's own gaps, and expect those gaps to be as
mundane as whether the tool parses `delete` or believes `"abc".equals("abc")`.

---

## Pattern 5 — a documented blocker ages badly and nothing re-tests it

Five recorded reasons-something-cannot-be-done were false on inspection
(F012): Go's F1 "cannot be expressed"; the service layer "cannot compose
`LockP()`"; the ids framing; Rust's "no `vstd::hash_set` model exists" (it
ships four); and `home_timeline` needing a sort spec, when there had been no
sort for weeks.

Two more were mine — F004's shim count, and the R5 claim in `ASSURANCE.md`.

F020 is the degenerate case: the interval between a blocker being true and
being recorded as true was **zero**. One commit discharged F8 on Go's id
generator, wrote `"status": "discharged"` into `obligations.json`, and left the
prose two files away saying `Next` "is `// @ trusted` and carries no
postcondition at all". Nothing compares a document to a data file, so nothing
noticed for two sessions.

Every one was written honestly, was true when written, and was then inherited
by every later reader as a fact about the world.

**The rule.** A blocker is a measurement with a timestamp. Re-run it before
building on it, and especially before citing it as a reason not to try.

---

## Pattern 6 — the ladder is not ordered

The rungs are numbered, drawn as a ladder, and cost more as they go up, all of
which invites the assumption that each one subsumes the ones below it. The
first five-rung measurement on the Go corner refutes that in a single mutant.

`id-first-is-two` shifts the id generator's origin from 1 to 2. It is visible
at request 0 of every input source. R0 and R1 kill it; R2, R4 and R5 all pass
it (F023). The reason is not effort or budget: every obligation about ids is
*relational* — the counter advances by one, each id equals the counter's
previous value, every id is at least 1 — and all three survive a consistent
renaming of the constants. The origin appears three times in English comments
and in no clause at all. R0 and R1 catch it only because they compare bytes
against a concrete reference trace, and a trace contains constants.

**The rule.** A rung's strength is a statement about *what it can express*, not
about where it sits on the ladder. Relational specifications are blind to
constants; reference traces are blind to interleavings (Pattern 0); corpora are
blind to inputs they cannot emit (Pattern 2). These blindnesses do not nest, so
one row per rung is the right shape for the table and a single "assurance
level" would be the wrong one — including for a port whose deductive rung is
blocked, where the tempting summary is that everything below it is subsumed by
the proof that is missing.

---

## What this says to the WebSocket port

The specific transfer, stated plainly:

1. **Audit the corpus alphabet, separately from its pass rate.** "Autobahn
   7.3.2, 0 FAILED across 247" and "100,000 requests, no mismatch" are the same
   kind of number, and neither speaks to a construct the corpus cannot emit
   (F008).
2. **The Java oracle is structurally stronger than this project's was.** F006
   is a warning about *sibling* differential testing; the WebSocket port's
   oracle is the pinned jar itself. That advantage is real and worth stating —
   but it does not protect against F008 or F009, which are about inputs.
3. **Do not count `rust_identity_verified` rows.** F007, F011, F013 and F015
   are four different ways a verification count can be produced without
   verification happening. The blocked deductive rung there should be assessed
   by asking what the obligations *reach*, not how many there are.
4. **Re-test the recorded blockers before treating them as settled.** Five of
   five checked here were stale.
5. **Expect the ceiling to be a library gap.** If the Rust side is to carry
   deductive proof over state behind a lock, that is blocked today for a reason
   that has nothing to do with the port and will not be fixed by porting harder
   (F012).
6. **A WebSocket server is concurrent and its oracle is not.** This is the
   sharpest transfer of the three added since the first write-up. A pinned
   Java jar replaying frames is a sequential oracle in exactly the sense F018
   is about: however many frames it agrees on, it says nothing about two
   connections interleaving, and neither does a race detector, which was silent
   on F018 throughout. Budget a counting check that consults no oracle —
   accepted writes against durable records — and expect it to be the only thing
   that can see this class.
