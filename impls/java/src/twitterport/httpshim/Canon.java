package twitterport.httpshim;

import java.util.List;

import twitterport.dom.Page;
import twitterport.dom.Tweet;
import twitterport.dom.User;

/**
 * TRUSTED SHIM. Canonical response encoding (D8).
 *
 * <p>Byte-identical replay across four languages needs the encoding pinned, not left to each
 * language's default JSON writer. The rules:
 *
 * <pre>
 *   R1  Object keys appear in the fixed order declared by each writer below, NOT alphabetically.
 *       The order is part of the contract.
 *   R2  No insignificant whitespace.
 *   R3  Integers only. No floating point anywhere in the response surface.
 *   R4  Strings escape only what RFC 8259 requires, plus \b \f \n \r \t as short forms. No \u
 *       escaping of non-ASCII, no HTML escaping of &lt; &gt; &amp;.
 *   R5  null is written literally as null, never omitted.
 * </pre>
 *
 * <p>And no trailing newline: under a byte-equality conformance rule a trailing newline is a real
 * observable difference, and it accounted for 8 of the 54 R0 baseline steps in the Go corner.
 *
 * <p>These are hand-rolled rather than delegated to a JSON library on purpose. Every library
 * writer has its own opinions about key order, whitespace, and escaping, and the contract has
 * exactly one.
 */
public final class Canon {

    private Canon() {}

    private static final char[] HEX = "0123456789abcdef".toCharArray();

    static void encodeString(StringBuilder sb, String s) {
        sb.append('"');
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"' -> sb.append("\\\"");
                case '\\' -> sb.append("\\\\");
                case '\b' -> sb.append("\\b");
                case '\f' -> sb.append("\\f");
                case '\n' -> sb.append("\\n");
                case '\r' -> sb.append("\\r");
                case '\t' -> sb.append("\\t");
                default -> {
                    if (c < 0x20) {
                        sb.append("\\u00").append(HEX[(c >> 4) & 0xF]).append(HEX[c & 0xF]);
                    } else {
                        sb.append(c);
                    }
                }
            }
        }
        sb.append('"');
    }

    /** {@code {"error":"<code>"}} */
    public static String errorBody(String code) {
        StringBuilder sb = new StringBuilder();
        sb.append("{\"error\":");
        encodeString(sb, code);
        sb.append('}');
        return sb.toString();
    }

    /** {@code {"handle":"<h>","id":<n>}} */
    public static String userBody(User u) {
        StringBuilder sb = new StringBuilder();
        sb.append("{\"handle\":");
        encodeString(sb, u.handle());
        sb.append(",\"id\":").append(u.id()).append('}');
        return sb.toString();
    }

    /** {@code {"id":<n>,"author":"<h>","text":"<t>","created_at":<n>}} */
    private static void tweetObject(StringBuilder sb, Tweet t) {
        sb.append("{\"id\":").append(t.id()).append(",\"author\":");
        encodeString(sb, t.author());
        sb.append(",\"text\":");
        encodeString(sb, t.text());
        sb.append(",\"created_at\":").append(t.createdAt()).append('}');
    }

    public static String tweetBody(Tweet t) {
        StringBuilder sb = new StringBuilder();
        tweetObject(sb, t);
        return sb.toString();
    }

    /** {@code {"tweets":[...],"next_cursor":<n>|null}} */
    public static String timelineBody(Page p) {
        StringBuilder sb = new StringBuilder();
        sb.append("{\"tweets\":[");
        List<Tweet> tweets = p.tweets();
        for (int i = 0; i < tweets.size(); i++) {
            if (i > 0) {
                sb.append(',');
            }
            tweetObject(sb, tweets.get(i));
        }
        sb.append("],\"next_cursor\":");
        if (p.nextCursor() == null) {
            sb.append("null");
        } else {
            sb.append(p.nextCursor().longValue());
        }
        sb.append('}');
        return sb.toString();
    }

    /** {@code {"clock":<n>}} */
    public static String clockBody(long n) {
        return "{\"clock\":" + n + "}";
    }
}
