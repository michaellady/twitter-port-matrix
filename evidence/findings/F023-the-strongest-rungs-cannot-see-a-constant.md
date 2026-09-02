# F023 — The two strongest rungs cannot see a constant, and the two cheapest kill it instantly

**Status:** measured while adding R5 to `calibrate` as a rung
**Class:** a limit of relational specification, not of Gobra — and the first
measured case where assurance is not monotone in cost

## The result

`go/id-first-is-two` shifts the id generator's origin from 1 to 2. Every id in
the system is off by one, from the first request onward. Across all five rungs
that now exist on the Go corner:

| rung | outcome | what said so |
|---|---|---|
| R0 corpus | **killed** | `R0 FAILED: 17 step(s) disagree with S_obs` |
| R1 diff-fuzz | **killed** | `R1 FAILED: at least one implementation diverges from S_obs` |
| R2 property | survived | `R2 PASSED: every relation holds on every generated state` |
| R4 proof | survived | `R4 PASSED: Gobra has found 0 error(s) over 5 package(s)` |
| R5 refinement | survived | `R5 PASSED: Gobra has found 0 error(s); no refinement clause failed` |

`mutate probe` puts the defect at the very first request of every input source:

```
go/id-first-is-two     live
    corpus     differs at request 0
    seed=1     differs at request 0
    seed=2     differs at request 0
    seed=3     differs at request 0
    seed=4     differs at request 0
```

There is no reachability excuse. The mutant is as visible as a defect can be,
and the three most expensive rungs pass it.

## Why

The whole contract on the id generator is relational. `(*Generator).Next`
carries R5 clause 20, in three parts:

```
// @ ensures result == old(unfolding acc(g.LockP()) in g.next)
// @ ensures unfolding acc(g.LockP()) in g.next == result + 1
// @ ensures result >= 1
```

Each one still holds when the origin is 2. The counter still advances by
exactly one; each issued id still equals the counter's previous value; 2 is
still at least 1. And `New`, which is where the constant actually lives,
carries no functional postcondition at all:

```
// New returns a Generator that issues 1, 2, 3, …
// @ ensures acc(g.LockP())
// @ ensures g != nil
func New() (g *Generator) {
	g = &Generator{next: 1}
```

The origin is stated three times in English — the package comment, the type
comment, the function comment — and zero times in a clause. R2 misses it for
the same reason: its nine relations are relations.

R0 and R1 catch it because they do not reason about the implementation at all.
They compare its bytes against a concrete reference trace, and a reference
trace contains constants.

## This is F002 with a number attached

F002 recorded that `S_obs` starts ids at 0 while both implementations start at
1, and that nothing compared them. The fix made the corpus authoritative. What
was not known until now is which rungs would have caught it: the answer is the
two cheapest, and only those.

That is worth stating precisely because the intuition the ladder invites is
that each rung subsumes the ones below. It does not. R4 and R5 are strictly
stronger than R0 about *relations between states* and strictly weaker about
*which state the machine starts in*, and no amount of proof effort closes that
gap, because the gap is in what the obligations say rather than in how well
they are discharged.

## What to do about it

Not "add `ensures g.next == 1`", although that would close this one instance.
The transferable form is the checklist item:

**For every constant the specification pins, ask which rung would notice if it
changed.** If the answer is only the rungs that compare against a reference
trace, then the deductive layer is not carrying that part of the spec, however
green it is — and a port whose reference corpus is thinner than this one's has
nothing carrying it at all.

`verified-java-to-rust-port` inherits this directly. Its deductive rung is
blocked; if the argument for unblocking it is "then the identity properties
would be proved", this finding is the counter-question: proved to be *what*?
A relational contract over identities admits any consistent renaming of them.

## What this does not say

It does not say R4 and R5 are weak. On the same five-mutant run they killed
`tick-goes-backwards`, `timeline-scan-reversed` and `limit-off-by-one` — three
defects a thinner corpus could miss, caught without running the program at
all. It says the layers are not ordered, so a table with one row per rung is
the right shape and a single "assurance level" would be the wrong one.

Nor is one mutant a rate. Five mutants is a gate for the rung entry, not a
measurement of the corner; the number that belongs in the deliverable comes
from the full catalogue, which is the next queue item.
