# The Kotlin R5 rung's gate

Standing rule 2: no gate is trusted until it has been shown to fail. This
directory is `jbmc r5verify` shown doing all four things it can do, on real
trees, with the verdict read from the tool's own line and never from an exit
code.

| file | tree | verdict |
|---|---|---|
| `00-clean.log` | `impls/kotlin` | `R5 PASSED` — 5 of 5 clauses, every one refutable |
| `01-kill-...` | mutant `tick-advances-by-two` | `R5 FAILED`, clause 36 named |
| `02-survival-...` | mutant `limit-off-by-one` | `R5 PASSED` — a live mutant R4 catches and R5 does not |
| `03-undecided-...` | the `lastOrNull` vacuity tree | `R5 UNDECIDED`, no verdict |
| `04-undecided-budget.log` | `impls/kotlin`, `-budget=25s` | `R5 UNDECIDED`, no verdict |
| `05-kill-with-one-unplaceable.log` | the off-by-one projection tree | `R5 FAILED`, 1 of 2 failures placed |
| `06-survival-...` | mutant `follow-toggles` | `R5 PASSED` — and the limitation that is |
| `07-calibrate-end-to-end.log` | three mutants through `calibrate -rungs R5` | the R5 kotlin cell, measured |
| `08-raw-jbmc-goal-lines.txt` | — | the JBMC lines the attribution is read out of |
| `09-r4-unaffected.log` | `impls/kotlin` | `R4 PASSED` — 7 of 7, unchanged by the abs projections |

## Reproducing each one

```
go build -o /tmp/jbmc ./tools/cmd/jbmc

# 00
/tmp/jbmc r5verify -impl=kotlin -budget=15m

# 01, 02, 06
go run ./tools/cmd/mutate apply -impl=kotlin -id=tick-advances-by-two -out=/tmp/m
/tmp/jbmc r5verify -impl=kotlin@tick-advances-by-two -registry=/tmp/m/registry.json -budget=15m

# 04
/tmp/jbmc r5verify -impl=kotlin -budget=25s -ob-budget=25s
```

The two UNDECIDED canary trees are ordinary copies of `impls/kotlin` with one
line changed each. They are not committed, for the same reason the Go corner's
CANARY H lives under `_broken/`: a tree that must not verify does not belong in
the tree that must.

```
# 03 -- the documented vacuity recipe. Store.kt's own comment records that
# `lastOrNull` adds a checkcast JBMC cannot discharge, which makes everything
# after it infeasible. Expect bad-dynamic-cast FAILURE in Store, and k11/k13
# coming back VERIFIED instead of refuted: a claim and its negation both
# verifying is the F013 vacuity signature and nothing else produces it.
cp -R impls/kotlin /tmp/u2 && sed -i \
  's|val last = if (log.isEmpty()) null else log\[log.size - 1\]|val last = log.lastOrNull()|' \
  /tmp/u2/src/twitterport/store/Store.kt
/tmp/jbmc r5verify -impl=/tmp/u2 -corner=kotlin -budget=15m

# 05 -- the log-axis projection reads one position past the one it names, so
# clause 13 is genuinely refuted AND a null-pointer check fails in Store. The
# verdict is the kill, and the sentence discloses that only 1 of the 2 failures
# could be placed on a clause.
cp -R impls/kotlin /tmp/u1 && sed -i \
  's|fun absLogIdAt(i: Int): Long = log\[i\].id|fun absLogIdAt(i: Int): Long = log[i + 1].id|' \
  /tmp/u1/src/twitterport/store/Store.kt
/tmp/jbmc r5verify -impl=/tmp/u1 -corner=kotlin -budget=15m
```

## Why 09 is here

The R5 rung needed an abstraction function, which meant adding eight methods to
the shipped `Store` class (F045). R4 runs over the same bytecode, so "did that
change R4's answer" is a question the gate has to answer rather than assume.
It did not: 7 of 7 decidable obligations VERIFIED, 0 REFUTED, 0 VACUOUS, 0
UNDECIDED, every canary refuted — the same numbers `ASSURANCE.md` already
records. `R0` is likewise unchanged: `replay -impl=kotlin` still reports
`R0 PASSED: every step matches S_obs byte-for-byte`.

## What 06 is, and why it is here rather than hidden

`follow-toggles` makes `Store.addFollow` toggle an edge instead of adding it.
That breaks R5 clause 7 — and clause 7 is BLOCKED on this corner by F014, so
the rung never runs it and the mutant reads as an R5 survival.

That is the honest consequence of F022's accounting applied to a rung rather
than to an obligation: the blocked clauses are in neither the numerator nor the
denominator of the *obligation* count, and the verdict sentence says so, but
the *mutant* still gets a cell and that cell says "survived". A reader of the
R5 kotlin row has to know that the follows axis is invisible to it. The
transcript is kept so the row is not read as though it were not.
