//! Logical clock for the verified core.
//!
//! # F-properties
//! - **F7**: timestamps are monotonically *non-decreasing*. Two consecutive
//!   `now()` calls with no intervening `tick()` must return the same value.
//!   `tick()` advances by exactly 1. Ties are explicitly allowed and are the
//!   reason F2's tweet-id tiebreak exists.
//!
//! # Verus annotations — the state is lifted OUT of the lock
//!
//! Until 2026-09-02 F7 was discharged on `verus_proof::now_ensures` and
//! `verus_proof::tick_ensures`: hand-written twins inside a
//! `#[cfg(verus_only)]` module, whose bodies called two `external_body`
//! shims (`proof_lock_value`, `proof_lock_increment`) that stood in for
//! `std::sync::Mutex::lock`, chained through an `external_body` ghost view
//! `lock_state_value`. Three assumed hooks, and nothing mechanically tying
//! the twins to `Logical::now` / `Logical::tick`, the functions that ship.
//!
//! The blocker was never F7. It was that the clock's state lived behind
//! `std::sync::Mutex<i64>`, and `vstd 0.0.0-2026-04-20-1748` ships no
//! `std_specs/sync.rs` — no model of `std::sync::Mutex` or
//! `std::sync::RwLock`. A `spec fn` cannot project through a lock the
//! verifier cannot see, so the projection had to be assumed.
//!
//! The repair is a refactor, not an annotation. The timestamp is now a plain
//! owned value type, [`Ts`], defined inside a top-level `verus! { .. }` block
//! with `&mut self` methods carrying `ensures` clauses that Verus discharges
//! **against their own bodies**. The lock is pushed out to [`LockState`], a
//! thin trusted boundary holding `Mutex<Ts>` — the same verified-core /
//! trusted-shim split `internal/httpshim` has on the Go corner.
//!
//! Discharged contract, on the shipped function (`Ts::tick`):
//!
//! ```text
//! requires
//!     old(self).wf(),
//!     old(self).value < i64::MAX,
//! ensures
//!     self.value == old(self).value + 1,
//!     self.value > old(self).value,
//!     self.wf(),
//! ```
//!
//! `Ts::get` carries `out == self.value`, which is the other half of F7:
//! two `now()` calls with no intervening `tick()` return the same value
//! because `get` does not write.
//!
//! Verus is **not** required to compile this crate: the `verus!` macro erases
//! its ghost annotations under stable rustc, exactly as `crates/domain` has
//! always done. What remains trusted is the lock, and only the lock:
//! `LockState` and `Logical` sit OUTSIDE the `verus!` block, which is how
//! Verus is told not to look at them. `TCB.md` carries the row.

use std::sync::Mutex;

use vstd::prelude::*;

verus! {

/// The clock's entire state, as a plain owned value.
///
/// This is the verified core of `crates/clock`. It has no interior mutability
/// and no lock: the state IS the value, so Verus can name it, project it and
/// reason about a transition on it. Everything F7 asserts is asserted here,
/// about the functions that ship.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Ts {
    /// The current logical timestamp.
    pub value: i64,
}

impl Ts {
    /// The clock's invariant: the logical timestamp never goes negative.
    ///
    /// `open` on purpose — a caller outside this crate must be able to
    /// re-establish it, and there is nothing to hide behind.
    pub open spec fn wf(&self) -> bool {
        self.value >= 0
    }

    /// A timestamp holding `v`.
    ///
    /// No `requires`: `Logical::new_at` accepts any `i64` and this function
    /// must not claim otherwise. `wf()` is the caller's to establish.
    pub fn new(v: i64) -> (t: Ts)
        ensures
            t.value == v,
    {
        Ts { value: v }
    }

    /// F7, read half. Returns the timestamp without advancing it: two
    /// consecutive calls with no intervening `tick` return the same value
    /// because this function does not write.
    pub fn get(&self) -> (out: i64)
        ensures
            out == self.value,
    {
        self.value
    }

    /// F7, advance half. Advances by exactly 1.
    ///
    /// The `requires` is the honest half of the contract. `self.value + 1`
    /// overflows at `i64::MAX`, and Verus will not let that pass unstated;
    /// the caller that has to discharge it is [`LockState::lock_increment`],
    /// which is the trusted boundary.
    pub fn tick(&mut self)
        requires
            old(self).wf(),
            old(self).value < i64::MAX,
        ensures
            self.value == old(self).value + 1,
            self.value > old(self).value,
            self.wf(),
    {
        self.value = self.value + 1;
    }

    /// **Trusted entry (TCB).** Overwrites the timestamp. This is the one
    /// operation that can move the clock backwards, and it is the one
    /// `Logical::set_now` already documents as bypassing F7.
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
/// It is left outside the `verus!` block for that reason, which is also why
/// it holds as little as possible: it wraps a [`Ts`] and forwards, and every
/// claim about what the clock does is discharged on `Ts` above rather than
/// assumed here.
///
/// The one obligation this shim owes and Verus does not check is `Ts::tick`'s
/// `requires old(self).value < i64::MAX`.
#[derive(Debug)]
pub(crate) struct LockState {
    inner: Mutex<Ts>,
}

impl LockState {
    pub(crate) fn new(v: i64) -> Self {
        Self { inner: Mutex::new(Ts::new(v)) }
    }

    /// Atomic read of the protected timestamp.
    pub(crate) fn lock_value(&self) -> i64 {
        self.inner.lock().expect("clock mutex poisoned").get()
    }

    /// Atomic advance-by-one of the protected timestamp. The critical
    /// section is one call to the verified `Ts::tick`.
    pub(crate) fn lock_increment(&self) {
        self.inner.lock().expect("clock mutex poisoned").tick();
    }

    /// Atomic set of the protected timestamp. **Trusted (TCB).** Used by
    /// the Stream 2 snapshot-load admin path; bypasses F7 if `value`
    /// goes backwards.
    pub(crate) fn lock_set(&self, value: i64) {
        self.inner.lock().expect("clock mutex poisoned").set(value);
    }
}

/// Trait abstracting the logical clock so callers (service) can be tested
/// against a deterministic stub.
pub trait Clock: Send + Sync {
    /// Returns the current logical timestamp without advancing.
    fn now(&self) -> i64;
    /// Advances the clock by 1 tick.
    fn tick(&self);
    /// Forces the clock to read `value` on the next `now()` call.
    ///
    /// Default impl: if `value > now()`, calls `tick()` `(value - now())`
    /// times. If `value <= now()`, this is a no-op (F7 forbids going
    /// backwards). Slow but correct: a stub clock built on top of `tick`
    /// requires nothing more.
    ///
    /// Concrete impls (`Logical`) may override this with a single-mutex-op
    /// implementation; that override is **trusted** because it can violate
    /// F7's monotonicity if `value < now()` is requested.
    fn set_now(&self, value: i64) {
        let mut cur = self.now();
        while cur < value {
            self.tick();
            cur += 1;
        }
    }
}

/// `Logical` is the single concrete clock used by the verified core. It
/// starts at zero and only advances when `tick()` is called explicitly,
/// which makes timeline timestamps fully reproducible from the conformance
/// suite.
///
/// `Logical` is the trusted shim: it owns a [`LockState`] and forwards. The
/// transitions it forwards to, [`Ts::get`] and [`Ts::tick`], are verified.
pub struct Logical {
    pub(crate) inner: LockState,
}

impl Logical {
    /// Returns a fresh clock at t=0.
    pub fn new() -> Self {
        Self { inner: LockState::new(0) }
    }

    /// Returns a fresh clock at t=`start`.
    pub fn new_at(start: i64) -> Self {
        Self { inner: LockState::new(start) }
    }
}

impl Default for Logical {
    fn default() -> Self {
        Self::new()
    }
}

impl Clock for Logical {
    fn now(&self) -> i64 {
        self.inner.lock_value()
    }

    fn tick(&self) {
        self.inner.lock_increment();
    }

    /// **Trusted (TCB).** Single-mutex-op override of the trait default.
    /// Bypasses F7's monotonic-non-decreasing invariant if `value < now()`
    /// is requested — used only by the Stream 2 snapshot-load admin path,
    /// where the producer's snapshot is the source of truth.
    fn set_now(&self, value: i64) {
        self.inner.lock_set(value);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn starts_at_zero() {
        let c = Logical::new();
        assert_eq!(c.now(), 0);
    }

    #[test]
    fn new_at_starts_where_asked() {
        let c = Logical::new_at(42);
        assert_eq!(c.now(), 42);
    }

    #[test]
    fn now_does_not_advance() {
        let c = Logical::new();
        assert_eq!(c.now(), 0);
        assert_eq!(c.now(), 0);
        assert_eq!(c.now(), 0);
    }

    #[test]
    fn tick_advances_by_one() {
        let c = Logical::new();
        c.tick();
        assert_eq!(c.now(), 1);
        c.tick();
        assert_eq!(c.now(), 2);
    }

    #[test]
    fn non_decreasing_under_concurrent_ticks() {
        // F7: even with many threads ticking, the clock never goes backwards.
        use std::sync::Arc;
        use std::thread;
        let c = Arc::new(Logical::new());
        let mut handles = vec![];
        for _ in 0..8 {
            let c = c.clone();
            handles.push(thread::spawn(move || {
                for _ in 0..1000 {
                    c.tick();
                }
            }));
        }
        for h in handles {
            h.join().unwrap();
        }
        assert_eq!(c.now(), 8 * 1000);
    }

    #[test]
    fn default_is_new() {
        let c = Logical::default();
        assert_eq!(c.now(), 0);
    }

    #[test]
    fn trait_object_works() {
        let c: Box<dyn Clock> = Box::new(Logical::new());
        assert_eq!(c.now(), 0);
        c.tick();
        assert_eq!(c.now(), 1);
    }

    #[test]
    fn lock_state_value_matches_now() {
        // Sanity: the ghost view's production realization (lock_value)
        // returns exactly what `now()` returns. This is the property
        // Phase 1b will discharge through the vstd lock primitive.
        let c = Logical::new_at(7);
        assert_eq!(c.inner.lock_value(), c.now());
        c.tick();
        assert_eq!(c.inner.lock_value(), c.now());
        assert_eq!(c.inner.lock_value(), 8);
    }

    #[test]
    fn logical_set_now_one_op() {
        // Trusted override: jumps directly.
        let c = Logical::new();
        c.set_now(1000);
        assert_eq!(c.now(), 1000);
        // Trusted: can also rewind on Logical (the snapshot-load case).
        c.set_now(50);
        assert_eq!(c.now(), 50);
    }

    /// Stub clock that delegates to the trait's default `set_now` impl.
    /// Verifies the slow-but-correct fallback works for any `Clock`.
    struct CountingTickClock {
        inner: Mutex<(i64, usize)>,
    }
    impl Clock for CountingTickClock {
        fn now(&self) -> i64 {
            self.inner.lock().unwrap().0
        }
        fn tick(&self) {
            let mut g = self.inner.lock().unwrap();
            g.0 += 1;
            g.1 += 1;
        }
        // intentionally do NOT override set_now
    }

    #[test]
    fn default_set_now_uses_tick_repeatedly() {
        let c = CountingTickClock { inner: Mutex::new((0, 0)) };
        c.set_now(5);
        assert_eq!(c.now(), 5);
        assert_eq!(c.inner.lock().unwrap().1, 5);
    }

    #[test]
    fn default_set_now_is_no_op_when_value_lte_now() {
        let c = CountingTickClock { inner: Mutex::new((0, 0)) };
        c.tick();
        c.tick();
        let pre_ticks = c.inner.lock().unwrap().1;
        c.set_now(1); // value < now
        assert_eq!(c.now(), 2);
        assert_eq!(c.inner.lock().unwrap().1, pre_ticks);
        c.set_now(2); // value == now
        assert_eq!(c.now(), 2);
        assert_eq!(c.inner.lock().unwrap().1, pre_ticks);
    }
}

#[cfg(test)]
mod ts_tests {
    use super::*;

    #[test]
    fn ts_new_holds_value() {
        assert_eq!(Ts::new(7).get(), 7);
    }

    #[test]
    fn ts_tick_advances_by_exactly_one() {
        // The runtime witness for the clauses Verus discharges on `Ts::tick`.
        let mut t = Ts::new(0);
        for expected in 1..=100 {
            let before = t.get();
            t.tick();
            assert_eq!(t.get(), expected);
            assert_eq!(t.get(), before + 1);
            assert!(t.get() > before);
        }
    }

    #[test]
    fn ts_get_does_not_advance() {
        // F7's read half: no intervening tick, same value.
        let t = Ts::new(3);
        assert_eq!(t.get(), 3);
        assert_eq!(t.get(), 3);
    }

    #[test]
    fn ts_set_overwrites_and_may_rewind() {
        // Trusted entry: this is the one operation that can go backwards.
        let mut t = Ts::new(10);
        t.set(4);
        assert_eq!(t.get(), 4);
    }
}
