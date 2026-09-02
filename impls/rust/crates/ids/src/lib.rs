//! Strictly-monotonic ID generator.
//!
//! # F-properties
//! - **F8**: every call to `next()` returns a value strictly greater than the
//!   previous return. Combined with one generator per kind (users, tweets),
//!   this gives global uniqueness within that kind. Per-author monotonicity
//!   for tweet IDs follows because all tweets share the same generator and
//!   IDs are total-ordered.
//!
//! # Verus annotations — the state is lifted OUT of the lock
//!
//! Until 2026-09-02 this crate carried F8 on `verus_proof::next_id_ensures`,
//! an `external_body` function with an `unimplemented!()` body, stated over
//! `count(g) == lock_state_value(inner_state(g))` where both projectors were
//! themselves `external_body`. That is an assumed postcondition on a function
//! nothing calls, written in terms of uninterpreted symbols; `cargo-verus`
//! reported `0 verified, 0 errors` for the whole crate. See
//! `evidence/findings/F012`, `F016`, `F024`.
//!
//! The blocker was never F8. It was that `Generator`'s state lived behind
//! `std::sync::Mutex<i64>`, and `vstd 0.0.0-2026-04-20-1748` ships no
//! `std_specs/sync.rs` — no model of `std::sync::Mutex` or
//! `std::sync::RwLock`. A spec function cannot project through a lock the
//! verifier cannot see, so the projection had to be assumed.
//!
//! The repair is a refactor, not an annotation. The counter is now a plain
//! owned value type, [`Counter`], defined inside a top-level `verus! { .. }`
//! block with `&mut self` methods carrying real `ensures` clauses that Verus
//! discharges **against their own bodies**. The lock is pushed out to
//! [`LockState`], a thin trusted boundary holding `Mutex<Counter>` — the same
//! verified-core / trusted-shim split `internal/httpshim` has on the Go
//! corner and `TCB.md` already describes.
//!
//! Discharged contract, on the shipped function (`Counter::next`):
//!
//! ```text
//! requires
//!     old(self).wf(),
//!     old(self).value < i64::MAX,
//! ensures
//!     out == old(self).value + 1,
//!     self.value == out,
//!     self.value > old(self).value,
//!     out >= 1,
//!     self.wf(),
//! ```
//!
//! That is F8, on the function `Generator::next_id` actually executes,
//! discharged against its body. What remains trusted is the lock, and only
//! the lock: `LockState` and `Generator` sit OUTSIDE the `verus!` block, which
//! is how Verus is told not to look at them. `TCB.md` carries the row.

use std::sync::Mutex;

use vstd::prelude::*;

verus! {

/// The generator's entire state, as a plain owned value.
///
/// This is the verified core of `crates/ids`. It has no interior mutability
/// and no lock: the state IS the value, so Verus can name it, project it and
/// reason about a transition on it. Everything F8 asserts is asserted here,
/// about the function that ships.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Counter {
    /// The value most recently returned by [`Counter::next`], or 0 before the
    /// first call.
    pub value: i64,
}

impl Counter {
    /// The counter's invariant: it never goes negative, so the first `next()`
    /// after `new()` returns 1 and every later one is larger.
    ///
    /// `open` on purpose — a caller outside this crate must be able to
    /// re-establish it, and there is nothing to hide behind.
    pub open spec fn wf(&self) -> bool {
        self.value >= 0
    }

    /// A counter that will yield 1, 2, 3, ... on successive `next()` calls.
    pub fn new() -> (c: Counter)
        ensures
            c.value == 0,
            c.wf(),
    {
        Counter { value: 0 }
    }

    /// F8. Returns the next ID: strictly greater than the previous return,
    /// and at least 1.
    ///
    /// The `requires` is the honest half of the contract. `self.value + 1`
    /// overflows at `i64::MAX`, and Verus will not let that pass unstated;
    /// the caller that has to discharge it is [`LockState::lock_increment`],
    /// which is the trusted boundary.
    pub fn next(&mut self) -> (out: i64)
        requires
            old(self).wf(),
            old(self).value < i64::MAX,
        ensures
            out == old(self).value + 1,
            self.value == out,
            self.value > old(self).value,
            out >= 1,
            self.wf(),
    {
        self.value = self.value + 1;
        self.value
    }

    /// Reads the counter without advancing it.
    pub fn get(&self) -> (out: i64)
        ensures
            out == self.value,
    {
        self.value
    }

    /// **Trusted entry (TCB).** Overwrites the counter. This is the one
    /// operation that can put the counter outside `wf()`, and it is the one
    /// `Generator::set_current` already documents as bypassing F8.
    pub fn set(&mut self, v: i64)
        ensures
            self.value == v,
    {
        self.value = v;
    }
}

} // verus!

/// **Trusted (TCB).** The lock, and nothing but the lock.
///
/// `vstd 0.0.0-2026-04-20-1748` ships no `std_specs/sync.rs`, so
/// `std::sync::Mutex` has no model and Verus cannot look inside this type.
/// It is left outside the `verus!` block for that reason, which is also why it
/// holds as little as possible: it wraps a [`Counter`] and forwards, and every
/// claim about what the counter does is discharged on `Counter` above rather
/// than assumed here.
///
/// The one obligation this shim owes and Verus does not check is
/// `Counter::next`'s `requires old(self).value < i64::MAX`. A generator that
/// actually reached `i64::MAX` would have had to be called 9.2e18 times.
#[derive(Debug)]
pub(crate) struct LockState {
    inner: Mutex<Counter>,
}

impl LockState {
    pub(crate) fn new(v: i64) -> Self {
        Self { inner: Mutex::new(Counter { value: v }) }
    }

    /// Atomic read of the protected counter.
    pub(crate) fn lock_value(&self) -> i64 {
        self.inner.lock().expect("ids mutex poisoned").get()
    }

    /// Atomic increment-by-one of the protected counter, returning the new
    /// value (i.e. the next ID). The critical section is one call to the
    /// verified `Counter::next`.
    pub(crate) fn lock_increment(&self) -> i64 {
        let mut g = self.inner.lock().expect("ids mutex poisoned");
        g.next()
    }

    /// **Trusted (TCB).** Atomic overwrite of the protected counter.
    /// Bypasses F8's strict-monotonic-from-1 invariant if abused — see
    /// `Generator::set_current`.
    pub(crate) fn lock_set(&self, v: i64) {
        self.inner.lock().expect("ids mutex poisoned").set(v);
    }
}

/// A monotonically-increasing 64-bit ID generator. The first `next()` call
/// returns 1; subsequent calls return strictly greater values.
///
/// The generator is the trusted shim: it owns a [`LockState`] and forwards.
/// The transition it forwards to, [`Counter::next`], is verified.
pub struct Generator {
    pub(crate) inner: LockState,
}

impl Generator {
    /// Returns a fresh generator that will yield 1, 2, 3, ... on successive
    /// `next()` calls.
    pub fn new() -> Self {
        Self { inner: LockState::new(0) }
    }

    /// Returns the next ID. Always strictly greater than the previous return.
    pub fn next_id(&self) -> i64 {
        self.inner.lock_increment()
    }

    /// Returns the current counter value (the value that was most recently
    /// returned by `next_id`, or 0 if `next_id` has never been called).
    ///
    /// Trusted (TCB): used by Stream 2 snapshot to capture the generator
    /// state. Does not mutate the counter.
    pub fn current(&self) -> i64 {
        self.inner.lock_value()
    }

    /// **Trusted (TCB).** Overwrites the counter to `value`. Bypasses F8's
    /// strict-monotonic-from-1 invariant if abused — use only via the
    /// snapshot/load admin path or the seed loader. Stream 2 Phase 0
    /// requires this to materialize a generator state captured on a peer.
    pub fn set_current(&self, value: i64) {
        self.inner.lock_set(value);
    }
}

impl Default for Generator {
    fn default() -> Self {
        Self::new()
    }
}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn first_id_is_one() {
        let g = Generator::new();
        assert_eq!(g.next_id(), 1);
    }

    #[test]
    fn ids_are_strictly_monotonic() {
        let g = Generator::new();
        let mut prev = 0;
        for _ in 0..100 {
            let id = g.next_id();
            assert!(id > prev, "id={id} prev={prev}");
            prev = id;
        }
    }

    #[test]
    fn ids_are_unique_under_concurrent_callers() {
        // F8: even with many threads racing, every ID is unique.
        use std::collections::HashSet;
        use std::sync::Arc;
        use std::thread;
        let g = Arc::new(Generator::new());
        let mut handles = vec![];
        for _ in 0..8 {
            let g = g.clone();
            handles.push(thread::spawn(move || {
                let mut local = vec![];
                for _ in 0..1000 {
                    local.push(g.next_id());
                }
                local
            }));
        }
        let mut all = HashSet::new();
        for h in handles {
            for id in h.join().unwrap() {
                assert!(all.insert(id), "duplicate id {id}");
            }
        }
        assert_eq!(all.len(), 8 * 1000);
    }

    #[test]
    fn separate_generators_are_independent() {
        let a = Generator::new();
        let b = Generator::new();
        assert_eq!(a.next_id(), 1);
        assert_eq!(b.next_id(), 1);
        assert_eq!(a.next_id(), 2);
        assert_eq!(b.next_id(), 2);
    }

    #[test]
    fn default_is_new() {
        let g = Generator::default();
        assert_eq!(g.next_id(), 1);
    }

    #[test]
    fn current_starts_at_zero() {
        let g = Generator::new();
        assert_eq!(g.current(), 0);
    }

    #[test]
    fn current_tracks_next_id() {
        let g = Generator::new();
        g.next_id();
        g.next_id();
        g.next_id();
        assert_eq!(g.current(), 3);
    }

    #[test]
    fn set_current_overrides_counter() {
        // Trusted: bypasses F8 strict-monotonic-from-1.
        let g = Generator::new();
        g.set_current(42);
        assert_eq!(g.current(), 42);
        assert_eq!(g.next_id(), 43);
    }

    #[test]
    fn lock_state_value_matches_current() {
        // Sanity: the ghost view's production realization (lock_value)
        // returns exactly what `current()` returns. This is the
        // property Phase 2b will discharge through the vstd lock
        // primitive.
        let g = Generator::new();
        assert_eq!(g.inner.lock_value(), g.current());
        g.next_id();
        assert_eq!(g.inner.lock_value(), g.current());
        assert_eq!(g.inner.lock_value(), 1);
    }
}

#[cfg(test)]
mod counter_tests {
    use super::*;

    #[test]
    fn counter_starts_at_zero() {
        let c = Counter::new();
        assert_eq!(c.get(), 0);
    }

    #[test]
    fn counter_next_is_strictly_increasing_from_one() {
        // The runtime witness for the clauses Verus discharges on
        // `Counter::next`: out == old.value + 1, out >= 1, monotone.
        let mut c = Counter::new();
        let mut prev = 0;
        for _ in 0..100 {
            let out = c.next();
            assert_eq!(out, prev + 1);
            assert!(out > prev);
            assert!(out >= 1);
            assert_eq!(c.get(), out);
            prev = out;
        }
    }

    #[test]
    fn counter_set_overwrites() {
        let mut c = Counter::new();
        c.set(42);
        assert_eq!(c.get(), 42);
        assert_eq!(c.next(), 43);
    }
}
