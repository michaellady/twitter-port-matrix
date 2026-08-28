@file:JvmName("Canon")

package twitterport.httpshim

import twitterport.dom.Page
import twitterport.dom.Tweet
import twitterport.dom.User

/**
 * TRUSTED SHIM. Canonical response encoding (D8).
 *
 * Byte-identical replay across four languages needs the encoding pinned, not left to each
 * language's default JSON writer. The rules:
 * ```
 *   R1  Object keys appear in the fixed order declared by each writer below, NOT alphabetically.
 *       The order is part of the contract.
 *   R2  No insignificant whitespace.
 *   R3  Integers only. No floating point anywhere in the response surface.
 *   R4  Strings escape only what RFC 8259 requires, plus the short forms for backspace, form
 *       feed, newline, carriage return and tab. No escaping of non-ASCII, no HTML escaping of
 *       < > &.
 *   R5  null is written literally as null, never omitted.
 * ```
 *
 * And no trailing newline: under a byte-equality conformance rule a trailing newline is a real
 * observable difference, and it accounted for 8 of the 54 R0 baseline steps in the Go corner.
 *
 * Hand-rolled rather than delegated to a serialization library on purpose, and in Kotlin the
 * decision is not even close: the language has no JSON in its standard library at all, so the
 * alternative is `kotlinx.serialization` -- a compiler-plugin dependency with its own opinions
 * about key order and null omission, pulled in to emit strings this file emits in forty lines.
 */

private val HEX = "0123456789abcdef".toCharArray()

internal fun encodeString(sb: StringBuilder, s: String) {
    sb.append('"')
    for (c in s) {
        when (c) {
            '"' -> sb.append("\\\"")
            '\\' -> sb.append("\\\\")
            '\b' -> sb.append("\\b")
            // Kotlin has no '\f' escape -- its escape set is \t \b \n \r \' \" \\ \$ \uXXXX,
            // and form feed is simply missing from it. Written as the code point rather than
            // as an invisible literal form-feed byte in the source.
            '\u000C' -> sb.append("\\f")
            '\n' -> sb.append("\\n")
            '\r' -> sb.append("\\r")
            '\t' -> sb.append("\\t")
            else ->
                if (c.code < 0x20) {
                    sb.append("\\u00").append(HEX[(c.code shr 4) and 0xF]).append(HEX[c.code and 0xF])
                } else {
                    sb.append(c)
                }
        }
    }
    sb.append('"')
}

/** `{"error":"<code>"}` */
fun errorBody(code: String): String {
    val sb = StringBuilder()
    sb.append("{\"error\":")
    encodeString(sb, code)
    sb.append('}')
    return sb.toString()
}

/** `{"handle":"<h>","id":<n>}` */
fun userBody(u: User): String {
    val sb = StringBuilder()
    sb.append("{\"handle\":")
    encodeString(sb, u.handle)
    sb.append(",\"id\":").append(u.id).append('}')
    return sb.toString()
}

/** `{"id":<n>,"author":"<h>","text":"<t>","created_at":<n>}` */
private fun tweetObject(sb: StringBuilder, t: Tweet) {
    sb.append("{\"id\":").append(t.id).append(",\"author\":")
    encodeString(sb, t.author)
    sb.append(",\"text\":")
    encodeString(sb, t.text)
    sb.append(",\"created_at\":").append(t.createdAt).append('}')
}

fun tweetBody(t: Tweet): String {
    val sb = StringBuilder()
    tweetObject(sb, t)
    return sb.toString()
}

/** `{"tweets":[...],"next_cursor":<n>|null}` */
fun timelineBody(p: Page): String {
    val sb = StringBuilder()
    sb.append("{\"tweets\":[")
    for (i in p.tweets.indices) {
        if (i > 0) {
            sb.append(',')
        }
        tweetObject(sb, p.tweets[i])
    }
    sb.append("],\"next_cursor\":")
    val nc = p.nextCursor
    if (nc == null) {
        sb.append("null")
    } else {
        // `nc` is smart-cast to a non-null Long by the null check above, so this resolves to
        // StringBuilder.append(long) -- the primitive overload -- rather than append(Any?) and a
        // boxed toString. The Java corner had to write `.longValue()` to say the same thing.
        sb.append(nc)
    }
    sb.append('}')
    return sb.toString()
}

/** `{"clock":<n>}` */
fun clockBody(n: Long): String = "{\"clock\":$n}"
