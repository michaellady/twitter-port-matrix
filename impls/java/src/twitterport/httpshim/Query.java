package twitterport.httpshim;

import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * TRUSTED SHIM. Query-string parsing, ported from Go's {@code net/url.ParseQuery}.
 *
 * <p>Repeated parameters are the one place Go and Rust silently disagreed before the retarget
 * (finding F006): {@code GET /timeline?user=bob&user=alice} served Go a 200 with five tweets where
 * Rust returned 400. Neither answer contradicted {@code twitter.tla}. That is why this returns a
 * multimap and lets the caller reject any key that arrived more than once, instead of taking
 * "first wins" or "last wins" from whatever the platform's query API happens to do --
 * {@code java.net.URI} offers no query parser at all, and the servlet-style
 * {@code getParameter} shape would have silently made Java agree with Go by accident.
 *
 * <p>Go's parser is reproduced exactly, including its two quirks: a segment containing {@code ';'}
 * is an error, and empty segments are skipped.
 */
public final class Query {

    private Query() {}

    /**
     * @return the decoded parameters in first-seen key order, or null if the query is malformed
     */
    public static Map<String, List<String>> parse(String query) {
        Map<String, List<String>> out = new LinkedHashMap<>();
        String rest = query;
        while (!rest.isEmpty()) {
            String segment;
            int amp = rest.indexOf('&');
            if (amp >= 0) {
                segment = rest.substring(0, amp);
                rest = rest.substring(amp + 1);
            } else {
                segment = rest;
                rest = "";
            }
            if (segment.indexOf(';') >= 0) {
                // Go: "invalid semicolon separator in query". It keeps scanning and returns the
                // error at the end; S_obs rejects on any error, so stopping here is equivalent.
                return null;
            }
            if (segment.isEmpty()) {
                continue;
            }
            String key = segment;
            String value = "";
            int eq = segment.indexOf('=');
            if (eq >= 0) {
                key = segment.substring(0, eq);
                value = segment.substring(eq + 1);
            }
            String dk = unescape(key);
            String dv = unescape(value);
            if (dk == null || dv == null) {
                return null;
            }
            out.computeIfAbsent(dk, k -> new ArrayList<>()).add(dv);
        }
        return out;
    }

    /** Go's {@code url.QueryUnescape}: {@code +} is a space, {@code %XX} must be valid hex. */
    private static String unescape(String s) {
        ByteArrayOutputStream buf = new ByteArrayOutputStream(s.length());
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '%') {
                if (i + 2 >= s.length()) {
                    return null;
                }
                int hi = hexDigit(s.charAt(i + 1));
                int lo = hexDigit(s.charAt(i + 2));
                if (hi < 0 || lo < 0) {
                    return null;
                }
                buf.write((hi << 4) | lo);
                i += 2;
            } else if (c == '+') {
                buf.write(' ');
            } else {
                byte[] b = String.valueOf(c).getBytes(StandardCharsets.UTF_8);
                buf.write(b, 0, b.length);
            }
        }
        return new String(buf.toByteArray(), StandardCharsets.UTF_8);
    }

    private static int hexDigit(char c) {
        if (c >= '0' && c <= '9') {
            return c - '0';
        }
        if (c >= 'a' && c <= 'f') {
            return c - 'a' + 10;
        }
        if (c >= 'A' && c <= 'F') {
            return c - 'A' + 10;
        }
        return -1;
    }
}
