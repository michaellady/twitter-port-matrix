# F015 — F4 is enforced twice, so removing either guard tests nothing

**Status:** found by `mutate probe` refusing to score a mutant
**Class:** measurement blindness caused by defence in depth

## What the probe found

`go/self-follow-guard-dropped` removes the `from == to` guard from
`dom.NewFollow`. It had been killing R0 earlier in the session. On re-probe:

```
verdict   NO OBSERVABLE CHANGE in 536 requests
live: 17/18   no observable change: 1
probe FAILED: [go/self-follow-guard-dropped]
```

Removing the guard left R0 at a clean 56/56. The edit had landed — the mutant
tree's `NewFollow` really is a bare `return Follow{From: from, To: to}, nil`.

## Why

`service.Follow` acquired its **own** self-follow check during the R5
refinement work, added so the D4 ordering (existence before semantics) is
explicit for the proof:

```go
if !s.st.HasUser(from) || !s.st.HasUser(to) {
	return store.ErrUnknownUser
}
if from == to {
	return dom.ErrSelfFollow
}
f, derr := dom.NewFollow(from, to)   // guard here too
```

F4 is now enforced in two places on the same path. Removing either alone
changes nothing observable, so **a single-point mutation cannot measure F4's
enforcement at all.**

Both mutants would have been recorded as "no observable change" and — if the
distinction from F009 had not been implemented — silently written off as
equivalent. They are not equivalent. They are individually masked.

## The second edge: a proved contract about an unreachable branch

`dom.NewFollow` carries a Gobra contract that is real and discharged:

```go
// @ ensures from == to ==> (err == ErrSelfFollow && f == Follow{})
```

Nothing reaches it. `service.Follow` returns before calling `NewFollow` when
`from == to`, so the branch that postcondition describes is dead on the only
path a request can take.

The proof is sound. Gobra verified something true. It just describes code no
observable behaviour depends on any more — the obligation drifted out from
under its own relevance while remaining green.

This is F013's shape without F013's mechanism. There, an obligation was
vacuous because the verifier could not reach it. Here it is *irrelevant*
because the program cannot reach it. Neither shows up as anything but a
passing check, and **counting verified obligations conflates both with
obligations that matter.**

## The fix, and the rule

The mutant now removes **both** guards, and R0 kills it immediately:

```
DIFF 12 reject_self_follow_known   [status 204, want 400]
     expected 400 {"error":"self_follow_forbidden"}
     got      204
```

**A mutant must remove a property's enforcement, not one of its
implementations.** Where a property is defended in depth, a per-site mutant
measures the redundancy rather than the property. The catalogue needs mutants
scoped to *properties*, and defence in depth has to be discovered rather than
assumed absent — nothing in the source announces that two guards enforce one
rule.

Worth stating plainly: defence in depth is good engineering and it is not the
problem. The problem is that it is invisible to a measurement technique built
on single-point edits, and nothing warns you.

## What made this catchable

The probe refused to guess. Given a mutant that changed nothing observable, it
reported `NO OBSERVABLE CHANGE` and failed, rather than classifying it as
equivalent and quietly removing it from the denominators.

That refusal is the whole value. An equivalent mutant is legitimately excluded;
a masked one is a hole in the catalogue wearing the same clothes. F009 built
the distinction to separate *unreached* from *equivalent*; this is a third
category the same machinery caught — **masked** — and the honest handling is
identical: refuse to score, and make a human look.
