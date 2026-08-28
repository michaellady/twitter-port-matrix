package twitterport.service;

import twitterport.dom.Dom;
import twitterport.dom.Page;
import twitterport.dom.Tweet;
import twitterport.dom.User;
import twitterport.store.Store;

/**
 * VERIFIED CORE. The semantics of {@code S_obs}: validation order, the error vocabulary, and the
 * state transitions.
 *
 * <p>Nothing in this file knows what HTTP is. Everything the contract pins that is not wire format
 * lives here, because putting contract semantics in the shim would produce a green R0 over code no
 * verifier ever reads, and R5 would then prove nothing about observable behaviour (TCB.md).
 *
 * <p><b>Validation order is part of the contract.</b> Syntax before existence, existence before
 * semantics -- uniformly. {@code twitter.tla}'s {@code Follow} is an unordered conjunction, so the
 * model does not disambiguate {@code follow(eve, eve)} where {@code eve} is unknown; both answers
 * refine it (finding F003). {@code S_obs} picks existence-before-semantics, so that request is
 * {@code unknown_user} and not {@code self_follow_forbidden} (D4).
 *
 * <p>Every method is {@code synchronized}: the HTTP shim serves on a pool, and the observable
 * transition function is a single atomic step.
 */
public final class Service {

    private final Store store = new Store();

    /**
     * POST /users.
     *
     * <p>Order: handle syntax, then handle already taken. (The malformed-body check happens in the
     * shim, where the bytes are.)
     */
    public synchronized Result<User> createUser(String handle) {
        if (!Dom.validHandle(handle)) {
            return Result.err(Dom.ERR_INVALID_HANDLE);
        }
        if (store.hasUser(handle)) {
            return Result.err(Dom.ERR_HANDLE_TAKEN);
        }
        return Result.ok(store.createUser(handle));
    }

    /**
     * POST /follow.
     *
     * <p>Order: from syntax, to syntax, from existence, to existence, self-follow. Idempotent (F3):
     * re-following an existing edge is a success and leaves the follow set unchanged.
     */
    public synchronized Result<Void> follow(String from, String to) {
        if (!Dom.validHandle(from) || !Dom.validHandle(to)) {
            return Result.err(Dom.ERR_INVALID_HANDLE);
        }
        if (!store.hasUser(from)) {
            return Result.err(Dom.ERR_UNKNOWN_USER);
        }
        if (!store.hasUser(to)) {
            return Result.err(Dom.ERR_UNKNOWN_USER);
        }
        if (from.equals(to)) {
            return Result.err(Dom.ERR_SELF_FOLLOW);
        }
        store.addFollow(from, to);
        return Result.ok(null);
    }

    /**
     * DELETE /follow.
     *
     * <p>Same order as follow minus the self-edge check: {@code twitter.tla}'s {@code Unfollow}
     * requires both users to be known but, unlike {@code Follow}, does not require them to differ,
     * so self-unfollow of a known user is a legal no-op (D5). Idempotent (F3).
     */
    public synchronized Result<Void> unfollow(String from, String to) {
        if (!Dom.validHandle(from) || !Dom.validHandle(to)) {
            return Result.err(Dom.ERR_INVALID_HANDLE);
        }
        if (!store.hasUser(from)) {
            return Result.err(Dom.ERR_UNKNOWN_USER);
        }
        if (!store.hasUser(to)) {
            return Result.err(Dom.ERR_UNKNOWN_USER);
        }
        store.removeFollow(from, to);
        return Result.ok(null);
    }

    /**
     * POST /tweets.
     *
     * <p>Order: author syntax, text syntax, author existence. Syntax before existence throughout.
     *
     * <p>An id is burned only on success: a rejected request leaves the state completely unchanged,
     * so rejection is observable in the response and never in the state.
     */
    public synchronized Result<Tweet> postTweet(String author, String text) {
        if (!Dom.validHandle(author)) {
            return Result.err(Dom.ERR_INVALID_HANDLE);
        }
        if (!Dom.validText(text)) {
            return Result.err(Dom.ERR_INVALID_TEXT);
        }
        if (!store.hasUser(author)) {
            return Result.err(Dom.ERR_UNKNOWN_USER);
        }
        return Result.ok(store.appendTweet(author, text));
    }

    /** POST /tick: advances the clock by exactly 1 and returns it (D3). Total, never rejected. */
    public synchronized long tick() {
        return store.tick();
    }

    /**
     * GET /timeline.
     *
     * <p>Order: user syntax, user existence, limit, cursor. The raw limit and cursor arrive as
     * strings because which strings are legal is contract (D10) rather than wire format, and
     * putting the parse in the shim would move part of the pinned validation order out of the
     * verified core.
     */
    public synchronized Result<Page> timeline(String user, String rawLimit, String rawCursor) {
        if (!Dom.validHandle(user)) {
            return Result.err(Dom.ERR_INVALID_HANDLE);
        }
        if (!store.hasUser(user)) {
            return Result.err(Dom.ERR_UNKNOWN_USER);
        }

        long limit = Dom.DEFAULT_LIMIT;
        if (rawLimit != null) {
            Long n = Dom.parseInt64(rawLimit);
            if (n == null || n < 1 || n > Dom.MAX_LIMIT) {
                return Result.err(Dom.ERR_INVALID_LIMIT);
            }
            limit = n;
        }

        Long cursor = null;
        if (rawCursor != null) {
            Long n = Dom.parseInt64(rawCursor);
            if (n == null || n < 1) {
                return Result.err(Dom.ERR_INVALID_CURSOR);
            }
            cursor = n;
        }

        return Result.ok(store.timelinePage(user, limit, cursor));
    }
}
