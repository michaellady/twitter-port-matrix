# F009 — R0 could not kill a defect that had actually shipped

**Status:** found by the mutation catalogue, closed, verified
**Kill rate:** 17/18 → **18/18** after a one-step corpus change

## The survivor

`id-burned-on-reject` moves the id allocation above the duplicate-handle
check, so a rejected registration consumes an id. This is not a hypothetical
defect — it is the **exact behaviour the Go corner shipped with**, visible in
the very first R0 baseline as `{"handle":"Alice","id":5}` after four
rejections.

Injected into the fixed implementation, it survived R0 at 54/54, and R1 could
not reach it either.

## Why, and why that is not a rung weakness

The corpus rejected a duplicate handle at step 4 and then **never registered
another user**. An id burned at step 4 has no later allocation to be visible
in. `tracegen` had the same blind spot from the other direction: `createUser`
only ever emitted fresh `u{n}` handles, so it never produced the
rejected-then-successful pair the defect needs.

Neither rung was weak. The defect was **unreachable from the inputs**, at any
volume.

## The distinction that makes the kill table honest

A mutant that no rung kills has two very different explanations:

- **equivalent** — it does not change observable behaviour, and should not
  count against any rung;
- **unreached** — it changes behaviour, but no input in the corpus or the
  generator's alphabet elicits the change.

Conflating them corrupts the deliverable in opposite directions. Score an
unreached mutant as equivalent and the rung gets credit it has not earned;
score an equivalent one as a survivor and the rung is blamed for a defect that
does not exist.

`mutate probe` settles it by running mutant and original side by side over
every trace and reporting which inputs reach the defect. For this mutant it
reported **live: 1/1**, with a declared witness — so "unreached", and the fix
belongs in the inputs.

## The fix

One corpus step and one generator change:

- `id_not_burned_by_rejections` registers a user *after* the block of rejected
  registrations, asserting `{"handle":"dave","id":4}`.
- `tracegen.createUser` now re-uses an existing handle one time in four, so
  generated traces contain rejected-then-successful registration pairs.

R0 now kills it: `expected 201 {"handle":"dave","id":4}`, got
`201 {"handle":"dave","id":5}`. All three corners remain byte-exact at 55/55.

**Full R0 sweep, Go corner: 18/18 mutants killed** — see
`evidence/runs/mutants/r0-go-sweep.txt`.

## A hazard for the calibration, hit while confirming this

`mutate apply` writes a registry naming **both** the mutant (`go@<id>`) and the
untouched original (`go`). Running `replay -impl=go` against that registry
tests the *original* and reports a clean pass.

That is what happened on the first attempt to confirm the fix, and it looked
exactly like the mutant surviving. In S-15 the same slip would report **every
mutant as a survivor and every rung as worthless**, with no error anywhere —
the run would simply be measuring the unmutated implementation many times.

S-15 must assert that the implementation it exercised was the mutant: check
the resolved directory, or have `calibrate` fail when the selected impl name
lacks the `@<id>` suffix.

## The pattern across three findings

This is the third instance of the same shape, and they compose:

| finding | what was too narrow |
|---|---|
| F006 | the **oracle** — two sibling implementations agreed on 30 of 39 shared divergences |
| F008 | the **alphabet** — 100,000 requests could not express `?x=1` on a POST path |
| F009 | the **corpus** — no input registered a user after a rejection |

Each was invisible from inside the rung reporting green. Each was found by
something outside it: F006 by comparing against a new oracle, F008 by a hand
cross-check while building a third corner, F009 by injecting a defect and
asking what fails to notice.

That last one generalises best. **A green rung tells you nothing about what it
cannot see, and the cheapest way to map its blind spots is to inject defects
and watch which ones it sleeps through.** That is the argument for the
calibration table being the deliverable rather than a by-product.
