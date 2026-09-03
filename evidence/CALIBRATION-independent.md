# The kill table over a catalogue the contract did not write

43 mutants, 12 defects, four corners. The mutants are
`tools/cmd/mutate/mutants-independent.json`; where their defect ideas came from,
and the honest accounting of how independent that is, is F047.

Raw runs: `evidence/runs/independent-catalogue/`.
Baseline to compare against: `evidence/CALIBRATION-four-corner.md`.

---

## The headline, both tables side by side

```
                      ORIGINAL catalogue (72)              INDEPENDENT catalogue (43)
rung             live  kill/reach   kill/live    wall  live  kill/reach   kill/live    wall
R0 corpus          72  72/72 =100%  72/72 =100%   188s    43  31/31 =100%  31/43 = 72%   118s
R1 diff-fuzz       72  72/72 =100%  72/72 =100%  6202s    43  31/31 =100%  31/43 = 72%  3151s
R2 property        72  28/68 = 41%  28/72 = 39% 10206s    43  17/35 = 49%  17/43 = 40%  6206s
```

**The oracles generalise. The inputs do not.**

Read the two rate columns against each other, because they answer different
questions and they move in opposite directions here.

- `kill/reach` is the rung's **oracle**: of the defects this rung's inputs
  actually elicit, how many does it notice? It is **unchanged**. R0 and R1 still
  catch 100% of what reaches them. R2 lands at 49% where it has always landed
  around 41%.
- `kill/live` is the rung **as configured**, inputs included. It falls from
  100% to 72% at R0 and R1.

The entire difference is one number: **12 of 43 mutants are `unreached` — live
defects that no corpus step and no `tracegen` trace tells apart from the
original.** The original catalogue has **zero** such mutants at R0 and R1.

So the answer to "do the published 100%s measure the rungs, or measure the fact
that catalogue and corpus share a parent" is: **both, and separably.** The
oracle half of those 100%s survives contact with a catalogue drawn from
somewhere else. The *coverage* half does not, and it was never what the
percentage said.

## By corner — and every corner gives the identical answer

```
corner   rung             live  killed  survived  unreached  kill/reach     wall
go       R0 corpus          11       8         0          3   8/8 = 100%      6s
go       R1 diff-fuzz       11       8         0          3   8/8 = 100%    159s
go       R2 property        11       5         4          2    5/9 =  56%    325s
rust     R0 corpus          10       7         0          3   7/7 = 100%      1s
rust     R1 diff-fuzz       10       7         0          3   7/7 = 100%     40s
rust     R2 property        10       4         4          2    4/8 =  50%     75s
java     R0 corpus          11       8         0          3   8/8 = 100%     11s
java     R1 diff-fuzz       11       8         0          3   8/8 = 100%    313s
java     R2 property        11       4         5          2    4/9 =  44%    653s
kotlin   R0 corpus          11       8         0          3   8/8 = 100%    100s
kotlin   R1 diff-fuzz       11       8         0          3   8/8 = 100%   2640s
kotlin   R2 property        11       4         5          2    4/9 =  44%   5152s
```

Per defect, `K` killed, `s` survived (the rung's inputs reach it and it passed
anyway), `-` unreached, `...` no twin in that corner:

```
defect                            go     rust   java   kotlin      R0 R1 R2
handle-bound-off-by-one          ---    ---    ---    ---
limit-bound-off-by-one           --K    --K    --K    --K
text-rejects-space               KKs    KKs    KKs    KKs
timeline-drops-oldest            KKs    KKs    KKs    KKs
visibility-args-swapped          KKK    KKK    KKK    KKK
visibility-negation-dropped      KKK    KKK    KKK    KKK
wrong-error-branch-cursor        KKs    KKs    KKs    KKs
handle-case-folded               KKs    KKs    KKs    KKs
next-cursor-off-by-one           KKK    KKK    KKK    KKK
text-length-in-code-points       ---    ---    ---    ---
go-slice-len-aliasing            KKK    ...    ...    ...
jvm-string-identity-compare      ...    ...    KKs    KKs
```

**Every corner produces the identical outcome vector, defect for defect and
rung for rung.** F017 predicted exactly this for behavioural rungs — they only
see the API, so the same defect is the same defect — and it was established on
the original catalogue. It now holds on a catalogue F017 never saw, which is
the first evidence that F017 is a statement about the rungs rather than about
that catalogue.

## What the 12 unreached mutants are, and why that is the finding

Three defects, rendered in all four corners:

| defect | the input it needs | is that input anywhere in the rig? |
|---|---|---|
| `handle-bound-off-by-one` | a handle of exactly 32 bytes | no. Corpus handles are `alice`, `bob`, `carol`, `dave`, `x`, `Alice`, `""` — longest 5 bytes. `tracegen` emits `u{n}` and `ghost{n}` — longest 8 |
| `limit-bound-off-by-one` | `?limit=100` | not in the corpus (`0`, `2`, `101`, `abc`) and not in `tracegen`'s alphabet (`1`–`5`, `0`, `101`, `abc`) |
| `text-length-in-code-points` | a tweet over 280 **bytes** but under 280 **characters** | no. Every generated text is ASCII; the one long text is 281 ASCII bytes, which both the original and the mutant reject |

None of the three is exotic. They are the **boundaries of the contract's own
constants** — 32, 100, 280 — and the values that sit exactly on them are absent
from every input source the rig has. The corpus was generated from `S_obs` to
exercise its **decisions**; nothing generated it to exercise its **bounds**.

That is a sharper statement than "the catalogue and the corpus share a parent".
The parent is shared *and the child was not asked for boundary values*. A
mutation catalogue derived from the same decisions never notices, because it
never proposes a boundary mutant. An operator taxonomy proposes them
immediately — it is the first thing ROR does — and 3 of its 7 defects landed
outside everything the rig can see.

**F009's lesson, at four times the scale.** F009 found one unreachable mutant
and closed it by adding one corpus step. This finds 12, and the fix is the same
shape: widen the input alphabet to include the boundary of every constant the
contract pins. That is an inputs change, not a rung change, and the table
already separates the two.

## R2 kills something R1 cannot — the first time in this project

`limit-bound-off-by-one` is `unreached` at R0 and R1 and **killed at R2**, in
all four corners. `mutate probe` agrees with R0 and R1: neither the corpus nor
any of four `tracegen` traces tells the mutant apart. R2's own generators reach
`?limit=100` anyway.

`evidence/CALIBRATION-four-corner.md` says of the original catalogue that "R1
added *zero* kills over R0 across all four corners" and that "this catalogue has
no discriminating power between R0 and R1". The same is still true here — R0 and
R1 have identical outcome vectors on all 43 — but **R2 is no longer dominated**.
On the original catalogue R2 is strictly weaker than R0 and R1; here it catches
one defect they both miss, because its inputs are generated rather than drawn
from a fixed alphabet. That is a property of R2 nobody could see before.

## Cost

Wall time is roughly halved against the baseline because the catalogue is 43
mutants rather than 72. Per mutant the two runs agree closely, which is the
useful reading — the independent catalogue is not cheaper or dearer to judge:

```
                       original (72)        independent (43)
rung             total s   per mutant   total s   per mutant
R0 corpus            188        2.6s        118        2.7s
R1 diff-fuzz        6202       86.1s       3151       73.3s
R2 property        10206      141.8s       6206      144.3s
```

Launch floors re-measured on this run: go 462 ms, java 1012 ms, kotlin 8929 ms,
rust 97 ms per launch. R1 pays 20 launches per mutant and R2 pays 54, so of
kotlin's 207.9 s mean clean R1 pass, 20 x 8.929 = 178.6 s is process startup
before any rung work happens — 86% of it. The rung's own cost there is 29.3 s.
That is the same split the baseline reports and it is why raw seconds are never
quoted here without the floor beside them.

## What this does and does not bound

**It does bound the published R0/R1 100%s.** Quoted as `killed/live` they are
specific to a catalogue every one of whose defects the rig's inputs happen to
reach. On a catalogue built from an operator taxonomy the same rungs score
72%. Anyone transferring "R0 catches everything" to another project should
transfer `31/31 = 100% of what the corpus reaches, over inputs that reach 31 of
43 defects`, and the second half is the part that will not survive the move.

**It does not bound R2's 41%.** R2 scores 49% here against 41% there, which is
noise at this sample size and in the same band either way.

**It does not establish that the catalogue is fully independent.** F047 records
the accounting mutant by mutant, including one — `text-length-in-code-points` —
that both JVM corners had already anticipated in comments and which is therefore
*not* independent evidence. It is one of the three unreachable defects, so the
`kill/live` drop does not rest on it: removing it entirely leaves 2 of 11
defects unreachable and `kill/live` at 31/39 = 79%, still far from 100%.

## Gate verdicts

Neither table means anything unless every mutant was really injected and really
changes behaviour. Both gates were run over the whole catalogue and both were
shown able to fail.

```
mutate verify (verify.log)
  anchors: 43/43 match exactly one site
  compile: 43/43 build clean
  verify PASSED: every anchor matches one site; every mutant compiles

mutate verify, canary (canary-verify-drift.log)
  anchors: 0/1 match exactly one site
  verify FAILED: 1 mutant(s): go/drifted-anchor-canary

mutate probe (probe-{go,rust,java,kotlin}.log)
  live: 11/11   no observable change: 0     probe PASSED    [go]
  live: 10/10   no observable change: 0     probe PASSED    [rust]
  live: 11/11   no observable change: 0     probe PASSED    [java]
  live: 11/11   no observable change: 0     probe PASSED    [kotlin]

mutate probe, canaries (canary-probe-*.log)
  verdict   NO OBSERVABLE CHANGE in 536 requests   live: 0/1   probe FAILED
  verdict   NO OBSERVABLE CHANGE in 539 requests   live: 0/1   probe FAILED
```

`calibrate` additionally guards, per mutant, that the tree a rung resolved is
the mutant tree and not the untouched original — `guard 5/5 checks` on every one
of the 129 cells.
