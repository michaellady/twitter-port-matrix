# F005 — A derived property is only as good as its enforced premises

**Status:** found and fixed during step 1c
**Found by:** an existing upstream test, not by design review

## The design

F2 (timeline ordered by `created_at desc, id desc`) is *derived* rather than
proved-about-a-sort. The argument is the monotonicity lemma:

> For log positions `i < j`: `tweets[i].ID < tweets[j].ID` because ids are
> allocated monotonically, and `tweets[i].CreatedAt <= tweets[j].CreatedAt`
> because the clock never decreases. Therefore reverse iteration over the log
> is exactly descending `(created_at, id)`.

That is the argument that deletes the `sortTimeline` trusted shim and the
sort-specification obligation with it (finding F004).

## The gap

The lemma has two premises, and **nothing enforced either of them.** They
existed only in a comment. `PutTweet` appended whatever it was handed.

A caller appending out of order would have produced a silently mis-ordered
timeline — no failing test, no failing proof, because the property was
*assumed* derived rather than *made* derivable.

## How it surfaced

The store's own upstream test appended out of order on purpose:

```go
s.PutTweet(dom.Tweet{ID: 7, Author: "carol", Text: "z", CreatedAt: 1})
s.PutTweet(dom.Tweet{ID: 3, Author: "alice", Text: "y", CreatedAt: 0})
```

Entirely legitimate against the old per-author-map design, where `Snapshot`
sorted afterwards. Illegal against a log. The test failed with
`tweets not sorted by id`, which read at first like a stale assertion about a
changed contract — the same category as the ten other tests updated in this
step.

It was not. It was the design's own premise being violated, by the only code
that had ever tried.

## The fix

`PutTweet` now rejects an append that would break the invariant:

```go
if n := len(s.tweets); n > 0 {
	last := s.tweets[n-1]
	if t.ID <= last.ID || t.CreatedAt < last.CreatedAt {
		return ErrNonMonotonic
	}
}
```

The premise is now an invariant the type maintains, so F2 is genuinely derived
from a structural fact rather than resting on a convention callers happened to
follow. `Snapshot` needs no sort on tweets, and that is now justified instead
of merely true.

The test was updated to assert the rejection, which is a stronger claim than
the one it made before.

## The general rule

Replacing "prove a property about an operation" with "derive it from a data
structure" does not remove the obligation. It **relocates** it onto the
structure's invariants. If those invariants are not enforced at every mutation
site, the property has not been derived — it has been assumed, and the
assumption is now invisible because there is no longer a sort to point at.

Worth stating because the relocation is otherwise a clear win, which makes it
easy to bank the win and skip the bookkeeping. The two-line check is the part
that makes F004's shim deletion sound rather than merely smaller.

## A note on how this was nearly missed

Ten tests in this step genuinely did encode a superseded contract and were
correctly updated. This one failed in the same batch, looked like an eleventh,
and would have been "fixed" by relaxing it.

When a batch of tests fails after a design change, the ones that assert a
*changed contract* and the ones that assert a *violated invariant* look
identical from the failure message. They have to be told apart one at a time.
