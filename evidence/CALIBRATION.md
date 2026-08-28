# The kill table

36 mutants (18 defects × Go and Rust), three rungs, every cell guarded so the
mutant — not the original — was the thing measured.

Raw run: `evidence/runs/calibration/full/`.

```
rung             live  killed  survived  unreached   kill%      wall
R0 corpus          36      36         0          0    100%       57s
R1 diff-fuzz       35      35         0          0    100%     1465s
R2 property        35      14        19          2     42%     2495s
```

By corner, the two are close to identical — 18/18/18 on R0, and R2 killing 7
of 17 in Go and 7 of 18 in Rust. The corners behave the same under mutation,
which is what F006's fix was supposed to achieve and is the first direct
evidence it did.

---

## Read the 100%s carefully — they are partly a selection effect

**The catalogue and the corpus are drawn from the same source.** Every mutant
injects a violation of something `S_obs` pins, and `generated/conformance.jsonl`
is generated *from* `S_obs` to exercise exactly those terms. R0 scoring 100% is
therefore as much a statement about that alignment as about R0's power.

This is not a flaw in the measurement so much as the boundary of what it can
claim. The table says: **against defects in the contract's own terms, the
corpus catches all of them.** It does not say the corpus catches defects
nobody thought to specify — and F008 and F009 are two occasions when it
demonstrably did not, both closed by *adding inputs*, which is what makes R0
look this good now.

A catalogue built from a different source — production incident reports, a
fuzzer's crash corpus, defects found in the real Java library — would produce a
different and more informative table. That is the honest next step for this
number, and it is not yet done.

---

## R1 killed nothing R0 missed, at 26× the cost

Zero additional kills for 1465 seconds against 57.

That reads like an argument against differential testing, and it is the
opposite. **R1's value was real and has been absorbed.** When F008 was found,
R1 caught four live divergences that R0 could not see — and the fix was to add
those shapes to the corpus generator, which R0 replays. The differential rung
found the gap; closing the gap moved the coverage into the cheaper rung.

That is the dynamic worth carrying: *a differential rung earns its cost by
discovering what the corpus lacks, not by re-catching it forever.* Its steady
state should be quiet. Judged only on a steady-state kill rate it looks
worthless, and dropping it on that basis would remove the only mechanism that
finds the next F008.

---

## R2 is the weakest rung and the most expensive

42% kill rate, 2495 seconds. It missed 19 live mutants, spread across every
family:

| family | missed |
|---|---|
| clock | 4 |
| pagination | 4 |
| parsing | 4 |
| id-alloc | 3 |
| existence | 2 |
| precedence | 2 (+2 unreached) |

This is not a defect in R2 — it is what R2 *is*. Its nine relations assert
things like "following twice equals following once" and "pages partition the
timeline exactly." A wrong error-code vocabulary, an off-by-one id, a frozen
clock, a lenient parser: none of these violate any relation. The timeline stays
ordered, the pages still partition, follow stays idempotent. **R2 checks
consistency, not conformance**, and most of this catalogue is conformance.

R2's justification was never its kill rate. It is the one rung that never
consults `S_obs`, so it is the only one that would survive the reference
machine being wrong — which F010 showed is a live risk, not a hypothetical.
**It is insurance, and this table prices it: about 44× R0 per defect caught.**

---

## The cost figures are mostly not measuring the rungs

`calibrate` measures each corner's per-launch floor and subtracts it. For R2
the correction goes **negative on both corners** — go −91.0s, rust −230.3s —
which means the rung's own work costs less than the servers it launches. At
this configuration **R2 is process startup.**

That reframes the 44× multiple: it is a property of the harness, not of
property-based testing. R2 relaunches once per property-round; batching rounds
against one server would collapse most of the difference. The comparison to
make between rungs is **launches**, not seconds.

Go's floor is 4040 ms/launch and Rust's 4853 ms — with build checks of 479 ms
and 89 ms respectively, so the bulk is process start and health-wait, not
compilation.

---

## What actually transfers

1. **A cheap conformance rung can dominate an expensive one, and the reason
   may be that the corpus already absorbed the expensive rung's findings.**
   Check whether your differential lane is quiet because it is redundant or
   because it is blind — those look identical from the kill rate.
2. **Judge a discovery rung on what it found once, not on what it catches
   now.** R1's steady-state contribution here is zero and it is still the most
   valuable rung in the stack.
3. **Price insurance as insurance.** R2 is 44× R0 per defect and worth keeping
   for a reason that never appears in the kill column.
4. **Compare rungs by launches, not by seconds**, unless you have measured the
   floor and subtracted it — otherwise you are ranking languages and process
   startup.
5. **A kill table is only as good as where its defects came from.** These came
   from the contract, and the corpus is generated from the contract. Ours is
   the easy case.
