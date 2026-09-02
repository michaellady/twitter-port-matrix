# F043 — the next blocker down: `String`'s view is not known injective, and the twin looked stronger because it trusted more

**Status:** measured 2026-09-02 while discharging R5's commutation obligations
on the lifted `crates/store` (F041)
**Class:** a real limit, one level below the one everybody was looking at — and
a demonstration that deleting a trusted shim can *lower* what a corner appears
to prove while raising what it actually proves

## What was being proved

With `Inner` lifted out of `RwLock` (F041), `abs_users` has a body:

```rust
pub open spec fn abs_users(i: &Inner) -> Set<Seq<char>> {
    Set::new(|h: Seq<char>| exists|k: String| i.users@.contains_key(k) && k@ == h)
}
```

R5's commutation obligation for `put_user` wants both directions of the
accept/reject condition:

```
result is Err ==> abs_users(old(self)).contains(u.handle@)     // rejected  => taken
result is Ok  ==> !abs_users(old(self)).contains(u.handle@)    // accepted  => free
```

The first is discharged. The second is not, and Verus says why by refusing it:

```
error: postcondition not satisfied
   --> crates/store/src/lib.rs:298:13
    |
298 |             result is Ok ==> !abs_users(old(self)).contains(u.handle@),
    |             ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^ failed this postcondition
...
315 |         Ok(())
    |         ------ at the end of the function body
```

## Why

The accept branch establishes `!self.users@.contains_key(u.handle)` — no key
**equal to** `u.handle` is in the domain. The clause needs
`!abs_users(self).contains(u.handle@)` — no key whose **view** is `u.handle@`
is in the domain. Getting from one to the other needs

```
k@ == u.handle@  ==>  k == u.handle
```

— injectivity of `String`'s view — and `vstd` does not have it.
`vstd/string.rs`:

```rust
impl View for String {
    type V = Seq<char>;
    uninterp spec fn view(&self) -> Seq<char>;
}
```

`view` is **uninterp**. vstd gives the exec side of it —
`assume_specification[<String as PartialEq>::eq] ensures res == (s@ == other@)`
— so a *runtime* comparison of two `String`s decides view-equality. Nothing
gives the spec-level converse, so as far as the verifier is concerned two
distinct `String` values may share a view and the abstraction is not known to
be injective.

**This is not the `RwLock` blocker.** It survives the lift untouched, and it is
the reason R5's remaining obligation — `resp_L(s, r) == resp_S(abs_L(s), r)` —
is not discharged: every read on this corner (`has_user`, `follows`,
`follow_set`, `home_timeline`) reduces to a membership test, and the direction
"the abstract set contains it, so the concrete map does" is exactly this.

## What was done instead

The three commutation clauses that do **not** need injectivity are stated and
discharged on the shipped functions:

```
result is Err ==> abs_users(old(self)).contains(u.handle@)
result is Err ==> abs_users(self) == abs_users(old(self))
result is Ok  ==> abs_users(self) == abs_users(old(self)).insert(u.handle@)
```

The `insert` direction needs no injectivity because the new abstract set is the
old one plus one element regardless of whether some other key happens to share
a view. `delete_follow` is the mirror case and **is** blocked: removing one
concrete key removes exactly one abstract edge only if the view is injective,
so no `remove` commutation clause is claimed.

## And this is why the twin looked stronger

The deleted twin `store::verus_proof::put_user_ensures` stated the direction
that is now refused:

```rust
users_keys(old(s)).contains(u.handle@) ==> result is Err,
!users_keys(old(s)).contains(u.handle@) ==> result is Ok,
```

It could, because it did not read a `HashMap`. It called
`proof_users_contains`, an `external_body` shim whose postcondition is
`out == users_keys(s).contains(handle@)` — **injectivity, assumed**, one layer
down, in a shim nobody was counting as an assumption.

So the twin's extra strength was borrowed. Deleting it lowers the clause count
and raises the evidence, which is F024's lesson (*"a count that goes down is
the repair"*) arriving a second time from a different direction: the count fell
because a hidden assumption stopped being counted as a proof.

## What would have to change

One of three, and they are not equally good:

1. **An injectivity axiom.** Three lines, and sound in the sense that `String`'s
   view is the only thing about it Verus interprets. It is still an axiom, and
   it would be the corner's third — the two `obeys_key_model` ones are already
   named in `crates/store` and in `TCB.md`. An axiom that makes the read side
   go through is precisely the shim that was just deleted, renamed.
2. **`abs` over `Set<String>` rather than `Set<Seq<char>>`.** No axiom needed;
   `Map::dom()` gives it directly. It abandons the `Seq<char>` vocabulary the
   Go corner's `abs` and `domain::Follow::new`'s F4 are written in, so the two
   corners' abstraction functions would no longer be stated over the same
   alphabet — which is most of what makes an ordered pair comparable at all.
3. **vstd ships it.** `View for String` becoming injective by axiom upstream is
   the only route that costs this project nothing, and it is not in
   `0.0.0-2026-04-20-1748`.

Recorded rather than chosen. Option 1 is cheap and honest if it is *counted*;
option 2 is free and quietly breaks the pairing; option 3 is not available. The
decision belongs with whoever writes the Rust R5 rung.
