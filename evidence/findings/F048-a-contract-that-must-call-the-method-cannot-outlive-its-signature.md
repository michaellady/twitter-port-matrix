# F048 — A contract *attached* to a method survives that method's signature change; a contract that must *call* it does not

**Status:** measured on the Kotlin corner while decoupling its obligation set
from `Store.createUser` (branch `claude/task-kotlin-obligation-coupling`)
**Class:** the second cost of having no ghost mode — F045 recorded the first
**Effect:** five of seven couplings were decoration and are gone; two are the
property itself and cannot go. The Kotlin corner's R5 clause 2 is permanently
coupled to a signature the mutation catalogue is entitled to rewrite.

## What was asked

F031 and F035 name seven `Store.createUser` call sites in
`impls/kotlin/verification/`, and the question for each is the same: does the
obligation need *the store's* `createUser`, or does it merely need *a user to
exist*? An obligation that only needs a user can go through `Service.createUser`
— or, better, drop the call entirely — and stops being coupled to a signature
the `id-burned-on-reject` mutant **must** change.

## The answer, site by site

| site | obligation | needs | disposition |
|---|---|---|---|
| `Obligations.kt:179` | `o4a_pageRespectsLimit` | nothing | **removed** |
| `Obligations.kt:195` | `o4b_cursorNullMeansExhausted` | nothing | **removed** |
| `Obligations.kt:208` | `o4c_pageIsNewestFirst` | nothing | **removed** |
| `Canaries.kt:61` | `c4_pageMayExceedLimit` | nothing | **removed** |
| `Canaries.kt:73` | `c5_timelineIsOldestFirst` | nothing | **removed** |
| `Refinement.kt:72` | `c02_createUserAddsExactlyThatHandle` | **`Store.createUser` itself** | **kept** |
| `Refinement.kt:161` | `k02_createUserDoesNotAddThatHandle` | **`Store.createUser` itself** | **kept** |

The five removals are F035's repair and change no property. `Store.timelinePage`
decides visibility with `t.author == user || isFollowing(user, t.author)` and
never reads `userByHandle` (`Store.kt:142-161`), so registering `"a"` before
appending tweets authored by `"a"` changed no answer at any of the five. They
were not routed through the service, they were deleted: an obligation that needs
no user at all should mention no `createUser` at all, which is a smaller surface
than moving the mention one layer up. That is what the Java twin already does.

## Why clause 2 stays, and why moving it would be the worst available outcome

`c02` and its negation canary `k02` carry **R5 clause 2**. Three separate things
make them store obligations rather than service obligations, and any one of them
is sufficient:

**1. The spec says so.** `spec/refinement/obligations.json` records clause 2 as

```json
{ "corner": "go", "layer": "store", "op": "CreateUser",
  "clause": "fresh handle => AbsUsers' == AbsUsers + {h}",
  "site": "(*MemStore).PutUser", "status": "discharged", "verifier": "gobra" }
```

`layer: "store"`. The whole content of the clause is that *this store method's*
effect on the users axis commutes with `S_obs`. Stating it about a different
method is not a smaller mention of the same sentence; it is a different sentence.

**2. The antecedent would change under the reader's nose.** `Store.createUser`'s
own docstring is "the caller has already established that it is valid and free",
so `s.createUser("!")` succeeds and adds `"!"`. `svc.createUser("!")` returns
`invalid_handle`. Clause 2's antecedent is *fresh*; through the service it would
silently become *valid and fresh*. Strictly weaker, and no longer the sentence
Gobra discharges at the other end of the cell.

**3. The cross-corner cell would stop meaning anything.**
`spec/refinement/clause-sites-kotlin.json` keys clause 2 to `(member, text)` at
`c02_createUserAddsExactlyThatHandle`, and the file says why in its own note: the
clause numbers are `obligations.json`'s, "which is the only thing that makes a
`go <- kotlin` cell mean anything". A clause stated at the service layer on one
corner and the store layer on the other is not one clause measured twice.

And the sharpest reason to refuse: `id-burned-on-reject` is *precisely* the
mutant whose signature change this coupling costs a cell to. Moving the
obligation off the method the mutant edits, in order to make the mutant compile,
is rewriting an obligation to accommodate a held-out defect — the move F031
already names and forbids.

## The Go corner does not have this problem, and the reason is not luck

Clause 2 on the Go corner is not a caller of `PutUser`. It is **attached to**
`PutUser` (`impls/go/internal/store/memstore.go:135-141`):

```go
// @ ensures !(u.Handle in old(s.AbsUsers())) ==> err == nil
// @ ensures !(u.Handle in old(s.AbsUsers())) ==>
// @            s.AbsUsers() == old(s.AbsUsers()) union set[string]{u.Handle}
// @ ensures (u.Handle in old(s.AbsUsers())) ==> s.AbsUsers() == old(s.AbsUsers())
func (s *MemStore) PutUser(u dom.User) (err error) {
```

A mutant that changed `PutUser`'s signature would edit the very lines the
contract sits on; the contract travels with the method because it is *part of*
the method. The Kotlin obligation is an ordinary `@JvmStatic` function that has
to construct a `Store` and call `createUser` to have anything to talk about, so
it is a **client** of the signature, and clients break.

F045 recorded that Kotlin's lack of ghost mode widens the corner's public
surface (eight `abs*` methods that Go keeps inside `// @` comments). This is the
same absence billed a second time, and the second bill is larger: a wider surface
is a TCB cost you can write down once, while a contract that can only be stated
as a call is a **standing coupling to every signature it names**, re-charged by
every future catalogue defect that touches one of them.

## What it costs, exactly

`go run ./tools/cmd/mutate verify -impl kotlin` before the five removals — the
gate itself is F031's repair, and this is it doing its job:

```
  FAIL   kotlin/id-burned-on-reject             implementation compiles, OBLIGATIONS do not
         | verification/Obligations.kt:195:22: error: no value passed for parameter 'id'.
         | verification/Obligations.kt:208:22: error: no value passed for parameter 'id'.
         | verification/Refinement.kt:72:22: error: no value passed for parameter 'id'.
         | verification/Refinement.kt:161:22: error: no value passed for parameter 'id'.
```

and after:

```
  FAIL   kotlin/id-burned-on-reject             implementation compiles, OBLIGATIONS do not
         | verification/Refinement.kt:104:22: error: no value passed for parameter 'id'.
         | verification/Refinement.kt:197:22: error: no value passed for parameter 'id'.

anchors: 18/18 match exactly one site
compile: 17/18 build clean

verify FAILED: 1 mutant(s): kotlin/id-burned-on-reject
```

(`kotlinc` does not report every instance of the same error, which is why the
"before" list is four lines and not five; the fifth is `Canaries.kt:61`, read
from the file. Same caveat F031 records.)

**The residue is irreducible.** No edit to `Refinement.kt` removes the last two
without moving clause 2 to a layer it is not stated at. So the Kotlin corner's
R5 denominator is 17 of 18, permanently, for as long as the catalogue contains a
defect that must re-arity `Store.createUser` — and it must: allocation is
encapsulated *inside* `createUser` on this corner, so "burn an id before the
duplicate check" cannot be expressed without splitting the method.

The R4 denominator is a separate question with a different answer, and it is
**not** irreducible. See F049.

## The transferable form

A verifier with ghost annotations lets a contract be **attached** to the thing it
constrains. A verifier that reads only compiled artefacts forces the contract to
be a **client** of the thing it constrains.

Attached contracts are invariant under signature change, because the edit that
changes the signature is the edit that carries them. Client contracts are not.
On a mutation rung, where signature-changing defects are legitimate catalogue
entries, that difference has a unit: **cells**.

The rule from F035 — *an obligation should touch the smallest surface its
property needs* — survives this finding intact, and this is its boundary. Five of
these seven sites were above the floor and came down. Two of them **are** the
floor. When an obligation is already at the smallest surface its property needs
and is still coupled, the coupling is not a defect in the obligation; it is the
price of stating that property in this language, and the honest move is to write
the price down rather than shave the property until it fits.
