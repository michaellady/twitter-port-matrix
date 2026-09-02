# F040 — R5's attribution engine can name the wrong member, and which one it names is random

**Status:** established by the tool's own output plus a probe over the five
contract files
**Class:** a latent defect in the instrument that decides the R5 column. It
flips no verdict recorded so far, and the bound on that claim is derived below
rather than assumed
**Found:** while reading Gobra's own error for F038, as standing rule 1 requires

## The symptom

Running the R5 rung by hand on the `handle-alphabet-widened` tree (F038):

```
  internal/dom/dom.go:206 (*invalidHandleError).Error member carries no R5 clause: Loop invariant might not be preserved.
```

Line 206 of `internal/dom/dom.go` is the third loop invariant of `ValidHandle`,
whose `func` line is 199. It is not in `(*invalidHandleError).Error`, which is a
one-line method at line 115.

## The cause

`memberSpans` in `tools/cmd/gobra/audit.go` finds a member's end by scanning
forward for the **first line that is exactly `}`**:

```go
end := len(lines)
for j := i + 1; j < len(lines); j++ {
    if lines[j] == "}" {
        end = j + 1
        break
    }
}
```

A function whose body is on the `func` line itself —

```go
func (e invalidHandleError) Error() string { return "invalid_handle" }
```

— never produces such a line. The scan runs on to the next bare `}` anywhere in
the file, and the span swallows everything in between. In `dom.go` that is line
215, `ValidHandle`'s closing brace, so `(*invalidHandleError).Error` is recorded
as spanning **[112, 215]** while `ValidHandle` spans **[165, 215]**.

`memberAt` then resolves the containing member like this:

```go
for m, span := range byMember {
    if e.Line >= span[0] && e.Line <= span[1] {
        return f, m, true
    }
}
```

**Go map iteration order is randomized.** When two spans both contain the error
line, which member is returned is not determined by the code. The output above
is one of at least two answers the same input can produce.

## The full overlap set

`evidence/experiments/r4-r5-separation/memberspans-overlap-probe_test.go.txt`
calls the real `memberSpans` over the five contract files. Its output, verbatim
and deduplicated:

```
OVERLAP internal/dom/dom.go: (*invalidHandleError).Duplicate[125,130] and (*invalidHandleError).Error[112,215] both claim lines 125..130
OVERLAP internal/dom/dom.go: (*invalidHandleError).Error[112,215] and (*invalidHandleError).IsDuplicableMem[119,123] both claim lines 119..123
OVERLAP internal/dom/dom.go: (*invalidHandleError).Error[112,215] and (*invalidTextError).Duplicate[149,154] both claim lines 149..154
OVERLAP internal/dom/dom.go: (*invalidHandleError).Error[112,215] and (*invalidTextError).Error[136,215] both claim lines 136..215
OVERLAP internal/dom/dom.go: (*invalidHandleError).Error[112,215] and (*invalidTextError).IsDuplicableMem[143,147] both claim lines 143..147
OVERLAP internal/dom/dom.go: (*invalidHandleError).Error[112,215] and ValidHandle[165,215] both claim lines 165..215
OVERLAP internal/dom/dom.go: (*invalidTextError).Duplicate[149,154] and (*invalidTextError).Error[136,215] both claim lines 149..154
OVERLAP internal/dom/dom.go: (*invalidTextError).Error[136,215] and (*invalidTextError).IsDuplicableMem[143,147] both claim lines 143..147
OVERLAP internal/dom/dom.go: (*invalidTextError).Error[136,215] and ValidHandle[165,215] both claim lines 165..215
OVERLAP internal/dom/dom.go: (*selfFollowError).Duplicate[40,45] and (*selfFollowError).Error[21,58] both claim lines 40..45
OVERLAP internal/dom/dom.go: (*selfFollowError).Error[21,58] and (*selfFollowError).IsDuplicableMem[34,38] both claim lines 34..38
OVERLAP internal/store/memstore.go: newErrHandleTaken[74,89] and newErrNonMonotonic[78,89] both claim lines 78..89
OVERLAP internal/store/memstore.go: newErrHandleTaken[74,89] and newErrUnknownUser[70,89] both claim lines 74..89
OVERLAP internal/store/memstore.go: newErrNonMonotonic[78,89] and newErrUnknownUser[70,89] both claim lines 78..89
```

`internal/clock/clock.go`, `internal/ids/ids.go` and `internal/service/service.go`
have **no** overlaps. `service.go` has a one-line `newErr` too, at line 36, but a
struct declaration's closing brace follows it before the next member, so its span
is truncated by accident rather than by design.

## Why no recorded verdict is wrong

The R5 verdict turns on one question only: does the member the error is placed in
carry a refinement clause? So an overlap can flip a verdict **only if the
candidates disagree on that**. They do not, anywhere in the current tree:

- In `memstore.go`, the ambiguous region is lines 70–89, and all three candidates
  (`newErrUnknownUser`, `newErrHandleTaken`, `newErrNonMonotonic`) carry no
  clause site. Any resolution gives the same answer.
- In `dom.go`, every candidate carries no clause site — `dom.go` carries none at
  all, which is the whole subject of F038.
- The three files that do most of the clause-carrying work, `service.go` (21
  sites), `ids.go` (3) and `clock.go` (1), have no overlaps to resolve.

So F028's nine R4 kills and their R5 attributions (4 on the clause line, 5
elsewhere in a clause-carrying member) stand, and F038's and F039's cells stand.
**This is a bound on the damage, not a defence of the code.**

## Why it still matters

The condition that makes it harmless is a coincidence of the current source
layout, and nothing tests for it. One one-line function added directly above a
clause-carrying member — in Go a thoroughly ordinary thing to write — would put
a clause-free member and a clause-carrying member in the same span. R5 would then
report `member carries no R5 clause` for an error that broke a refinement
obligation: a **false survival** in the R5 column, arriving randomly, on some
runs and not others, with no error and no UNDECIDED.

That is precisely the failure the widening recorded in F028 was introduced to
avoid — a real refinement failure scored as "not a refinement failure" — arriving
by a different door.

It also makes the attribution *line* untrustworthy today, which is not nothing:
that line is the human-readable evidence for every R5 cell in
`evidence/CALIBRATION-go-proof.md`, and in `dom.go` it currently prints a member
that does not contain the error.

## The fix, and why it is not applied here

The end of a member's span should be bounded by the start of the next member,
not by the next bare `}`:

```go
// after all spans are collected, clamp each end to the next member's start
```

and `memberAt` should resolve ties deterministically — narrowest span wins — so
that a residual ambiguity cannot depend on map order.

**This branch does not make that change.** `tools/cmd/gobra/audit.go` is shared
attribution machinery, five other agents are measuring against it concurrently,
and the defect flips no verdict today by the argument above — so changing the
instrument mid-measurement costs more than it buys. The patch belongs to the
integrating session, together with the assertion form of the probe (fail when
any two member spans overlap), which fails on the current tree and is therefore
the gate that proves the fix landed.

## Where

- `tools/cmd/gobra/audit.go` — `memberSpans`, the forward scan for `}`
- `tools/cmd/gobra/r5rung.go` — `memberAt`, the unordered map iteration
- `evidence/experiments/r4-r5-separation/memberspans-overlap-probe_test.go.txt` — the probe
- `evidence/findings/F038-*.md` — the run whose output surfaced it
