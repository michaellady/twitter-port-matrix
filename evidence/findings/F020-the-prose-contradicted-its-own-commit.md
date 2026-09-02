# F020 — The prose contradicted its own commit's data, in the same commit

**Status:** found while mapping the R5 clauses to their Gobra sites
**Class:** a documented blocker that aged badly — `FINDINGS.md` Pattern 5, with
the age reduced to zero

## The contradiction

`spec/refinement/OBLIGATION.md` §6, "The id axis, and why it is not projected":

> **Go `(*ids.Generator).Next` is `// @ trusted` and carries no postcondition at
> all.** The `GhostNext` declaration in `ids.gobra` — which does state F8 — is a
> separate ghost function that nothing calls.
>
> F8 is therefore unproved in both corners, symmetrically.

`spec/refinement/obligations.json`, clause 20, in the **same commit**
(`4bc2706`, S-13):

```json
{ "corner": "go", "layer": "ids", "op": "PostTweet",
  "clause": "F8: every issued id is the previous counter value, and the
             counter advances by exactly 1; result >= 1",
  "site": "(*ids.Generator).Next",
  "status": "discharged", "verifier": "gobra",
  "new_in": "S-13", "was": "// @ trusted with no postcondition" }
```

The JSON's `"was"` field is the tell: it records the exact state the prose
still describes in the present tense.

## The code agrees with the table

`impls/go/internal/ids/ids.go`, at that same commit, carries four `ensures` on
`Next` — three functional, one permission framing — and no `// @ trusted`:

```go
// @ requires acc(g.LockP())
// @ ensures acc(g.LockP())
// @ ensures result == old(unfolding acc(g.LockP()) in g.next)
// @ ensures unfolding acc(g.LockP()) in g.next == result + 1
// @ ensures result >= 1
func (g *Generator) Next() (result int64) {
```

The comment block immediately above it even argues the point the §6 paragraph
denies: *"The sidecar's claim that the discharge 'happens in Phase 2b' was a
statement about work not done, not about a tool limitation."*

## And the verifier agrees with the code

All three functional clauses are refutable — Gobra rejects each negation, at
that member:

```
internal/ids/ids.go:55  result == old(unfolding acc(g.LockP()) in g.next)   REFUTABLE
internal/ids/ids.go:56  unfolding acc(g.LockP()) in g.next == result + 1    REFUTABLE
internal/ids/ids.go:57  result >= 1                                         REFUTABLE
```

`(*Generator).Next` also refutes `ensures false`, so its exit is reachable and
the obligations are not vacuous. **F8 is proved in Go.**

## What the false sentence was load-bearing for

Not much, and that is the interesting part — the error survived because nothing
depended on it enough to re-check it:

- The symmetry claim is wrong. F8 is proved in Go and unproved in Rust
  (`crates/ids`: `0 verified, 0 errors`, all ten items `external_body`). The
  corners are asymmetric, which is the kind of result this repository exists to
  surface.
- §6's downstream argument — that F8 is "exactly the premise
  `(*MemStore).PutTweet`'s accept condition needs" — is still correct, and is
  now *closer* to being dischargeable than the paragraph admits.
- R5 clause 33 stays blocked, but on B1 (no string indexing) and on the
  concurrency argument F018 settled, **not** on F8 being unproved.

## Why the usual defence missed it

Pattern 5 says a blocker is a measurement with a timestamp, and that the danger
is a later reader inheriting it as a fact about the world. That framing assumes
some interval in which the world moves.

Here the interval was zero. One commit both discharged the obligation and wrote
the paragraph saying it was undischargeable. The machine-readable table was
updated; the prose beside it was not. Nothing compares them, so nothing noticed
— for the same reason F011's drifted anchor and F013's vacuous obligations went
unnoticed: **there was no check whose failure would have said so.**

## The rule

**Where a document and a data file state the same fact, one of them is going to
be wrong, and the prose is the one that rots.** Either generate the prose from
the data, or add a check that compares them.

`spec/refinement/clause-sites.json` plus `gobra r5` is the concrete form of
that here: the per-clause table is now *derived* from Gobra's output joined to
`obligations.json`, so a claim in it cannot drift from the verifier without the
join failing.
