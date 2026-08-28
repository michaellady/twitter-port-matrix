@file:JvmName("Query")

package twitterport.httpshim

import java.io.ByteArrayOutputStream

/**
 * TRUSTED SHIM. Query-string parsing, ported from Go's `net/url.ParseQuery`.
 *
 * Repeated parameters are the one place Go and Rust silently disagreed before the retarget
 * (finding F006): `GET /timeline?user=bob&user=alice` served Go a 200 with five tweets where Rust
 * returned 400. Neither answer contradicted `twitter.tla`. That is why this returns a multimap and
 * lets the caller reject any key that arrived more than once, instead of taking "first wins" or
 * "last wins" from whatever the platform's query API happens to do.
 *
 * Kotlin inherits the JDK here and gains nothing: `java.net.URI` still offers no query parser, and
 * the Kotlin standard library adds none. There is no `kotlin.net` -- the closest thing on the
 * shelf is `URLDecoder.decode`, which is worse than useless for this contract because it decodes
 * a malformed escape by throwing `IllegalArgumentException` on some inputs and silently passing
 * others through, where `S_obs` needs one uniform `malformed_request`.
 *
 * Go's parser is reproduced exactly, including its two quirks: a segment containing `;` is an
 * error, and empty segments are skipped.
 */

/**
 * @return the decoded parameters in first-seen key order, or null if the query is malformed
 */
fun parseQuery(query: String): Map<String, List<String>>? {
    val out = LinkedHashMap<String, MutableList<String>>()
    var rest = query
    while (rest.isNotEmpty()) {
        val segment: String
        val amp = rest.indexOf('&')
        if (amp >= 0) {
            segment = rest.substring(0, amp)
            rest = rest.substring(amp + 1)
        } else {
            segment = rest
            rest = ""
        }
        if (segment.indexOf(';') >= 0) {
            // Go: "invalid semicolon separator in query". It keeps scanning and returns the error
            // at the end; S_obs rejects on any error, so stopping here is equivalent.
            return null
        }
        if (segment.isEmpty()) {
            continue
        }
        var key = segment
        var value = ""
        val eq = segment.indexOf('=')
        if (eq >= 0) {
            key = segment.substring(0, eq)
            value = segment.substring(eq + 1)
        }
        val dk = unescape(key) ?: return null
        val dv = unescape(value) ?: return null
        out.getOrPut(dk) { ArrayList() }.add(dv)
    }
    return out
}

/** Go's `url.QueryUnescape`: `+` is a space, `%XX` must be valid hex. */
private fun unescape(s: String): String? {
    val buf = ByteArrayOutputStream(s.length)
    var i = 0
    while (i < s.length) {
        val c = s[i]
        if (c == '%') {
            if (i + 2 >= s.length) {
                return null
            }
            val hi = hexDigit(s[i + 1])
            val lo = hexDigit(s[i + 2])
            if (hi < 0 || lo < 0) {
                return null
            }
            buf.write((hi shl 4) or lo)
            i += 2
        } else if (c == '+') {
            buf.write(' '.code)
        } else if (c.code < 0x100) {
            // The raw target reaches here byte-per-char (ISO-8859-1), exactly as Go's parser sees
            // it, so a byte is written back as a byte rather than re-encoded.
            buf.write(c.code)
        } else {
            val b = c.toString().toByteArray(Charsets.UTF_8)
            buf.write(b, 0, b.size)
        }
        i++
    }
    return String(buf.toByteArray(), Charsets.UTF_8)
}

private fun hexDigit(c: Char): Int =
    when (c) {
        in '0'..'9' -> c - '0'
        in 'a'..'f' -> c - 'a' + 10
        in 'A'..'F' -> c - 'A' + 10
        else -> -1
    }
