# F042 — the vacuity instrument counted two trusted axioms as shipped obligations, under a third function's name

**Status:** found the day after the instrument was built, by the first tree
that gave it something new to read
**Class:** a defect in the rig, not in any implementation — and one that would
have inflated the number the rig exists to keep honest

## What happened

`tools/cmd/verus canary` splits every `ensures` block into *shipped* and
*twin*, sweeps the shipped ones, and reports the ratio. F030 built it and
reported `5 shipped, 57 twin`.

The F041 lift added two trusted axioms to `crates/store`:

```rust
pub broadcast proof fn axiom_string_obeys_key_model()
    ensures
        #[trigger] vstd::std_specs::hash::obeys_key_model::<String>(),
{
    admit();
}
```

`canary -list` reported them like this:

```
  crates/store/src/lib.rs:257 fmt
      clause #[trigger] vstd::std_specs::hash::obeys_key_model::<String>()
      canary !(#[trigger] vstd::std_specs::hash::obeys_key_model::<String>())
```

Two defects, one line.

## Defect 1 — `fmt`

`fnName` accepted a signature only when the text before `fn ` ended in `pub `,
`unsafe `, `const ` or `async `. `pub broadcast proof fn` ends in `proof `, so
the line was not recognised as a signature at all and the scanner kept the
**previous** function's name. That previous function was `fmt`, from
`impl std::fmt::Display for StoreError`, sixty lines earlier and in an
unrelated impl.

The consequence is worse than a bad label. `clause.Key()` is
`(file, func, text)`, and it exists because a `(file, text)` key collided on
the Go corner and cost a withdrawn number (F019). A `func` field that silently
holds the wrong function rebuilds that collision inside the fix for it.

Verus signature modifiers are `spec fn`, `proof fn`, `open spec fn`,
`closed spec fn`, `broadcast proof fn` and `exec fn`. The scanner knew none of
them.

## Defect 2 — an admitted postcondition swept as a shipped obligation

The deeper problem is that both axioms were entering the sweep at all.

Their bodies are `admit()`. Under an admitted body Verus proves **every**
postcondition, including the negation of any clause spliced in its place — so
the canary comes back `VACUOUS` with the note *"Verus PROVED the negated
antecedent, so the clause's antecedent is unsatisfiable and the obligation is
discharged over nothing"*. That sentence would have been printed, and it would
have been a **tautology about `admit()` rather than a fact about the code**.

Either way the number moves and neither direction is right: counted as shipped
and REFUTABLE, the shipped count is inflated by two clauses that are assumed
rather than proved; counted as shipped and VACUOUS, the corner acquires two
"vacuous obligations" that are nothing of the kind. This is the F016 mistake —
a count that includes obligations nobody audited — rebuilt inside the
instrument built to catch it.

The same applies to the `external_body` shims that were already there:
`proof_service_put_user`, `proof_follow_remove` and four others carry
`ensures` clauses whose bodies Verus never checks.

## The repair

`clauseBlock` now carries two more flags, and `splitBlocks` returns four
categories instead of two:

- **Ghost** — a `spec fn` or `proof fn`. Its `ensures` is a lemma about the
  ghost world, not an obligation on code that runs.
- **Assumed** — `#[verifier::external_body]`, or a body that is `admit()` or
  `unimplemented!()`. Its postcondition is assumed.

Neither is swept, neither is counted in the shipped number, and both are
**printed by name with their clause counts** on every run, so an assumed
clause is visible rather than absent.

## The number this corrects, in place

F027 and F030 both quote `5 of 62 clauses on shipped functions`, with the rest
described as twins. Re-measuring the **pre-lift tree** with the fixed
classifier — same commit, same tool, new split:

```
pre-lift  (origin/claude/goal-loop):  5 shipped, 36 twin, 21 assumed
post-lift (this branch):             37 shipped, 20 twin, 13 assumed
```

So the "57 twins" of F030 was **36 twins plus 21 assumed clauses**, and the
distinction matters exactly as much as the shipped/twin one does: a twin is
checked against a body, an assumed clause is not checked at all. F027 and F030
are corrected in place rather than rewritten, because the mistake — folding
"not checked" into "checked somewhere else" — is the finding.

## The tripwire fired, which is what it was for

`TestShippedClausesAreExactlyFollowNew` asserted the corner had exactly one
shipped `ensures` block and said in its own failure message: *"If a contract
moved onto a shipped function this is the good news — update the test and F027
together."* It failed on the lifted tree. It is replaced by
`TestShippedClausesCoverTheLiftedCrates`, re-armed against the new truth: every
lifted crate must keep a shipped block, shipped must outnumber twin, and
`crates/service` acquiring one trips the test in turn. Shown able to fail —
reverting `crates/ids` to its pre-lift source gives

```
--- FAIL: TestShippedClausesCoverTheLiftedCrates
    canary_test.go:119: crate ids has no shipped ensures block; the lift regressed.
    shipped = map[clock:[new get tick set] domain:[new] store:[new put_user put_follow put_tweet]]
```
