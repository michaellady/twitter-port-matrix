# F036 — The Java proof row kills 0 of 15, and the zero decomposes into four different zeroes

**Status:** complete 18-mutant sweep, `evidence/runs/calibration/java-proof/`
**Class:** the weakest R4 row on the table — and the first one whose survivals
separate cleanly into causes that a single kill rate would blend into mush

## The row

```
rung             live  killed  survived  unreached  equiv  killed/reached     killed/live     wall
R4 proof           17       0        15          2      0       0/15 = 0%       0/17 = 0%     812s
```

`go run ./tools/cmd/calibrate -impls java -rungs R4 -out
evidence/runs/calibration/java-proof -resume`, 18 mutants, window
2026-09-02T20:29:40Z .. 2026-09-02T20:44:36Z, defaults as the four-corner run
declares them.

`calibrate` says the right thing about it without being asked:

```
 1. R4 killed NOTHING (0/17 live mutants). A rung that never fires has not
    been shown to be able to fire; check it against a known-bad canary before
    reading this row as evidence about the mutants.
```

It has been. `evidence/runs/calibration/java-r4-gate/canary-injection.log` —
one line of `Dom.parseInt64` changed so a bare sign parses as zero:

```
o1a_oneCharAcceptSet               REFUTED    0 ok, 1 failed           20.6s     VERIFICATION FAILED
o1c_emptyAndBareSignRejected       REFUTED    2 ok, 1 failed            1.6s     VERIFICATION FAILED

R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s) (2 of 11 own assertion
goals FAILURE): o1a_oneCharAcceptSet, o1c_emptyAndBareSignRejected; 8
obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no
denominator   [40.9s]
```

So the rung fires, and the zero is about the mutants and the obligations rather
than about the rung being broken.

## The zero is four zeroes

Every one of the eighteen, by why it was not killed:

| n | reason | mutants |
|---|---|---|
| **9** | an obligation states the property and **JBMC cannot read it** (F014) | `timeline-scan-reversed`, `timeline-tiebreak-by-id-asc` (`o4c`, equals); `next-cursor-always-emitted`, `next-cursor-is-first-id`, `cursor-inclusive`, `limit-off-by-one` (`o4a`/`o4b`, SAT + equals); `self-follow-guard-dropped`, `follow-precedence-flipped` (`o5a`/`o5b`, getBytes); `id-burned-on-reject` (`o5d`, SAT) |
| **3** | **no obligation states the property at all** | `orphan-author-accepted` (F6, author existence); `follow-toggles`, `unfollow-rejects-missing-edge` (F3, idempotence) |
| **3** | the obligation is **relational and the mutant is not** | `id-first-is-two` (ids increase; nothing says from where — F023); `created-at-frozen` and `tick-advances-by-two` (`createdAt` never decreases; 0,0 and 0,2 both satisfy that) |
| **1** | the mutant makes the obligation **vacuous**, so the run is UNDECIDED and the cell is an error | `tick-goes-backwards` |
| **2** | **unreached**: the edit is confined to the trusted transport shim (F004, F022) | `unknown-json-fields-accepted`, `repeated-query-param-accepted` |

Four rows, four different things to do about them, and a single "0%" says none
of it. The 9 want a fixed checker; the 3 want obligations that do not exist
yet; the 3 want stronger obligations; the 1 wants F015's design tension
resolved. Only the third group is a criticism of the contract as written.

## The obligation set is aimed away from the catalogue

The sharpest number here is not the zero. It is this: **three of the seven
decidable obligations — `o1a`, `o1b`, `o1c` — are over `Dom.parseInt64`, and
not one of the eighteen mutants edits `Dom.java`.** They are the obligations
that verify most convincingly (over ALL one- and two-character strings, not a
list of examples), they are the ones the injection canary refutes, and they
have no mutant to catch.

Three more (`o3a`, `o3b`, `o3c`) are the F005 monotonicity premise, which the
`clock` and `id-alloc` families do aim at — and F015 arrives exactly there.
`Store.appendTweet` *throws* when an append would break monotonicity, because
F005's whole point is that the premise is enforced at the mutation site rather
than assumed. So `tick-goes-backwards` does not refute `o3b`; it makes the
second append throw, which makes the assertion after it unreachable, which
makes `o3b` **and its negation** both verify:

```
  ! c3_clockCanDecrease guards o3b_createdAtNonDecreasing and was NOT refuted
    (VERIFIED); under vacuity a claim and its negation both verify, so
    o3b_createdAtNonDecreasing decides nothing (F013)

R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read (...); nothing
was decided about this tree   [48.6s]
```

**The same design decision that makes the property true at runtime is what
makes it unprovable-against on a mutant.** A guard that throws converts every
downstream obligation from a test of the property into a test of the guard.
That is F015 at the proof rung, and this is the second corner it has been
measured on.

The seventh, `o5c`, is one precedence question (`follow("EVE","eve")` is
`invalid_handle`) that no mutant in the catalogue flips: `follow-precedence-
flipped` inserts the self-follow check *after* the syntax check, so `"EVE"` is
still rejected on syntax.

## Where this leaves the four R4 rows

| corner | verifier | killed/reached | what the row measures |
|---|---|---|---|
| Go | Gobra | 9/14 = 64% | deductive proof over the verified core |
| Rust | Verus | 1/14 = 7% | 57 of 62 clauses are on hand-written twins (F027) |
| Java | JBMC | **0/15 = 0%** | 7 decidable obligations, 3 of them over a file no mutant edits |
| Kotlin | JBMC | not yet swept | the same 7 obligations, the same 8 blocked (F034) |

The three numbers are not three measurements of the same thing, and the row
that reads worst is not the corner that is worst. Go's 64% is a deductive proof
over contracts written *for* this catalogue. Rust's 7% is F012's twins. Java's
0% is a bounded checker with **8 of its 15 obligations turned off by a tool
defect**, and — the part that is nobody's tool's fault — an obligation set
inherited from a corner that wrote it to answer F005 and F014, not to catch
this catalogue.

## What would actually move it, in order of cost

1. **Two obligations that do not exist on either JVM corner: F3 idempotence
   and F6 author existence.** Three mutants are waiting for them. Both reduce
   to string equality or `validHandle`, so both would land BLOCKED today —
   which is worth knowing before writing them, and is the honest reason not to
   write them yet.
2. **An origin obligation** — `assert firstTweet.id() == 1` — kills
   `id-first-is-two` and is not blocked by anything. F023 has now recorded that
   defect surviving R2, R4 and R5 across three corners; it survives here as a
   fourth. One line.
3. **A tick obligation** — `assert after == before + 1` — kills
   `tick-advances-by-two`, again unblocked. D3 says "advances the clock by
   exactly 1" and no obligation on either JVM corner says so.
4. **Fixing JBMC's `String.equals`** unblocks 9 of the 15 survivals at once.
   That is the largest single lever on this row and it is not in this
   repository.

Items 2 and 3 are two lines of Java and two canaries, and they were not written
here on purpose: the Java set is the **twin** of the Kotlin set, and its value
this week is that the two corners are comparable (F034). Adding obligations to
one twin and not the other would buy two kills and spend the comparison.

## The rule

**A proof rung's kill rate measures the aim of its obligations at least as much
as the strength of its verifier.** Three of this corner's seven working
obligations are over a file the defect catalogue never touches — they are not
weak, they are pointed somewhere else. Before reading a 0%, ask which files the
decidable obligations are over and which files the mutants edit; if those two
sets barely intersect, the number is about that and not about the proof.
