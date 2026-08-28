package twitterport.httpshim;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * TRUSTED SHIM. Strict request decoding (D7).
 *
 * <p>Exactly one JSON object, no unknown fields, no trailing content. Lenient parsing is a classic
 * source of cross-language divergence -- it is precisely where two implementations accept
 * different inputs and both look correct -- so this rejects rather than guesses. Ten of the 54 R0
 * baseline steps in the existing corners failed here.
 *
 * <p>Written by hand against Go's {@code encoding/json} behaviour rather than delegated to a
 * parser, because the contract is "whatever {@code S_obs} accepts", and {@code S_obs} is a Go
 * program. Two consequences of that are inherited deliberately and are worth stating rather than
 * discovering later:
 *
 * <ol>
 *   <li><b>Duplicate keys resolve last-wins.</b> Documented in D7 as a known limitation.
 *   <li><b>Field names match case-insensitively.</b> Go's decoder falls back to a case-insensitive
 *       match when no exact match exists, and {@code DisallowUnknownFields} therefore treats
 *       {@code {"Handle":"alice"}} as a <em>known</em> field, not an unknown one. D7 says unknown
 *       fields are rejected and says nothing about case, so the written contract and the reference
 *       implementation disagree here. This file follows the reference implementation, because
 *       {@code step.go} is the contract; see the report note on D7.
 * </ol>
 */
public final class Json {

    private Json() {}

    /** Sentinel for a JSON null, so a present-but-null key is distinguishable from an absent one. */
    private static final Object NULL = new Object();

    private static final class ParseError extends RuntimeException {
        private static final long serialVersionUID = 1L;

        ParseError() {
            super(null, null, false, false);
        }
    }

    private static final ParseError FAIL = new ParseError();

    /** A JSON object kept as ordered key/value pairs, so duplicates and order are both preserved. */
    private static final class Obj {
        final List<String> keys = new ArrayList<>();
        final List<Object> values = new ArrayList<>();
    }

    /** A JSON number. Never a legal value for any field of this API, but it must still parse. */
    private static final class Num {}

    private static final Num NUMBER = new Num();

    /**
     * Decodes exactly one JSON object whose fields are all drawn from {@code fields} and whose
     * values are all JSON strings or null.
     *
     * @return a map from field name to value, omitting fields whose value was JSON null (which
     *     leaves the corresponding pointer nil in {@code S_obs}, and therefore fails the
     *     required-field check in the caller); or null if the body is not acceptable at all
     */
    public static Map<String, String> decodeStrictStrings(String body, String[] fields) {
        Object v;
        Parser p = new Parser(body);
        try {
            p.skipWhitespace();
            if (p.atEnd()) {
                // Go returns io.EOF from Decode on an empty body.
                return null;
            }
            v = p.value();
            p.skipWhitespace();
            if (!p.atEnd()) {
                // Trailing content after the JSON value.
                return null;
            }
        } catch (ParseError e) {
            return null;
        }
        if (!(v instanceof Obj)) {
            // A non-object top level cannot populate a required field, so the answer is the same
            // malformed_request either way: a type error for scalars and arrays, and a no-op decode
            // followed by a missing required field for a literal null.
            return null;
        }
        Obj o = (Obj) v;
        Map<String, String> out = new HashMap<>();
        for (int i = 0; i < o.keys.size(); i++) {
            String field = matchField(o.keys.get(i), fields);
            if (field == null) {
                return null; // unknown field
            }
            Object val = o.values.get(i);
            if (val == NULL) {
                out.remove(field); // last-wins, and a null leaves the field unset
                continue;
            }
            if (!(val instanceof String)) {
                return null; // type error: this API has only string-valued fields
            }
            out.put(field, (String) val);
        }
        return out;
    }

    private static String matchField(String key, String[] fields) {
        for (String f : fields) {
            if (f.equals(key)) {
                return f;
            }
        }
        for (String f : fields) {
            if (f.equalsIgnoreCase(key)) {
                return f;
            }
        }
        return null;
    }

    // --- the parser ----------------------------------------------------------

    private static final class Parser {
        private final String s;
        private int i;

        Parser(String s) {
            this.s = s;
        }

        boolean atEnd() {
            return i >= s.length();
        }

        void skipWhitespace() {
            while (i < s.length()) {
                char c = s.charAt(i);
                if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                    i++;
                } else {
                    return;
                }
            }
        }

        private char peek() {
            if (i >= s.length()) {
                throw FAIL;
            }
            return s.charAt(i);
        }

        private void expect(char c) {
            if (i >= s.length() || s.charAt(i) != c) {
                throw FAIL;
            }
            i++;
        }

        private void literal(String lit) {
            if (!s.startsWith(lit, i)) {
                throw FAIL;
            }
            i += lit.length();
        }

        Object value() {
            char c = peek();
            switch (c) {
                case '{':
                    return object();
                case '[':
                    return array();
                case '"':
                    return string();
                case 't':
                    literal("true");
                    return Boolean.TRUE;
                case 'f':
                    literal("false");
                    return Boolean.FALSE;
                case 'n':
                    literal("null");
                    return NULL;
                default:
                    return number();
            }
        }

        private Obj object() {
            expect('{');
            Obj o = new Obj();
            skipWhitespace();
            if (peek() == '}') {
                i++;
                return o;
            }
            while (true) {
                skipWhitespace();
                if (peek() != '"') {
                    throw FAIL;
                }
                String k = string();
                skipWhitespace();
                expect(':');
                skipWhitespace();
                Object v = value();
                o.keys.add(k);
                o.values.add(v);
                skipWhitespace();
                char c = peek();
                if (c == ',') {
                    i++;
                    continue;
                }
                if (c == '}') {
                    i++;
                    return o;
                }
                throw FAIL;
            }
        }

        private List<Object> array() {
            expect('[');
            List<Object> out = new ArrayList<>();
            skipWhitespace();
            if (peek() == ']') {
                i++;
                return out;
            }
            while (true) {
                skipWhitespace();
                out.add(value());
                skipWhitespace();
                char c = peek();
                if (c == ',') {
                    i++;
                    continue;
                }
                if (c == ']') {
                    i++;
                    return out;
                }
                throw FAIL;
            }
        }

        private String string() {
            expect('"');
            StringBuilder sb = new StringBuilder();
            while (true) {
                if (i >= s.length()) {
                    throw FAIL;
                }
                char c = s.charAt(i);
                if (c == '"') {
                    i++;
                    return sb.toString();
                }
                if (c < 0x20) {
                    throw FAIL; // unescaped control character
                }
                if (c != '\\') {
                    sb.append(c);
                    i++;
                    continue;
                }
                i++;
                if (i >= s.length()) {
                    throw FAIL;
                }
                char e = s.charAt(i);
                i++;
                switch (e) {
                    case '"' -> sb.append('"');
                    case '\\' -> sb.append('\\');
                    case '/' -> sb.append('/');
                    case 'b' -> sb.append('\b');
                    case 'f' -> sb.append('\f');
                    case 'n' -> sb.append('\n');
                    case 'r' -> sb.append('\r');
                    case 't' -> sb.append('\t');
                    case 'u' -> sb.append(unicodeEscape());
                    default -> throw FAIL;
                }
            }
        }

        /**
         * Reads the four hex digits of a backslash-u escape, pairing surrogates. An unpaired one
         * becomes U+FFFD, which is what Go's decoder substitutes.
         */
        private String unicodeEscape() {
            char hi = hex4();
            if (Character.isHighSurrogate(hi)
                    && i + 1 < s.length()
                    && s.charAt(i) == '\\'
                    && s.charAt(i + 1) == 'u') {
                int save = i;
                i += 2;
                char lo = hex4();
                if (Character.isLowSurrogate(lo)) {
                    return new String(new char[] {hi, lo});
                }
                i = save;
                return "�";
            }
            if (Character.isSurrogate(hi)) {
                return "�";
            }
            return String.valueOf(hi);
        }

        private char hex4() {
            if (i + 4 > s.length()) {
                throw FAIL;
            }
            int v = 0;
            for (int k = 0; k < 4; k++) {
                int d = Character.digit(s.charAt(i + k), 16);
                if (d < 0 || s.charAt(i + k) > 'f') {
                    throw FAIL;
                }
                v = v * 16 + d;
            }
            i += 4;
            return (char) v;
        }

        /** RFC 8259 number grammar: -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)? */
        private Num number() {
            int start = i;
            if (i < s.length() && s.charAt(i) == '-') {
                i++;
            }
            if (i >= s.length()) {
                throw FAIL;
            }
            if (s.charAt(i) == '0') {
                i++;
            } else if (isDigit(s.charAt(i))) {
                while (i < s.length() && isDigit(s.charAt(i))) {
                    i++;
                }
            } else {
                throw FAIL;
            }
            if (i < s.length() && s.charAt(i) == '.') {
                i++;
                if (i >= s.length() || !isDigit(s.charAt(i))) {
                    throw FAIL;
                }
                while (i < s.length() && isDigit(s.charAt(i))) {
                    i++;
                }
            }
            if (i < s.length() && (s.charAt(i) == 'e' || s.charAt(i) == 'E')) {
                i++;
                if (i < s.length() && (s.charAt(i) == '+' || s.charAt(i) == '-')) {
                    i++;
                }
                if (i >= s.length() || !isDigit(s.charAt(i))) {
                    throw FAIL;
                }
                while (i < s.length() && isDigit(s.charAt(i))) {
                    i++;
                }
            }
            if (i == start) {
                throw FAIL;
            }
            return NUMBER;
        }

        private static boolean isDigit(char c) {
            return c >= '0' && c <= '9';
        }
    }
}
