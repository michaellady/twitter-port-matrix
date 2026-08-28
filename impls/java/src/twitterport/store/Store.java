package twitterport.store;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

import twitterport.dom.Edge;
import twitterport.dom.Page;
import twitterport.dom.Tweet;
import twitterport.dom.User;

/**
 * VERIFIED CORE. The whole observable state, in flat containers.
 *
 * <p>THE TWEET LOG IS NEVER SORTED. There is exactly one list of tweets, it is only ever appended
 * to, and the timeline is a reverse scan over it. See D9 and finding F004.
 *
 * <p><b>Monotonicity lemma.</b> For log positions {@code i < j}:
 *
 * <pre>
 *   log[i].id        &lt;  log[j].id          (ids are allocated monotonically)
 *   log[i].createdAt &lt;= log[j].createdAt   (the clock never decreases)
 * </pre>
 *
 * Therefore reverse iteration over the log yields exactly descending lexicographic
 * {@code (createdAt, id)}: if the timestamps differ the later post wins on timestamp, and if they
 * tie the later post wins on id. So F2 (ordering) is a <em>derived</em> property of an
 * insertion-ordered structure and needs no verified sort specification.
 *
 * <p><b>The premises are enforced, not assumed.</b> {@link #appendTweet} rejects any append that
 * would break either premise. Finding F005 is the reason this is not a comment: the same lemma was
 * relied on in both existing corners with nothing enforcing it, so F2 was assumed rather than
 * derived, and the only code that ever appended out of order produced a silently mis-ordered
 * timeline. Deriving a property from a data structure relocates the obligation onto that
 * structure's invariants; it does not remove it.
 */
public final class Store {

    /** Thrown when an append would break the premises of the monotonicity lemma. */
    public static final class LogInvariantViolation extends RuntimeException {
        private static final long serialVersionUID = 1L;

        LogInvariantViolation(String message) {
            super(message);
        }
    }

    private final Map<String, User> userByHandle = new HashMap<>();
    private final List<User> users = new ArrayList<>();
    private final Set<Edge> follows = new HashSet<>();

    /** The append-ordered tweet log. Appended to, scanned in reverse, never sorted. */
    private final List<Tweet> log = new ArrayList<>();

    private long nextUserId = 1;
    private long nextTweetId = 1;
    private long clock = 0;

    // --- users ---------------------------------------------------------------

    public boolean hasUser(String handle) {
        return userByHandle.containsKey(handle);
    }

    /** Registers a handle. The caller has already established that it is valid and free. */
    public User createUser(String handle) {
        User u = new User(handle, nextUserId);
        nextUserId++;
        users.add(u);
        userByHandle.put(handle, u);
        return u;
    }

    // --- follow edges --------------------------------------------------------

    public void addFollow(String from, String to) {
        follows.add(new Edge(from, to));
    }

    public void removeFollow(String from, String to) {
        follows.remove(new Edge(from, to));
    }

    public boolean isFollowing(String from, String to) {
        return follows.contains(new Edge(from, to));
    }

    // --- clock ---------------------------------------------------------------

    public long clock() {
        return clock;
    }

    /** Advances the clock by exactly 1 and returns it (D3). */
    public long tick() {
        clock++;
        return clock;
    }

    // --- the log -------------------------------------------------------------

    /**
     * Appends one tweet, enforcing the log invariant at the mutation site.
     *
     * <p>The id and the timestamp are allocated here rather than accepted from the caller, so the
     * invariant cannot be broken by a caller at all; the explicit check is the guard that makes
     * that structural fact checkable rather than merely true.
     */
    public Tweet appendTweet(String author, String text) {
        Tweet t = new Tweet(nextTweetId, author, text, clock);
        if (!log.isEmpty()) {
            Tweet last = log.get(log.size() - 1);
            if (t.id() <= last.id()) {
                throw new LogInvariantViolation(
                        "append would break id monotonicity: last=" + last.id() + " new=" + t.id());
            }
            if (t.createdAt() < last.createdAt()) {
                throw new LogInvariantViolation(
                        "append would break clock monotonicity: last="
                                + last.createdAt()
                                + " new="
                                + t.createdAt());
            }
        }
        log.add(t);
        nextTweetId++;
        return t;
    }

    /**
     * THE SORT-FREE TIMELINE: a single reverse scan over the append log.
     *
     * <p>A tweet is visible to {@code user} when the user authored it or follows its author (F1).
     * The cursor is exclusive: only tweets with {@code id < cursor} are considered (D10).
     * {@code nextCursor} is the id of the last tweet on the page when at least one further visible
     * tweet exists below it, and null otherwise.
     */
    public Page timelinePage(String user, long limit, Long cursor) {
        List<Tweet> page = new ArrayList<>();
        Long next = null;
        for (int i = log.size() - 1; i >= 0; i--) {
            Tweet t = log.get(i);
            if (cursor != null && t.id() >= cursor) {
                continue;
            }
            if (!t.author().equals(user) && !isFollowing(user, t.author())) {
                continue;
            }
            if ((long) page.size() == limit) {
                // A further visible tweet exists below the page: emit a cursor.
                next = page.get(page.size() - 1).id();
                break;
            }
            page.add(t);
        }
        return new Page(page, next);
    }
}
