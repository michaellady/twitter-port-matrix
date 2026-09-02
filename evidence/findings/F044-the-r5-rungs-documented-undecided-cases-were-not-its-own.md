# F044 — the R5 rung's documented undecided cases were not the ones it implements

**Status:** found by reading `tools/cmd/calibrate/rungs.go` against
`tools/cmd/gobra/r5rung.go` before copying the R5 shape to a second corner;
corrected in place
**Class:** prose and code stating different facts, with nothing comparing them
— the same failure mode as F020, in the file that tells `calibrate` what a rung
means

## What the two files said

`tools/cmd/calibrate/rungs.go`, on the R5 entry:

> The tool prints R5 UNDECIDED, and no verdict, in the two cases where the
> answer cannot be read off the run: an error outside every clause span, or a
> non-R5 clause failing on a member that also carries R5 sites (Gobra reports
> one failing postcondition per member, so the rest of that member's contract
> may not have been reached). calibrate records those as error cells.

`tools/cmd/gobra/r5rung.go`, the tool that paragraph describes, in its own
header:

> A clause is not the only thing that can fail, and the first run of this rung
> got that wrong. Two memstore mutants failed at a LOOP INVARIANT inside
> `(*MemStore).HomeTimeline` […] which sits in no `ensures` span at all, so
> both cells came back UNDECIDED. […] So attribution is by clause AND by
> member: an error anywhere inside a member whose contract carries R5 sites
> means that member's proof did not complete, and the refinement clauses on it
> are no longer discharged.

And the code:

```go
case len(a.onClause)+len(a.inMember) > 0:
	fmt.Println(r5RungVerdict(true, ...))
	return errR4Failed
...
case len(a.unlocated) > 0:
	fmt.Printf("R5 UNDECIDED: %d error(s) could not be placed in any member ...")
	return errR5Undecided
```

with `TestAttribute` pinning it case by case, including the comment it
contradicts: *"a non-R5 clause on a member that carries R5 clauses"* is
asserted to produce `inMember == 1`, and `inMember` is a **kill**.

## Both halves of the sentence are wrong, and one is wrong twice

- **"a non-R5 clause failing on a member that also carries R5 sites"** is
  `inMember`. It is a FAILED verdict and has been since the fix `r5rung.go`
  describes. There is no reading of the code under which it is UNDECIDED.
- **"an error outside every clause span"** is stated over *clause* spans. An
  error outside every clause span but inside a member is also `inMember`, so
  also a kill. The tool's one undecidable case is an error outside every
  **member** span — a stricter condition than the words used.

So the comment describes the rung's **first** version, before the two
`HomeTimeline` mutants forced the change, and it survived the change in the
one file whose job is to tell `calibrate` what the column means.

## Why it mattered here rather than being a stale comment

This is not decoration. The instruction to build a second R5 corner said the
new rung "needs the analogous honesty" and named the two cases from
`rungs.go` — which would have produced a Kotlin rung that reported UNDECIDED
where the Go rung reports a kill. Two corners answering the same question with
different verdicts is precisely what the one-rung-many-drivers design exists to
prevent, and the disagreement would have been invisible: both rungs would have
been individually self-consistent, and the R5 column would have meant one thing
in the `go` cell and another in the `kotlin` cell.

## What was done

`tools/cmd/jbmc r5verify` implements the code's semantics, not the comment's:
one undecidable case (a failing goal that is not an own assertion of any R5
entry point, so it sits on no clause and in no member), and an assert inside an
R5 entry point that is not a registered clause site counted as a kill
"elsewhere in its member". `TestR5Attribute` pins all four answers case for
case against `gobra`'s `TestAttribute`.

The `rungs.go` comment is corrected in place and says that it was wrong, rather
than being quietly rewritten — F020's disposition, for F020's reason.

## What is still not gated

Nothing compares a rung's prose to its tool. `TestAttribute` pins the tool
against itself and `TestR5Attribute` pins the second tool against the first,
but the sentence in `rungs.go` is still only checked by someone reading both.
The cheap version of the missing gate is the one this project already applies
to `r5Files` and `verusVerified` — re-derive the claim from its source and fail
when the two disagree — and a comment is not re-derivable. Recorded as a limit,
not closed.
