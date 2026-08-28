package twitterport.httpshim;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.ServerSocket;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;

import twitterport.dom.Dom;
import twitterport.dom.Page;
import twitterport.dom.Tweet;
import twitterport.dom.User;
import twitterport.service.Result;
import twitterport.service.Service;

/**
 * TRUSTED SHIM: the explicitly trusted HTTP boundary.
 *
 * <p>This file is transport, not contract. It decodes the request, calls one core function, and
 * encodes the answer. Handlers hold no semantics: no validation order, no error precedence, no
 * visibility rule, no pagination arithmetic. Putting any of that here would produce a green R0 over
 * code no verifier ever reads, and R5 would then prove nothing about observable behaviour -- see
 * TCB.md, and the live demonstration in step 1c where R0 climbed from 7/54 to 44/54 on shim changes
 * alone while three core patches had silently failed to apply.
 *
 * <p>The dispatch below mirrors {@code S_obs}'s {@code Step} exactly, on the same inputs: the raw
 * request target is cut at the first {@code '?'} and the resulting path is compared as a string,
 * with no percent-decoding and no URI normalisation. That is why {@code POST /%75sers} is
 * {@code not_found} here, and why the five body routes match only when there is <em>no</em> query
 * string, so {@code POST /users?x=1} is {@code not_found} rather than a create.
 *
 * <p>Every syntactically representable request has an answer. The default arm is part of the
 * contract, not a fallback.
 */
public final class Server {

    private static final String[] USER_FIELDS = {"handle"};
    private static final String[] EDGE_FIELDS = {"from", "to"};
    private static final String[] TWEET_FIELDS = {"author", "text"};

    private static final String JSON = "application/json";

    private final Service service = new Service();

    public static void start(String host, int port) throws IOException {
        Server self = new Server();
        ServerSocket listener = new ServerSocket();
        listener.setReuseAddress(true);
        listener.bind(new InetSocketAddress(host, port), 128);
        System.out.println("twitterport listening on " + host + ":" + port);
        Http.serve(listener, self::route);
    }

    // --- dispatch ------------------------------------------------------------

    private Http.Response route(Http.Request r) {
        try {
            String target = r.target();
            int q = target.indexOf('?');
            String path = q < 0 ? target : target.substring(0, q);
            String query = q < 0 ? null : target.substring(q + 1);
            boolean hasQuery = q >= 0;
            String method = r.method();
            String body = r.body();

            if (method.equals("POST") && path.equals("/users") && !hasQuery) {
                return createUser(body);
            }
            if (method.equals("POST") && path.equals("/follow") && !hasQuery) {
                return edge(body, true);
            }
            if (method.equals("DELETE") && path.equals("/follow") && !hasQuery) {
                return edge(body, false);
            }
            if (method.equals("POST") && path.equals("/tweets") && !hasQuery) {
                return postTweet(body);
            }
            if (method.equals("POST") && path.equals("/tick") && !hasQuery) {
                return tick(body);
            }
            if (method.equals("GET") && path.equals("/timeline")) {
                return timeline(query, hasQuery);
            }
            if (method.equals("GET") && path.equals("/healthz")) {
                // NOT part of S_obs, which answers not_found here. It exists because
                // impls/registry.json declares a health_path that the harness probes before a run.
                // The Go corner makes the same exception, in the same shape. It is the one
                // deliberate divergence in this implementation and it is listed as such.
                return new Http.Response(200, "text/plain", "ok\n".getBytes(StandardCharsets.UTF_8));
            }
            return json(404, Canon.errorBody(Dom.ERR_NOT_FOUND));
        } catch (RuntimeException e) {
            // The core throws only when one of its own invariants is broken -- see
            // Store.LogInvariantViolation. That is a defect in this implementation, not an answer
            // to the client, so say so loudly rather than dressing it up as a contract error code.
            e.printStackTrace();
            return json(500, "{\"error\":\"internal_error\"}");
        }
    }

    // --- routes --------------------------------------------------------------

    private Http.Response createUser(String body) {
        Map<String, String> f = Json.decodeStrictStrings(body, USER_FIELDS);
        if (f == null || !f.containsKey("handle")) {
            return malformed();
        }
        Result<User> r = service.createUser(f.get("handle"));
        if (r.isErr()) {
            return error(r.error());
        }
        return json(201, Canon.userBody(r.value()));
    }

    private Http.Response edge(String body, boolean add) {
        Map<String, String> f = Json.decodeStrictStrings(body, EDGE_FIELDS);
        if (f == null || !f.containsKey("from") || !f.containsKey("to")) {
            return malformed();
        }
        Result<Void> r =
                add
                        ? service.follow(f.get("from"), f.get("to"))
                        : service.unfollow(f.get("from"), f.get("to"));
        if (r.isErr()) {
            return error(r.error());
        }
        return new Http.Response(204, JSON, new byte[0]);
    }

    private Http.Response postTweet(String body) {
        Map<String, String> f = Json.decodeStrictStrings(body, TWEET_FIELDS);
        if (f == null || !f.containsKey("author") || !f.containsKey("text")) {
            return malformed();
        }
        Result<Tweet> r = service.postTweet(f.get("author"), f.get("text"));
        if (r.isErr()) {
            return error(r.error());
        }
        return json(201, Canon.tweetBody(r.value()));
    }

    /**
     * POST /tick. The body is compared raw: {@code S_obs} accepts an empty body or exactly
     * {@code {}} and nothing else -- not even {@code " {} "}, because it does not trim.
     */
    private Http.Response tick(String body) {
        if (!body.isEmpty() && !body.equals("{}")) {
            return malformed();
        }
        return json(200, Canon.clockBody(service.tick()));
    }

    private Http.Response timeline(String query, boolean hasQuery) {
        if (!hasQuery) {
            return malformed();
        }
        Map<String, List<String>> q = Query.parse(query);
        if (q == null) {
            return malformed();
        }
        // Unknown or repeated query parameters are rejected (D7). Repeated parameters are where Go
        // and Rust silently disagreed before the retarget -- finding F006.
        for (Map.Entry<String, List<String>> e : q.entrySet()) {
            String k = e.getKey();
            if (!k.equals("user") && !k.equals("limit") && !k.equals("cursor")) {
                return malformed();
            }
            if (e.getValue().size() != 1) {
                return malformed();
            }
        }
        List<String> user = q.get("user");
        if (user == null) {
            return malformed();
        }
        List<String> limit = q.get("limit");
        List<String> cursor = q.get("cursor");
        Result<Page> r =
                service.timeline(
                        user.get(0),
                        limit == null ? null : limit.get(0),
                        cursor == null ? null : cursor.get(0));
        if (r.isErr()) {
            return error(r.error());
        }
        return json(200, Canon.timelineBody(r.value()));
    }

    // --- responses -----------------------------------------------------------

    private static Http.Response malformed() {
        return json(400, Canon.errorBody(Dom.ERR_MALFORMED_REQUEST));
    }

    /** Status lookup, not a renaming: the core already speaks the observable vocabulary. */
    private static Http.Response error(String code) {
        int status = code.equals(Dom.ERR_HANDLE_TAKEN) ? 409 : 400;
        return json(status, Canon.errorBody(code));
    }

    private static Http.Response json(int status, String body) {
        // Exactly these bytes. No trailing newline: under a byte-equality conformance rule a
        // trailing newline is a real observable difference (D8), and it accounted for 8 of the 54
        // R0 baseline steps in the Go corner.
        return new Http.Response(status, JSON, body.getBytes(StandardCharsets.UTF_8));
    }
}
