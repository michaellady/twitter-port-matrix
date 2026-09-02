package twitterport.verification;

import twitterport.dom.Dom;
import twitterport.dom.Page;
import twitterport.dom.Tweet;
import twitterport.dom.User;
import twitterport.service.Result;
import twitterport.service.Service;
import twitterport.store.Store;

/**
 * The Java corner's verification obligations, written as JBMC entry points.
 *
 * <p><b>This is not a test suite.</b> Each method below is a bounded proof obligation: JBMC treats
 * its parameters as nondeterministic, explores every path within the unwinding bound, and reports
 * each assertion as SUCCESS (no counterexample exists within the bound) or FAILURE (here is one).
 * Nothing here runs during a normal build -- the registry compiles {@code impls/java/src} through
 * {@code src/twitterport/Main.java}, and this directory is compiled and driven separately by
 * {@code tools/cmd/jbmc}.
 *
 * <p><b>Why this file exists.</b> {@code ASSURANCE.md} recorded the Java corner's R4 route as
 * "not attempted; {@code impls/java} has no obligation set at all", and {@code evidence/MATRIX.md}
 * capped six of the twelve R4 cells on exactly that sentence. F014 had already established that
 * the operative limit on the JVM corners is a JBMC defect rather than a language cost, and that
 * <em>the wall is shared with Java</em> -- but shared with a corner that had nothing to run. This
 * file is the twin of {@code impls/kotlin/verification/Obligations.kt}: the same fifteen
 * obligations, in the same five groups, over this corner's own classes, so that the Java column
 * measures the same question the Kotlin column measures.
 *
 * <p><b>The twin is deliberate and it is the measurement.</b> Every obligation below states the
 * same property as the Kotlin obligation of the same name. Where the two corners' verdicts differ,
 * the difference is a fact about Java-versus-Kotlin bytecode under one checker, which is the only
 * reason a fourth corner is worth its cost. Where they agree, F014's "the wall is shared" stops
 * being an inference and becomes a measurement.
 *
 * <h2>Three JBMC limits shape everything below, and none of them is Java's fault</h2>
 *
 * <ol>
 *   <li><b>{@code String.equals} is unsound in JBMC 6.11.</b> {@code assert "abc".equals("abc")}
 *       is reported FAILURE (F014, reduced to a two-line repro in plain {@code javac} output --
 *       which is to say the repro <em>is</em> this corner's language). {@code compareTo},
 *       {@code startsWith}, {@code isEmpty}, {@code length} and {@code charAt} are all fine, so
 *       every assertion below compares strings with {@code compareTo(x) == 0}. What that cannot
 *       route around is the implementation's own use of string equality: {@code Store.timelinePage}
 *       tests visibility with {@code t.author().equals(user)} and membership with a
 *       {@code HashSet<Edge>} whose {@code Edge} is a record, so its generated {@code equals}
 *       reduces to {@code String.equals} too.
 *   <li><b>{@code String.getBytes(Charset)} is nondeterministic in JBMC 6.11.</b> The model
 *       dispatches on {@code Charset.name()} through that same broken intrinsic and falls through
 *       to an opaque stub returning an array of unconstrained length. {@code Dom.validHandle} and
 *       {@code Dom.validText} measure UTF-8 <em>bytes</em> -- which is what makes them byte-exact
 *       against a Go reference machine where {@code len(s)} is a byte count -- so both predicates,
 *       and every service path that begins with one, are outside what this checker can say
 *       anything about. Rewriting {@code validHandle} as a {@code char} loop would make it
 *       checkable and would be a change made to please a tool, so it is not made.
 *   <li><b>The SAT instance does not scale.</b> A nondeterministic {@code limit} over a four-entry
 *       log exhausts memory. An ordinary bounded-model-checking wall.
 * </ol>
 *
 * <p>Which of the three (if any) actually binds each obligation is <em>measured</em> per obligation
 * by {@code tools/cmd/jbmc}, not assumed from the Kotlin corner's table. The Kotlin numbers are the
 * prediction; this corner's run is the test of it.
 *
 * <h2>One shape difference from the Kotlin twin, and it is not cosmetic</h2>
 *
 * <p>Kotlin's obligations are {@code @JvmStatic} members of an {@code object} so they compile to
 * plain static methods. Java's are static methods because Java has no other kind. The Java corner
 * also carries two erasure sites the Kotlin corner does not present in the same place:
 * {@code Result<T>.value()} erases to {@code Object} and every call site casts, and
 * {@code List<Tweet>.get(int)} does the same. F013 is the finding that an undischargeable erased
 * checkcast makes everything after it infeasible and turns a whole group of obligations vacuously
 * VERIFIED. So every obligation here is named by at least one negation canary in
 * {@link Canaries} -- all fifteen, not just the ones anybody suspected, which is F025's rule.
 */
public final class Obligations {

    private Obligations() {}

    // ======================================================================
    // GROUP 1 -- Dom.parseInt64: the integer accept set (D10).
    // ======================================================================
    //
    // The property that motivated hand-writing the parser instead of calling Long.parseLong:
    // Long.parseLong resolves digits through Character.digit, which accepts non-ASCII decimal
    // digits, so `?limit=١` would be a limit of 1 in Java and an `invalid_limit` in S_obs. These
    // obligations pin the accept set over every string of the given length, not over a list of
    // examples.
    //
    // The parameter is a String rather than a char the body converts, because String.valueOf(char)
    // is one of the opaque methods: constructing a string inside the obligation makes its contents
    // nondeterministic and the obligation vacuous. JBMC synthesises nondeterministic strings for
    // String parameters directly, bounded by --max-nondet-string-length.

    /** O1a: over ALL one-character strings, parseInt64 accepts exactly the ASCII digits. */
    public static void o1a_oneCharAcceptSet(String s) {
        if (s.length() != 1) {
            return;
        }
        char c = s.charAt(0);
        assert (Dom.parseInt64(s) != null) == (c >= '0' && c <= '9');
    }

    /** O1b: over ALL two-character strings, parseInt64 accepts exactly {@code [+-0-9][0-9]}. */
    public static void o1b_twoCharAcceptSet(String s) {
        if (s.length() != 2) {
            return;
        }
        char a = s.charAt(0);
        char b = s.charAt(1);
        boolean ok = ((a >= '0' && a <= '9') || a == '+' || a == '-') && (b >= '0' && b <= '9');
        assert (Dom.parseInt64(s) != null) == ok;
    }

    /** O1c: the empty string and a bare sign are not numbers. */
    public static void o1c_emptyAndBareSignRejected() {
        assert Dom.parseInt64("") == null;
        assert Dom.parseInt64("+") == null;
        assert Dom.parseInt64("-") == null;
    }

    // ======================================================================
    // GROUP 2 -- Dom.validHandle / Dom.validText (D6).
    // ======================================================================
    //
    // Kept, and expected to be reported BLOCKED by the getBytes stub. Their failure would not be a
    // defect in the predicates; it is JBMC returning an unconstrained byte array from
    // String.getBytes(Charset), which lets it claim validText("") is true. The obligations are the
    // record of which part of the contract this rung cannot see.

    /** O2a: the empty handle and the empty text are invalid. Corpus steps. */
    public static void o2a_emptyIsInvalid() {
        assert !Dom.validHandle("");
        assert !Dom.validText("");
    }

    /** O2b: a plainly legal handle is valid. */
    public static void o2b_goodHandleIsValid() {
        assert Dom.validHandle("alice");
    }

    // ======================================================================
    // GROUP 3 -- Store.appendTweet: the enforced premise of the F2 lemma.
    // ======================================================================
    //
    // THE obligation of this corner, as it is of the Kotlin one. D9 derives timeline ordering from
    // an insertion-ordered log instead of proving a sort correct, and finding F005 records that the
    // derivation is sound only if the log's two premises are enforced at the mutation site rather
    // than assumed. This asks a checker to confirm the enforcement instead of the comment.

    /** O3a: two consecutive appends yield strictly increasing ids. */
    public static void o3a_idsStrictlyIncrease() {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "x");
        Tweet t2 = s.appendTweet("a", "y");
        assert t1.id() < t2.id();
    }

    /**
     * O3b: {@code createdAt} is non-decreasing across appends whether or not the clock advanced
     * between them, and ids still increase. Both premises of the monotonicity lemma, over both
     * branches.
     */
    public static void o3b_createdAtNonDecreasing(boolean tickFirst) {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "x");
        if (tickFirst) {
            s.tick();
        }
        Tweet t2 = s.appendTweet("a", "y");
        assert t1.createdAt() <= t2.createdAt();
        assert t1.id() < t2.id();
    }

    /** O3c: three appends, arbitrary tick pattern -- the lemma over a longer log. */
    public static void o3c_lemmaOverThreeAppends(boolean tick1, boolean tick2) {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "x");
        if (tick1) {
            s.tick();
        }
        Tweet t2 = s.appendTweet("a", "y");
        if (tick2) {
            s.tick();
        }
        Tweet t3 = s.appendTweet("a", "z");
        assert t1.id() < t2.id() && t2.id() < t3.id();
        assert t1.createdAt() <= t2.createdAt() && t2.createdAt() <= t3.createdAt();
    }

    // ======================================================================
    // GROUP 4 -- Store.timelinePage: pagination arithmetic (D10).
    // ======================================================================
    //
    // These three do NOT register the author with Store.createUser first, and the Kotlin twin
    // does. The call is decoration: timelinePage decides visibility with
    // `t.author().equals(user) || isFollowing(user, t.author())` and never consults the user
    // registry, so registering "a" changes no answer. It changes something else, though, and it
    // is not free -- the whole obligation set is compiled against the tree under test, so an
    // obligation that mentions a method it does not need is coupled to that method's SIGNATURE.
    // The `id-burned-on-reject` mutant splits `Store.createUser(String)` into `allocUserId()`
    // plus `createUser(String, long)`, which makes the Kotlin obligation set fail to COMPILE and
    // turns a reached mutant into an error cell -- a measurement lost to a call nobody needed.
    // F035 records it. The rule that falls out: an obligation should touch the smallest surface
    // its property needs, because on a mutation rung every extra mention is a coupling to a
    // signature the mutant is allowed to change.

    /** O4a: over every legal limit, a page never exceeds it. */
    public static void o4a_pageRespectsLimit(int limit) {
        if (limit < 1 || limit > 3) {
            return;
        }
        Store s = new Store();
        s.appendTweet("a", "1");
        s.appendTweet("a", "2");
        s.appendTweet("a", "3");
        s.appendTweet("a", "4");
        Page p = s.timelinePage("a", limit, null);
        assert p.tweets().size() <= limit;
    }

    /**
     * O4b: {@code nextCursor} is null exactly when nothing remains below the page (D10) -- null
     * means "nothing remains", never "unknown". Both directions.
     */
    public static void o4b_cursorNullMeansExhausted() {
        Store s = new Store();
        s.appendTweet("a", "1");
        s.appendTweet("a", "2");
        Page all = s.timelinePage("a", 50, null);
        assert all.nextCursor() == null;
        Page partial = s.timelinePage("a", 1, null);
        assert partial.nextCursor() != null;
    }

    /** O4c: the page is a prefix of the reverse log -- newest first, no fabrication (F2). */
    public static void o4c_pageIsNewestFirst() {
        Store s = new Store();
        Tweet t1 = s.appendTweet("a", "1");
        s.tick();
        Tweet t2 = s.appendTweet("a", "2");
        Page p = s.timelinePage("a", 50, null);
        assert p.tweets().size() == 2;
        assert p.tweets().get(0).id() == t2.id();
        assert p.tweets().get(1).id() == t1.id();
        assert p.tweets().get(0).createdAt() >= p.tweets().get(1).createdAt();
    }

    // ======================================================================
    // GROUP 5 -- service validation order (D4, D6, F003).
    // ======================================================================
    //
    // The precedence twitter.tla leaves open, and the reason this repository exists: two
    // implementations can disagree here and both still refine the model. Every one of these paths
    // begins with Dom.validHandle, so all of them are exposed to the Group 2 blockage.

    /** O5a: follow(eve, eve) with eve unknown is unknown_user, NOT self_follow_forbidden (D4). */
    public static void o5a_unknownBeatsSelfFollow() {
        Result<Void> r = new Service().follow("eve", "eve");
        assert r.isErr() && r.error().compareTo(Dom.ERR_UNKNOWN_USER) == 0;
    }

    /** O5b: follow(alice, alice) with alice known is self_follow_forbidden (D4). */
    public static void o5b_knownSelfFollowIsForbidden() {
        Service svc = new Service();
        svc.createUser("alice");
        Result<Void> r = svc.follow("alice", "alice");
        assert r.isErr() && r.error().compareTo(Dom.ERR_SELF_FOLLOW) == 0;
    }

    /** O5c: syntax beats existence -- follow("EVE","eve") is invalid_handle (D6). */
    public static void o5c_syntaxBeatsExistence() {
        Result<Void> r = new Service().follow("EVE", "eve");
        assert r.isErr() && r.error().compareTo(Dom.ERR_INVALID_HANDLE) == 0;
    }

    /** O5d: a rejected registration burns no id (F8). Corpus step 11. */
    public static void o5d_rejectionBurnsNoId() {
        Service svc = new Service();
        svc.createUser("a");
        svc.createUser("a"); // handle_taken
        svc.createUser("!"); // invalid_handle
        Result<User> r = svc.createUser("b");
        assert !r.isErr() && r.value().id() == 2L;
    }
}
