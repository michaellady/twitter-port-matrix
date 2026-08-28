package twitterport.httpshim

import java.net.InetSocketAddress
import java.net.ServerSocket
import twitterport.dom.ERR_HANDLE_TAKEN
import twitterport.dom.ERR_MALFORMED_REQUEST
import twitterport.dom.ERR_NOT_FOUND
import twitterport.service.Outcome
import twitterport.service.Service

/**
 * TRUSTED SHIM: the explicitly trusted HTTP boundary.
 *
 * This file is transport, not contract. It decodes the request, calls one core function, and
 * encodes the answer. Handlers hold no semantics: no validation order, no error precedence, no
 * visibility rule, no pagination arithmetic. Putting any of that here would produce a green R0
 * over code no verifier ever reads, and R5 would then prove nothing about observable behaviour --
 * see TCB.md, and the live demonstration in step 1c where R0 climbed from 7/54 to 44/54 on shim
 * changes alone while three core patches had silently failed to apply.
 *
 * The dispatch below mirrors `S_obs`'s `Step` exactly, on the same inputs: the raw request target
 * is cut at the first `?` and the resulting path is compared as a string, with no percent-decoding
 * and no URI normalisation. That is why `POST /%75sers` is `not_found` here, and why the five body
 * routes match only when there is *no* query string, so `POST /users?x=1` is `not_found` rather
 * than a create. Both of those were live defects in the Go and Rust corners, produced by their
 * routers rather than by their authors (F008).
 *
 * Every syntactically representable request has an answer. The default arm is part of the
 * contract, not a fallback.
 */
class Server private constructor() {

    private val service = Service()

    companion object {
        private val USER_FIELDS = arrayOf("handle")
        private val EDGE_FIELDS = arrayOf("from", "to")
        private val TWEET_FIELDS = arrayOf("author", "text")

        private const val JSON = "application/json"

        fun start(host: String, port: Int) {
            val self = Server()
            val listener = ServerSocket()
            listener.reuseAddress = true
            listener.bind(InetSocketAddress(host, port), 128)
            println("twitterport listening on $host:$port")
            Http.serve(listener, self::route)
        }

        private fun malformed(): Http.Response = json(400, errorBody(ERR_MALFORMED_REQUEST))

        /** Status lookup, not a renaming: the core already speaks the observable vocabulary. */
        private fun error(code: String): Http.Response =
            json(if (code == ERR_HANDLE_TAKEN) 409 else 400, errorBody(code))

        private fun json(status: Int, body: String): Http.Response =
            // Exactly these bytes. No trailing newline: under a byte-equality conformance rule a
            // trailing newline is a real observable difference (D8), and it accounted for 8 of the
            // 54 R0 baseline steps in the Go corner.
            Http.Response(status, JSON, body.toByteArray(Charsets.UTF_8))
    }

    // --- dispatch ------------------------------------------------------------

    private fun route(r: Http.Request): Http.Response {
        try {
            val target = r.target
            val q = target.indexOf('?')
            val path = if (q < 0) target else target.substring(0, q)
            val query = if (q < 0) null else target.substring(q + 1)
            val hasQuery = q >= 0
            val method = r.method
            val body = r.body

            if (method == "POST" && path == "/users" && !hasQuery) {
                return createUser(body)
            }
            if (method == "POST" && path == "/follow" && !hasQuery) {
                return edge(body, true)
            }
            if (method == "DELETE" && path == "/follow" && !hasQuery) {
                return edge(body, false)
            }
            if (method == "POST" && path == "/tweets" && !hasQuery) {
                return postTweet(body)
            }
            if (method == "POST" && path == "/tick" && !hasQuery) {
                return tick(body)
            }
            if (method == "GET" && path == "/timeline") {
                return timeline(query, hasQuery)
            }
            if (method == "GET" && path == "/healthz") {
                // NOT part of S_obs, which answers not_found here. It exists because
                // impls/registry.json declares a health_path that the harness probes before a run.
                // The Go and Java corners make the same exception, in the same shape. It is the
                // one deliberate divergence in this implementation and it is listed as such.
                return Http.Response(200, "text/plain", "ok\n".toByteArray(Charsets.UTF_8))
            }
            return json(404, errorBody(ERR_NOT_FOUND))
        } catch (e: RuntimeException) {
            // The core throws only when one of its own invariants is broken -- see
            // LogInvariantViolation. That is a defect in this implementation, not an answer to the
            // client, so say so loudly rather than dressing it up as a contract error code.
            e.printStackTrace()
            return json(500, "{\"error\":\"internal_error\"}")
        }
    }

    // --- routes --------------------------------------------------------------

    private fun createUser(body: String): Http.Response {
        val f = decodeStrictStrings(body, USER_FIELDS)
        val handle = f?.get("handle") ?: return malformed()
        return when (val r = service.createUser(handle)) {
            is Outcome.Err -> error(r.code)
            is Outcome.Ok -> json(201, userBody(r.value))
        }
    }

    private fun edge(body: String, add: Boolean): Http.Response {
        val f = decodeStrictStrings(body, EDGE_FIELDS) ?: return malformed()
        val from = f["from"] ?: return malformed()
        val to = f["to"] ?: return malformed()
        val r = if (add) service.follow(from, to) else service.unfollow(from, to)
        return when (r) {
            is Outcome.Err -> error(r.code)
            is Outcome.Ok -> Http.Response(204, JSON, ByteArray(0))
        }
    }

    private fun postTweet(body: String): Http.Response {
        val f = decodeStrictStrings(body, TWEET_FIELDS) ?: return malformed()
        val author = f["author"] ?: return malformed()
        val text = f["text"] ?: return malformed()
        return when (val r = service.postTweet(author, text)) {
            is Outcome.Err -> error(r.code)
            is Outcome.Ok -> json(201, tweetBody(r.value))
        }
    }

    /**
     * POST /tick. The body is compared raw: `S_obs` accepts an empty body or exactly `{}` and
     * nothing else -- not even `" {} "`, because it does not trim.
     */
    private fun tick(body: String): Http.Response {
        if (body.isNotEmpty() && body != "{}") {
            return malformed()
        }
        return json(200, clockBody(service.tick()))
    }

    private fun timeline(query: String?, hasQuery: Boolean): Http.Response {
        if (!hasQuery || query == null) {
            return malformed()
        }
        val q = parseQuery(query) ?: return malformed()
        // Unknown or repeated query parameters are rejected (D7). Repeated parameters are where Go
        // and Rust silently disagreed before the retarget -- finding F006.
        for ((k, v) in q) {
            if (k != "user" && k != "limit" && k != "cursor") {
                return malformed()
            }
            if (v.size != 1) {
                return malformed()
            }
        }
        val user = q["user"] ?: return malformed()
        val limit = q["limit"]
        val cursor = q["cursor"]
        return when (
            val r = service.timeline(user[0], limit?.get(0), cursor?.get(0))
        ) {
            is Outcome.Err -> error(r.code)
            is Outcome.Ok -> json(200, timelineBody(r.value))
        }
    }
}
