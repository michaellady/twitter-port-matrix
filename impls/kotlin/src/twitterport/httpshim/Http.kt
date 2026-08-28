package twitterport.httpshim

import java.io.BufferedInputStream
import java.io.BufferedOutputStream
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.net.ServerSocket
import java.net.Socket
import java.util.Locale
import java.util.concurrent.Executors
import twitterport.dom.ERR_MALFORMED_REQUEST

/**
 * TRUSTED SHIM: HTTP/1.1 framing, by hand, on a raw socket.
 *
 * **Why this is not `com.sun.net.httpserver`, and why Kotlin does not change the answer.** The
 * JDK's built-in server parses the request target with `java.net.URI` before any handler runs, and
 * answers anything that class rejects with its own `<h1>400 Bad Request</h1>URISyntaxException
 * thrown` page. Go's `net/http` client will happily put all of these on the wire, and `S_obs` has
 * a defined answer for every one of them:
 * ```
 *   /timeline?user=bob&limit=%zz   S_obs: malformed_request   JDK: HTML 400
 *   /timeline?user={bob}           S_obs: invalid_handle      JDK: HTML 400
 *   /timeline?user=bob|alice       S_obs: invalid_handle      JDK: HTML 400
 *   /timeline?user=a^b             S_obs: invalid_handle      JDK: HTML 400
 * ```
 * That is a hole in totality, and totality is the first design point of this corner: every
 * syntactically representable request has a defined response. The hole cannot be closed from
 * inside the JDK server -- a `Filter` runs after the URI has already been parsed and rejected --
 * so the framing is done here instead, and the request target is handed to the router as the exact
 * bytes that arrived. That is also the more honest split: `S_obs`'s `Step` works on a raw path
 * string, not on a parsed URI.
 *
 * Kotlin brings no HTTP server of its own. Ktor is the ecosystem's answer and it is a Gradle
 * dependency with a coroutine runtime attached; on the evidence of F008 -- where `net/http`'s
 * `ServeMux` and `axum`'s router each independently produced the *same* routing defect by
 * percent-decoding the path and ignoring the query -- reaching for a framework is exactly the move
 * that cost the other two corners four divergences apiece. This corner is deliberately built the
 * way the Java corner was, for that reason and not for austerity.
 *
 * The cost is stated rather than hidden: this file is about 150 lines of trusted transport that a
 * framework would otherwise have provided. It buys exact control over the one thing the contract
 * cares about at this layer -- the bytes in and the bytes out.
 */
object Http {

    /** A request, as it arrived. [target] is raw: undecoded, query included. */
    data class Request(val method: String, val target: String, val body: String)

    /** A response. [body] is the exact byte sequence to send, with no trailing newline. */
    class Response(val status: Int, val contentType: String, val body: ByteArray)

    /** Bodies larger than this are refused. The contract's largest legal body is a few hundred bytes. */
    private const val MAX_BODY = 8 * 1024 * 1024

    private const val MAX_LINE = 64 * 1024
    private const val MAX_HEADERS = 100

    /** Accepts forever on [listener], serving each connection with [handler]. */
    fun serve(listener: ServerSocket, handler: (Request) -> Response) {
        val pool =
            Executors.newCachedThreadPool { r ->
                val t = Thread(r, "conn")
                t.isDaemon = true
                t
            }
        while (true) {
            val sock =
                try {
                    listener.accept()
                } catch (e: IOException) {
                    return
                }
            pool.execute { connection(sock, handler) }
        }
    }

    private fun connection(sock: Socket, handler: (Request) -> Response) {
        try {
            sock.use { s ->
                s.tcpNoDelay = true
                s.soTimeout = 300_000
                val input = BufferedInputStream(s.getInputStream(), 8192)
                val out = BufferedOutputStream(s.getOutputStream(), 8192)
                while (exchange(input, out, handler)) {
                    out.flush()
                }
                out.flush()
            }
        } catch (e: IOException) {
            // A client that disappears mid-request is not an observable event.
        }
    }

    /** Handles one request/response pair. Returns true if the connection may be reused. */
    private fun exchange(
        input: InputStream,
        out: OutputStream,
        handler: (Request) -> Response,
    ): Boolean {
        var line = readHeaderLine(input) ?: return false // clean EOF between requests
        while (line.isEmpty()) { // tolerate stray CRLF before a request line
            line = readHeaderLine(input) ?: return false
        }

        // The request target is everything between the FIRST space and the LAST space.
        //
        // For a well-formed request line those are the only two spaces and this is the ordinary
        // split. It matters for the ill-formed ones: S_obs's request alphabet contains paths
        // holding a raw space -- "/timeline?user=bob&limit=5 " has a defined answer of
        // invalid_limit -- and Go's client puts that space on the wire unencoded, producing a
        // request line with three spaces. Splitting at the second space would silently truncate
        // the target and answer a different question from the one the oracle was asked.
        val sp1 = line.indexOf(' ')
        val sp2 = line.lastIndexOf(' ')
        if (sp1 <= 0 || sp2 <= sp1) {
            writeResponse(out, badRequest(), false)
            return false
        }
        val method = line.substring(0, sp1)
        val target = line.substring(sp1 + 1, sp2)
        val version = line.substring(sp2 + 1)

        val headers = HashMap<String, String>()
        var n = 0
        while (true) {
            val h = readHeaderLine(input) ?: return false
            if (h.isEmpty()) {
                break
            }
            if (n >= MAX_HEADERS) {
                writeResponse(out, badRequest(), false)
                return false
            }
            val colon = h.indexOf(':')
            if (colon > 0) {
                headers[h.substring(0, colon).lowercase(Locale.ROOT)] =
                    h.substring(colon + 1).trim()
            }
            n++
        }

        val body: ByteArray?
        val te = headers["transfer-encoding"]
        if (te != null && te.lowercase(Locale.ROOT).contains("chunked")) {
            body = readChunked(input)
        } else {
            var len = 0L
            val cl = headers["content-length"]
            if (cl != null) {
                len = cl.trim().toLongOrNull()
                    ?: run {
                        writeResponse(out, badRequest(), false)
                        return false
                    }
            }
            if (len < 0 || len > MAX_BODY) {
                writeResponse(out, badRequest(), false)
                return false
            }
            body = readExactly(input, len.toInt())
        }
        if (body == null) {
            return false // truncated body: the client went away
        }

        var keepAlive = version == "HTTP/1.1"
        val conn = headers["connection"]
        if (conn != null && conn.lowercase(Locale.ROOT).contains("close")) {
            keepAlive = false
        }

        val resp = handler(Request(method, target, String(body, Charsets.UTF_8)))
        writeResponse(out, resp, keepAlive)
        return keepAlive
    }

    /** The one answer this layer produces on its own: a request that is not HTTP at all. */
    private fun badRequest(): Response =
        Response(
            400,
            "application/json",
            errorBody(ERR_MALFORMED_REQUEST).toByteArray(Charsets.UTF_8),
        )

    private fun writeResponse(out: OutputStream, r: Response, keepAlive: Boolean) {
        val head = StringBuilder(128)
        head.append("HTTP/1.1 ").append(r.status).append(' ').append(reason(r.status))
        head.append("\r\n")
        if (r.status != 204) {
            head.append("Content-Type: ").append(r.contentType).append("\r\n")
            head.append("Content-Length: ").append(r.body.size).append("\r\n")
        }
        head.append("Connection: ").append(if (keepAlive) "keep-alive" else "close").append("\r\n")
        head.append("\r\n")
        out.write(head.toString().toByteArray(Charsets.ISO_8859_1))
        if (r.status != 204) {
            out.write(r.body)
        }
        out.flush()
    }

    private fun reason(status: Int): String =
        when (status) {
            200 -> "OK"
            201 -> "Created"
            204 -> "No Content"
            400 -> "Bad Request"
            404 -> "Not Found"
            409 -> "Conflict"
            else -> "Internal Server Error"
        }

    /**
     * Reads one CRLF- or LF-terminated line as raw bytes.
     *
     * ISO-8859-1 keeps the byte-to-char mapping one-to-one, so the request target reaches the
     * router as exactly the bytes that arrived. `S_obs` compares raw path strings, so any decoding
     * here would be a behaviour change disguised as a convenience.
     *
     * Named `readHeaderLine` rather than `readLine` because `kotlin.io.readLine()` is a
     * default-imported top-level function that reads standard input. A member named `readLine`
     * would resolve correctly by arity here and shadow confusingly everywhere else.
     */
    private fun readHeaderLine(input: InputStream): String? {
        val buf = ByteArrayOutputStream(128)
        while (true) {
            val c = input.read()
            if (c < 0) {
                return if (buf.size() == 0) null else String(buf.toByteArray(), Charsets.ISO_8859_1)
            }
            if (c == '\n'.code) {
                val b = buf.toByteArray()
                var len = b.size
                if (len > 0 && b[len - 1] == '\r'.code.toByte()) {
                    len--
                }
                return String(b, 0, len, Charsets.ISO_8859_1)
            }
            if (buf.size() >= MAX_LINE) {
                return null
            }
            buf.write(c)
        }
    }

    private fun readExactly(input: InputStream, n: Int): ByteArray? {
        val b = ByteArray(n)
        var off = 0
        while (off < n) {
            val got = input.read(b, off, n - off)
            if (got < 0) {
                return null
            }
            off += got
        }
        return b
    }

    /**
     * Chunked request bodies. Go's client never uses them here -- every harness body is a sized
     * byte reader -- but a framing layer that answers "some requests" is not a framing layer.
     */
    private fun readChunked(input: InputStream): ByteArray? {
        val body = ByteArrayOutputStream()
        while (true) {
            var line = readHeaderLine(input) ?: return null
            val semi = line.indexOf(';')
            if (semi >= 0) {
                line = line.substring(0, semi)
            }
            val size = line.trim().toIntOrNull(16) ?: return null
            if (size < 0 || body.size() + size > MAX_BODY) {
                return null
            }
            if (size == 0) {
                while (true) { // trailers
                    val t = readHeaderLine(input)
                    if (t == null || t.isEmpty()) {
                        break
                    }
                }
                return body.toByteArray()
            }
            val chunk = readExactly(input, size) ?: return null
            body.write(chunk, 0, chunk.size)
            readHeaderLine(input) // trailing CRLF
        }
    }
}
