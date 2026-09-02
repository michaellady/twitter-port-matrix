package twitterport.store

import twitterport.dom.Edge
import twitterport.dom.Page
import twitterport.dom.Tweet
import twitterport.dom.User

/** Thrown when an append would break the premises of the monotonicity lemma. */
class LogInvariantViolation(message: String) : RuntimeException(message)

/**
 * VERIFIED CORE. The whole observable state, in flat containers.
 *
 * THE TWEET LOG IS NEVER SORTED. There is exactly one list of tweets, it is only ever appended to,
 * and the timeline is a reverse scan over it. See D9 and finding F004. There is no call to
 * `sortedBy`, `sortedWith`, `sortBy` or `sort` anywhere in this corner -- Kotlin's collection
 * library makes an incidental sort a one-word edit, which is precisely why its absence is worth
 * asserting rather than assuming.
 *
 * **Monotonicity lemma.** For log positions `i < j`:
 * ```
 *   log[i].id        <  log[j].id          (ids are allocated monotonically)
 *   log[i].createdAt <= log[j].createdAt   (the clock never decreases)
 * ```
 * Therefore reverse iteration over the log yields exactly descending lexicographic
 * `(createdAt, id)`: if the timestamps differ the later post wins on timestamp, and if they tie
 * the later post wins on id. So F2 (ordering) is a *derived* property of an insertion-ordered
 * structure and needs no verified sort specification.
 *
 * **The premises are enforced, not assumed.** [appendTweet] rejects any append that would break
 * either premise. Finding F005 is the reason this is not a comment: the same lemma was relied on
 * in both original corners with nothing enforcing it, so F2 was assumed rather than derived, and
 * the only code that ever appended out of order produced a silently mis-ordered timeline. Deriving
 * a property from a data structure relocates the obligation onto that structure's invariants; it
 * does not remove it.
 */
class Store {

    private val userByHandle = HashMap<String, User>()
    private val users = ArrayList<User>()
    private val follows = HashSet<Edge>()

    /** The append-ordered tweet log. Appended to, scanned in reverse, never sorted. */
    private val log = ArrayList<Tweet>()

    private var nextUserId = 1L
    private var nextTweetId = 1L
    private var clockValue = 0L

    // --- users ---------------------------------------------------------------

    fun hasUser(handle: String): Boolean = userByHandle.containsKey(handle)

    /** Registers a handle. The caller has already established that it is valid and free. */
    fun createUser(handle: String): User {
        val u = User(handle, nextUserId)
        nextUserId++
        users.add(u)
        userByHandle[handle] = u
        return u
    }

    // --- follow edges --------------------------------------------------------

    fun addFollow(from: String, to: String) {
        follows.add(Edge(from, to))
    }

    fun removeFollow(from: String, to: String) {
        follows.remove(Edge(from, to))
    }

    fun isFollowing(from: String, to: String): Boolean = follows.contains(Edge(from, to))

    // --- clock ---------------------------------------------------------------

    fun clock(): Long = clockValue

    /** Advances the clock by exactly 1 and returns it (D3). */
    fun tick(): Long {
        clockValue++
        return clockValue
    }

    // --- the log -------------------------------------------------------------

    /**
     * Appends one tweet, enforcing the log invariant at the mutation site.
     *
     * The id and the timestamp are allocated here rather than accepted from the caller, so the
     * invariant cannot be broken by a caller at all; the explicit check is the guard that makes
     * that structural fact checkable rather than merely true.
     */
    fun appendTweet(author: String, text: String): Tweet {
        val t = Tweet(nextTweetId, author, text, clockValue)
        // Indexed access, not `log.lastOrNull()`.
        //
        // The two are behaviourally identical and the extension function reads better, but
        // `lastOrNull` is a generic stdlib extension: it returns `Object` and the call site adds
        // a checkcast to `Tweet` that JBMC cannot discharge, because its model of `ArrayList`
        // does not track element types through erasure. CBMC assumes a failed check held for the
        // remainder of the path, so an undischargeable cast makes every statement AFTER this line
        // infeasible -- and every assertion about the log downstream comes back SUCCESS for free.
        // Measured: with `lastOrNull` all six store obligations reported VERIFIED and all four
        // store canaries reported SUCCESS, which is what caught it. With indexed access the
        // canaries are refuted and o3a verifies with zero failing goals of any kind.
        //
        // This is not the tool being appeased at the contract's expense: the Java corner writes
        // `log.get(log.size() - 1)` here, so the two corners now differ by less, not more.
        val last = if (log.isEmpty()) null else log[log.size - 1]
        if (last != null) {
            if (t.id <= last.id) {
                throw LogInvariantViolation(
                    "append would break id monotonicity: last=${last.id} new=${t.id}"
                )
            }
            if (t.createdAt < last.createdAt) {
                throw LogInvariantViolation(
                    "append would break clock monotonicity: " +
                        "last=${last.createdAt} new=${t.createdAt}"
                )
            }
        }
        log.add(t)
        nextTweetId++
        return t
    }

    /**
     * THE SORT-FREE TIMELINE: a single reverse scan over the append log.
     *
     * A tweet is visible to [user] when the user authored it or follows its author (F1). The
     * cursor is exclusive: only tweets with `id < cursor` are considered (D10). `nextCursor` is the
     * id of the last tweet on the page when at least one further visible tweet exists below it,
     * and null otherwise.
     *
     * Written as an index loop rather than `log.asReversed().filter { ... }.take(limit)` on
     * purpose. The cursor rule is not "take n" -- it needs to know whether a further visible tweet
     * exists *below* the page, which a `take` cannot observe, and the sequence version would have
     * to peek one past the limit and then discard it. An index loop says what it does.
     */
    fun timelinePage(user: String, limit: Long, cursor: Long?): Page {
        val page = ArrayList<Tweet>()
        var next: Long? = null
        for (i in log.indices.reversed()) {
            val t = log[i]
            if (cursor != null && t.id >= cursor) {
                continue
            }
            if (t.author != user && !isFollowing(user, t.author)) {
                continue
            }
            if (page.size.toLong() == limit) {
                // A further visible tweet exists below the page: emit a cursor.
                next = page[page.size - 1].id
                break
            }
            page.add(t)
        }
        return Page(page, next)
    }

    // --- R5: the abstraction function ---------------------------------------
    //
    // `spec/refinement/OBLIGATION.md` §4 states abs as one pure projection per
    // axis of the `S_obs` state. These are this corner's, and they are here for
    // the same reason the Go corner's `AbsUsers` / `AbsFollows` / `AbsLogLen`
    // are in `memstore.gobra`: an abstraction function over state that is not
    // reachable from outside the object is not definable at all, and an
    // UNINTERPRETED abs would make every commutation clause an axiom instead of
    // a theorem (which is exactly what B4 costs the Rust corner).
    //
    // TWO DIFFERENCES FROM THE GO CORNER, both real and neither hideable:
    //
    //  1. Go's projections are GHOST: they live in `// @` comments, `go build`
    //     erases them, and the shipped binary has no such methods. Kotlin has
    //     no ghost mode, so these are ordinary methods on the shipped class.
    //     They are not called by `src/` -- `Main.kt` and the shim never touch
    //     them -- but they are in the compiled artefact, and that is a widening
    //     of this corner's surface that the Go corner does not pay. Recorded in
    //     evidence/findings/F045 rather than worked around.
    //
    //  2. Go projects the users and follows axes as SETS (`set[string]`,
    //     `set[dom.Follow]`), which Gobra's ghost language has. JBMC has no set
    //     theory, so each axis is decomposed into a membership test and a
    //     cardinality. `absFollows` is measured UNDECIDABLE by JBMC 6.11.0
    //     (F014: `HashSet.contains` reduces to `Edge.equals`, which reduces to
    //     `String.equals`); it is kept because the clause it would carry is
    //     part of the obligation, and `jbmc r5verify` reports it BLOCKED rather
    //     than dropping it from the denominator.
    //
    // `clock()` above is the clock axis; it needs no separate projection.

    /** abs, users axis: how many handles are registered. */
    fun absUserCount(): Int = users.size

    /** abs, users axis: is this handle registered. */
    fun absHasUser(handle: String): Boolean = userByHandle.containsKey(handle)

    /** abs, follows axis: how many directed edges exist. */
    fun absFollowCount(): Int = follows.size

    /** abs, follows axis: does this edge exist. BLOCKED by F014 -- see above. */
    fun absFollows(from: String, to: String): Boolean = follows.contains(Edge(from, to))

    /** abs, log axis: the length of the append-ordered log. */
    fun absLogLen(): Int = log.size

    /** abs, log axis: the id at log position [i]. */
    fun absLogIdAt(i: Int): Long = log[i].id

    /** abs, log axis: the timestamp at log position [i]. */
    fun absLogCreatedAtAt(i: Int): Long = log[i].createdAt

    /** abs, log axis: the author at log position [i]. */
    fun absLogAuthorAt(i: Int): String = log[i].author
}
