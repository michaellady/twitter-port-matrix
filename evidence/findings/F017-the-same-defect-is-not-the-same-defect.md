# F017 — The same defect id needs a different number of edits in each corner

**Status:** found while extending the catalogue from 2 corners to 4
**Effect:** limits what a cross-corner kill table can compare

The catalogue is built so one id names one defect across every corner —
`go/cursor-inclusive` and `java/cursor-inclusive` are the same defect injected
two ways. That is what lets the kill table put corners side by side.

Extending it to Java and Kotlin showed the assumption is shakier than it looks.

## Enforcement site counts differ by corner

| property | Go | Rust | Java | Kotlin |
|---|---|---|---|---|
| F4 self-follow | **2** | 1 | **1** | **1** |
| F6 orphan author | 2 | 2 | **1** | **1** |
| unknown JSON field rejected | 2 | **3** | **1** | **1** |

Every one of these is a defensible design. None is announced anywhere. And
each changes what a single-point mutant measures:

- `self-follow-guard-dropped` is **masked** in Go — F015, where a mutant
  removing one of two guards reported no observable change across 536
  requests. In Rust, Java and Kotlin the property has exactly one site, so the
  same id, at one edit, is live.
- `id-first-is-two` needs **two edits** in Java and Kotlin. Go and Rust seed
  users and tweets from one shared generator; Java and Kotlin count them in two
  independent fields. A one-field edit compiles, verifies, and injects "user
  ids off by one" — strictly less than the id names. That is F011's
  under-injection shape, and nothing mechanical would have caught it.
- `unknown-json-fields-accepted` needed the same treatment for a third reason:
  the obvious rendering — make the field matcher return the unknown key —
  still rejects `{"handle":"a","x":1}`, because a type check one line below
  fires on the number instead.

## The one that limits the calibration: two properties crossed the TCB boundary

`next_cursor` arithmetic and the `created_at` stamp live in the **verified
core** in Java and Kotlin (`Store.timelinePage`, `Store.appendTweet`) and in
the **trusted HTTP shim or service layer** in Go and Rust.

So `next-cursor-always-emitted` and `next-cursor-is-first-id` inject into
verifier-readable code in two corners and into unverifiable code in the other
two.

**R0, R1 and R2 do not care** — they drive the observable API and see the same
defect either way. Those rows stay comparable.

**R4 and R5 rows for those defects are not comparable across corners**, and
nothing in the table says so. A corner scoring a proof-rung kill on
`next-cursor-*` has done something structurally different from a corner that
cannot, and the difference is where the author drew the TCB line, not how
strong the verifier is.

## The rule

**"The same defect" is a claim about the specification, not about the code.**
Two corners implementing one contract can enforce a property at different
numbers of sites, in different layers, on different sides of the trust
boundary — all while passing byte-identical conformance.

For a cross-corner kill table that means:

1. Behavioural rungs (R0/R1/R2) compare cleanly, because they only see the API.
2. Proof rungs (R4/R5) compare only where the defect sits on the same side of
   the TCB boundary in both corners. That has to be recorded per defect, per
   corner — it is not a property of the corner alone.
3. Enforcement-site count has to be **discovered per corner**, not assumed
   from the first corner written. Three of the eighteen defects needed a
   different edit shape in the JVM corners, and all three were found by reading
   the code rather than by any check.

## Note

No defect was inexpressible in any corner: all 18 render in all four, and both
new corners report 18/18 anchors unique, 18/18 compiling, and 18/18 live under
probe. The catalogue is now 72 mutants. The limitation above is about what the
resulting table can be read to mean, not about coverage.
