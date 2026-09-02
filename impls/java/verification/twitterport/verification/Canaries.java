package twitterport.verification;

import twitterport.dom.Dom;
import twitterport.dom.Page;
import twitterport.dom.Tweet;
import twitterport.dom.User;
import twitterport.service.Result;
import twitterport.service.Service;
import twitterport.store.Store;

/**
 * Known-bad obligations. Every one of these MUST be refuted.
 *
 * <p>GOAL.md standing rule 2: no gate is trusted until it has been shown to fail. A bounded model
 * checker that answers SUCCESS for everything is worth exactly nothing, and there are four distinct
 * ways to get a vacuous SUCCESS out of JBMC on JVM bytecode:
 *
 * <ol>
 *   <li>an entry point whose name does not resolve -- JBMC then reports VERIFICATION SUCCESSFUL for
 *       having checked nothing;
 *   <li>an assertion the compiler dropped;
 *   <li>a path guarded off by an early {@code return};
 *   <li><b>a check JBMC could not discharge earlier on the same path.</b> CBMC assumes a failed
 *       check held for the remainder of the trace, so one undischargeable goal makes every later
 *       assertion infeasible and therefore vacuously true.
 * </ol>
 *
 * <p>The fourth is not hypothetical and it is not hypothetical <em>here</em>. F013 records six
 * Kotlin store obligations that reported VERIFIED because {@code log.lastOrNull()} emitted an
 * erased checkcast JBMC could not discharge. The fix was to write what "the Java corner already
 * does" -- {@code log.get(log.size() - 1)}. That sentence is why this corner needs its own
 * canaries rather than inheriting the Kotlin corner's conclusions: the Java corner was the
 * <em>reference</em> for the fix and has never itself been checked, and it carries two erasure
 * sites of its own that the Kotlin twin does not present in the same place --
 * {@code Result<T>.value()} and {@code List<Tweet>.get(int)}.
 *
 * <p><b>Every obligation in {@link Obligations} is named by at least one canary below -- all
 * fifteen.</b> That is F025's rule applied at the start rather than after the fact: an audit
 * indexed by the checks you wrote cannot find the check you did not write, so the index here is the
 * claim, not the canary. The Kotlin corner shipped nine canaries covering four of its seven
 * decidable obligations and nothing in the run said so for two months.
 *
 * <p>Each canary is either the negation of a real obligation or an {@code assert false} immediately
 * after a call into a layer, so a canary that is NOT refuted names exactly which obligation is
 * vacuous. {@code tools/cmd/jbmc} demotes that obligation to VACUOUS and the whole run to
 * UNDECIDED -- no verdict at all, which {@code calibrate} records as an error cell rather than as a
 * survival.
 */
public final class Canaries {

    private Canaries() {}

    /** Negation of O1c: claims a bare sign is a legal integer. */
    public static void c1_bareSignIsANumber() {
        assert Dom.parseInt64("+") != null;
    }

    /** Negation of O3a: claims ids do not increase across appends. */
    public static void c2_idsDoNotIncrease() {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "x");
        Tweet t2 = s.appendTweet("a", "y");
        assert t1.id() >= t2.id();
    }

    /** Negation of O3b: claims a tick can push createdAt backwards. */
    public static void c3_clockCanDecrease() {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "x");
        s.tick();
        Tweet t2 = s.appendTweet("a", "y");
        assert t1.createdAt() > t2.createdAt();
    }

    // c4, c5 and c13 do not register the author either, for the reason the GROUP 4 header in
    // Obligations.java gives: the visibility test never consults the user registry, and a mention
    // the property does not need is a coupling to a signature a mutant may change (F035).

    /** Negation of O4a: claims a page may exceed the requested limit. */
    public static void c4_pageMayExceedLimit() {
        Store s = new Store();
        s.appendTweet("a", "1");
        s.appendTweet("a", "2");
        s.appendTweet("a", "3");
        Page p = s.timelinePage("a", 1, null);
        assert p.tweets().size() > 1;
    }

    /** Negation of O4c: claims the timeline is oldest-first, the F2 ordering defect. */
    public static void c5_timelineIsOldestFirst() {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "1");
        s.appendTweet("a", "2");
        Page p = s.timelinePage("a", 50, null);
        assert p.tweets().get(0).id() == t1.id();
    }

    // --- reachability canaries -----------------------------------------------
    //
    // The three below assert `false` immediately after calling into a layer. Each MUST be refuted,
    // because `false` is refutable on any reachable path. One reported SUCCESS means JBMC could not
    // get past the preceding call at all, so every obligation over that layer is vacuously true and
    // its VERIFIED verdict means nothing.
    //
    // Each holds exactly ONE assertion, and it is `false`. A second assertion would muddy the
    // signal: if it failed for its own reasons the canary would be recorded as refuted without the
    // layer being reachable at all.

    /** The dom layer is reachable: a syntax predicate returns and execution continues. */
    public static void c6_domIsReachable() {
        sink = Dom.validHandle("alice");
        assert false;
    }

    /** The store layer is reachable past an append. */
    public static void c7_storeIsReachable() {
        sink = new Store().appendTweet("a", "x").id() == 1L;
        assert false;
    }

    /** The service layer is reachable past a validation-ordered call. */
    public static void c8_serviceIsReachable() {
        sink = new Service().follow("eve", "eve").isErr();
        assert false;
    }

    /**
     * The negation of O5c. O5c claims {@code follow("EVE","eve")} is {@code invalid_handle} (syntax
     * before existence, D6); this claims it is {@code unknown_user}, the other answer
     * {@code twitter.tla} permits.
     *
     * <p>Exactly one of the two can hold. If JBMC reports SUCCESS for BOTH, it has proved a
     * contradiction, which means neither assertion is reachable and O5c's VERIFIED is vacuous. A
     * layer-level reachability canary cannot show that: it establishes only that <em>something</em>
     * after a service call is reachable, not that this particular obligation's assertion is.
     */
    public static void c9_syntaxDoesNotBeatExistence() {
        Result<Void> r = new Service().follow("EVE", "eve");
        assert r.isErr() && r.error().compareTo(Dom.ERR_UNKNOWN_USER) == 0;
    }

    /**
     * Negation of O1a over a specific non-digit. O1a quantifies over ALL one-character strings;
     * this fixes one that must be rejected, so a refutation here witnesses that the accept-set
     * assertion is reachable with a concrete counterexample rather than only over nondeterministic
     * input.
     */
    public static void c10_nonDigitIsANumber() {
        assert Dom.parseInt64("x") != null;
    }

    /** Negation of O1b: claims a sign followed by a sign is a legal two-character integer. */
    public static void c11_signThenSignIsANumber() {
        assert Dom.parseInt64("+-") != null;
    }

    /**
     * Negation of O3c at the THIRD append. O3c is the monotonicity lemma over a three-entry log,
     * and c2 only reaches the second: a checkcast JBMC cannot discharge at the third append would
     * leave O3c vacuous while c2 stayed refutable.
     */
    public static void c12_thirdAppendDoesNotIncrease() {
        Store s = new Store();
        s.appendTweet("a", "x");
        Tweet t2 = s.appendTweet("a", "y");
        Tweet t3 = s.appendTweet("a", "z");
        assert t2.id() >= t3.id();
    }

    // --- canaries the Kotlin twin never had ----------------------------------
    //
    // The five below name the five obligations the Kotlin corner leaves unguarded because they are
    // BLOCKED there. Writing them costs nothing and is what F025's rule actually asks for: the
    // index is the claim, so a claim is guarded before anyone knows whether the tool can decide it.
    // If any of these five turns out DECIDABLE on Java bytecode where it is blocked on Kotlin's --
    // which is the whole reason a second JVM corner is worth running -- the guard is already there
    // and the obligation can be claimed the same day it is measured, rather than being claimed
    // unaudited the way F025 records.

    /** Negation of O4b: claims a full page emits a cursor when nothing remains below it. */
    public static void c13_cursorEmittedWhenExhausted() {
        Store s = new Store();
        s.appendTweet("a", "1");
        s.appendTweet("a", "2");
        Page all = s.timelinePage("a", 50, null);
        assert all.nextCursor() != null;
    }

    /** Negation of O2b: claims a plainly legal handle is invalid. */
    public static void c14_goodHandleIsInvalid() {
        assert !Dom.validHandle("alice");
    }

    /**
     * Negation of O2a: claims the empty text is valid. This is the exact answer the getBytes stub
     * produces, so a SUCCESS here is not merely a vacuity signal -- it is the F014 defect itself,
     * visible as a refutation that does not arrive.
     */
    public static void c15_emptyTextIsValid() {
        assert Dom.validText("");
    }

    /** Negation of O5a: claims an unknown self-follow is self_follow_forbidden (the D4 flip). */
    public static void c16_selfFollowBeatsUnknown() {
        Result<Void> r = new Service().follow("eve", "eve");
        assert r.isErr() && r.error().compareTo(Dom.ERR_SELF_FOLLOW) == 0;
    }

    /** Negation of O5b: claims a known self-follow is accepted. */
    public static void c17_knownSelfFollowIsAllowed() {
        Service svc = new Service();
        svc.createUser("alice");
        Result<Void> r = svc.follow("alice", "alice");
        assert !r.isErr();
    }

    /**
     * Negation of O5d: claims a rejected registration burns an id, so the second accepted handle
     * gets id 3 rather than 2. This is also the canary for the {@code Result<T>.value()} erasure:
     * the call site casts an erased {@code Object} to {@code User}, and an undischargeable
     * checkcast there would leave O5d and this canary both verifying.
     */
    public static void c18_rejectionBurnsAnId() {
        Service svc = new Service();
        svc.createUser("a");
        svc.createUser("a");
        svc.createUser("!");
        Result<User> r = svc.createUser("b");
        assert !r.isErr() && r.value().id() == 3L;
    }

    /** Keeps the calls above live; a discarded expression could in principle be elided. */
    public static boolean sink = false;
}
