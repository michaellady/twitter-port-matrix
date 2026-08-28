# F004 — The sort-free timeline removes two trusted shims, not just a proof step

**Status:** confirmed from upstream source comments
**Significance:** the design move in the plan is a TCB reduction, and upstream
already identified it as an option before scoping it out

## What upstream says

`internal/store/memstore.go` carries an unusually candid block above
`HomeTimeline`, headed **"EXPLICIT NON-DISCHARGE — F1 + F2 ARE NOT FORMALLY
VERIFIED HERE."** On F2:

> "F2 (returned list is sorted by (created_at desc, id desc)) requires a
> sort-spec on `sort.Slice` plus a stability proof on the closure comparator.
> Neither is expressible against the current opaque `stubs/sort` `Slice`
> declaration (kept in TCB.md)..."

Two helpers are quarantined as trusted to contain the gap:

> "The two compound inner operations are quarantined into tiny `// @ trusted`
> shim helpers: `gatherTimeline` (build the authors set + concatenate every
> contributing author's per-author tweet slice) and `sortTimeline` (the
> `sort.Slice` call with the F2 closure comparator)."

And the escalation path is named explicitly:

> "Strengthening either F1 or F2 here would require either (a) a substantial
> extension of `LockP()` to per-author/per-edge-set quantified permission
> tokens, or **(b) a flat reshape of `s.byAuthor` + `s.follows`.** Both are out
> of scope."

The Verus repo declines the same obligation for the same reason.

## Why this matters

**Option (b) is the sort-free design.** Upstream found the right fix and
scoped it out, which is a reasonable call inside a per-method discharge
cadence — and exactly the kind of thing that stays scoped out indefinitely
once the surrounding structure is settled.

`S_obs` takes option (b) by construction (decision D9). Tweets live in one
append-ordered log; the timeline is a reverse scan over it. The monotonicity
lemma — ids strictly increase, the clock never decreases, therefore reverse
log order *is* descending `(created_at, id)` — makes F2 derivable from the
data structure instead of provable about a sort.

The consequence is larger than a proof convenience:

| Trusted shim | Why it exists upstream | Under the sort-free design |
|---|---|---|
| `sortTimeline` | wraps opaque `sort.Slice`; F2 needs a sort-spec plus stability proof | **gone** — there is no sort |
| `gatherTimeline` | composes per-author slice permissions out of `s.byAuthor` | **gone** — there is no per-author map to gather from |

Both disappear because the shape they were quarantining disappears. That is a
reduction in the trusted computing base, not a relocation of it — which is the
distinction that matters, since quarantining an obligation into a "tiny
trusted shim" leaves the obligation undischarged however small the shim gets.

## Consequence for step 1c

Retargeting must put the reshape in the **verified core**, not in the HTTP
shim. Upstream's Gobra matrix verifies `[clock, ids, dom, store, service]`
and does not verify `httpshim`; putting timeline semantics in the shim would
produce a green R0 over code no verifier ever looks at, and R5 would then
prove nothing about observable behaviour.

The split for 1c is therefore:

- **verified core** (`dom`, `store`, `service`) — handle and text validation,
  validation order, error vocabulary, the append-log reshape, cursor
  pagination, and the tick advance
- **trusted shim** (`httpshim`) — wire format only: strict JSON decoding,
  canonical byte encoding, routing. Transport, matching upstream's own
  boundary

That boundary goes in `TCB.md` when 1c lands.
