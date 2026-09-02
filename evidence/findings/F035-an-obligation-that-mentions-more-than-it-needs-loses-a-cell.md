# F035 — One decorative line in an obligation file costs a whole matrix cell

**Status:** reproduced on the Kotlin corner, avoided on the Java corner
**Class:** a coupling nobody designs and no rung reports — the obligation set
is *compiled* against the tree under test, so it is coupled to every signature
it mentions, needed or not

## The mechanism

A proof rung over a JVM corner compiles the implementation and its obligations
**together**, on purpose: the obligations must link against the tree under
test, not against a pristine copy. `tools/cmd/jbmc/run.go` says so —
"On a mutant tree that is the whole point".

The consequence nobody wrote down: an obligation that *mentions* a method is
coupled to that method's **signature**, whether or not the property needs the
call. A mutant is allowed to change signatures. When it does, the obligation
set fails to compile, `jbmc verify` never gets as far as printing a verdict,
and `calibrate` records an error cell — a missing measurement — for a mutant
the rung reaches and could have judged.

## The instance

`id-burned-on-reject` splits `Store.createUser(handle)` into `allocUserId()`
plus `createUser(handle, id)`, so an id is burned before the duplicate check.
It is one of the eighteen catalogue mutants and it edits `store` and `service`,
both inside the verifier's reach.

The Kotlin corner's `Obligations.kt` opens each of its three timeline
obligations with `s.createUser("a")`, and `Canaries.kt` does it twice more.
Against that mutant:

```
$ kotlinc -jvm-target 17 -nowarn -d classes tree/src tree/verification
tree/verification/Canaries.kt:61:22: error: no value passed for parameter 'id'.
        s.createUser("a")
                     ^^^^
tree/verification/Canaries.kt:73:22: error: no value passed for parameter 'id'.
tree/verification/Obligations.kt:179:22: error: no value passed for parameter 'id'.
tree/verification/Obligations.kt:195:22: error: no value passed for parameter 'id'.
tree/verification/Obligations.kt:208:22: error: no value passed for parameter 'id'.
```

Five errors, one cell lost. The Kotlin R4 sweep has not been run yet, so this
has not cost anything on the table so far — it will cost `kotlin/
id-burned-on-reject` the moment it is.

## The line is not needed

`Store.timelinePage` decides visibility with
`t.author() == user || isFollowing(user, t.author())`. It never consults the
user registry. Registering `"a"` before appending tweets authored by `"a"`
changes no answer in any of the five sites — it is there because it reads like
how a real request sequence would go.

That is exactly the shape of the trap: the decoration is *good style* in a
test, and a proof obligation is not a test. In a test the setup is free. Here
the setup is a compile-time dependency on a signature the defect catalogue is
allowed to rewrite.

## What was done

The Java twin drops the call from all six sites (`o4a`, `o4b`, `o4c`, `c4`,
`c5`, `c13`) and says why in the GROUP 4 header. With it dropped, **all
eighteen** Java mutants compile against the obligation set:

```
id-first-is-two COMPILES        timeline-tiebreak-by-id-asc COMPILES
id-burned-on-reject COMPILES    follow-toggles COMPILES
self-follow-guard-dropped COMPILES   unfollow-rejects-missing-edge COMPILES
follow-precedence-flipped COMPILES   orphan-author-accepted COMPILES
timeline-scan-reversed COMPILES ... (18 of 18)
```

Before the change, exactly one failed, and it was `id-burned-on-reject`.

**The Kotlin corner is deliberately not fixed here.** Its R4 sweep is being run
concurrently by another lane, and editing its obligation file mid-sweep would
change the tree hash under a journal that is already recording cells against
the old one. The fix is five deletions of `s.createUser("a")` from
`impls/kotlin/verification/Obligations.kt` (lines 179, 195, 208) and
`Canaries.kt` (lines 61, 73), and it changes no verdict on the clean tree
because it changes no property.

## Why no gate catches this

Every gate in this repository is about what a verifier *says*. This failure
happens one step earlier, in the compiler, and it presents as an ordinary tool
error — which `calibrate` handles correctly and conservatively: no verdict, no
kill, no survival, an error cell. Nothing is scored wrongly. What is lost is
that the cell had an answer available and the obligation set gave it away.

The cheap check is the one run above: apply every catalogue mutant and compile
the obligation set against each. It is seconds per mutant on `javac`, it needs
no solver, and it turns this class from "discovered during a sweep" into
"discovered before one".

## The rule

**An obligation should touch the smallest surface its property needs.** On a
mutation rung that is not an aesthetic preference: the obligation set is
compiled against the mutant, so every extra mention is a coupling to a
signature the mutant is entitled to change, and every such coupling converts a
measurable cell into a missing measurement.

Stated the other way round, for anyone writing obligations for a corner that
will be mutation-tested: **setup you would write in a test is a liability in an
obligation.** A test is run against one tree. An obligation is compiled against
every tree the catalogue can produce.
