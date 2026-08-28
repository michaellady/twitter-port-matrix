@file:JvmName("Dom")

package twitterport.dom

/**
 * VERIFIED CORE.
 *
 * Bounds, the error vocabulary, and the two syntax predicates of `S_obs`. Nothing here touches
 * HTTP, JSON, or any wire concern: this file is the part of the Kotlin corner a verifier is
 * expected to read (see TCB.md, "the verified-core / trusted-shim boundary").
 *
 * Declared as top-level functions with `@file:JvmName("Dom")` rather than as an `object`, so the
 * compiled bytecode is `twitterport.dom.Dom` holding plain `static` methods with no `INSTANCE`
 * receiver. A Kotlin `object` compiles to a singleton whose methods take an implicit `this`, which
 * a bytecode-level checker has to model before it can say anything about `validHandle`. The
 * verified core is the one place where the shape of the emitted bytecode is worth choosing
 * deliberately.
 *
 * Every length and character test operates on UTF-8 **bytes**, not on Kotlin `Char`s. `S_obs` is
 * written in Go, where `len(s)` is a byte count and `s[i]` is a byte; a Kotlin implementation that
 * measured `String.length` would accept a 280-code-point tweet that `S_obs` rejects, because
 * `String.length` counts UTF-16 code units. That is a real observable divergence, so it is closed
 * here rather than left to chance. Kotlin inherits this trap from Java unchanged.
 */

// Bounds on the observable surface. Part of the contract: an implementation that accepts a
// 300-character tweet does not refine S_obs.
const val MAX_HANDLE_LEN = 32
const val MAX_TEXT_LEN = 280
const val MIN_TEXT_LEN = 1
const val DEFAULT_LIMIT = 50
const val MAX_LIMIT = 100

// Error codes. Exactly this set; no implementation may invent another.
const val ERR_MALFORMED_REQUEST = "malformed_request"
const val ERR_INVALID_HANDLE = "invalid_handle"
const val ERR_INVALID_TEXT = "invalid_text"
const val ERR_INVALID_LIMIT = "invalid_limit"
const val ERR_INVALID_CURSOR = "invalid_cursor"
const val ERR_UNKNOWN_USER = "unknown_user"
const val ERR_SELF_FOLLOW = "self_follow_forbidden"
const val ERR_HANDLE_TAKEN = "handle_taken"
const val ERR_NOT_FOUND = "not_found"

/**
 * Accepts 1..[MAX_HANDLE_LEN] bytes of `[a-z0-9_]`. Deliberately narrow: a narrow alphabet is a
 * narrow divergence surface.
 */
fun validHandle(h: String): Boolean {
    val b = h.toByteArray(Charsets.UTF_8)
    if (b.isEmpty() || b.size > MAX_HANDLE_LEN) {
        return false
    }
    for (raw in b) {
        val c = raw.toInt() and 0xFF
        val ok = (c >= 'a'.code && c <= 'z'.code) ||
            (c >= '0'.code && c <= '9'.code) ||
            c == '_'.code
        if (!ok) {
            return false
        }
    }
    return true
}

/** Accepts [MIN_TEXT_LEN]..[MAX_TEXT_LEN] bytes, no control characters. */
fun validText(t: String): Boolean {
    val b = t.toByteArray(Charsets.UTF_8)
    if (b.size < MIN_TEXT_LEN || b.size > MAX_TEXT_LEN) {
        return false
    }
    for (raw in b) {
        if ((raw.toInt() and 0xFF) < 0x20) {
            return false
        }
    }
    return true
}

/**
 * Base-10 signed 64-bit parse with exactly Go's `strconv.ParseInt(s, 10, 64)` accept set.
 *
 * Which strings are legal `limit` and `cursor` values is contract, not wire format, so this lives
 * in the core. It is hand-written rather than delegated to Kotlin's `String.toLongOrNull()` for
 * the same reason the Java corner does not use `Long.parseLong`, and the reason survives the
 * change of language: `toLongOrNull` resolves each character through `Character.digit`, which
 * accepts non-ASCII decimal digits. `"١".toLongOrNull()` is `1` in Kotlin and an
 * `invalid_limit` in `S_obs`, so `?limit=١` would have been a silent divergence.
 *
 * Kotlin's nullable return type carries "not a number" without a sentinel or a boxed `Long`
 * out-parameter, which is the one place in this file where the language helps rather than
 * matching Java exactly.
 *
 * @return the parsed value, or null when the string is not a legal base-10 int64
 */
fun parseInt64(s: String): Long? {
    if (s.isEmpty()) {
        return null
    }
    var i = 0
    var neg = false
    val sign = s[0]
    if (sign == '+' || sign == '-') {
        neg = sign == '-'
        i = 1
        if (s.length == 1) {
            return null
        }
    }
    var acc = 0L
    while (i < s.length) {
        val c = s[i]
        if (c < '0' || c > '9') {
            return null
        }
        val d = c - '0'
        // Range error. Go reports out-of-range as an error too; every value that overflows is
        // outside both the limit and the cursor windows, so the observable answer is identical.
        if (acc > (Long.MAX_VALUE - d) / 10) {
            return null
        }
        acc = acc * 10 + d
        i++
    }
    return if (neg) -acc else acc
}
