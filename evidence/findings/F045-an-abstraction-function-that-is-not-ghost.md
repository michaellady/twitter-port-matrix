# F045 — the Kotlin abstraction function is shipped code, and the Go one is not

**Status:** found while building the Kotlin R5 rung; recorded as a cost, not
worked around
**Class:** a TCB difference between two corners that their R5 cells do not show

## The difference

R5 needs `abs`, and `OBLIGATION.md` §4 is emphatic that it must be **concrete**:
"an uninterpreted `abs` with axiomatised commutation clauses would verify
trivially and prove nothing". The Go corner satisfies that with four ghost
projections on `*MemStore`:

```go
// @ ensures ...
// @ pure func (s *MemStore) AbsUsers() set[string] { ... }
```

They live inside `// @` comments. `go build` does not see them, the shipped
binary has no such methods, and the corner's surface is exactly what it would
be with no verifier at all. **Gobra's ghost mode makes the abstraction function
free.**

Kotlin has no ghost mode, and JBMC checks bytecode. So this corner's `abs` is
eight ordinary public methods on `Store`:

```kotlin
fun absUserCount(): Int = users.size
fun absHasUser(handle: String): Boolean = userByHandle.containsKey(handle)
fun absFollowCount(): Int = follows.size
fun absFollows(from: String, to: String): Boolean = follows.contains(Edge(from, to))
fun absLogLen(): Int = log.size
fun absLogIdAt(i: Int): Long = log[i].id
fun absLogCreatedAtAt(i: Int): Long = log[i].createdAt
fun absLogAuthorAt(i: Int): String = log[i].author
```

Nothing in `src/` calls them — `Main.kt` and the shim do not — but they are in
the compiled artefact the registry builds and the R0/R1/R2 rungs run. The
corner's public surface is wider than it was, and wider than the Go corner's,
**because it has an R5 cell**.

## What was tried and does not work

`internal` visibility would keep them out of the published API. Kotlin mangles
`internal` function names in bytecode as `name$moduleName`, and the JBMC
`--function` argument is a JVM descriptor, so the rung's entry points would
depend on a module name that is not fixed by anything in the build. Trading a
surface widening for a name the rung resolves by luck is a worse deal, so the
methods are public and labelled.

Reading the log through `timelinePage` instead — the one public method that
already exposes it — does not work either: its visibility filter is
`t.author != user`, which compiles to `Intrinsics.areEqual` → `String.equals`
→ the F014 defect. That is exactly why R4's `o4c` is BLOCKED, and it is why the
projections have to reach the private fields directly.

## Why this is a finding rather than a note

`ASSURANCE.md` compares corners by which rung they reach. Two corners reaching
R5 look equal in that table, and here they are not: one paid nothing for it and
the other widened its shipped class. The cost is small and it is real, and the
direction generalises — **a verifier without a ghost language cannot state an
abstraction function without changing the program it is about**. Verus has
`spec fn`, Gobra has `// @`; JBMC has neither, because it is a model checker
over bytecode and there is no bytecode for a specification.

`TCB.md` records the trusted/verified split per corner. This is a different
axis — what the verification *adds* to the shipped artefact rather than what it
leaves out of the perimeter — and no table in this repository has a column for
it. Recorded here so the Kotlin R5 cell is not read as free.
