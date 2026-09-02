# F047 — A second mutant catalogue, and the honest accounting of what "independent" bought

**Status:** built and gated; the measurement it exists for is F048
**Class:** an instrument, plus a provenance audit of that instrument
**Artefact:** `tools/cmd/mutate/mutants-independent.json` — 12 defects, 43 mutants

## The threat this addresses

`tools/cmd/mutate/mutants.json` says of itself, in its own first line:

> each family is drawn from a defect this project has actually seen (the R0
> baseline, findings F001-F006, or a decision in `spec/s_obs/DECISIONS.md`)

Every rung is also built from `S_obs`. R0 replays a corpus generated from it,
R1 and R2 draw from an alphabet written against it, R4 and R5 check clauses
transcribed from it. So a kill rate measured over that catalogue may be
reporting **how completely the rungs cover their own specification**, not how
well they catch defects — and a rig that scored highly for that reason would
produce exactly the table `evidence/CALIBRATION-four-corner.md` already shows.

`evidence/CALIBRATION-four-corner.md` states the caveat and names the fix:

> A catalogue drawn from a different source — production incidents, a fuzzer's
> crash corpus, real defect history in the Java library — would produce a
> different and more informative table. That remains the highest-value
> follow-up, and it is still not done.

This is that catalogue. It is a **separate file**: the original catalogue's ids
are shared across corners so one defect can be compared port-to-port, and its
denominators are quoted throughout `evidence/`. Nothing here disturbs them, and
`TestCataloguesHaveDisjointIDs` fails the day an id appears in both.

## The three sources, and the argument for each

Every mutant declares a `source`, and every source declares its own
independence argument in the manifest, where it can be read against the mutant.
`mutants.Manifest.validate` rejects a `source` that is not declared, and
`TestUnknownSourceIsRejected` shows that check failing — a provenance claim
that cannot be refused is not a claim.

| source | defects | mutants | what it is |
|---|---|---|---|
| `mutation-operator-taxonomy` | 7 | 28 | the classical operator families — relational-operator replacement, loop-bound off-by-one, argument swap, dropped negation, wrong error branch |
| `service-failure-mode` | 3 | 12 | failure modes real feed and social-graph services have: keyset pagination that repeats an item, handle normalisation applied at one layer only, a length limit in the wrong text unit |
| `language-idiom` | 2 | 3 | slips that exist only inside one language's semantics |

### What the independence argument is actually about

**The generator, not the effect.** Every live mutant necessarily deviates from
`S_obs` observably — all four corners are R0 byte-exact against it, so
"observably different from the original" and "observably different from
`S_obs`" are the same statement here. Claiming a mutant's *effect* is
independent of the contract would be incoherent. What is independent is the
**procedure that chose it**:

- For `mutation-operator-taxonomy` the operator is defined over program syntax
  and the site is found by scanning for that syntax — every relational operator
  whose operands are a collection length and a constant; every loop that walks a
  collection to its end; every call with two adjacent same-typed arguments. No
  clause of `S_obs` selects the operator or the site. This is the **weakest**
  independence in the catalogue and it is labelled that way: the sites the scan
  finds are still sites in code written against the contract.
- For `service-failure-mode`, two of the three concepts have **no term in
  `S_obs` at all**. `S_obs` pins a byte length and an ASCII alphabet; it never
  raises the question of normalisation, and it never distinguishes a page
  boundary from the items on it. The defect ideas come from the operational
  vocabulary of production feed services.
- For `language-idiom`, the defect is a property of a language rather than of
  this system. `S_obs` is written in Go and says nothing about how a Java corner
  compares strings.

## Where the argument fails, recorded as failing

**`text-length-in-code-points` is NOT independent, and it ships saying so.**
The taxonomy proposed it — measure a length in the wrong unit — and then both
JVM corners turned out to have anticipated it in their own source. `Dom.java`:

> Every length and character test operates on UTF-8 *bytes*, not on Java
> `char`s. […] a Java implementation that measured `String.length()` would
> accept a 280-code-point tweet that `S_obs` rejects. That is a real observable
> divergence, so it is closed here rather than left to chance.

`Dom.kt` carries the same paragraph. A kill on this mutant is evidence about a
defect the authors **were** looking at. It is kept deliberately, as the
calibration point that makes the other eleven readable: it is the row where the
independent catalogue is expected to behave like the original one.

## Two mutants the taxonomy produced that had to be dropped

Requirement: every mutant passes `mutate probe`. An equivalent mutant is
counted as survived by every rung and drags every rate down for a reason that
has nothing to do with any rung. Two candidates failed that gate, and both
failures are informative rather than clerical.

**1. ROR on the append-log monotonicity guard is equivalent.** `t.ID <= last.ID`
→ `t.ID < last.ID` in `(*MemStore).PutTweet` is a textbook relational-operator
mutant on a guard the project considers important enough to have a finding
about (F005). Neither form can fire: ids come from `ids.Generator` strictly
increasing, so no state reachable through the HTTP API trips either test. The
operator landed on **live-looking defensive code that no input can reach.**
It is kept as a canary in
`evidence/experiments/independent-catalogue-canaries/manifest.json` and its
probe output is `evidence/runs/independent-catalogue/canary-probe-equivalent.log`:

```
  verdict   NO OBSERVABLE CHANGE in 536 requests

live: 0/1   no observable change: 1

probe FAILED: [go/dead-guard-equivalent-canary]
```

**2. Measuring the HANDLE length in code points is equivalent, and the reason
is a contract decision.** The same encoding defect that is live on `validText`
is dead on `validHandle`, because the handle alphabet is `[a-z0-9_]`: any
multi-byte input is rejected by the alphabet scan whatever the length test
said, so the two versions cannot disagree. `S_obs` D6's deliberately narrow
alphabet — chosen, per `dom.go`, because "a narrow alphabet is a narrow surface
on which two implementations can disagree" — **makes an entire
literature-standard defect family unobservable on that field.** That is a
property of the contract worth stating: narrowing the input surface does not
only reduce divergence, it reduces what a mutation catalogue can measure there.

This one was measured rather than argued, against a witness written
specifically to break it — 20 and 40 multi-byte characters, and a 33-byte ASCII
handle (`canary-probe-handle-codepoints.log`):

```
  witness   no difference in 3 requests
  corpus    no difference in 56 requests
  seed=1    no difference in 120 requests
  verdict   NO OBSERVABLE CHANGE in 539 requests

live: 0/1   no observable change: 1
```

Neither mutant is in `mutants-independent.json` and neither is in any
denominator.

## The port-to-port constraint, met head on

Requirement 4 asks for coverage across all four corners "where the defect is
expressible", and warns that corner-specific ids cannot be compared
port-to-port. The catalogue's answer is to put the restriction **in the id**:

- **10 of 12 defects are rendered in all four corners** and keep a shared id, so
  a kill row for `visibility-args-swapped` means the same defect in every
  column. That is 40 of the 43 mutants.
- `go-slice-len-aliasing` is Go only. Go's `make([]T, n)` sets **length**, so
  returning `buf` instead of `buf[:n]` pads the page with zero-valued tweets.
  Rust builds the page with `Vec::with_capacity`, Java and Kotlin with an empty
  `ArrayList`; in all three the container's length already is the element
  count, so there is no length/capacity confusion available to make.
- `jvm-string-identity-compare` is Java and Kotlin only. `==` / `===` on
  reference types is the JVM's defining footgun; the same operator on strings in
  Go and Rust is value equality, so the defect is not expressible there.

`TestPortableIDsCoverEveryCorner` enforces the convention: an id that does not
begin with a corner or corner-family prefix must exist in all four corners, or
the test fails and names the corners it is missing.

### The Rust overflow twin that does not exist — and has no Rust site either

The brief's own example is that "a Rust overflow mutant has no Go twin". The
stronger result here is that it has **no reachable Rust site**. Rust's
release-versus-debug overflow behaviour needs an arithmetic operation that can
overflow; every arithmetic operation in the Rust verified core is on a small
counter — `*g += 1` in `ids::LockState::lock_increment`, `out.len() == limit`
in `home_timeline`, `t.id + 1` — and none of them can be driven near `i64::MAX`
through the observable API, which accepts no ids as input. The corner-specific
slot that requirement 4 anticipated for Rust is empty for a stated reason
rather than filled with a mutant that would sit in the denominator unreached.

## Gate verdicts

`mutate verify` over the whole catalogue
(`evidence/runs/independent-catalogue/verify.log`):

```
anchors: 43/43 match exactly one site
compile: 43/43 build clean

verify PASSED: every anchor matches one site; every mutant compiles
```

Shown able to fail, on this manifest's own machinery
(`canary-verify-drift.log`):

```
anchors: 0/1 match exactly one site
compile: 0/1 build clean

verify FAILED: 1 mutant(s): go/drifted-anchor-canary
```

`mutate probe` verdicts are in F048 with the reachability profile they belong
to. Anchors were not hand-typed: they are lifted verbatim out of the four
corners' current source by a generator that asserts each occurs exactly once
before writing it, which removes the authoring half of the failure mode
`verify` catches at read time.
