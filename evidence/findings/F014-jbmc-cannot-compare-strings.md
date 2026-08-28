# F014 — JBMC reports `"abc".equals("abc")` as FALSE

**Status:** reproduced independently, in plain Java, on JBMC 6.11.0
**Effect:** the Kotlin ceiling is a tool defect, not a language cost — and the
wall is shared with Java

## The reproduction

```java
public class T { public static void main(String[] a) { assert "abc".equals("abc"); } }
```

```
[java::T.main...assertion.1] line 3 ... : FAILURE
VERIFICATION FAILED
```

The same string, compared to itself, with `compareTo`:

```java
assert "abc".compareTo("abc") == 0;
```

```
[java::U.main...assertion.1] line 3 ... : SUCCESS
VERIFICATION SUCCESSFUL
```

`compareTo`, `startsWith`, `isEmpty`, `length`, `charAt` and `instanceof` all
behave. The defect is in `CProverString.equals`. No Kotlin involved — this is
`javac` output.

## Why it matters here

`ASSURANCE.md` predicted the Kotlin corner would top out at "R3 + bounded"
because **Kotlin has no mature deductive verifier**. That prediction is
correct, and the reason given is also correct as far as it goes: no deductive
verifier for Kotlin was found, and ESBMC 8.4 advertises Kotlin via Soot/Jimple
but the shipped build answers `frontend for Jimple was not built on this
version of ESBMC` — and is a BMC regardless, so it could not lift the ceiling.

But the *operative* limit turned out to be something else entirely, and it is
sharper and more transferable:

> **JBMC can reason about anything that does not compare two strings.**

Of 15 obligations built as JBMC entry points: **7 VERIFIED, 0 REFUTED, 8
BLOCKED**. Every blocker traces to one of three tool defects, not to Kotlin:

1. `CProverString.equals` — above. This alone blocks every timeline
   obligation, because `t.author != user` and `HashSet<Edge>` membership both
   reduce to string equality. The tell is decisive: **the same obligation over
   an empty log verifies with 0 of 964 goals failing.** The logic is fine; the
   comparison is not.
2. `String.getBytes(Charset)` is nondeterministic — its model dispatches on
   `Charset.name()` through that same broken intrinsic. This blocks
   `validHandle` / `validText`, and therefore every service path that begins
   with validation: exactly the D4/D6 ordering decisions the TLA+ model leaves
   open.
3. SAT exhaustion — 11 GB RSS on a nondeterministic `limit` over a 4-entry log.

## What was proved

The load-bearing one: **F005's monotonicity premise** — ids strictly increase
and `createdAt` never falls, over every tick pattern, three appends deep. That
is the premise the sort-free timeline rests on, and it is the one obligation
this corner most needed. Also the `parseInt64` accept set, over *all* one- and
two-character strings.

Every canary guarding a claimed obligation was refuted, so the seven are real
in the F013 sense, not vacuous.

## The consequence nobody had noticed

**This wall is shared with the Java corner.** `ASSURANCE.md` listed Java's R4
route as "OpenJML / KeY", neither of which has been attempted — already
corrected to R3 in F012. But if the intent was ever to reach Java's R4 via
JBMC, that path is blocked by the same defect, for the same reason, and no
amount of Java-side effort moves it.

So the corrected reading of the ceiling table is:

| corner | limited by |
|---|---|
| Kotlin | no deductive verifier **and** JBMC cannot compare strings |
| Java | JBMC cannot compare strings; OpenJML/KeY unattempted and unprobed |
| Rust | `RwLock` has no Verus model (F012); proofs are on twins |
| Go | Gobra ghost-language limits — no string indexing, single-expression `pure` |

Every corner is limited by a *tool* gap rather than a language one, and three
of the four gaps are about the standard library — strings, locks, collections.
That is the same shape as F007, arriving for the fourth time.

## Note on how this was found

The agent that hit the wall did not report "Kotlin is hard." It reproduced each
blocker in plain Java and separated tool defect from language cost. Without
that step the finding would have been recorded as a property of Kotlin, which
is false and would have mis-priced the whole corner — and would have left the
Java corner's identical exposure undiscovered.
