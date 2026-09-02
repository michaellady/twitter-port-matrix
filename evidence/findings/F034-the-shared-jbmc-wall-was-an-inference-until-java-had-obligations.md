# F034 — The shared JBMC wall was an inference for two months; measured, it holds exactly

**Status:** measured on `impls/java`, 15 obligations, 33 JBMC entry points
**Class:** a prediction confirmed — and a prediction that was blocking six
matrix cells while nobody could check it

## What was claimed, and by what

F014 reduced each of the Kotlin corner's JBMC blockers to a two-line repro,
observed that every repro **reproduces identically in plain Java compiled by
javac**, and drew the consequence:

> **This wall is shared with the Java corner.** … if the intent was ever to
> reach Java's R4 via JBMC, that path is blocked by the same defect, for the
> same reason, and no amount of Java-side effort moves it.

That is a correct inference from a two-line repro. It was not a measurement of
this corner, because `impls/java` had no obligation set: `evidence/MATRIX.md`
said so in as many words — "Java's cap is not a matter of effort spent on JBMC
either: `impls/java` has no obligation set for a rung to run, so the Kotlin
corner's `Obligations.kt` has no Java twin" — and capped six of the twelve R4
cells on it.

So the position was: a tool defect demonstrated in Java, blocking a corner
written in Java, with nothing in Java ever run against it.

## What was done

`impls/java/verification/twitterport/verification/Obligations.java` and
`Canaries.java`: the twin of the Kotlin corner's set. Same fifteen obligations,
same five groups, same properties, over this corner's own classes. Every one
was run as a JBMC entry point **before any `Blocked` reason was written down**,
so `tools/cmd/jbmc/obligations.go`'s Java table is a record of this run and not
a copy of the Kotlin table. Transcripts:
`evidence/runs/calibration/java-obligation-probe/goal-lines.txt`.

## The result: identical, obligation for obligation

| group | obligation | Kotlin | Java |
|---|---|---|---|
| parseInt64 | `o1a`, `o1b`, `o1c` | decidable | **decidable** |
| syntax predicates | `o2a`, `o2b` | blocked, getBytes | **blocked, getBytes** |
| append monotonicity | `o3a`, `o3b`, `o3c` | decidable | **decidable** |
| pagination | `o4a` | blocked, SAT | **blocked, SAT** |
| pagination | `o4b`, `o4c` | blocked, equals | **blocked, equals** |
| precedence | `o5a`, `o5b` | blocked, getBytes | **blocked, getBytes** |
| precedence | `o5c` | decidable | **decidable** |
| precedence | `o5d` | blocked, SAT | **blocked, SAT** |

**7 decidable, 8 blocked, on both corners, with the same eight blocked and the
same three reasons.** The clean-tree run, verbatim:

```
decidable 7   VERIFIED 7   REFUTED 0   VACUOUS 0   UNDECIDED 0
blocked   8   (recorded JBMC 6.11.0 limits; in neither the numerator nor the denominator)

R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion
goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a
recorded JBMC 6.11.0 limit (F014), in no denominator   [47.4s]
```

Eleven own assertion goals on each corner as well. The two JVM corners are not
similar here; they are the same measurement.

## Two blockers came out sharper than the Kotlin entry states them

### 1. `getBytes` is unconstrained in BOTH length and contents

The recorded reason says "an opaque stub returning an array of unconstrained
length". Probed directly, over `impls/java`'s own `Dom` (the last four rows are
the load-bearing ones):

```
assert "alice".getBytes(UTF_8).length == 5      -> FAILURE
assert "alice".getBytes(UTF_8).length != 5      -> FAILURE
assert "alice".getBytes(UTF_8)[0] == (byte)'a'  -> FAILURE
assert "alice".getBytes(UTF_8)[0] != (byte)'a'  -> FAILURE
```

Neither the length nor the contents is determined. And then, one layer up:

```
assert  Dom.validHandle("alice")   -> FAILURE
assert !Dom.validHandle("alice")   -> FAILURE
assert  Dom.validHandle("EVE")     -> FAILURE
assert !Dom.validHandle("EVE")     -> SUCCESS
assert  Dom.validText("")          -> FAILURE
assert !Dom.validText("")          -> FAILURE
```

**Both a claim and its negation are refuted.** That is the exact dual of the
vacuity signature F013 is about, where both a claim and its negation *verify*.
Both-verify means nothing reaches the claim; both-refute means the claim is
nondeterministic. Neither is a decision, and the two are not interchangeable —
F037 is what that costs the rung as it stands.

The asymmetry in the middle rows is why `o5c` survives the blockage while `o2b`
does not: `!validHandle("EVE")` is decidable and `validHandle("alice")` is not,
so an obligation that needs a handle to be **invalid** can still be discharged
while one that needs a handle to be **valid** cannot. *Why* JBMC decides one
and not the other is not pinned here and is not claimed; the measurements are.

### 2. The SAT wall is a machine wall, not an induced one

The first four "out of memory" runs were made under an artificial
`ulimit -v 8000000`, which proves only that an 8 GB cap can be hit. Re-run with
no cap at all on a 15 GB machine, the kernel's own report:

```
Memory cgroup out of memory: Killed process 2811 (jbmc)
  total-vm:16420540kB, anon-rss:13951040kB, file-rss:4528kB
```

13.9 GB resident on `c4_pageMayExceedLimit`, and `o4a_pageRespectsLimit` the
same. The Kotlin entry records 11 GB; this corner wants more. Either way it is
a bounded-model-checking scalability wall and not a modelling gap.

## What this changes

- **Six matrix cells stop being capped for a reason nobody could check.** The
  cap was correct while it stood — a row over an empty denominator is worse
  than a capped cell — but "no obligation set" is a fact about the repository,
  not about Java, and it was doing the work of a ceiling.
- **F014's cross-corner table is now measured on two corners rather than one.**
  The line "Java | JBMC cannot compare strings; OpenJML/KeY unattempted and
  unprobed" was half inference; the first clause is now a run.
- **`ASSURANCE.md`'s Java row said "not attempted; `impls/java` has no
  obligation set at all".** It is attempted. What it reaches is F036.

## The rule

**An inference from a repro is not a measurement of a corner, and the
difference is invisible in a ceiling table.** F014's inference was right — that
is the good case, and it is still the case where the cost showed up: six cells
stayed capped for two months on a sentence that took an afternoon to check.
When a ceiling is recorded as "the tool cannot", ask which corners it has
actually been *run* on; when the answer is "one, and the others by argument",
the others are unmeasured, not capped.
