package twitterport.verification

import twitterport.dom.parseInt64
import twitterport.store.Store

/**
 * Known-bad obligations. Every one of these MUST be refuted.
 *
 * GOAL.md standing rule 2: no gate is trusted until it has been shown to fail. A bounded model
 * checker that answers SUCCESS for everything is worth exactly nothing, and there are four
 * distinct ways to get a vacuous SUCCESS out of JBMC on Kotlin bytecode:
 *
 *  1. an entry point whose name does not resolve -- JBMC then reports VERIFICATION SUCCESSFUL for
 *     having checked nothing;
 *  2. an assertion the compiler dropped;
 *  3. a path guarded off by an early `return`;
 *  4. **a check JBMC could not discharge earlier on the same path.** CBMC assumes a failed check
 *     held for the remainder of the trace, so one undischargeable goal makes every later
 *     assertion infeasible and therefore vacuously true.
 *
 * The fourth is not hypothetical. `Store.appendTweet` originally used `log.lastOrNull()`, whose
 * erased checkcast JBMC cannot discharge; all six store obligations came back VERIFIED and the run
 * read as a proof. The canaries below are what showed otherwise.
 *
 * Each is either the negation of a real obligation or an `assert(false)` immediately after a call
 * into a layer, so a canary that is NOT refuted names exactly which obligation is vacuous. The
 * driver in `main.go` treats that as a hard failure of the run -- unless the obligation it guards
 * is already reported BLOCKED, in which case nothing is being claimed and nothing is at risk.
 */
object Canaries {

    /** Negation of O1a: claims a non-digit is a legal integer. */
    @JvmStatic
    fun c1_bareSignIsANumber() {
        assert(parseInt64("+") != null)
    }

    /** Negation of O3a: claims ids do not increase across appends. */
    @JvmStatic
    fun c2_idsDoNotIncrease() {
        val s = Store()
        val t1 = s.appendTweet("a", "x")
        val t2 = s.appendTweet("a", "y")
        assert(t1.id >= t2.id)
    }

    /** Negation of O3b: claims a tick can push createdAt backwards. */
    @JvmStatic
    fun c3_clockCanDecrease() {
        val s = Store()
        val t1 = s.appendTweet("a", "x")
        s.tick()
        val t2 = s.appendTweet("a", "y")
        assert(t1.createdAt > t2.createdAt)
    }

    /** Negation of O4a: claims a page may exceed the requested limit. */
    @JvmStatic
    fun c4_pageMayExceedLimit() {
        val s = Store()
        s.createUser("a")
        s.appendTweet("a", "1")
        s.appendTweet("a", "2")
        s.appendTweet("a", "3")
        val p = s.timelinePage("a", 1, null)
        assert(p.tweets.size > 1)
    }

    /** Negation of O4c: claims the timeline is oldest-first, the F2 ordering defect. */
    @JvmStatic
    fun c5_timelineIsOldestFirst() {
        val s = Store()
        s.createUser("a")
        val t1 = s.appendTweet("a", "1")
        s.appendTweet("a", "2")
        val p = s.timelinePage("a", 50, null)
        assert(p.tweets[0].id == t1.id)
    }

    // --- reachability canaries -----------------------------------------------
    //
    // The three below assert `false` immediately after calling into a layer. Each MUST be
    // refuted, because `false` is refutable on any reachable path. One reported SUCCESS means
    // JBMC could not get past the preceding call at all, so every obligation over that layer is
    // vacuously true and its VERIFIED verdict means nothing.
    //
    // This instrument exists because that is exactly what happened: `Store.appendTweet` used
    // `log.lastOrNull()`, whose erased checkcast JBMC cannot discharge, and CBMC assumes a failed
    // check held for the rest of the path -- making everything after it infeasible. All six store
    // obligations reported VERIFIED and the run looked like a proof. Only the negation canaries
    // showed otherwise. A reachability canary says the same thing one layer earlier and names the
    // layer.

    // Each holds exactly ONE assertion, and it is `false`. A second assertion would muddy the
    // signal: if it failed for its own reasons the canary would be recorded as refuted without
    // the layer being reachable at all.

    /** The dom layer is reachable: a syntax predicate returns and execution continues. */
    @JvmStatic
    fun c6_domIsReachable() {
        sink = twitterport.dom.validHandle("alice")
        assert(false)
    }

    /** The store layer is reachable past an append. */
    @JvmStatic
    fun c7_storeIsReachable() {
        sink = Store().appendTweet("a", "x").id == 1L
        assert(false)
    }

    /** The service layer is reachable past a validation-ordered call. */
    @JvmStatic
    fun c8_serviceIsReachable() {
        sink = twitterport.service.Service().follow("eve", "eve") is twitterport.service.Outcome.Err
        assert(false)
    }

    /**
     * The negation of O5c. O5c claims `follow("EVE","eve")` is `invalid_handle` (syntax before
     * existence, D6); this claims it is `unknown_user`, the other answer `twitter.tla` permits.
     *
     * Exactly one of the two can hold. If JBMC reports SUCCESS for BOTH, it has proved a
     * contradiction, which means neither assertion is reachable and O5c's VERIFIED is vacuous.
     * A layer-level reachability canary cannot show that: it establishes only that *something*
     * after a service call is reachable, not that this particular obligation's assertion is.
     */
    @JvmStatic
    fun c9_syntaxDoesNotBeatExistence() {
        val r = twitterport.service.Service().follow("EVE", "eve")
        assert(r is twitterport.service.Outcome.Err && r.code.compareTo("unknown_user") == 0)
    }

    /** Keeps the calls above live; a discarded expression could in principle be elided. */
    @JvmStatic
    var sink: Boolean = false
}
