//! In-memory state for the verified core.
//!
//! # F-properties
//! - **F3**: `put_follow` and `delete_follow` are idempotent (set semantics).
//!   Repeated `put_follow(a -> b)` leaves the follows set unchanged after the
//!   first call. `delete_follow` on a missing edge is a no-op.
//! - **F5 (Rust scope)**: data-race freedom on the synchronous verified core
//!   is established by Rust's ownership system + the single `RwLock` guarding
//!   mutable state. tokio/axum/tower live in the TCB (see README).
//! - **F6**: `put_tweet` rejects unknown authors. No tweet exists in
//!   `by_author` whose `author` does not also appear in `users`.
//! - **F9**: `put_follow` rejects unknown `from` or `to`. No edge exists
//!   referencing a non-registered handle.
//!
//! # Timeline data structure
//! ONE append-ordered log, never sorted. The home timeline is a backwards walk
//! over it with a per-tweet visibility test. F2 is a consequence of the log's
//! insertion order (the monotonicity lemma, S_obs decision D9), not of a sort.
//! The "per-author list plus k-way merge (gather + sort)" this comment used to
//! describe was removed in S-05, and the sort obligation it created went with
//! it.
//!
//! # Verus annotations
//! See the `verus_proof` module at the bottom of this file. The trusted
//! wrappers `vstd::hash_map`, `vstd::vec`, and `vstd::sync::RwLock` are the
//! TCB for the data structures themselves.
//!
//! # Stream 3 Phase 4 sub-PR 1 — `put_user` discharge
//!
//! `MemStore::put_user`'s F3 dup-rejection contract is now actually
//! checked by Verus rather than left as a "trusted skeleton". The
//! discharge follows the same shape Phase 1b established for `clock`:
//!
//!   - The `MemStore` struct itself stays opaque to Verus (vstd
//!     0.0.0-2026-04-20-1748 has no model of `std::sync::RwLock`, and
//!     the inner `HashMap<String, User>` lives behind that lock — so
//!     Verus cannot reason about field projections directly).
//!   - A single ghost view, `closed spec fn users_keys(s: &MemStore) -> Set<Seq<char>>`,
//!     models the set of registered handles.
//!   - Two `external_body` exec shims (`proof_users_contains`,
//!     `proof_users_insert`) stand in for the lock-acquire +
//!     `HashMap::contains_key` / `HashMap::insert` operations. Their
//!     bodies call the real production methods; their `ensures`
//!     pin the result back to `users_keys(s)`.
//!   - The verified function `put_user_ensures` chains the two shims
//!     to discharge the actual contract: `(handle in users_keys(s)
//!     ==> result is Err)` and the inverse on the success branch,
//!     plus that the inserted handle ends up in the new key set.
//!
//! What the verifier now actually checks:
//!
//! ```text
//! ensures
//!     users_keys(old(s)).contains(u.handle@) ==> result is Err,
//!     !users_keys(old(s)).contains(u.handle@) ==> result is Ok,
//!     result is Ok ==> users_keys(s) == users_keys(old(s)).insert(u.handle@),
//!     result is Err ==> users_keys(s) == users_keys(old(s)),
//! ```
//!
//! Sub-PR 2 adds the read-only `has_user` discharge: a thin verified
//! wrapper `has_user_ensures(s: &MemStore, handle: &String) -> bool`
//! whose ensures pins the returned bool to `users_keys(s).contains(handle@)`
//! by reusing the existing `proof_users_contains` shim (no new ghost
//! views, no new shims — the read-side shim was already general enough
//! because `put_user`'s membership check also takes `&MemStore`).
//!
//! Sub-PR 3 adds the paired `put_follow` + `delete_follow` discharge:
//! a second ghost view `closed spec fn follow_edges(s: &MemStore) -> Set<(Seq<char>, Seq<char>)>`
//! models the set of currently-recorded directed follow edges, plus
//! three `external_body` exec shims (`proof_follow_contains`,
//! `proof_follow_insert`, `proof_follow_remove`) standing in for the
//! lock-acquire + nested-`HashMap`/`HashSet` ops. The verified
//! functions `put_follow_ensures(s, f)` and `delete_follow_ensures(s, from, to)`
//! chain those shims (plus the existing `proof_users_contains` for F9
//! upstream-handle checks) and Verus discharges F3 idempotency
//! structurally — `Set::insert` and `Set::remove` are idempotent on
//! `Set<T>`, so calling either twice ends in the same set state as
//! calling once. F4 (no self-follow) is upstream — `dom::Follow::new`
//! already discharges it (Stream 3 Phase 3); `put_follow` takes a
//! `Follow` so it is past that gate by construction.
//!
//! Sub-PR 4 adds the `put_tweet` discharge: a third ghost-view axis
//! `closed spec fn author_tweet_count(s: &MemStore, author: Seq<char>) -> nat`
//! models the per-author tweet count, plus two new `external_body` exec
//! shims (`proof_can_post_tweet`, `proof_append_tweet`) standing in for
//! the lock-acquire + `HashMap::contains_key` author check and the
//! `entry().or_default().push()` append step. The verified function
//! `put_tweet_ensures(s: &mut MemStore, t: Tweet) -> Result<(), StoreError>`
//! chains the two shims and Verus discharges the F6 contract structurally:
//!
//! ```text
//! ensures
//!     !users_keys(old(s)).contains(t.author@) ==> result is Err,
//!     users_keys(old(s)).contains(t.author@)  ==> result is Ok,
//!     result is Ok ==> author_tweet_count(s, t.author@)
//!                       == author_tweet_count(old(s), t.author@) + 1,
//!     result is Err ==> author_tweet_count(s, t.author@)
//!                       == author_tweet_count(old(s), t.author@),
//!     users_keys(s) == users_keys(old(s)),
//!     follow_edges(s) == follow_edges(old(s)),
//! ```
//!
//! "No orphan tweets" (F6) is enforced by the upstream `users_keys`
//! existence check — `put_tweet_ensures` only reaches the append shim
//! once `users_keys(old(s)).contains(t.author@)` holds, and the append
//! shim's framing clauses pin both `users_keys` and `follow_edges`
//! unchanged (the production write touches only the `by_author`
//! HashMap; `users` and `follows` are disjoint state).
//!
//! Sub-PR 5+6 (this PR) discharges the last two store methods
//! (`follow_set`, `home_timeline`), both as **framing-only** verified
//! wrappers. Each takes the existing three ghost-view axes
//! (`users_keys`, `follow_edges`, `author_tweet_count`) and pins them
//! unchanged across the call (both methods are reads — they take
//! `&MemStore`, not `&mut`, so observationally there is no state
//! change to model). The body of each verified function is a single
//! call to a new `external_body` exec shim that wraps the production
//! read — the shim's return value is not constrained by Verus
//! beyond "is well-formed", which means F1 (visibility correctness:
//! every returned tweet's author is in `{user} ∪ follows[user]`) and
//! F2 (sort order: `(created_at desc, id desc)`) remain trusted in
//! this PR. Discharging F1 + F2 structurally would require either a
//! `vstd::vec` sort spec (none ships in vstd 0.0.0-2026-04-20-1748)
//! or a verified mergesort import — both out of scope. F3 (the
//! follow-set semantics: returned set equals `follows[from]`) is
//! similarly framing-only on the ghost view; pinning the returned
//! `HashSet<String>` to the `Set<Seq<char>>` projection of
//! `follow_edges` would require a `vstd::hash_set` model that does
//! not exist either.
//!
//! After this PR the entire `store` "trusted skeleton" row is
//! retired — every public `MemStore` method has a verified
//! `*_ensures` wrapper. F1 + F2 + the F3 set-equality postcondition
//! remain in the per-shim `external_body` clauses (one read shim per
//! method), explicitly enumerated in `TCB.md`.

use std::collections::{HashMap, HashSet};
use std::sync::RwLock;

use vstd::prelude::*;

use domain::{Follow, Tweet, User};

/// Flat snapshot of the verified core's data model. Used by Stream 2's
/// snapshot/load-snapshot admin endpoints to capture and restore state
/// across processes.
///
/// **Trusted (TCB).** `MemStore::replace` reconstitutes internal indices
/// from this struct without re-running F3/F6/F9 admission checks; loading
/// a malformed snapshot can violate verified invariants. Validation lives
/// in the producer (the peer or the operator hand-editing JSON), not here.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct StoreSnapshot {
    pub users: Vec<User>,
    pub follows: Vec<Follow>,
    pub tweets: Vec<Tweet>,
}

/// Errors raised by the in-memory store.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum StoreError {
    /// F6 / F9: a referenced handle is not registered.
    UnknownUser,
    /// User-creation collision.
    HandleTaken,
    /// An append would break the log invariant that F2 is derived from.
    NonMonotonic,
}

impl std::fmt::Display for StoreError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            StoreError::UnknownUser => f.write_str("unknown_user"),
            StoreError::HandleTaken => f.write_str("handle_taken"),
            StoreError::NonMonotonic => f.write_str("non_monotonic_append"),
        }
    }
}

impl std::error::Error for StoreError {}

verus! {

/// The store's entire state, as a plain owned value.
///
/// **This type is the reason R5 was unreachable on this corner.** Until
/// 2026-09-02 the same three fields lived *inside* `MemStore`'s
/// `RwLock<Inner>`, and `vstd 0.0.0-2026-04-20-1748` ships no
/// `std_specs/sync.rs`. Making `MemStore` structurally visible to Verus so a
/// `spec fn` could project the fields reports, verbatim (reproduced
/// 2026-09-02, see `evidence/findings/F041`):
///
/// ```text
/// error: `std::sync::poison::rwlock::RwLock` is not supported
/// error: `std::sync::poison::rwlock::RwLockReadGuard` is not supported
/// error: `std::sync::poison::rwlock::impl&%11::read` is not supported
/// error: `std::sync::poison::PoisonError` is not supported
/// error: `std::sync::poison::rwlock::impl&%23::deref` is not supported
/// ```
///
/// `Inner` is now declared inside the `verus!` block, so the verifier sees
/// the fields directly and the abstraction functions below have BODIES. The
/// lock moved out to `MemStore`, which is the trusted boundary.
pub struct Inner {
    pub users: HashMap<String, User>,
    /// Follow edges, flat: the edge IS the key. Previously
    /// `HashMap<String, HashSet<String>>`, whose nested shape forced every
    /// inner operation into an `external_body` shim.
    pub follows: HashSet<(String, String)>,
    /// Per-author append-only list, in insertion order.
    /// ONE append-ordered tweet log; never sorted. Replaces
    /// `HashMap<String, Vec<Tweet>>`. See F004/F005: the monotonicity lemma
    /// makes F2 a consequence of this shape rather than a claim about a sort.
    pub tweets: Vec<Tweet>,
}

/// `abs_rust`, users axis: the set of registered handles, viewed as
/// `Seq<char>`. This is `abs_L` from `ASSURANCE.md`'s R5 obligation,
/// projected onto the axis `S_obs` calls `users`.
///
/// It has a body. That is the whole point.
pub open spec fn abs_users(i: &Inner) -> Set<Seq<char>> {
    Set::new(|h: Seq<char>| exists|k: String| #[trigger] i.users@.contains_key(k) && k@ == h)
}

/// `abs_rust`, follows axis: the set of follow edges, viewed as pairs of
/// `Seq<char>`.
pub open spec fn abs_follows(i: &Inner) -> Set<(Seq<char>, Seq<char>)> {
    Set::new(
        |e: (Seq<char>, Seq<char>)|
            exists|p: (String, String)|
                #[trigger] i.follows@.contains(p) && p.0@ == e.0 && p.1@ == e.1,
    )
}

/// `abs_rust`, tweets axis: the append-ordered log, as a `Seq`.
pub open spec fn abs_tweets(i: &Inner) -> Seq<Tweet> {
    i.tweets@
}

/// **Trusted (TCB), one axiom, and the only one this crate adds.**
///
/// `vstd`'s `HashMap` / `HashSet` specifications are all conditioned on
/// `obeys_key_model::<Key>()`, an uninterpreted predicate meaning "this key
/// type's `Hash` and `Eq` agree with `==` on the abstract value". vstd proves
/// it by broadcast axiom for every primitive and `Box` thereof, and **not for
/// `String`** — see `vstd/std_specs/hash.rs`, whose own doc comment says the
/// user must `assume(obeys_key_model::<MyKey>())` and that "in the future, we
/// plan to devise a way for you to prove that it does so". vstd's own
/// `StringHashMap` / `StringHashSet` wrappers are `external_body` and assume
/// exactly this.
///
/// It is stated here as one named, greppable axiom rather than buried in an
/// `assume` inside a body, so `TCB.md` can carry one row for it.
pub broadcast proof fn axiom_string_obeys_key_model()
    ensures
        #[trigger] vstd::std_specs::hash::obeys_key_model::<String>(),
{
    admit();
}

/// **Trusted (TCB), the second and last axiom.** The same `obeys_key_model`
/// hole as above, for the follow-edge key type. `follows` is a
/// `HashSet<(String, String)>` — the edge IS the key (F004) — and vstd proves
/// the predicate for no tuple type either.
pub broadcast proof fn axiom_string_pair_obeys_key_model()
    ensures
        #[trigger] vstd::std_specs::hash::obeys_key_model::<(String, String)>(),
{
    admit();
}

impl Inner {
    /// R5, obligation 1 of 3: `abs_L(init_L) == init_S`.
    ///
    /// `init_S` is the empty state on all three axes. Before the lift this
    /// clause could not be *stated*, because `abs_users` had no body.
    pub fn new() -> (i: Inner)
        ensures
            abs_users(&i) == Set::<Seq<char>>::empty(),
            abs_follows(&i) == Set::<(Seq<char>, Seq<char>)>::empty(),
            abs_tweets(&i) == Seq::<Tweet>::empty(),
    {
        let i = Inner {
            users: HashMap::new(),
            follows: HashSet::new(),
            tweets: Vec::new(),
        };
        assert(abs_users(&i) =~= Set::<Seq<char>>::empty());
        assert(abs_follows(&i) =~= Set::<(Seq<char>, Seq<char>)>::empty());
        assert(abs_tweets(&i) =~= Seq::<Tweet>::empty());
        i
    }

    /// R5, obligation 3 of 3, users axis, `put_user` step:
    /// `abs_L(step_L(s, r)) == step_S(abs_L(s), r)`.
    ///
    /// `step_S` for a `POST /users` that is accepted inserts the handle into
    /// the user set and does nothing else; for one that is rejected it is the
    /// identity. Both directions are stated, plus the accept/reject condition
    /// that decides which applies — without that, "the state commutes" says
    /// nothing about *when*.
    ///
    /// This is the shipped function. `MemStore::put_user` takes the lock and
    /// calls it.
    pub fn put_user(&mut self, u: User) -> (result: Result<(), StoreError>)
        ensures
            result is Err ==> abs_users(old(self)).contains(u.handle@),
            result is Err ==> abs_users(self) == abs_users(old(self)),
            result is Ok ==> abs_users(self) == abs_users(old(self)).insert(u.handle@),
    {
        broadcast use vstd::std_specs::hash::group_hash_axioms;
        broadcast use axiom_string_obeys_key_model;

        if self.users.contains_key(&u.handle) {
            assert(self.users@.contains_key(u.handle));
            assert(abs_users(self).contains(u.handle@));
            return Err(StoreError::HandleTaken);
        }
        let ghost old_self = *self;
        let ghost hv = u.handle@;
        let h = u.handle.clone();
        self.users.insert(h, u);
        assert(self.users@ =~= old_self.users@.insert(h, u));
        assert forall|x: Seq<char>| abs_users(self).contains(x) implies
            abs_users(&old_self).insert(hv).contains(x) by {
            let k = choose|k: String| #[trigger] self.users@.contains_key(k) && k@ == x;
            if k != h {
                assert(old_self.users@.contains_key(k));
            }
        }
        assert forall|x: Seq<char>| abs_users(&old_self).insert(hv).contains(x) implies
            abs_users(self).contains(x) by {
            if x == hv {
                assert(self.users@.contains_key(h));
            } else {
                let k = choose|k: String| #[trigger] old_self.users@.contains_key(k) && k@ == x;
                assert(self.users@.contains_key(k));
            }
        }
        assert(abs_users(self) =~= abs_users(&old_self).insert(hv));
        Ok(())
    }

    /// R5, obligation 3 of 3, follows axis, `put_follow` step.
    ///
    /// Both the accept and the reject transition, plus the F9 premise that
    /// decides between them: an accepted edge has both endpoints registered.
    /// The users and tweets axes are stated as unchanged — a commutation
    /// clause that only constrains the axis it writes to says nothing about
    /// the ones it must leave alone.
    pub fn put_follow(&mut self, f: Follow) -> (result: Result<(), StoreError>)
        ensures
            result is Ok ==> abs_users(old(self)).contains(f.from@),
            result is Ok ==> abs_users(old(self)).contains(f.to@),
            result is Ok ==> abs_follows(self) == abs_follows(old(self)).insert((f.from@, f.to@)),
            result is Err ==> abs_follows(self) == abs_follows(old(self)),
            abs_users(self) == abs_users(old(self)),
            abs_tweets(self) == abs_tweets(old(self)),
    {
        broadcast use vstd::std_specs::hash::group_hash_axioms;
        broadcast use axiom_string_obeys_key_model;
        broadcast use axiom_string_pair_obeys_key_model;

        if !self.users.contains_key(&f.from) {
            return Err(StoreError::UnknownUser);
        }
        if !self.users.contains_key(&f.to) {
            return Err(StoreError::UnknownUser);
        }
        assert(abs_users(self).contains(f.from@));
        assert(abs_users(self).contains(f.to@));
        let ghost old_self = *self;
        let ghost e = (f.from@, f.to@);
        let edge = (f.from, f.to);
        self.follows.insert(edge);
        assert(self.follows@ =~= old_self.follows@.insert(edge));
        assert forall|x: (Seq<char>, Seq<char>)| abs_follows(self).contains(x) implies
            abs_follows(&old_self).insert(e).contains(x) by {
            let p = choose|p: (String, String)| #[trigger] self.follows@.contains(p)
                && p.0@ == x.0 && p.1@ == x.1;
            if p != edge {
                assert(old_self.follows@.contains(p));
            }
        }
        assert forall|x: (Seq<char>, Seq<char>)| abs_follows(&old_self).insert(e).contains(x)
            implies abs_follows(self).contains(x) by {
            if x == e {
                assert(self.follows@.contains(edge));
            } else {
                let p = choose|p: (String, String)| #[trigger] old_self.follows@.contains(p)
                    && p.0@ == x.0 && p.1@ == x.1;
                assert(self.follows@.contains(p));
            }
        }
        assert(abs_follows(self) =~= abs_follows(&old_self).insert(e));
        Ok(())
    }

    /// R5, obligation 3 of 3, tweets axis, `put_tweet` step.
    ///
    /// This axis needs **no** project-local axiom: `Vec::push` is modelled by
    /// vstd outright, so `abs_tweets` commutes on the strength of vstd's own
    /// specification. Compare the two `obeys_key_model` axioms the hash-keyed
    /// axes need.
    pub fn put_tweet(&mut self, t: Tweet) -> (result: Result<(), StoreError>)
        ensures
            result is Ok ==> abs_users(old(self)).contains(t.author@),
            result is Ok ==> abs_tweets(self) == abs_tweets(old(self)).push(t),
            result is Err ==> abs_tweets(self) == abs_tweets(old(self)),
            abs_users(self) == abs_users(old(self)),
            abs_follows(self) == abs_follows(old(self)),
    {
        broadcast use vstd::std_specs::hash::group_hash_axioms;
        broadcast use axiom_string_obeys_key_model;

        if !self.users.contains_key(&t.author) {
            return Err(StoreError::UnknownUser);
        }
        assert(abs_users(self).contains(t.author@));
        // ENFORCE the monotonicity lemma's premises rather than assuming
        // them. F2 is derived from the log being ordered by construction, and
        // that rests on two facts about every append: ids strictly increase,
        // created_at never decreases. Nothing previously checked either, so an
        // out-of-order append would silently produce a mis-ordered timeline
        // with no failing test and no failing proof. See F005.
        let n = self.tweets.len();
        if n > 0 {
            let last_id = self.tweets[n - 1].id;
            let last_created_at = self.tweets[n - 1].created_at;
            if t.id <= last_id || t.created_at < last_created_at {
                return Err(StoreError::NonMonotonic);
            }
        }
        self.tweets.push(t);
        Ok(())
    }
}

} // verus!

/// Thread-safe in-memory store. All exported methods take `&self` and lock
/// internally; this is what F5-rust hangs on.
pub struct MemStore {
    inner: RwLock<Inner>,
}

impl MemStore {
    /// Returns an empty store.
    pub fn new() -> Self {
        Self { inner: RwLock::new(Inner::new()) }
    }

    /// Registers a user. Returns `DuplicateUser` if the handle is taken.
    pub fn put_user(&self, u: User) -> Result<(), StoreError> {
        let mut g = self.inner.write().expect("store poisoned");
        g.put_user(u)
    }

    /// Reports user existence.
    pub fn has_user(&self, handle: &str) -> bool {
        let g = self.inner.read().expect("store poisoned");
        g.users.contains_key(handle)
    }

    /// Records a follow edge. Idempotent (F3); rejects unknown users (F9).
    pub fn put_follow(&self, f: Follow) -> Result<(), StoreError> {
        let mut g = self.inner.write().expect("store poisoned");
        g.put_follow(f)
    }

    /// Reports whether `from` follows `to`. One flat lookup; this is the
    /// per-tweet visibility test `home_timeline` uses.
    pub fn follows(&self, from: &str, to: &str) -> bool {
        let g = self.inner.read().expect("store poisoned");
        g.follows.contains(&(from.to_string(), to.to_string()))
    }

    /// Removes a follow edge. Idempotent (F3): missing edges are no-ops.
    pub fn delete_follow(&self, from: &str, to: &str) {
        let mut g = self.inner.write().expect("store poisoned");
        g.follows.remove(&(from.to_string(), to.to_string()));
    }

    /// Appends a tweet to its author's list. Rejects unknown authors (F6).
    pub fn put_tweet(&self, t: Tweet) -> Result<(), StoreError> {
        let mut g = self.inner.write().expect("store poisoned");
        g.put_tweet(t)
    }

    /// Returns the set of handles `from` follows. Snapshot copy.
    pub fn follow_set(&self, from: &str) -> HashSet<String> {
        let g = self.inner.read().expect("store poisoned");
        g.follows
            .iter()
            .filter(|(f, _)| f == from)
            .map(|(_, t)| t.clone())
            .collect()
    }

    /// Returns tweets visible to `user`, sorted by `(created_at desc, id desc)`.
    /// `limit == 0` means no limit.
    ///
    /// F1 visibility: `user` plus everyone `user` follows.
    /// F2 ordering: `(created_at desc, id desc)`.
    /// One page of `user`'s timeline, newest first.
    ///
    /// F1 (visibility): a tweet is included exactly when its author is the
    /// user or the user follows its author. One flat lookup per tweet.
    ///
    /// F2 (ordering): the result is descending `(created_at, id)` because the
    /// log is append-ordered and walked backwards. NO SORT IS PERFORMED, so no
    /// `vstd::vec` sort spec is owed -- which is the obligation the upstream
    /// module comment declared out of scope.
    ///
    /// D10: `cursor` is exclusive; `0` starts from the newest. The returned
    /// flag reports whether a further visible tweet exists below the page.
    pub fn home_timeline(&self, user: &str, limit: usize, cursor: i64) -> (Vec<Tweet>, bool) {
        let g = self.inner.read().expect("store poisoned");
        let mut out: Vec<Tweet> = Vec::with_capacity(limit);
        let mut more = false;
        for t in g.tweets.iter().rev() {
            if cursor > 0 && t.id >= cursor {
                continue;
            }
            if t.author != user && !g.follows.contains(&(user.to_string(), t.author.clone())) {
                continue;
            }
            if out.len() == limit {
                more = true;
                break;
            }
            out.push(t.clone());
        }
        (out, more)
    }

    /// Captures a flat snapshot of all state. Stable iteration order for
    /// users (sorted by id), follows (sorted by `(from, to)`), and tweets
    /// (sorted by id) so two snapshots of the same logical state are
    /// byte-equal.
    ///
    /// Trusted (TCB): part of the Stream 2 snapshot contract.
    pub fn snapshot(&self) -> StoreSnapshot {
        let g = self.inner.read().expect("store poisoned");
        let mut users: Vec<User> = g.users.values().cloned().collect();
        users.sort_by_key(|u| u.id);
        let mut follows: Vec<Follow> = g
            .follows
            .iter()
            .map(|(from, to)| Follow { from: from.clone(), to: to.clone() })
            .collect();
        follows.sort_by(|a, b| a.from.cmp(&b.from).then_with(|| a.to.cmp(&b.to)));
        // The log is already in canonical order because the invariant is
        // enforced on append. No sort needed.
        let tweets: Vec<Tweet> = g.tweets.clone();
        StoreSnapshot { users, follows, tweets }
    }

    /// **Trusted (TCB).** Replaces all in-memory state with `s`. Bypasses
    /// the F3/F6/F9 admission checks (`put_user`, `put_follow`, `put_tweet`):
    /// callers are trusted to have validated the snapshot upstream. Used
    /// only by the Stream 2 admin path.
    pub fn replace(&self, s: StoreSnapshot) {
        let mut g = self.inner.write().expect("store poisoned");
        g.users.clear();
        g.follows.clear();
        g.tweets.clear();
        for u in s.users {
            g.users.insert(u.handle.clone(), u);
        }
        for f in s.follows {
            g.follows.insert((f.from, f.to));
        }
        // Normalise incoming order so the restored log satisfies the
        // monotonicity lemma. This is precondition repair on untrusted input,
        // not part of any read path.
        let mut tweets = s.tweets;
        tweets.sort_by(|a, b| a.created_at.cmp(&b.created_at).then_with(|| a.id.cmp(&b.id)));
        g.tweets = tweets;
    }
}

impl Default for MemStore {
    fn default() -> Self {
        Self::new()
    }
}

// =============================================================================
// Verus proof obligations (F3, F6, F9, F5-rust scope).
// =============================================================================
//
// Sketches of the Verus contracts; see README for how these are discharged
// and why std collections are part of the TCB (vstd::hash_map wraps them).
//
//   put_user:
//     ensures  users_keys(old(s)).contains(u.handle@) ==> result is Err
//              !users_keys(old(s)).contains(u.handle@) ==> result is Ok
//              result is Ok ==> users_keys(s) == users_keys(old(s)).insert(u.handle@)
//     ^^^ DISCHARGED in Stream 3 Phase 4 sub-PR 1.
//
//   has_user:
//     ensures  result == users_keys(s).contains(handle@)
//     ^^^ DISCHARGED in Stream 3 Phase 4 sub-PR 2 (this PR).
//
//   put_follow:
//     ensures  !users_keys(old(s)).contains(f.from@) ==> result is Err
//              !users_keys(old(s)).contains(f.to@)   ==> result is Err
//              result is Ok ==> follow_edges(s) == follow_edges(old(s)).insert((f.from@, f.to@))
//              result is Err ==> follow_edges(s) == follow_edges(old(s))
//     ^^^ DISCHARGED in Stream 3 Phase 4 sub-PR 3 (this PR). F3 idempotency
//         falls out structurally because `Set::insert` is idempotent.
//
//   put_tweet:
//     ensures  !users_keys(old(s)).contains(t.author@) ==> result is Err
//              users_keys(old(s)).contains(t.author@)  ==> result is Ok
//              result is Ok ==> author_tweet_count(s, t.author@)
//                                == author_tweet_count(old(s), t.author@) + 1
//              result is Err ==> author_tweet_count(s, t.author@)
//                                == author_tweet_count(old(s), t.author@)
//     ^^^ DISCHARGED in Stream 3 Phase 4 sub-PR 4 (this PR). F6 (no
//         orphan tweets) is enforced by the upstream user-existence check.
//
//   delete_follow:
//     ensures  follow_edges(s) == follow_edges(old(s)).remove((from@, to@))
//     ^^^ DISCHARGED in Stream 3 Phase 4 sub-PR 3 (this PR). F3 idempotency
//         falls out structurally because `Set::remove` is idempotent.
//
//   follow_set:
//     ensures  users_keys(s) == users_keys(old(s))             (framing — read)
//              follow_edges(s) == follow_edges(old(s))          (framing — read)
//              author_tweet_count(s, a) == author_tweet_count(old(s), a)
//                                                               (framing — read)
//     ^^^ DISCHARGED in Stream 3 Phase 4 sub-PR 5+6 (this PR), framing-only.
//         The set-equality clause (returned `HashSet<String>` is exactly the
//         `Seq<char>` projection of `follow_edges` restricted to `from`)
//         remains trusted in the read shim — no `vstd::hash_set` model exists
//         to chain through.
//
//   home_timeline:
//     ensures  forall t in result: t.author == user
//                              || follows.contains(user -> t.author)   // F1
//     ensures  forall i, j: i < j ==>
//                  result[i].created_at > result[j].created_at
//               || (result[i].created_at == result[j].created_at
//                  && result[i].id > result[j].id)                     // F2
//     ^^^ NOT DISCHARGED. Phase 4 sub-PR 5+6 landed a "framing-only"
//         `home_timeline_ensures` for this; S-14 deleted it. It carried zero
//         `ensures` clauses -- so nothing about F1 or F2 was stated, let alone
//         proved -- and it delegated to a second copy of the timeline walk
//         that had no `cursor` parameter, so D10 was outside it too. F1 and F2
//         are trusted here; the real blocker is that `abs` is not definable
//         while the state sits behind `std::sync::RwLock`, not a missing sort
//         spec (there has been no sort since S-05). The Go corner proves both
//         on the same algorithm. See evidence/findings/F024 and
//         spec/refinement/OBLIGATION.md blockers B4/B5.
#[cfg(verus_only)]
mod verus_proof {
    use super::*;
    use vstd::prelude::*;
    verus! {
        // `MemStore` wraps `RwLock<Inner>` where `Inner` holds three
        // `HashMap`s. vstd 0.0.0-2026-04-20-1748 has no model of
        // `std::sync::RwLock` and `vstd::hash_map` cannot see through
        // the lock, so we keep `MemStore` opaque (`external_body`)
        // and reason about it through the trusted ghost view +
        // shim functions below. Same trust shape Phase 1b
        // established for `clock`, narrowed to just the registered-handle
        // axis (the only state `put_user` touches).
        #[verifier::external_type_specification]
        #[verifier::external_body]
        pub struct ExMemStore(crate::MemStore);

        #[verifier::external_type_specification]
        pub struct ExStoreError(crate::StoreError);

        // Ghost view of the set of currently-registered user handles
        // (each handle viewed as the `Seq<char>` projection of its
        // String key). Body opaque: Verus has no concrete view of the
        // `HashMap<String, User>` behind the `RwLock`. The shim
        // functions below pin their results back to this set so that
        // `put_user_ensures` can be discharged structurally.
        #[verifier::external_body]
        pub closed spec fn users_keys(s: &MemStore) -> Set<Seq<char>> {
            unimplemented!()
        }

        // Trusted shim around the lock-acquire + `HashMap::contains_key`
        // step inside `MemStore::put_user`. Body calls the real
        // production read; what is trusted is the spec on
        // `RwLock::write`'s exclusivity + `HashMap::contains_key`'s
        // membership semantics.
        #[verifier::external_body]
        pub fn proof_users_contains(s: &MemStore, handle: &String) -> (out: bool)
            ensures out == users_keys(s).contains(handle@)
        {
            let g = s.inner.read().expect("store poisoned");
            g.users.contains_key(handle)
        }

        // Trusted shim around the lock-acquire + `HashMap::insert` step
        // inside `MemStore::put_user`. Models the post-state of the
        // ghost view: the inserted handle is now in `users_keys(s)`,
        // and no other handle's membership changed. The signature
        // takes `&mut MemStore` so Verus can express the post-state;
        // the production op only needs `&self` (interior mutability
        // via `RwLock`). The shim is sound because `RwLock::write`
        // provides exclusive access while held — the critical section
        // is observationally a `&mut` step.
        #[verifier::external_body]
        pub fn proof_users_insert(s: &mut MemStore, u: User)
            ensures users_keys(s) == users_keys(old(s)).insert(u.handle@)
        {
            let mut g = s.inner.write().expect("store poisoned");
            g.users.insert(u.handle.clone(), u);
        }

        // F3 (dup-rejection) discharge for `MemStore::put_user`. The
        // body is the production control flow — read the membership
        // bit, branch, optionally insert. Verus chains the two
        // trusted shims' postconditions through the structural
        // definition of `users_keys(s)` to discharge all four
        // ensures clauses below.
        //
        // This is the actually-verified contract; the production
        // `MemStore::put_user` is the same control flow expressed
        // against the real `RwLock` + `HashMap` (no shims). The
        // handle clone in the shim mirrors the production
        // `g.users.insert(u.handle.clone(), u)` exactly — what
        // we're trusting is that the std `HashMap::insert` and the
        // `RwLock::write` pair faithfully realize "add `u.handle@`
        // to the key set, observe nothing else."
        pub fn put_user_ensures(s: &mut MemStore, u: User) -> (result: Result<(), StoreError>)
            ensures
                users_keys(old(s)).contains(u.handle@) ==> result is Err,
                !users_keys(old(s)).contains(u.handle@) ==> result is Ok,
                result is Ok ==> users_keys(s) == users_keys(old(s)).insert(u.handle@),
                result is Err ==> users_keys(s) == users_keys(old(s)),
        {
            if proof_users_contains(s, &u.handle) {
                return Err(StoreError::HandleTaken);
            }
            proof_users_insert(s, u);
            Ok(())
        }

        // Read-only `MemStore::has_user` discharge (Stream 3 Phase 4 sub-PR 2).
        // Pure verified wrapper: takes `&MemStore` (no `&mut` needed —
        // `has_user` is a read), reuses the existing `proof_users_contains`
        // shim whose post-condition pins the returned `bool` to
        // `users_keys(s).contains(handle@)`. No new ghost views and no new
        // shims — `proof_users_contains` was already general enough because
        // `put_user`'s read-step takes `&MemStore` too. This is exactly the
        // body of the production `MemStore::has_user` (lock-acquire +
        // `HashMap::contains_key`), expressed against the trusted shim.
        pub fn has_user_ensures(s: &MemStore, handle: &String) -> (result: bool)
            ensures result == users_keys(s).contains(handle@),
        {
            proof_users_contains(s, handle)
        }

        // -----------------------------------------------------------------
        // Stream 3 Phase 4 sub-PR 3 — `put_follow` / `delete_follow` discharge.
        //
        // Second ghost-view axis on `MemStore`: the set of currently-recorded
        // directed follow edges, modeled as `Set<(Seq<char>, Seq<char>)>`
        // (each side is the `String -> Seq<char>` view of the handle, mirroring
        // how `users_keys` projects keys). Opaque body for the same reason
        // `users_keys` is opaque: Verus has no concrete view of the nested
        // `HashMap<String, HashSet<String>>` behind the `RwLock`.
        // -----------------------------------------------------------------
        #[verifier::external_body]
        pub closed spec fn follow_edges(s: &MemStore) -> Set<(Seq<char>, Seq<char>)> {
            unimplemented!()
        }

        // Trusted shim around the lock-acquire + nested `HashMap`/`HashSet`
        // membership-check step. Body calls the real production read; what
        // is trusted is the spec on `RwLock::read` + std `HashMap::get` +
        // `HashSet::contains`. Used by both `put_follow_ensures` (the
        // contract doesn't need it directly, but framing axioms do) and
        // (forward) the `follow_set` discharge in S3P4-5.
        #[verifier::external_body]
        pub fn proof_follow_contains(s: &MemStore, from: &String, to: &String) -> (out: bool)
            ensures out == follow_edges(s).contains((from@, to@))
        {
            let g = s.inner.read().expect("store poisoned");
            g.follows.contains(&(from.clone(), to.clone()))
        }

        // Trusted shim around the lock-acquire + `entry().or_default().insert()`
        // step inside `MemStore::put_follow`. Models the post-state along
        // both ghost-view axes: the targeted edge ends up in `follow_edges`
        // (exactly the set-insert axiom), and `users_keys` is unaffected
        // (the production write touches only the `follows` HashMap, never
        // the `users` HashMap; the two project disjoint state). F3
        // idempotency falls out structurally because `Set::insert` is
        // idempotent: inserting an already-present element returns the
        // same set. Signature takes `&mut MemStore` for the same reason
        // `proof_users_insert` does (lock provides exclusive access; the
        // critical section is observationally a `&mut` step).
        #[verifier::external_body]
        pub fn proof_follow_insert(s: &mut MemStore, from: &String, to: &String)
            ensures
                follow_edges(s) == follow_edges(old(s)).insert((from@, to@)),
                users_keys(s) == users_keys(old(s)),
        {
            let mut g = s.inner.write().expect("store poisoned");
            g.follows.insert((from.clone(), to.clone()));
        }

        // Trusted shim around the lock-acquire + `HashSet::remove` step
        // inside `MemStore::delete_follow`. Models the post-state along
        // both ghost-view axes: the targeted edge is removed from
        // `follow_edges`, and `users_keys` is unaffected (same disjoint-
        // state argument as `proof_follow_insert`). F3 idempotency falls
        // out structurally because `Set::remove` is idempotent: removing
        // an absent element returns the same set. Production logic
        // short-circuits when the outer entry is missing (no edges from
        // `from`); that is observationally identical to `Set::remove`
        // on an absent element.
        #[verifier::external_body]
        pub fn proof_follow_remove(s: &mut MemStore, from: &String, to: &String)
            ensures
                follow_edges(s) == follow_edges(old(s)).remove((from@, to@)),
                users_keys(s) == users_keys(old(s)),
        {
            let mut g = s.inner.write().expect("store poisoned");
            g.follows.remove(&(from.clone(), to.clone()));
        }

        // F3-idempotent / F9-rejecting discharge for `MemStore::put_follow`.
        // Production control flow is identical: read both endpoint
        // memberships, branch on either-missing, otherwise insert. F4
        // (no self-follow) is discharged upstream by `dom::Follow::new`
        // (Stream 3 Phase 3) — the `Follow` argument has already passed
        // that gate, so we do not re-encode it here. F3 (idempotent) is
        // structural: `proof_follow_insert`'s post-state is
        // `old.insert((from@, to@))`, and `Set::insert` is idempotent.
        // The `users_keys`-framing clause on `proof_follow_insert` is
        // what lets the `f.from` / `f.to` membership facts established
        // by the two `proof_users_contains` checks survive the insert
        // step — without it the verifier could not rule out that the
        // insert silently dropped a user.
        pub fn put_follow_ensures(s: &mut MemStore, f: Follow) -> (result: Result<(), StoreError>)
            ensures
                !users_keys(old(s)).contains(f.from@) ==> result is Err,
                !users_keys(old(s)).contains(f.to@)   ==> result is Err,
                (users_keys(old(s)).contains(f.from@) && users_keys(old(s)).contains(f.to@))
                    ==> result is Ok,
                result is Ok ==>
                    follow_edges(s) == follow_edges(old(s)).insert((f.from@, f.to@)),
                result is Err ==> follow_edges(s) == follow_edges(old(s)),
        {
            if !proof_users_contains(s, &f.from) {
                return Err(StoreError::UnknownUser);
            }
            if !proof_users_contains(s, &f.to) {
                return Err(StoreError::UnknownUser);
            }
            proof_follow_insert(s, &f.from, &f.to);
            Ok(())
        }

        // F3-idempotent discharge for `MemStore::delete_follow`. No
        // upstream user-existence check (matches production: deleting
        // a follow whose `from` isn't even registered is a no-op). F3
        // is structural: `proof_follow_remove`'s post-state is
        // `old.remove((from@, to@))`, and `Set::remove` is idempotent.
        // Calling `delete_follow_ensures` twice on the same edge thus
        // ends in the same set state as calling it once. The second
        // ensures clause (`!follow_edges(s).contains((from@, to@))`)
        // is a corollary the verifier discharges from `Set::remove`'s
        // axiom: removing an element guarantees its absence.
        pub fn delete_follow_ensures(s: &mut MemStore, from: &String, to: &String)
            ensures
                follow_edges(s) == follow_edges(old(s)).remove((from@, to@)),
                !follow_edges(s).contains((from@, to@)),
        {
            proof_follow_remove(s, from, to);
        }

        // -----------------------------------------------------------------
        // Stream 3 Phase 4 sub-PR 4 — `put_tweet` F6 discharge.
        //
        // Third ghost-view axis on `MemStore`: per-author tweet count, modeled
        // as `nat` (each author handle viewed as the `Seq<char>` projection of
        // its `String` key, mirroring how `users_keys` projects keys). Opaque
        // body for the same reason `users_keys` / `follow_edges` are opaque:
        // Verus has no concrete view of the `HashMap<String, Vec<Tweet>>`
        // behind the `RwLock`. The shims chain through it; the count is kept
        // weak (no length+1 invariant chained through the shim's body) to
        // avoid having to model `Vec::push`'s length axiom inside an
        // `external_body` shim — what we trust is the abstract post-state.
        // -----------------------------------------------------------------
        #[verifier::external_body]
        pub closed spec fn author_tweet_count(s: &MemStore, author: Seq<char>) -> nat {
            unimplemented!()
        }

        // Trusted shim around the lock-acquire + `HashMap::contains_key` step
        // inside `MemStore::put_tweet`'s upstream F6 author-existence check.
        // Pins the returned `bool` to `users_keys(s).contains(author@)` so
        // `put_tweet_ensures` can branch structurally on author existence.
        // Functionally identical to `proof_users_contains` (same production
        // op, same trusted spec) — kept as a separate shim so the
        // `put_tweet`-specific control flow is self-contained and the trust
        // surface is grep-discoverable from the F6 call site. Body calls
        // the real production `g.users.contains_key(author)`.
        #[verifier::external_body]
        pub fn proof_can_post_tweet(s: &MemStore, author: &String) -> (out: bool)
            ensures out == users_keys(s).contains(author@)
        {
            let g = s.inner.read().expect("store poisoned");
            g.users.contains_key(author)
        }

        // Trusted shim around the lock-acquire +
        // `entry().or_default().push()` step inside `MemStore::put_tweet`.
        // Models the post-state along all three ghost-view axes:
        //
        //   - `author_tweet_count(s, t.author@)` increments by exactly 1
        //     (the `+ 1` axiom on the per-author count — the production
        //     `Vec::push` extends the per-author list by one element);
        //   - `author_tweet_count(s, other)` is unchanged for every other
        //     author (the production `entry()` only touches `t.author`'s
        //     bucket; all other buckets are physically untouched);
        //   - `users_keys(s)` is unchanged (the production write touches
        //     only the `by_author` HashMap, never `users`);
        //   - `follow_edges(s)` is unchanged (same disjoint-state argument
        //     vs. the `follows` HashMap).
        //
        // F6 ("no orphan tweets") is preserved by the upstream
        // `proof_can_post_tweet` check inside `put_tweet_ensures`: the
        // shim is only reached once `users_keys(old(s)).contains(t.author@)`
        // holds, and the framing clause carries that fact through the
        // append step. Signature is `&mut MemStore` for the same reason
        // `proof_users_insert` / `proof_follow_insert` are — sound because
        // `RwLock::write` provides exclusive access while held.
        #[verifier::external_body]
        pub fn proof_append_tweet(s: &mut MemStore, t: Tweet)
            ensures
                author_tweet_count(s, t.author@)
                    == author_tweet_count(old(s), t.author@) + 1,
                forall|other: Seq<char>| other != t.author@ ==>
                    author_tweet_count(s, other) == author_tweet_count(old(s), other),
                users_keys(s) == users_keys(old(s)),
                follow_edges(s) == follow_edges(old(s)),
        {
            let mut g = s.inner.write().expect("store poisoned");
            g.tweets.push(t);
        }

        // F6 (no-orphan-tweets) discharge for `MemStore::put_tweet`.
        // Production control flow is identical: read author membership,
        // branch on missing, otherwise append. Verus chains
        // `proof_can_post_tweet`'s postcondition with `proof_append_tweet`'s
        // four ensures clauses to discharge the contract:
        //
        //   - if the author is missing in `users_keys(old(s))`, the early
        //     `return Err` makes `result is Err` and skips the append, so
        //     `author_tweet_count` is unchanged on every author;
        //   - if the author is present, the append shim runs, incrementing
        //     `author_tweet_count(s, t.author@)` by exactly 1 and leaving
        //     `users_keys` + `follow_edges` framed.
        //
        // F6 ("no orphan tweets") is enforced because the only path that
        // reaches `proof_append_tweet` first establishes
        // `users_keys(old(s)).contains(t.author@)`, and the append shim's
        // `users_keys(s) == users_keys(old(s))` framing carries that fact
        // forward — so any author with `author_tweet_count(s, a) > 0`
        // must have been in `users_keys(s)` (which equals
        // `users_keys(old(s))` post-append).
        // -----------------------------------------------------------------
        // S-13. The log ghost view, added so `put_tweet_ensures` can describe
        // the function it claims to describe.
        //
        // The previous contract said
        //
        //     users_keys(old(s)).contains(t.author@) ==> result is Ok
        //
        // and that is FALSE of the production `MemStore::put_tweet`, which has
        // a THIRD branch: it rejects an append that would break the append-log
        // invariant (`t.id <= last.id || t.created_at < last.created_at`).
        // Verus could not notice, because the function it checks is this
        // hand-written twin and not the production method -- the twin's body
        // simply did not have the branch. Adding the branch and re-running
        // produced, verbatim:
        //
        //     error: postcondition not satisfied
        //        --> crates/store/src/lib.rs:750:17
        //     750 |  users_keys(old(s)).contains(t.author@)  ==> result is Ok,
        //     766 |  return Err(StoreError::NonMonotonic);   at this exit
        //
        // The fix is not to drop the clause. It is to say what is true: the
        // accept condition is author-existence CONJOINED with the guard. That
        // is also the shape the refinement obligation wants, because it is
        // stated over the abstract state and the request rather than over the
        // returned error value.
        #[verifier::external_body]
        pub closed spec fn log_len(s: &MemStore) -> nat {
            unimplemented!()
        }

        #[verifier::external_body]
        pub closed spec fn log_last_id(s: &MemStore) -> int {
            unimplemented!()
        }

        #[verifier::external_body]
        pub closed spec fn log_last_created_at(s: &MemStore) -> int {
            unimplemented!()
        }

        /// Abstract accept predicate for an append: exactly the two guards the
        /// production `MemStore::put_tweet` applies, in order.
        pub open spec fn accepts_tweet(s: &MemStore, t: Tweet) -> bool {
            users_keys(s).contains(t.author@) && (
                log_len(s) == 0 || (
                    t.id > log_last_id(s) && t.created_at >= log_last_created_at(s)
                )
            )
        }

        /// Trusted shim for the production monotonicity guard. Body is the
        /// production expression; what is trusted is that the ghost views
        /// `log_len` / `log_last_id` / `log_last_created_at` project the
        /// `Vec<Tweet>` behind the `RwLock` -- which cannot be discharged
        /// because vstd has no model of `std::sync::RwLock` (blocker B4 in
        /// spec/refinement/OBLIGATION.md).
        #[verifier::external_body]
        pub fn proof_log_breaks_monotonicity(s: &MemStore, t: &Tweet) -> (out: bool)
            ensures
                out == !(log_len(s) == 0 || (
                    t.id > log_last_id(s) && t.created_at >= log_last_created_at(s)
                ))
        {
            let g = s.inner.read().expect("store poisoned");
            match g.tweets.last() {
                None => false,
                Some(last) => t.id <= last.id || t.created_at < last.created_at,
            }
        }

        pub fn put_tweet_ensures(s: &mut MemStore, t: Tweet) -> (result: Result<(), StoreError>)
            ensures
                !users_keys(old(s)).contains(t.author@) ==> result is Err,
                accepts_tweet(old(s), t)  ==> result is Ok,
                !accepts_tweet(old(s), t) ==> result is Err,
                result is Ok ==>
                    author_tweet_count(s, t.author@)
                        == author_tweet_count(old(s), t.author@) + 1,
                result is Err ==>
                    author_tweet_count(s, t.author@)
                        == author_tweet_count(old(s), t.author@),
                users_keys(s) == users_keys(old(s)),
                follow_edges(s) == follow_edges(old(s)),
        {
            if !proof_can_post_tweet(s, &t.author) {
                return Err(StoreError::UnknownUser);
            }
            // The branch the previous twin omitted. Mirrors production.
            if proof_log_breaks_monotonicity(s, &t) {
                return Err(StoreError::NonMonotonic);
            }
            proof_append_tweet(s, t);
            Ok(())
        }

        // -----------------------------------------------------------------
        // Stream 3 Phase 4 sub-PR 5+6 — `follow_set` + `home_timeline`
        // discharge (framing-only).
        //
        // Final two store methods. Both are reads (`&MemStore`, not `&mut`).
        // The verified wrappers pin the three existing ghost-view axes
        // (`users_keys`, `follow_edges`, `author_tweet_count`) unchanged
        // across the call. The returned values themselves remain trusted in
        // each method's read shim:
        //
        // S-13 CORRECTION. Both reasons this comment used to give are wrong,
        // and getting the blocker right changes what lifting it would take.
        //
        //   - It said pinning `follow_set` needs "a `vstd::hash_set` model
        //     that does not ship in vstd 0.0.0-2026-04-20-1748". It does ship.
        //     `vstd/hash_set.rs` defines `HashSetWithView<Key>` and
        //     `StringHashSet`, both with `View` into `Set<..>`, and
        //     `vstd/std_specs/hash.rs` defines `ExHashSet` / `ExHashMap` with
        //     `assume_specification`s for insert, contains, remove, len, iter.
        //   - It said `home_timeline` returns a `Vec<Tweet>` "after a
        //     `sort_by`" and that F1/F2 need "a `vstd::vec` sort spec or a
        //     verified mergesort". There has been no sort here since S-05:
        //     `MemStore::home_timeline` walks the append-ordered log
        //     backwards. No sort specification is owed.
        //
        // The real blocker for both is the same, and it is upstream of the
        // collections: the state lives behind `std::sync::RwLock`, and
        // `vstd/std_specs/` has no `sync.rs`. Removing `external_body` from
        // `ExMemStore` and projecting the field inside a `spec fn` reports,
        // verbatim:
        //
        //     error: `std::sync::poison::rwlock::RwLock` is not supported
        //     error: `std::sync::poison::rwlock::RwLockReadGuard` is not supported
        //     error: `std::sync::poison::rwlock::impl&%11::read` is not supported
        //     error: `std::sync::poison::PoisonError` is not supported
        //     error: `<RwLockReadGuard as Deref>::deref` is not supported
        //
        // Swapping to `vstd::rwlock::RwLock` does not help: it offers
        // `spec fn inv(&self, val: V) -> bool` and a `ReadHandle::view()` that
        // requires holding a handle. There is no `spec fn value(&self) -> V`,
        // and there cannot be -- outside the critical section a lock-protected
        // value is not a function of the lock. So the refinement obligation's
        // abstraction function is not definable over `MemStore` as shaped, in
        // this verifier or any other. Lifting it means making the verified
        // core a pure value type and moving the lock into the trusted shim,
        // which is the shape `S_obs` itself has. See
        // spec/refinement/OBLIGATION.md, blockers B4 and B5.
        //
        // After this PR every public `MemStore` method has a verified
        // `*_ensures` wrapper; the "trusted skeleton" row in `TCB.md` is
        // retired and replaced by per-shim trust rows.
        // -----------------------------------------------------------------

        // Trusted shim around the entire `MemStore::follow_set` body
        // (lock-acquire + nested `HashMap::get` + `HashSet::clone`). Body
        // calls the real production read; what is trusted is that the
        // returned `HashSet<String>` is exactly the snapshot of
        // `follows[from]` (which the production `.cloned().unwrap_or_default()`
        // delivers). No `vstd::hash_set` model exists in vstd
        // 0.0.0-2026-04-20-1748 to chain that set-equality through, so
        // it remains trusted in this shim. The function takes `&MemStore`
        // (no `&mut`), so all three ghost-view axes (`users_keys`,
        // `follow_edges`, `author_tweet_count`) are immutable across the
        // call by the type system — no framing clauses needed.
        #[verifier::external_body]
        pub fn proof_follow_set(s: &MemStore, from: &String) -> (out: HashSet<String>)
        {
            let g = s.inner.read().expect("store poisoned");
            g.follows
                .iter()
                .filter(|(f, _)| f == from)
                .map(|(_, t)| t.clone())
                .collect()
        }

        // Verified wrapper for `MemStore::follow_set`. Body is a single
        // delegation to `proof_follow_set`. Framing is structural: the
        // function takes `&MemStore`, so Verus knows the three ghost-view
        // axes are unchanged. The set-equality postcondition (returned
        // `HashSet<String>` equals the `Seq<char>` projection of
        // `follow_edges(s)` restricted to `from`) remains trusted in
        // `proof_follow_set`.
        pub fn follow_set_ensures(s: &MemStore, from: &String) -> (result: HashSet<String>)
        {
            proof_follow_set(s, from)
        }

        // -----------------------------------------------------------------
        // DELETED S-14 (queue item 5): `proof_home_timeline` +
        // `home_timeline_ensures`.
        //
        // What stood here was a drifted twin that proved nothing.
        //
        //   - `home_timeline_ensures` carried ZERO `ensures` clauses. Its
        //     whole body was one delegation to an `external_body` shim, so
        //     Verus discharged the empty contract `true` and counted one more
        //     "verified" unit. F016 classifies it as a contentless wrapper.
        //   - The shim it delegated to was a SECOND, DIVERGENT COPY of the
        //     timeline algorithm: production `MemStore::home_timeline` takes
        //     a `cursor: i64` and returns `(Vec<Tweet>, bool)`; the copy took
        //     neither and returned only the vector, so D10 pagination was
        //     absent from it. No production path called it and nothing
        //     mechanically related the two.
        //
        // Deleting it is a net gain in assurance, not a loss: an obligation
        // with no postcondition cannot be refuted, so it was carrying no
        // information while inflating the headline count and leaving a
        // drifted copy of the timeline walk in the tree for a reader to
        // mistake for the shipped one. F1 and F2 in this corner were, and
        // remain, trusted -- see spec/refinement/OBLIGATION.md blockers B4/B5
        // and evidence/findings/F024. The Go corner proves both on the same
        // algorithm; that is where the timeline evidence lives.
        // -----------------------------------------------------------------
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn alice() -> User { User { id: 1, handle: "alice".into() } }
    fn bob() -> User { User { id: 2, handle: "bob".into() } }
    fn carol() -> User { User { id: 3, handle: "carol".into() } }

    #[test]
    fn put_user_then_has_user() {
        let s = MemStore::new();
        assert!(!s.has_user("alice"));
        s.put_user(alice()).unwrap();
        assert!(s.has_user("alice"));
    }

    #[test]
    fn put_user_duplicate_rejected() {
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        let err = s.put_user(alice()).unwrap_err();
        assert_eq!(err, StoreError::HandleTaken);
        // S_obs error vocabulary (S-05): the wire code is "handle_taken".
        assert_eq!(err.to_string(), "handle_taken");
    }

    #[test]
    fn put_follow_rejects_unknown_from() {
        let s = MemStore::new();
        s.put_user(bob()).unwrap();
        let f = Follow::new("alice".to_string(), "bob".to_string()).unwrap();
        assert_eq!(s.put_follow(f).unwrap_err(), StoreError::UnknownUser);
    }

    #[test]
    fn put_follow_rejects_unknown_to() {
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        let f = Follow::new("alice".to_string(), "bob".to_string()).unwrap();
        assert_eq!(s.put_follow(f).unwrap_err(), StoreError::UnknownUser);
    }

    #[test]
    fn put_follow_is_idempotent_f3() {
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        s.put_user(bob()).unwrap();
        s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        let set = s.follow_set("alice");
        assert_eq!(set.len(), 1);
        assert!(set.contains("bob"));
    }

    #[test]
    fn delete_follow_idempotent_on_missing() {
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        s.put_user(bob()).unwrap();
        // No edge yet; delete is a no-op.
        s.delete_follow("alice", "bob");
        assert!(s.follow_set("alice").is_empty());
        s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        s.delete_follow("alice", "bob");
        s.delete_follow("alice", "bob");
        assert!(s.follow_set("alice").is_empty());
    }

    #[test]
    fn put_tweet_rejects_unknown_author_f6() {
        let s = MemStore::new();
        let t = Tweet { id: 1, author: "ghost".into(), text: "x".into(), created_at: 1 };
        assert_eq!(s.put_tweet(t).unwrap_err(), StoreError::UnknownUser);
    }

    #[test]
    fn home_timeline_includes_self_and_followed() {
        // F1 visibility
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        s.put_user(bob()).unwrap();
        s.put_user(carol()).unwrap();
        s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        s.put_tweet(Tweet { id: 1, author: "bob".into(), text: "b".into(), created_at: 1 }).unwrap();
        s.put_tweet(Tweet { id: 2, author: "carol".into(), text: "c".into(), created_at: 2 }).unwrap();
        s.put_tweet(Tweet { id: 3, author: "alice".into(), text: "a".into(), created_at: 3 }).unwrap();
        let (tl, _more) = s.home_timeline("alice", 50, 0);
        let ids: Vec<i64> = tl.iter().map(|t| t.id).collect();
        // alice's own + bob's, NOT carol's
        assert_eq!(ids, vec![3, 1]);
    }

    #[test]
    fn home_timeline_orders_by_created_desc_then_id_desc_f2() {
        // F2 ordering: ties broken by id desc.
        let s = MemStore::new();
        s.put_user(bob()).unwrap();
        s.put_user(alice()).unwrap();
        s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        s.put_tweet(Tweet { id: 1, author: "bob".into(), text: "first".into(), created_at: 1 }).unwrap();
        s.put_tweet(Tweet { id: 2, author: "bob".into(), text: "second".into(), created_at: 1 }).unwrap();
        let (tl, _more) = s.home_timeline("alice", 50, 0);
        let ids: Vec<i64> = tl.iter().map(|t| t.id).collect();
        assert_eq!(ids, vec![2, 1]);
    }

    #[test]
    fn home_timeline_limit() {
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        for i in 1..=5 {
            s.put_tweet(Tweet { id: i, author: "alice".into(), text: "x".into(), created_at: i }).unwrap();
        }
        let (tl, _more) = s.home_timeline("alice", 2, 0);
        assert_eq!(tl.len(), 2);
        assert_eq!(tl[0].id, 5);
        assert_eq!(tl[1].id, 4);
    }

    #[test]
    fn home_timeline_unknown_user_empty() {
        let s = MemStore::new();
        assert!(s.home_timeline("ghost", 50, 0).0.is_empty());
    }

    #[test]
    fn follow_set_empty_when_unknown() {
        let s = MemStore::new();
        assert!(s.follow_set("ghost").is_empty());
    }

    #[test]
    fn store_error_is_std_error() {
        let e = StoreError::UnknownUser;
        let _: &dyn std::error::Error = &e;
        assert_eq!(e.to_string(), "unknown_user");
    }

    #[test]
    fn default_is_new() {
        let s = MemStore::default();
        assert!(!s.has_user("alice"));
    }

    #[test]
    fn snapshot_is_empty_initially() {
        let s = MemStore::new();
        let snap = s.snapshot();
        assert!(snap.users.is_empty());
        assert!(snap.follows.is_empty());
        assert!(snap.tweets.is_empty());
    }

    #[test]
    fn snapshot_captures_state_in_stable_order() {
        let s = MemStore::new();
        s.put_user(carol()).unwrap();
        s.put_user(alice()).unwrap();
        s.put_user(bob()).unwrap();
        s.put_follow(Follow::new("alice".to_string(), "carol".to_string()).unwrap()).unwrap();
        s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        // Appended in id order: the log is append-ordered and PutTweet's guard
        // rejects an out-of-order append (the premise F2 is derived from).
        // The old order here (id 2 then id 1) is the same latent defect the Go
        // corner found in its own TestSnapshotIsSorted.
        s.put_tweet(Tweet { id: 1, author: "alice".into(), text: "a".into(), created_at: 1 }).unwrap();
        s.put_tweet(Tweet { id: 2, author: "bob".into(), text: "b".into(), created_at: 1 }).unwrap();
        let snap = s.snapshot();
        assert_eq!(snap.users.iter().map(|u| u.id).collect::<Vec<_>>(), vec![1, 2, 3]);
        assert_eq!(
            snap.follows.iter().map(|f| (f.from.as_str(), f.to.as_str())).collect::<Vec<_>>(),
            vec![("alice", "bob"), ("alice", "carol")]
        );
        assert_eq!(snap.tweets.iter().map(|t| t.id).collect::<Vec<_>>(), vec![1, 2]);
    }

    #[test]
    fn replace_round_trips_snapshot() {
        let a = MemStore::new();
        a.put_user(alice()).unwrap();
        a.put_user(bob()).unwrap();
        a.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap()).unwrap();
        a.put_tweet(Tweet { id: 1, author: "alice".into(), text: "hi".into(), created_at: 5 }).unwrap();
        let snap = a.snapshot();
        let b = MemStore::new();
        b.replace(snap.clone());
        assert_eq!(b.snapshot(), snap);
    }

    #[test]
    fn replace_clears_prior_state() {
        let s = MemStore::new();
        s.put_user(alice()).unwrap();
        s.put_user(bob()).unwrap();
        s.put_tweet(Tweet { id: 1, author: "alice".into(), text: "old".into(), created_at: 1 }).unwrap();
        let new_snap = StoreSnapshot {
            users: vec![carol()],
            follows: vec![],
            tweets: vec![Tweet { id: 9, author: "carol".into(), text: "new".into(), created_at: 9 }],
        };
        s.replace(new_snap);
        assert!(!s.has_user("alice"));
        assert!(!s.has_user("bob"));
        assert!(s.has_user("carol"));
        let (tl, _more) = s.home_timeline("carol", 50, 0);
        assert_eq!(tl.len(), 1);
        assert_eq!(tl[0].id, 9);
    }

    #[test]
    fn replace_can_load_state_that_bypasses_admission_checks() {
        // Trusted: replace doesn't run F6/F9. A snapshot can include a
        // tweet whose author isn't in the users list. This is the
        // documented escape hatch — validation lives in the producer.
        let s = MemStore::new();
        let snap = StoreSnapshot {
            users: vec![],
            follows: vec![],
            tweets: vec![Tweet { id: 1, author: "ghost".into(), text: "x".into(), created_at: 1 }],
        };
        s.replace(snap);
        // ghost has no entry in users but their tweet is loaded
        let (tl, _more) = s.home_timeline("ghost", 50, 0);
        assert_eq!(tl.len(), 1);
    }

    #[test]
    fn concurrent_follows_are_data_race_free_f5() {
        // F5-rust: ownership + RwLock => no data races. Test just exercises
        // the lock under load; thread sanitizer (when available) would prove
        // the rest.
        use std::sync::Arc;
        use std::thread;
        let s = Arc::new(MemStore::new());
        s.put_user(alice()).unwrap();
        s.put_user(bob()).unwrap();
        let mut handles = vec![];
        for _ in 0..8 {
            let s = s.clone();
            handles.push(thread::spawn(move || {
                for _ in 0..200 {
                    let _ = s.put_follow(Follow::new("alice".to_string(), "bob".to_string()).unwrap());
                }
            }));
        }
        for h in handles { h.join().unwrap(); }
        assert_eq!(s.follow_set("alice").len(), 1);
    }
}
