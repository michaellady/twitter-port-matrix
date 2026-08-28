package twitterport.service

import twitterport.dom.DEFAULT_LIMIT
import twitterport.dom.ERR_HANDLE_TAKEN
import twitterport.dom.ERR_INVALID_CURSOR
import twitterport.dom.ERR_INVALID_HANDLE
import twitterport.dom.ERR_INVALID_LIMIT
import twitterport.dom.ERR_INVALID_TEXT
import twitterport.dom.ERR_SELF_FOLLOW
import twitterport.dom.ERR_UNKNOWN_USER
import twitterport.dom.MAX_LIMIT
import twitterport.dom.Page
import twitterport.dom.Tweet
import twitterport.dom.User
import twitterport.dom.parseInt64
import twitterport.dom.validHandle
import twitterport.dom.validText
import twitterport.store.Store

/**
 * VERIFIED CORE. The semantics of `S_obs`: validation order, the error vocabulary, and the state
 * transitions.
 *
 * Nothing in this file knows what HTTP is. Everything the contract pins that is not wire format
 * lives here, because putting contract semantics in the shim would produce a green R0 over code no
 * verifier ever reads, and R5 would then prove nothing about observable behaviour (TCB.md).
 *
 * **Validation order is part of the contract.** Syntax before existence, existence before
 * semantics -- uniformly. `twitter.tla`'s `Follow` is an unordered conjunction, so the model does
 * not disambiguate `follow(eve, eve)` where `eve` is unknown; both answers refine it (finding
 * F003). `S_obs` picks existence-before-semantics, so that request is `unknown_user` and not
 * `self_follow_forbidden` (D4).
 *
 * Every method is `@Synchronized`: the HTTP shim serves on a pool, and the observable transition
 * function is a single atomic step.
 */
class Service {

    private val store = Store()

    /**
     * POST /users.
     *
     * Order: handle syntax, then handle already taken. (The malformed-body check happens in the
     * shim, where the bytes are.)
     */
    @Synchronized
    fun createUser(handle: String): Outcome<User> {
        if (!validHandle(handle)) {
            return Outcome.Err(ERR_INVALID_HANDLE)
        }
        if (store.hasUser(handle)) {
            return Outcome.Err(ERR_HANDLE_TAKEN)
        }
        return Outcome.Ok(store.createUser(handle))
    }

    /**
     * POST /follow.
     *
     * Order: from syntax, to syntax, from existence, to existence, self-follow. Idempotent (F3):
     * re-following an existing edge is a success and leaves the follow set unchanged.
     */
    @Synchronized
    fun follow(from: String, to: String): Outcome<Unit> {
        if (!validHandle(from) || !validHandle(to)) {
            return Outcome.Err(ERR_INVALID_HANDLE)
        }
        if (!store.hasUser(from)) {
            return Outcome.Err(ERR_UNKNOWN_USER)
        }
        if (!store.hasUser(to)) {
            return Outcome.Err(ERR_UNKNOWN_USER)
        }
        if (from == to) {
            return Outcome.Err(ERR_SELF_FOLLOW)
        }
        store.addFollow(from, to)
        return Outcome.Ok(Unit)
    }

    /**
     * DELETE /follow.
     *
     * Same order as [follow] minus the self-edge check: `twitter.tla`'s `Unfollow` requires both
     * users to be known but, unlike `Follow`, does not require them to differ, so self-unfollow of
     * a known user is a legal no-op (D5). Idempotent (F3).
     */
    @Synchronized
    fun unfollow(from: String, to: String): Outcome<Unit> {
        if (!validHandle(from) || !validHandle(to)) {
            return Outcome.Err(ERR_INVALID_HANDLE)
        }
        if (!store.hasUser(from)) {
            return Outcome.Err(ERR_UNKNOWN_USER)
        }
        if (!store.hasUser(to)) {
            return Outcome.Err(ERR_UNKNOWN_USER)
        }
        store.removeFollow(from, to)
        return Outcome.Ok(Unit)
    }

    /**
     * POST /tweets.
     *
     * Order: author syntax, text syntax, author existence. Syntax before existence throughout.
     *
     * An id is burned only on success: a rejected request leaves the state completely unchanged,
     * so rejection is observable in the response and never in the state.
     */
    @Synchronized
    fun postTweet(author: String, text: String): Outcome<Tweet> {
        if (!validHandle(author)) {
            return Outcome.Err(ERR_INVALID_HANDLE)
        }
        if (!validText(text)) {
            return Outcome.Err(ERR_INVALID_TEXT)
        }
        if (!store.hasUser(author)) {
            return Outcome.Err(ERR_UNKNOWN_USER)
        }
        return Outcome.Ok(store.appendTweet(author, text))
    }

    /** POST /tick: advances the clock by exactly 1 and returns it (D3). Total, never rejected. */
    @Synchronized
    fun tick(): Long = store.tick()

    /**
     * GET /timeline.
     *
     * Order: user syntax, user existence, limit, cursor. The raw limit and cursor arrive as
     * strings because which strings are legal is contract (D10) rather than wire format, and
     * putting the parse in the shim would move part of the pinned validation order out of the
     * verified core.
     */
    @Synchronized
    fun timeline(user: String, rawLimit: String?, rawCursor: String?): Outcome<Page> {
        if (!validHandle(user)) {
            return Outcome.Err(ERR_INVALID_HANDLE)
        }
        if (!store.hasUser(user)) {
            return Outcome.Err(ERR_UNKNOWN_USER)
        }

        var limit = DEFAULT_LIMIT.toLong()
        if (rawLimit != null) {
            val n = parseInt64(rawLimit)
            if (n == null || n < 1 || n > MAX_LIMIT) {
                return Outcome.Err(ERR_INVALID_LIMIT)
            }
            limit = n
        }

        var cursor: Long? = null
        if (rawCursor != null) {
            val n = parseInt64(rawCursor)
            if (n == null || n < 1) {
                return Outcome.Err(ERR_INVALID_CURSOR)
            }
            cursor = n
        }

        return Outcome.Ok(store.timelinePage(user, limit, cursor))
    }
}
