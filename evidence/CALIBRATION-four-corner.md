# The kill table — four corners

72 mutants (18 defects × Go, Rust, Java, Kotlin), three rungs, 216 cells. Every
cell guarded so the mutant — not the original — was the thing measured.

Raw run: `evidence/runs/calibration/four-corner/`.
Window: 2026-08-30T22:33:29Z .. 2026-08-31T03:30:42Z.

```
rung             live  killed  survived  unreached  equiv   kill%   kill%     wall
                                                            reach    live
R0 corpus          72      72         0          0      0    100%    100%     188s
R1 diff-fuzz       72      72         0          0      0    100%    100%    6202s
R2 property        72      28        40          4      0     41%     39%   10206s
```

By corner:

```
corner   rung             live  killed  survived  unreached  equiv     wall
go       R0 corpus          18      18         0          0      0      10s
go       R1 diff-fuzz       18      18         0          0      0     349s
go       R2 property        18       7        10          1      0     588s
rust     R0 corpus          18      18         0          0      0       3s
rust     R1 diff-fuzz       18      18         0          0      0      82s
rust     R2 property        18       7        10          1      0     135s
java     R0 corpus          18      18         0          0      0      20s
java     R1 diff-fuzz       18      18         0          0      0     647s
java     R2 property        18       7        10          1      0    1084s
kotlin   R0 corpus          18      18         0          0      0     155s
kotlin   R1 diff-fuzz       18      18         0          0      0    5125s
kotlin   R2 property        18       7        10          1      0    8398s
```

---

## Read the 100%s carefully — they are partly a selection effect

This caveat leads the document because it bounds everything below it.

**The catalogue and the corpus are drawn from the same source.** Every mutant
injects a violation of something `S_obs` pins, and `generated/conformance.jsonl`
is generated *from* `S_obs` to exercise exactly those terms. R0 scoring 100% is
therefore as much a statement about that alignment as about R0's power.

The table says: **against defects stated in the contract's own terms, the corpus
catches all of them.** It does not say the corpus catches defects nobody thought
to specify. F008 and F009 are two recorded occasions when it demonstrably did
not, and both were closed by *adding inputs* — which is a large part of why R0
looks this good now.

A catalogue drawn from a different source — production incidents, a fuzzer's
crash corpus, real defect history in the Java library — would produce a
different and more informative table. That remains the highest-value follow-up,
and it is still not done.

## Both R0 and R1 killed everything — flag, not a result

R0 killed 72/72. R1 killed 72/72. **A rung that kills everything is as
suspicious as one that kills nothing**, and here two of the three did.

Neither number distinguishes the rung from a stronger one. A catalogue that R0
finds trivial cannot rank R0 against R1, and the fact that R1 added *zero*
kills over R0 across all four corners does not mean R1 is worthless — it means
this catalogue cannot see the difference. `CALIBRATION.md` argued the same point
for two corners and the argument is unchanged: R1's value was real and has been
absorbed. When F008 was found, R1 caught four live divergences R0 could not see,
and the fix moved that coverage into the cheaper rung. A discovery rung's steady
state should be quiet.

The honest reading is that **this catalogue has no discriminating power between
R0 and R1.** Judged on this table alone you would drop R1, and dropping it would
remove the only mechanism that finds the next F008.

R2 is the one rung with a non-degenerate number, at 41%.

## Per F017: R0/R1/R2 compare cleanly across corners — and they did, exactly

F017 predicted that behavioural rungs compare cleanly across corners **because
they only see the API**, while proof rungs would not. That prediction is
explicitly confirmed here, and more strongly than expected.

**All four corners produced identical outcome vectors on all 18 defects.** Not
similar — identical. Every defect has the same (R0, R1, R2) triple in Go, Rust,
Java and Kotlin. Every one of the 10 R2 survivors survived in all four corners;
`follow-precedence-flipped` was unreached in all four.

This is the first direct evidence for F017's rule 1, and it is worth stating
plainly what it does and does not license:

- It **does** license putting the four corners' R0/R1/R2 rows side by side.
  The behavioural rungs drive the observable API and saw the same defect
  regardless of how many sites each corner enforces a property at, or which
  side of the TCB boundary the property sits on.
- It **does not** extend to R4/R5, which were not run (see below). F017's
  rule 2 stands untested by this run.

### The uniformity is also worth being suspicious of

Perfect agreement across four independent implementations is a strong claim, and
this project has a recorded finding about exactly this failure mode. **F006
documented two parallel implementations sharing a blind spot** — they agreed on
30 of 39 shared divergences, and the agreement was a property of their common
author and common contract, not evidence of correctness.

The same caution applies here. These four corners were written by one author
against one contract, and they are byte-identical at R0 (56/56 each). Identical
mutation behaviour is what you would expect from four correct implementations of
one spec — and *also* what you would expect from four implementations sharing an
authorial blind spot. This table cannot tell those apart. It should not be read
as four independent confirmations; it is closer to one measurement taken four
times.

What it *does* rule out is the specific worry that drove F017: that differing
enforcement-site counts would make the behavioural rows incomparable. They did
not.

## R2 is the weakest rung and the most expensive

41% kill rate. It missed the same 10 defects in every corner:

| defect | R2 verdict, all four corners |
|---|---|
| `id-first-is-two` | SURVIVED |
| `id-burned-on-reject` | SURVIVED |
| `self-follow-guard-dropped` | SURVIVED |
| `orphan-author-accepted` | SURVIVED |
| `created-at-frozen` | SURVIVED |
| `tick-advances-by-two` | SURVIVED |
| `next-cursor-always-emitted` | SURVIVED |
| `limit-off-by-one` | SURVIVED |
| `unknown-json-fields-accepted` | SURVIVED |
| `repeated-query-param-accepted` | SURVIVED |
| `follow-precedence-flipped` | unreached |

This is not a defect in R2 — it is what R2 *is*. Its relations assert things
like "following twice equals following once" and "pages partition the timeline
exactly." A wrong error vocabulary, an off-by-one id, a frozen clock, a lenient
parser: none of these violate any relation. The timeline stays ordered, the
pages still partition, follow stays idempotent. **R2 checks consistency, not
conformance**, and most of this catalogue is conformance.

R2's justification was never its kill rate. It is the one rung that never
consults `S_obs`, so it is the only one that would survive the reference machine
being wrong — which F010 showed is a live risk, not a hypothetical.

## Cost is dominated by process launch, and the floor differs by language

**Compare rungs by launch count, not by seconds.** The floors make the reason
unavoidable:

```
corner   floor/launch   of which build   samples        spread
rust          101 ms          79 ms          3     100- 102 ms
go            562 ms         539 ms          3     542- 585 ms
java         1071 ms         993 ms          3    1046-1086 ms
kotlin       8755 ms        8654 ms          3    8684-8828 ms
```

Kotlin's floor is **87× Rust's**. R2 is 54 launches per mutant on every corner,
so the same rung doing the same work costs radically different wall time:

```
corner   rung          launches  wall clean  tool clean
go       R2 property         54       32.7s       2.4s
rust     R2 property         54        7.5s       2.0s
java     R2 property         54       60.2s       2.4s
kotlin   R2 property         54      465.0s      -7.8s *
```

`tool clean` is wall time minus (launches × measured floor) — the rung's own
cost with process startup removed. On three corners it lands within 0.4s of
itself: **2.0s, 2.4s, 2.4s**. R2's actual work is about two seconds and does not
depend meaningfully on the language. Everything else in that 7.5s-to-465s spread
is process startup.

On Kotlin the correction goes negative (−7.8s), which the tool reports rather
than hides: the rung's own work costs less than the servers it launches, so **at
this configuration Kotlin's R2 simply *is* process startup.** Ranking corners by
R2 seconds would rank JVM cold-start, not property testing.

The same holds at R1 without a launch count to correct by: identical kill
results (18/18 everywhere) at 82s on Rust and 5125s on Kotlin — a 62× spread
measuring nothing about the rung.

### These seconds are not comparable with `evidence/CALIBRATION.md`

That document measured Go at **4040 ms/launch** and Rust at **4853 ms**. This run
measures **562 ms** and **101 ms** — roughly 7× to 48× lower. This is a faster
host, and the earlier document's own conclusions reflect that: there, *both*
corners' R2 corrections went negative (−91.0s, −230.3s), whereas here three of
four are positive.

**Only kill rates and launch counts transfer between the two documents. Wall
clock does not.** The 44×-R0-per-defect figure quoted in `CALIBRATION.md` is a
property of that host, not of the rungs.

## Which rungs were unavailable, and what that excludes

**This run is R0/R1/R2 only. No R4 or R5 claim can be made from it.**

Two tools that the documented base image lists as pre-installed are not usable
on this host, and each costs a rung:

- **No Docker daemon.** `matrixctl doctor` reports `docker daemon — not
  available: exit status 1`. The Gobra jar is extracted from the image at
  container build time, so `/opt/gobra` is empty. **Go R4 unavailable.**
- **Verus present but refuses to run.** The binary exists at
  `/usr/local/bin/verus` and exits non-zero with
  `required rust toolchain 1.95.0-x86_64-unknown-linux-gnu not found`; the host
  has rustc 1.94.1. `doctor` reports it as `present but not runnable`.
  **Rust R4 unavailable.**

Both contradict the documented environment rather than reflecting a deliberate
scope choice, and both were discovered by running the tools rather than by
trusting the image description. `jbmc` is present, but with R4 out on two
corners there was no comparable proof row to build.

What this excludes:

1. **F017's rule 2 is untested.** Its central limitation — that R4/R5 rows are
   comparable only where a defect sits on the same side of the TCB boundary in
   both corners — is precisely the claim this run cannot examine. The
   `next-cursor-*` defects, which F017 identifies as crossing that boundary
   (verified core in Java and Kotlin, trusted shim in Go and Rust), appear in
   this table only through behavioural rungs, where F017 says they compare
   cleanly. They do.
2. **The proof rungs' cost is unmeasured**, so the question GOAL.md exists to
   answer — what closing a deductive rung is worth — is not advanced by this
   run. The kill table has three of its five rows.
3. The recorded ceilings from earlier work (Gobra's ~43 load-bearing clauses,
   Verus's 1 proved property per F016, JBMC's string-equality defect per F014)
   are **not re-confirmed here.**

## What actually transfers

1. **Behavioural rungs really do compare across ports.** F017 predicted it; four
   corners agreeing on all 18 defects at all three rungs is about as clean a
   confirmation as the design allows. If you are calibrating a port, the
   API-level rungs are the ones whose numbers you can put in one table.
2. **Perfect cross-implementation agreement is not independent confirmation**
   when one author wrote all the implementations against one contract. F006 is
   the worked example. Count it as one measurement, not four.
3. **Compare rungs by launches, not seconds.** Here the same rung's own work was
   ~2.2s on every corner where it could be measured, inside a wall-clock spread
   of 62×. Anything you conclude from raw seconds across languages is a
   statement about process startup.
4. **A rung that kills everything has told you about your catalogue, not your
   rung.** Two of three rungs here are at 100% and neither can be ranked from
   this table.
5. **A kill table is only as good as where its defects came from.** These came
   from the contract, and the corpus is generated from the contract. Ours is
   still the easy case, and a second catalogue from an independent source
   remains the work that would make this number informative.
