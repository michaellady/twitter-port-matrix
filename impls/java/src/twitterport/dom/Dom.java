package twitterport.dom;

import java.nio.charset.StandardCharsets;

/**
 * VERIFIED CORE.
 *
 * <p>Bounds, the error vocabulary, and the two syntax predicates of {@code S_obs}. Nothing here
 * touches HTTP, JSON, or any wire concern: this file is the part of the Java corner a deductive
 * verifier is expected to read (see TCB.md, "the verified-core / trusted-shim boundary").
 *
 * <p>Every length and character test operates on UTF-8 <em>bytes</em>, not on Java {@code char}s.
 * {@code S_obs} is written in Go, where {@code len(s)} is a byte count and {@code s[i]} is a byte;
 * a Java implementation that measured {@code String.length()} would accept a 280-code-point tweet
 * that {@code S_obs} rejects. That is a real observable divergence, so it is closed here rather
 * than left to chance.
 */
public final class Dom {

    private Dom() {}

    // Bounds on the observable surface. Part of the contract: an implementation that accepts a
    // 300-character tweet does not refine S_obs.
    public static final int MAX_HANDLE_LEN = 32;
    public static final int MAX_TEXT_LEN = 280;
    public static final int MIN_TEXT_LEN = 1;
    public static final int DEFAULT_LIMIT = 50;
    public static final int MAX_LIMIT = 100;

    // Error codes. Exactly this set; no implementation may invent another.
    public static final String ERR_MALFORMED_REQUEST = "malformed_request";
    public static final String ERR_INVALID_HANDLE = "invalid_handle";
    public static final String ERR_INVALID_TEXT = "invalid_text";
    public static final String ERR_INVALID_LIMIT = "invalid_limit";
    public static final String ERR_INVALID_CURSOR = "invalid_cursor";
    public static final String ERR_UNKNOWN_USER = "unknown_user";
    public static final String ERR_SELF_FOLLOW = "self_follow_forbidden";
    public static final String ERR_HANDLE_TAKEN = "handle_taken";
    public static final String ERR_NOT_FOUND = "not_found";

    /**
     * Accepts 1..MAX_HANDLE_LEN bytes of [a-z0-9_]. Deliberately narrow: a narrow alphabet is a
     * narrow divergence surface.
     */
    public static boolean validHandle(String h) {
        byte[] b = h.getBytes(StandardCharsets.UTF_8);
        if (b.length == 0 || b.length > MAX_HANDLE_LEN) {
            return false;
        }
        for (byte raw : b) {
            int c = raw & 0xFF;
            boolean ok = (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_';
            if (!ok) {
                return false;
            }
        }
        return true;
    }

    /** Accepts MIN_TEXT_LEN..MAX_TEXT_LEN bytes, no control characters. */
    public static boolean validText(String t) {
        byte[] b = t.getBytes(StandardCharsets.UTF_8);
        if (b.length < MIN_TEXT_LEN || b.length > MAX_TEXT_LEN) {
            return false;
        }
        for (byte raw : b) {
            if ((raw & 0xFF) < 0x20) {
                return false;
            }
        }
        return true;
    }

    /**
     * Base-10 signed 64-bit parse with exactly Go's {@code strconv.ParseInt(s, 10, 64)} accept set.
     *
     * <p>Which strings are legal {@code limit} and {@code cursor} values is contract, not wire
     * format, so this lives in the core. It is hand-written rather than delegated to
     * {@code Long.parseLong} because {@code Long.parseLong} accepts non-ASCII decimal digits --
     * {@code Character.digit('١', 10) == 1} -- so {@code ?limit=١} would be a valid limit
     * of 1 in Java and an {@code invalid_limit} in {@code S_obs}. No generator emits that today,
     * which is exactly why it would have been a silent divergence.
     *
     * @return the parsed value, or null when the string is not a legal base-10 int64
     */
    public static Long parseInt64(String s) {
        if (s.isEmpty()) {
            return null;
        }
        int i = 0;
        boolean neg = false;
        char sign = s.charAt(0);
        if (sign == '+' || sign == '-') {
            neg = (sign == '-');
            i = 1;
            if (s.length() == 1) {
                return null;
            }
        }
        long acc = 0;
        for (; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c < '0' || c > '9') {
                return null;
            }
            int d = c - '0';
            // Range error. Go reports out-of-range as an error too; every value that overflows is
            // outside both the limit and the cursor windows, so the observable answer is identical.
            if (acc > (Long.MAX_VALUE - d) / 10) {
                return null;
            }
            acc = acc * 10 + d;
        }
        return neg ? -acc : acc;
    }
}
