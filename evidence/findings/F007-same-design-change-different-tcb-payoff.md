# F007 — The same design change bought six shims in Go and zero in Rust

**Status:** measured, both corners, before-and-after
**Significance:** the first quantified per-language difference the rig has produced

## The measurement

The flat reshape — nested containers replaced by an edge-keyed set and one
append-ordered log — was applied identically to both corners.

| Corner | Trusted-shim marker | Before | After | Delta |
|---|---|---|---|---|
| Go | `// @ trusted` in `store` | 10 | 4 | **−6** |
| Rust | `external_body` in `store` | 21 | 21 | **0** |
| Rust | `external_body` in `service` | 13 | 13 | 0 |
| Rust | `external_body` in `clock` / `ids` | 8 / 10 | 8 / 10 | 0 |

Go lost `putFollowEdge`, `deleteFollowEdge`, `appendTweet`, `iterFollows`,
`gatherTimeline` and `sortTimeline`. Rust lost nothing.

Verified after the change: clock 2, ids 0, domain 9, store 7, service 5 —
**23 verified, 0 errors** across the workspace.

## Why the payoff differs

The two verifiers were trusting different things, and only one of them was
about nesting.

**Gobra** could not compose *nested permission*. `acc(s.follows)` grants access
to the outer map but not to `s.follows[k]`, so every operation reaching inside
had to be quarantined. Flattening the containers removes the inner level, and
with it the reason those six shims existed.

**Verus** is not blocked on nesting. It is blocked on `vstd` shipping no model
for `std::collections::HashMap` / `HashSet` / `Vec` held behind a
`std::sync::RwLock`. Flattening a container that the verifier cannot see into
at all changes nothing: the shim is trusted because of the *lock and the
standard library*, not because of the shape.

So the same design change is a substantial trust reduction in one language and
a no-op in the other, for reasons that have nothing to do with the design's
merit and everything to do with what each verifier's standard library covers.

## What Rust did gain

The count is unchanged; what the shims *assert* is not.

`proof_home_timeline` previously carried two trusted claims: F1 visibility and
F2 sort order. Its own comment named the blocker — F2 "would require a
`vstd::vec` sort spec or a verified mergesort import; neither ships in the
pinned vstd."

After the reshape it performs no sort. The sort-specification obligation is
retired: F2 now follows from the log being append-ordered. The shim still
exists, still `external_body`, still trusted for the lock-and-collection read
— but it no longer stands in for an ordering proof nobody could write.

**The trusted surface got narrower in what it claims without getting smaller
in count.** Counting `external_body` markers would have shown no progress and
been wrong; reading what each one asserts shows real progress.

## Why this matters for the calibration

This is the first per-language number the rig has produced, and it says
something the kill table alone would not: *the assurance value of a design
change is not a property of the design.* Porting a well-motivated
restructuring from one language to another can deliver most of its benefit,
or none of it, depending on the target verifier's library coverage.

For the Java→Rust work that cuts a specific way. A restructuring justified by
"it discharged an obligation in the source language" carries no guarantee in
the target. The obligation has to be re-costed against the target verifier's
own gaps, and those gaps are about the standard library at least as often as
about the language.

## Correction to an earlier claim

The step-1c write-up said the sort-free design "deletes two trusted shims,"
later revised to six once measured on the Go side. That number is Go's alone.
It does not generalise, and this file is the measurement that shows it does
not.
