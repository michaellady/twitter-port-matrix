# F022 — The proof rung's denominator is set by the trusted shim, not by the proof

**Status:** measured while adding R4 to `calibrate` as a rung
**Class:** a property of the verification matrix, not a defect — and one that
inverts a kill rate if it is not counted

## What was measured

The first R4 cells the kill table has. Two Go mutants, one Gobra invocation
each, verdict read from the tool's own last line:

```
go/next-cursor-is-first-id   R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [1m53.7s]
go/limit-off-by-one          R4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [1m9.8s]
```

Both mutants are live and both are pagination defects. The one Gobra refutes
edits `internal/store/memstore.go`. The one it passes edits
`internal/httpshim/shim.go`, which is **not in the verification matrix**: the
five verified packages are `clock`, `ids`, `dom`, `store` and `service`, and
the shim is trusted transport by the decision recorded in GOAL.md 1c and F004.

`mutate probe` says the passed mutant is live and easy to reach — the corpus
tells it apart at request 42, and three of four random traces do too:

```
go/next-cursor-is-first-id     live
    corpus     differs at request 42
    seed=1     no difference in 120 requests
    seed=2     differs at request 51
    seed=3     differs at request 11
    seed=4     differs at request 66
```

## Why it matters to the table

Scored the way the empirical rungs are scored, that cell reads **survived** —
a live defect the rung passed, i.e. a hole in the contract. It is not one. No
obligation in the five verified packages mentions `next_cursor`'s value,
because no obligation is written over the file that computes it. The rung was
never given the chance the word "survived" implies.

The kill rate flips on this distinction. Over this pair:

| scoring | kill% reachable |
|---|---|
| shim mutant counted as survived | 50% |
| shim mutant counted as unreached | 100% |

Neither number is wrong; they answer different questions. The first is R4 *as
configured on this corner*, shim included. The second is R4's *oracle* over
what it actually reads. The table already reports both columns for R0–R2, and
the proof rung needs the same split for the same reason — F009 made this
argument for the corpus, and F008 asked for the coverage denominator by name.

So `calibrate`'s R4 entry carries a `Covers` predicate: the analogue of "no
corpus step elicits it" for a rung with no input distribution is "the verifier
reads none of the files the mutant edits". The cell records it in those words:

```
live (reached by corpus, seed=2, seed=3, seed=4), but the verifier reads none
of the files this mutant edits (internal/httpshim/shim.go); no obligation
covers it
```

## The size of it

Over the whole Go catalogue, by the same test:

```
go mutants: 18   covered by Gobra: 14   outside the verified core: 4
  next-cursor-always-emitted     pagination   internal/httpshim/shim.go
  next-cursor-is-first-id        pagination   internal/httpshim/shim.go
  unknown-json-fields-accepted   parsing      internal/httpshim/shim.go
  repeated-query-param-accepted  parsing      internal/httpshim/shim.go
```

**R4 on the Go corner has a ceiling of 14 of 18, or 78%, before a single
obligation is written.** The four it cannot reach are exactly two families:
wire-format parsing and cursor emission. That is the price of the 1c decision
to keep semantics in the verified core and wire format in the trusted shim,
and it is the first time that price has a number rather than a rationale.

The decision still looks right — putting the wire format inside the proof
would green R4 over code no verifier reads, which is F004's argument — but a
transfer write-up that says "the deductive rung caught N%" without saying
"over 78% of the catalogue" is overstating the layer by up to a fifth.

## What this does not say

It says nothing about whether the 14 covered obligations are themselves
refutable. That is the negation canary's question (F013, F021), and it is
prior to this table: a rung that kills a mutant by way of a vacuous obligation
has still not earned the kill. The R4 rung entry does not re-audit vacuity per
mutant; it reads Gobra's verdict on a tree whose contract was audited once,
separately.
