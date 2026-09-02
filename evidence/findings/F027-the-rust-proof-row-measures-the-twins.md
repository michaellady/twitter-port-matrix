# F027 — the Rust R4 row cannot mean what the Go R4 row means

**Status:** measured while adding R4 to `calibrate` as a rung on the Rust corner
**Class:** a property of where the contracts live, not a defect in any mutant —
and one that makes two identically-named cells incomparable

## What was measured

Every Rust mutant in the catalogue that edits a **verify-enabled crate**, one
Verus run each, verdict read from the tool's own last line. Fourteen of the
eighteen Rust mutants qualify; the other four edit `crates/server`, the trusted
transport shim, and are unreached in the F022 sense before any of this starts.

```
self-follow-guard-dropped     R4 FAILED: verification results:: 8 verified, 1 errors over 1 of 5 verify-enabled crate(s)
id-first-is-two               R4 PASSED: verification results:: 23 verified, 0 errors over 5 of 5 verify-enabled crate(s)
id-burned-on-reject           R4 PASSED: ... 23 verified, 0 errors ...
follow-precedence-flipped     R4 PASSED: ... 23 verified, 0 errors ...
timeline-scan-reversed        R4 PASSED: ... 23 verified, 0 errors ...
timeline-tiebreak-by-id-asc   R4 PASSED: ... 23 verified, 0 errors ...
follow-toggles                R4 PASSED: ... 23 verified, 0 errors ...
unfollow-rejects-missing-edge R4 PASSED: ... 23 verified, 0 errors ...
orphan-author-accepted        R4 PASSED: ... 23 verified, 0 errors ...
created-at-frozen             R4 PASSED: ... 23 verified, 0 errors ...
tick-advances-by-two          R4 PASSED: ... 23 verified, 0 errors ...
tick-goes-backwards           R4 PASSED: ... 23 verified, 0 errors ...
cursor-inclusive              R4 PASSED: ... 23 verified, 0 errors ...
limit-off-by-one              R4 PASSED: ... 23 verified, 0 errors ...
```

**One kill in fourteen.** Every other mutant leaves the obligation count at
exactly 23, unmoved, to the unit. All eighteen are live: the four-corner
baseline records R0 killing 18 of 18 on this corner with zero equivalents, so
none of the thirteen passes is a mutant with no observable effect.

## Why it is one, and why it is that one

`crates/domain` is the **only** verify-enabled crate whose `verus! { ... }`
block encloses the shipped items. `Follow::new` is inside it, and its five
`ensures` clauses are clauses on the function the server actually calls.

The other four crates put every obligation inside
`#[cfg(verus_only)] mod verus_proof` — `put_tweet_ensures`,
`create_user_ensures`, `tick_ensures`, `next_id_ensures` and the rest. Those
are **separate functions from the ones that ship** (F012), reached from no
production path, and stated over `external_body` shims (F016).

So a mutant that edits production code in `store`, `service`, `clock` or `ids`
does not touch anything the contract mentions. The twin still verifies, because
the twin did not change. `R4 PASSED` here is not a proof that survived a
defect; it is a proof that was never shown the defect.

The single kill is `self-follow-guard-dropped`, and the catalogue says out loud
why it exists: *"the Verus `ensures from@ == to@ ==> result is Err` sitting
directly above the body is left in place, so R4 has something to catch."* It is
F016's one property — F4, functional, non-vacuous, about shipped code, not
resting on a project-local axiom — and it is the whole of the Rust R4 oracle.

## What a Rust R4 kill therefore means

Three readings are available, and they are not the same claim:

| a kill on this row could mean | happens here? |
|---|---|
| an obligation over the **shipped, mutated function** could no longer be discharged | **only in `crates/domain`** — 1 mutant of 18 |
| a **hand-written twin** broke | **never, today** — no catalogue mutant is anchored inside a `verus_proof` module |
| the tree stopped compiling | not in this catalogue; `mutate verify` reports 72/72 build clean |

So the row means the first thing, over a domain of one crate. The Go R4 row
means the first thing over five packages of directly-annotated shipped code.
**The two cells share a name, a tool contract and a verdict format, and they do
not measure the same thing.** F017 said a mutant catalogue does not transfer
across corners as cleanly as its name suggests; this is the same lesson one
level up, about the rung.

## The number, and the F022 comparison

| corner | catalogue | outside the verifier's files | inside, and killed | R4 ceiling |
|---|---|---|---|---|
| Go (Gobra) | 18 | 4 (`internal/httpshim`) | — | **14 / 18 = 78%** |
| Rust (Verus) | 18 | 4 (`crates/server`) | 1 of 14 | **1 / 18 = 6%** |

F022 found the Go proof rung's ceiling set by the trusted shim, at 78% before a
clause is written. The Rust ceiling is set by something else and is an order of
magnitude lower. `calibrate` will score the thirteen as **survived** — they are
live, and the verifier does read the files they edit — giving
`kill%reach = 1/14 = 7%`.

## The measurement bug this exposes in the rung entry

`Covers` decides a proof rung's reach by asking whether the verifier reads any
file the mutant edits. That is right on the Go corner, where Gobra's contracts
annotate the shipped functions, so reading the file and constraining the
function are the same thing.

**On the Rust corner it is too coarse, because the proof and the code it stands
for are in the same file.** `crates/store/src/lib.rs` holds production
`MemStore::put_tweet` at line 249 and its contract-bearing twin
`put_tweet_ensures` at line 820. A file-level predicate says "covered" for a
mutant that no obligation mentions.

Both `survived` and `unreached` are wrong words for those thirteen cells:

- **survived** implies the contract had a chance and missed. It did not have
  one. Nothing in the contract names the mutated function.
- **unreached** implies the verifier never saw the code, which is the
  `crates/server` case and reads as "out of scope by design". Here the code is
  emphatically in scope; the *contract* is somewhere else.

The honest word is a third one — the obligation is on a **copy** — and the
matrix has no cell for it yet. `verusReads` is left at file granularity because
narrowing it to "the mutated function carries an obligation" would need a Rust
parse the toolchain does not have, and because a predicate that quietly
reclassified thirteen survivors as unreached would hide exactly the result this
file is written to report. The row ships **with this finding cited beside it**,
which is the same remedy F022 applied to the Go row's denominator.

## What follows

1. The Rust R4 cell in `evidence/MATRIX.md` must carry `1/14` and a pointer
   here. A twelve-cell table with `R4 78%` in one row and `R4 7%` in another,
   unqualified, reads as "Verus is a worse verifier than Gobra". It is not a
   statement about the verifiers at all. It is a statement about where two
   projects chose to put their contracts.
2. Queue item 5 — "fix or delete the four drifted Verus twins" — is now the
   highest-value item for this corner rather than a tidy-up. Moving one
   contract from a twin onto its shipped function converts mutants from
   *not measurable* to *measurable*, and this file is the baseline that would
   show it: today the Rust R4 oracle is one clause in one crate.
3. The generalisation, and the transferable part: **a proof rung measures the
   distance between the contract and the code, not the strength of the
   verifier.** Before quoting any deductive kill rate, ask where the
   obligations are attached. F016 gave four questions to ask of a *count*;
   this adds the fifth, which is about the *rate*: how many of the mutants
   the rung is scored against does any obligation actually mention?

## Method, so it can be rerun cheaply

Verus is invoked through cargo and is therefore cached: a second run over an
unchanged tree prints no `verification results::` line at all and exits 0.
Measured — 0.3s, `Finished dev profile`, no result line — which is why
`tools/cmd/verus` touches every `.rs` file in the verify-enabled crates before
each run, and why it refuses to call a run PASSED unless all five crates
reported. A cold tree costs 1m45; a touched warm tree costs 3-4s, so this
fourteen-mutant audit ran in about a minute by applying each mutant's
`mutate apply` bytes over one warm tree and restoring them afterwards.

One more property of the tool worth recording: a crate that fails verification
fails to **compile**, so the crates downstream of it are never checked. The
failing verdict above reads `over 1 of 5 verify-enabled crate(s)` for that
reason. A FAILED Verus verdict is partial by construction and the verdict
sentence says so rather than reporting a quietly smaller `verified` count.
