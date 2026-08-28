@file:JvmName("Json")

package twitterport.httpshim

/**
 * TRUSTED SHIM. Strict request decoding (D7).
 *
 * Exactly one JSON object, no unknown fields, no trailing content. Lenient parsing is a classic
 * source of cross-language divergence -- it is precisely where two implementations accept
 * different inputs and both look correct -- so this rejects rather than guesses. Ten of the 54 R0
 * baseline steps in the original corners failed here.
 *
 * Written by hand rather than delegated to a parser. Kotlin has no JSON in its standard library at
 * all, so unlike the Go and Rust corners there was never a convenient wrong answer within reach:
 * the alternative was adding `kotlinx.serialization`, which is a compiler plugin plus a runtime
 * dependency and brings its own opinions about unknown fields and trailing content -- exactly the
 * class of inherited library opinion that F008 and F010 are both about.
 *
 * Two behaviours are pinned deliberately and are worth stating rather than discovering later:
 *
 *  1. **Duplicate keys resolve last-wins.** Documented in D7 as a known limitation.
 *  2. **Field names match case-sensitively.** See [matchField] and finding F010: the reverse was
 *     the reference machine's behaviour until it was corrected, and it is the trap this corner was
 *     written into rather than out of. `tracegen`'s hostile pool emits `{"Handle":...}`,
 *     `{"From":...}` and `{"TEXT":...}`, so R1 catches getting this wrong.
 */

/** Sentinel for a JSON null, so a present-but-null key is distinguishable from an absent one. */
private val JSON_NULL = Any()

/** A JSON number. Never a legal value for any field of this API, but it must still parse. */
private val JSON_NUMBER = Any()

/**
 * Stackless control-flow signal for a parse failure.
 *
 * `RuntimeException(message, cause, enableSuppression, writableStackTrace)` is `protected` in
 * Java; Kotlin can call it from a subclass constructor, which is what makes a single shared
 * throwable with no stack capture possible here. Filling in a stack trace per malformed body
 * would be the dominant cost of the hostile 25% of every R1 trace.
 */
private class ParseError : RuntimeException(null, null, false, false)

private val FAIL = ParseError()

/** A JSON object kept as ordered key/value pairs, so duplicates and order are both preserved. */
private class Obj {
    val keys = ArrayList<String>()
    val values = ArrayList<Any>()
}

/**
 * Decodes exactly one JSON object whose fields are all drawn from [fields] and whose values are
 * all JSON strings or null.
 *
 * @return a map from field name to value, omitting fields whose value was JSON null (which leaves
 *   the corresponding pointer nil in `S_obs`, and therefore fails the required-field check in the
 *   caller); or null if the body is not acceptable at all
 */
fun decodeStrictStrings(body: String, fields: Array<String>): Map<String, String>? {
    val v: Any
    val p = Parser(body)
    try {
        p.skipWhitespace()
        if (p.atEnd()) {
            // Go returns io.EOF from Decode on an empty body.
            return null
        }
        v = p.value()
        p.skipWhitespace()
        if (!p.atEnd()) {
            // Trailing content after the JSON value.
            return null
        }
    } catch (e: ParseError) {
        return null
    }
    if (v !is Obj) {
        // A non-object top level cannot populate a required field, so the answer is the same
        // malformed_request either way: a type error for scalars and arrays, and a no-op decode
        // followed by a missing required field for a literal null.
        return null
    }
    val out = HashMap<String, String>()
    for (i in v.keys.indices) {
        val field = matchField(v.keys[i], fields) ?: return null // unknown field
        val value = v.values[i]
        if (value === JSON_NULL) {
            out.remove(field) // last-wins, and a null leaves the field unset
            continue
        }
        if (value !is String) {
            return null // type error: this API has only string-valued fields
        }
        out[field] = value
    }
    return out
}

/**
 * Exact, case-sensitive match. `{"Handle":"dave"}` is an unknown field and the body is rejected.
 *
 * This is where finding F010 lands. A case-INSENSITIVE fallback here would have been the natural
 * thing to write, because it is what Go's `encoding/json` does and `S_obs` is a Go program: its
 * decoder resolves `Handle` to the `handle` field, so `DisallowUnknownFields` never fires. The Go
 * corner satisfied that for free, `serde`'s `deny_unknown_fields` rejected it, and the two corners
 * disagreed observably while three rungs called both of them green. `S_obs` now does the
 * case-sensitive comparison explicitly, so the written contract (D7: unknown fields are rejected)
 * wins over a library default nobody chose. Matching it is one loop; the reason is the finding.
 */
private fun matchField(key: String, fields: Array<String>): String? {
    for (f in fields) {
        if (f == key) {
            return f
        }
    }
    return null
}

// --- the parser --------------------------------------------------------------

private class Parser(private val s: String) {
    private var i = 0

    fun atEnd(): Boolean = i >= s.length

    fun skipWhitespace() {
        while (i < s.length) {
            val c = s[i]
            if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                i++
            } else {
                return
            }
        }
    }

    private fun peek(): Char {
        if (i >= s.length) {
            throw FAIL
        }
        return s[i]
    }

    private fun expect(c: Char) {
        if (i >= s.length || s[i] != c) {
            throw FAIL
        }
        i++
    }

    private fun literal(lit: String) {
        if (!s.startsWith(lit, i)) {
            throw FAIL
        }
        i += lit.length
    }

    fun value(): Any =
        when (peek()) {
            '{' -> obj()
            '[' -> array()
            '"' -> string()
            't' -> {
                literal("true")
                true
            }
            'f' -> {
                literal("false")
                false
            }
            'n' -> {
                literal("null")
                JSON_NULL
            }
            else -> number()
        }

    private fun obj(): Obj {
        expect('{')
        val o = Obj()
        skipWhitespace()
        if (peek() == '}') {
            i++
            return o
        }
        while (true) {
            skipWhitespace()
            if (peek() != '"') {
                throw FAIL
            }
            val k = string()
            skipWhitespace()
            expect(':')
            skipWhitespace()
            val v = value()
            o.keys.add(k)
            o.values.add(v)
            skipWhitespace()
            when (peek()) {
                ',' -> i++
                '}' -> {
                    i++
                    return o
                }
                else -> throw FAIL
            }
        }
    }

    private fun array(): List<Any> {
        expect('[')
        val out = ArrayList<Any>()
        skipWhitespace()
        if (peek() == ']') {
            i++
            return out
        }
        while (true) {
            skipWhitespace()
            out.add(value())
            skipWhitespace()
            when (peek()) {
                ',' -> i++
                ']' -> {
                    i++
                    return out
                }
                else -> throw FAIL
            }
        }
    }

    fun string(): String {
        expect('"')
        val sb = StringBuilder()
        while (true) {
            if (i >= s.length) {
                throw FAIL
            }
            val c = s[i]
            if (c == '"') {
                i++
                return sb.toString()
            }
            if (c.code < 0x20) {
                throw FAIL // unescaped control character
            }
            if (c != '\\') {
                sb.append(c)
                i++
                continue
            }
            i++
            if (i >= s.length) {
                throw FAIL
            }
            val e = s[i]
            i++
            when (e) {
                '"' -> sb.append('"')
                '\\' -> sb.append('\\')
                '/' -> sb.append('/')
                'b' -> sb.append('\b')
                'f' -> sb.append('\u000C')
                'n' -> sb.append('\n')
                'r' -> sb.append('\r')
                't' -> sb.append('\t')
                'u' -> sb.append(unicodeEscape())
                else -> throw FAIL
            }
        }
    }

    /**
     * Reads the four hex digits of a backslash-u escape, pairing surrogates. An unpaired one
     * becomes U+FFFD, which is what Go's decoder substitutes.
     */
    private fun unicodeEscape(): String {
        val hi = hex4()
        if (hi.isHighSurrogate() && i + 1 < s.length && s[i] == '\\' && s[i + 1] == 'u') {
            val save = i
            i += 2
            val lo = hex4()
            if (lo.isLowSurrogate()) {
                return String(charArrayOf(hi, lo))
            }
            i = save
            return "\uFFFD"
        }
        if (hi.isSurrogate()) {
            return "\uFFFD"
        }
        return hi.toString()
    }

    private fun hex4(): Char {
        if (i + 4 > s.length) {
            throw FAIL
        }
        var v = 0
        for (k in 0 until 4) {
            val ch = s[i + k]
            val d = Character.digit(ch, 16)
            // The `> 'f'` guard is not redundant: Character.digit accepts non-ASCII digits such
            // as the fullwidth forms, which Go's decoder does not. Same trap as parseInt64.
            if (d < 0 || ch > 'f') {
                throw FAIL
            }
            v = v * 16 + d
        }
        i += 4
        return v.toChar()
    }

    /** RFC 8259 number grammar: `-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?` */
    private fun number(): Any {
        val start = i
        if (i < s.length && s[i] == '-') {
            i++
        }
        if (i >= s.length) {
            throw FAIL
        }
        if (s[i] == '0') {
            i++
        } else if (isDigit(s[i])) {
            while (i < s.length && isDigit(s[i])) {
                i++
            }
        } else {
            throw FAIL
        }
        if (i < s.length && s[i] == '.') {
            i++
            if (i >= s.length || !isDigit(s[i])) {
                throw FAIL
            }
            while (i < s.length && isDigit(s[i])) {
                i++
            }
        }
        if (i < s.length && (s[i] == 'e' || s[i] == 'E')) {
            i++
            if (i < s.length && (s[i] == '+' || s[i] == '-')) {
                i++
            }
            if (i >= s.length || !isDigit(s[i])) {
                throw FAIL
            }
            while (i < s.length && isDigit(s[i])) {
                i++
            }
        }
        if (i == start) {
            throw FAIL
        }
        return JSON_NUMBER
    }

    private fun isDigit(c: Char): Boolean = c in '0'..'9'
}
