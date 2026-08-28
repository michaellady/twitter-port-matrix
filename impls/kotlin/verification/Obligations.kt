package twitterport.verification

import twitterport.dom.parseInt64
import twitterport.dom.validHandle
import twitterport.dom.validText
import twitterport.service.Outcome
import twitterport.service.Service
import twitterport.store.Store

/**
 * The Kotlin corner's verification obligations, written as JBMC entry points.
 *
 * **This is not a test suite.** Each function below is a bounded proof obligation: JBMC treats its
 * parameters as nondeterministic, explores every path within the unwinding bound, and reports each
 * assertion as SUCCESS (no counterexample exists within the bound) or FAILURE (here is one).
 * Nothing here runs during a normal build -- the registry compiles `impls/kotlin/src`, and this
 * directory is compiled and driven separately by `verification/main.go` (`go run .`).
 *
 * **Why this file exists.** ASSURANCE.md predicts the Kotlin corner tops out at "R3 + bounded"
 * because Kotlin has no mature deductive verifier -- no Verus, no Gobra, no OpenJML. The prediction
 * is worth testing rather than repeating, and the only way to test it is to write the obligations
 * down and find out which a tool can discharge. The ones that turn out to be *unreachable* are as
 * much of a result as the ones that pass, so they are kept here and labelled, not deleted.
 *
 * ## Three JBMC limits shape everything below, and none of them is Kotlin's fault
 *
 * 1. **`String.equals` is unsound in JBMC 6.11.** `assert "abc".equals("abc")` is reported
 *    FAILURE. So is `a.equals(a)` on a single reference. `compareTo`, `startsWith`, `isEmpty`,
 *    `length` and `charAt` are all fine, and `instanceof String` is fine, so the defect is
 *    localised to the `org.cprover.CProverString.equals` intrinsic that the model's `equals`
 *    delegates to. Every assertion below therefore compares strings with `compareTo(x) == 0`.
 *    What it cannot route around is the implementation's own use of string equality: the
 *    visibility test `t.author != user` and the `HashSet<Edge>` membership test both reduce to
 *    it, which is why every timeline obligation with content in the log is BLOCKED while the same
 *    obligation over an empty log verifies with 0 of 964 goals failing.
 *
 * 2. **`String.getBytes(Charset)` is nondeterministic in JBMC 6.11.** The model dispatches on
 *    `Charset.name()` compared with that same broken intrinsic, so it falls through to an opaque
 *    stub returning an array of unconstrained length. `Dom.validHandle` and `Dom.validText`
 *    measure UTF-8 **bytes** -- which is what makes them byte-exact against a Go reference machine
 *    where `len(s)` is a byte count -- so both predicates, and every service path that begins with
 *    one, are outside what this checker can say anything about. That is recorded in the O2/O5
 *    group rather than engineered around: rewriting `validHandle` as a char loop would make it
 *    checkable, and would be a change made to please a tool.
 *
 * 3. **The SAT instance does not scale.** A nondeterministic `limit` over a four-entry log
 *    exhausts memory ("SAT checker ran out of memory"). An ordinary bounded-model-checking wall,
 *    and the reason O4a and O5d are BLOCKED for a different reason than O4b and O4c.
 *
 * Limits 1 and 2 were each reduced to a two-line repro and both reproduce **identically in plain
 * Java** compiled by javac. They are properties of JBMC, not costs Kotlin imposes -- which
 * matters, because the whole question this corner exists to answer is what the language costs.
 *
 * Entry points are `@JvmStatic` members of an `object` so they compile to plain static methods.
 * An ordinary `object` member would work too, but JBMC would have to synthesise the `INSTANCE`
 * receiver first.
 */
object Obligations {

    // ======================================================================
    // GROUP 1 -- dom.parseInt64: the integer accept set (D10). REACHABLE.
    // ======================================================================
    //
    // The property that motivated hand-writing the parser instead of calling
    // `String.toLongOrNull()`: Kotlin's own conversion resolves digits through
    // `Character.digit`, which accepts non-ASCII decimal digits, so `?limit=١` would be a limit
    // of 1 in Kotlin and an `invalid_limit` in S_obs. These obligations pin the accept set over
    // every string of the given length, not over a list of examples.
    //
    // The parameter is a String rather than a Char that the body converts, because
    // `String.valueOf(char)` is one of the opaque methods: constructing a string inside the
    // obligation makes its contents nondeterministic and the obligation vacuous. JBMC synthesises
    // nondeterministic strings for String parameters directly, bounded by --max-nondet-string-length.

    /** O1a: over ALL one-character strings, parseInt64 accepts exactly the ASCII digits. */
    @JvmStatic
    fun o1a_oneCharAcceptSet(s: String) {
        if (s.length != 1) return
        val c = s[0]
        assert((parseInt64(s) != null) == (c in '0'..'9'))
    }

    /** O1b: over ALL two-character strings, parseInt64 accepts exactly `[+-0-9][0-9]`. */
    @JvmStatic
    fun o1b_twoCharAcceptSet(s: String) {
        if (s.length != 2) return
        val a = s[0]
        val b = s[1]
        val ok = (a in '0'..'9' || a == '+' || a == '-') && b in '0'..'9'
        assert((parseInt64(s) != null) == ok)
    }

    /** O1c: the empty string and a bare sign are not numbers. */
    @JvmStatic
    fun o1c_emptyAndBareSignRejected() {
        assert(parseInt64("") == null)
        assert(parseInt64("+") == null)
        assert(parseInt64("-") == null)
    }

    // ======================================================================
    // GROUP 2 -- dom.validHandle / dom.validText (D6). BLOCKED: getBytes.
    // ======================================================================
    //
    // Kept, and expected to be reported BLOCKED. Their failure is not a defect in the predicates;
    // it is JBMC returning an unconstrained byte array from `String.getBytes(Charset)`, which lets
    // it claim `validText("")` is true. The obligations are the record of which part of the
    // contract this rung cannot see.

    /** O2a: the empty handle and the empty text are invalid. Corpus steps. */
    @JvmStatic
    fun o2a_emptyIsInvalid() {
        assert(!validHandle(""))
        assert(!validText(""))
    }

    /** O2b: a plainly legal handle is valid. */
    @JvmStatic
    fun o2b_goodHandleIsValid() {
        assert(validHandle("alice"))
    }

    // ======================================================================
    // GROUP 3 -- store.appendTweet: the enforced premise of the F2 lemma. REACHABLE.
    // ======================================================================
    //
    // THE obligation of this corner. D9 derives timeline ordering from an insertion-ordered log
    // instead of proving a sort correct, and finding F005 records that the derivation is sound
    // only if the log's two premises are enforced at the mutation site rather than assumed. This
    // asks a checker to confirm the enforcement instead of the comment.

    /** O3a: two consecutive appends yield strictly increasing ids. */
    @JvmStatic
    fun o3a_idsStrictlyIncrease() {
        val s = Store()
        val t1 = s.appendTweet("a", "x")
        val t2 = s.appendTweet("a", "y")
        assert(t1.id < t2.id)
    }

    /**
     * O3b: `createdAt` is non-decreasing across appends whether or not the clock advanced between
     * them, and ids still increase. Both premises of the monotonicity lemma, over both branches.
     */
    @JvmStatic
    fun o3b_createdAtNonDecreasing(tickFirst: Boolean) {
        val s = Store()
        val t1 = s.appendTweet("a", "x")
        if (tickFirst) {
            s.tick()
        }
        val t2 = s.appendTweet("a", "y")
        assert(t1.createdAt <= t2.createdAt)
        assert(t1.id < t2.id)
    }

    /** O3c: three appends, arbitrary tick pattern -- the lemma over a longer log. */
    @JvmStatic
    fun o3c_lemmaOverThreeAppends(tick1: Boolean, tick2: Boolean) {
        val s = Store()
        val t1 = s.appendTweet("a", "x")
        if (tick1) s.tick()
        val t2 = s.appendTweet("a", "y")
        if (tick2) s.tick()
        val t3 = s.appendTweet("a", "z")
        assert(t1.id < t2.id && t2.id < t3.id)
        assert(t1.createdAt <= t2.createdAt && t2.createdAt <= t3.createdAt)
    }

    // ======================================================================
    // GROUP 4 -- store.timelinePage: pagination arithmetic (D10). REACHABLE.
    // ======================================================================

    /** O4a: over every legal limit, a page never exceeds it. */
    @JvmStatic
    fun o4a_pageRespectsLimit(limit: Int) {
        if (limit < 1 || limit > 3) return
        val s = Store()
        s.createUser("a")
        s.appendTweet("a", "1")
        s.appendTweet("a", "2")
        s.appendTweet("a", "3")
        s.appendTweet("a", "4")
        val p = s.timelinePage("a", limit.toLong(), null)
        assert(p.tweets.size <= limit)
    }

    /**
     * O4b: `next_cursor` is null exactly when nothing remains below the page (D10) -- null means
     * "nothing remains", never "unknown". Both directions.
     */
    @JvmStatic
    fun o4b_cursorNullMeansExhausted() {
        val s = Store()
        s.createUser("a")
        s.appendTweet("a", "1")
        s.appendTweet("a", "2")
        val all = s.timelinePage("a", 50, null)
        assert(all.nextCursor == null)
        val partial = s.timelinePage("a", 1, null)
        assert(partial.nextCursor != null)
    }

    /** O4c: the page is a prefix of the reverse log -- newest first, no fabrication (F2). */
    @JvmStatic
    fun o4c_pageIsNewestFirst() {
        val s = Store()
        s.createUser("a")
        val t1 = s.appendTweet("a", "1")
        s.tick()
        val t2 = s.appendTweet("a", "2")
        val p = s.timelinePage("a", 50, null)
        assert(p.tweets.size == 2)
        assert(p.tweets[0].id == t2.id)
        assert(p.tweets[1].id == t1.id)
        assert(p.tweets[0].createdAt >= p.tweets[1].createdAt)
    }

    // ======================================================================
    // GROUP 5 -- service validation order (D4, D6, F003). BLOCKED: getBytes.
    // ======================================================================
    //
    // The precedence `twitter.tla` leaves open, and the reason this repository exists: two
    // implementations can disagree here and both still refine the model. Every one of these paths
    // begins with `Dom.validHandle`, so all of them inherit the Group 2 blockage. Kept as the
    // record of what the bounded rung does NOT cover -- which is precisely the part of the
    // contract that the model does not pin.

    /** O5a: follow(eve, eve) with eve unknown is unknown_user, NOT self_follow_forbidden (D4). */
    @JvmStatic
    fun o5a_unknownBeatsSelfFollow() {
        val r = Service().follow("eve", "eve")
        assert(r is Outcome.Err && r.code.compareTo("unknown_user") == 0)
    }

    /** O5b: follow(alice, alice) with alice known is self_follow_forbidden (D4). */
    @JvmStatic
    fun o5b_knownSelfFollowIsForbidden() {
        val svc = Service()
        svc.createUser("alice")
        val r = svc.follow("alice", "alice")
        assert(r is Outcome.Err && r.code.compareTo("self_follow_forbidden") == 0)
    }

    /** O5c: syntax beats existence -- follow("EVE","eve") is invalid_handle (D6). */
    @JvmStatic
    fun o5c_syntaxBeatsExistence() {
        val r = Service().follow("EVE", "eve")
        assert(r is Outcome.Err && r.code.compareTo("invalid_handle") == 0)
    }

    /** O5d: a rejected registration burns no id (F8). Corpus step 11. */
    @JvmStatic
    fun o5d_rejectionBurnsNoId() {
        val svc = Service()
        svc.createUser("a")
        svc.createUser("a") // handle_taken
        svc.createUser("!") // invalid_handle
        val r = svc.createUser("b")
        assert(r is Outcome.Ok && r.value.id == 2L)
    }
}
