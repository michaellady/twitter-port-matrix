package twitterport.httpshim;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.Function;

/**
 * TRUSTED SHIM: HTTP/1.1 framing, by hand, on a raw socket.
 *
 * <p><b>Why this is not {@code com.sun.net.httpserver}.</b> The JDK's built-in server parses the
 * request target with {@code java.net.URI} before any handler runs, and answers anything that
 * class rejects with its own {@code <h1>400 Bad Request</h1>URISyntaxException thrown} page. Go's
 * {@code net/http} client will happily put all of these on the wire, and {@code S_obs} has a
 * defined answer for every one of them:
 *
 * <pre>
 *   /timeline?user=bob&amp;limit=%zz   S_obs: malformed_request   JDK: HTML 400
 *   /timeline?user={bob}            S_obs: invalid_handle       JDK: HTML 400
 *   /timeline?user=bob|alice        S_obs: invalid_handle       JDK: HTML 400
 *   /timeline?user=a^b              S_obs: invalid_handle       JDK: HTML 400
 * </pre>
 *
 * <p>That is a hole in totality, and totality is the first design point of this corner: every
 * syntactically representable request has a defined response. The hole cannot be closed from
 * inside the JDK server -- a {@code Filter} runs after the URI has already been parsed and
 * rejected -- so the framing is done here instead, and the request target is handed to the router
 * as the exact bytes that arrived. That is also the more honest split: {@code S_obs}'s
 * {@code Step} works on a raw path string, not on a parsed URI.
 *
 * <p>The cost is stated rather than hidden: this file is about 150 lines of trusted transport that
 * the JDK would otherwise have provided. It buys exact control over the one thing the contract
 * cares about at this layer -- the bytes in and the bytes out.
 */
public final class Http {

    private Http() {}

    /** A request, as it arrived. {@code target} is raw: undecoded, query included. */
    public record Request(String method, String target, String body) {}

    /** A response. {@code body} is the exact byte sequence to send, with no trailing newline. */
    public record Response(int status, String contentType, byte[] body) {}

    /** Bodies larger than this are refused. The contract's largest legal body is a few hundred bytes. */
    private static final int MAX_BODY = 8 * 1024 * 1024;

    private static final int MAX_LINE = 64 * 1024;
    private static final int MAX_HEADERS = 100;

    /** Accepts forever on {@code listener}, serving each connection with {@code handler}. */
    public static void serve(ServerSocket listener, Function<Request, Response> handler) {
        ExecutorService pool =
                Executors.newCachedThreadPool(
                        r -> {
                            Thread t = new Thread(r, "conn");
                            t.setDaemon(true);
                            return t;
                        });
        while (true) {
            Socket sock;
            try {
                sock = listener.accept();
            } catch (IOException e) {
                return;
            }
            pool.execute(() -> connection(sock, handler));
        }
    }

    private static void connection(Socket sock, Function<Request, Response> handler) {
        try (Socket s = sock) {
            s.setTcpNoDelay(true);
            s.setSoTimeout(300_000);
            InputStream in = new BufferedInputStream(s.getInputStream(), 8192);
            OutputStream out = new BufferedOutputStream(s.getOutputStream(), 8192);
            while (exchange(in, out, handler)) {
                out.flush();
            }
            out.flush();
        } catch (IOException e) {
            // A client that disappears mid-request is not an observable event.
        }
    }

    /** Handles one request/response pair. Returns true if the connection may be reused. */
    private static boolean exchange(
            InputStream in, OutputStream out, Function<Request, Response> handler)
            throws IOException {
        String line = readLine(in);
        if (line == null) {
            return false; // clean EOF between requests
        }
        while (line.isEmpty()) { // tolerate stray CRLF before a request line
            line = readLine(in);
            if (line == null) {
                return false;
            }
        }

        // The request target is everything between the FIRST space and the LAST space.
        //
        // For a well-formed request line those are the only two spaces and this is the ordinary
        // split. It matters for the ill-formed ones: S_obs's request alphabet contains paths
        // holding a raw space -- "/timeline?user=bob&limit=5 " has a defined answer of
        // invalid_limit -- and Go's client puts that space on the wire unencoded, producing a
        // request line with three spaces. Splitting at the second space would silently truncate
        // the target and answer a different question from the one the oracle was asked.
        int sp1 = line.indexOf(' ');
        int sp2 = line.lastIndexOf(' ');
        if (sp1 <= 0 || sp2 <= sp1) {
            writeResponse(out, badRequest(), false);
            return false;
        }
        String method = line.substring(0, sp1);
        String target = line.substring(sp1 + 1, sp2);
        String version = line.substring(sp2 + 1);

        Map<String, String> headers = new HashMap<>();
        for (int n = 0; ; n++) {
            String h = readLine(in);
            if (h == null) {
                return false;
            }
            if (h.isEmpty()) {
                break;
            }
            if (n >= MAX_HEADERS) {
                writeResponse(out, badRequest(), false);
                return false;
            }
            int colon = h.indexOf(':');
            if (colon > 0) {
                headers.put(
                        h.substring(0, colon).toLowerCase(java.util.Locale.ROOT),
                        h.substring(colon + 1).trim());
            }
        }

        byte[] body;
        String te = headers.get("transfer-encoding");
        if (te != null && te.toLowerCase(java.util.Locale.ROOT).contains("chunked")) {
            body = readChunked(in);
        } else {
            long len = 0;
            String cl = headers.get("content-length");
            if (cl != null) {
                try {
                    len = Long.parseLong(cl.trim());
                } catch (NumberFormatException e) {
                    writeResponse(out, badRequest(), false);
                    return false;
                }
            }
            if (len < 0 || len > MAX_BODY) {
                writeResponse(out, badRequest(), false);
                return false;
            }
            body = readExactly(in, (int) len);
        }
        if (body == null) {
            return false; // truncated body: the client went away
        }

        boolean keepAlive = version.equals("HTTP/1.1");
        String conn = headers.get("connection");
        if (conn != null && conn.toLowerCase(java.util.Locale.ROOT).contains("close")) {
            keepAlive = false;
        }

        Response resp =
                handler.apply(
                        new Request(method, target, new String(body, StandardCharsets.UTF_8)));
        writeResponse(out, resp, keepAlive);
        return keepAlive;
    }

    /** The one answer this layer produces on its own: a request that is not HTTP at all. */
    private static Response badRequest() {
        return new Response(
                400,
                "application/json",
                Canon.errorBody(twitterport.dom.Dom.ERR_MALFORMED_REQUEST)
                        .getBytes(StandardCharsets.UTF_8));
    }

    private static void writeResponse(OutputStream out, Response r, boolean keepAlive)
            throws IOException {
        StringBuilder head = new StringBuilder(128);
        head.append("HTTP/1.1 ").append(r.status()).append(' ').append(reason(r.status()));
        head.append("\r\n");
        if (r.status() != 204) {
            head.append("Content-Type: ").append(r.contentType()).append("\r\n");
            head.append("Content-Length: ").append(r.body().length).append("\r\n");
        }
        head.append("Connection: ").append(keepAlive ? "keep-alive" : "close").append("\r\n");
        head.append("\r\n");
        out.write(head.toString().getBytes(StandardCharsets.ISO_8859_1));
        if (r.status() != 204) {
            out.write(r.body());
        }
        out.flush();
    }

    private static String reason(int status) {
        return switch (status) {
            case 200 -> "OK";
            case 201 -> "Created";
            case 204 -> "No Content";
            case 400 -> "Bad Request";
            case 404 -> "Not Found";
            case 409 -> "Conflict";
            default -> "Internal Server Error";
        };
    }

    /**
     * Reads one CRLF- or LF-terminated line as raw bytes.
     *
     * <p>ISO-8859-1 keeps the byte-to-char mapping one-to-one, so the request target reaches the
     * router as exactly the bytes that arrived. {@code S_obs} compares raw path strings, so any
     * decoding here would be a behaviour change disguised as a convenience.
     */
    private static String readLine(InputStream in) throws IOException {
        ByteArrayOutputStream buf = new ByteArrayOutputStream(128);
        while (true) {
            int c = in.read();
            if (c < 0) {
                return buf.size() == 0 ? null : buf.toString(StandardCharsets.ISO_8859_1);
            }
            if (c == '\n') {
                byte[] b = buf.toByteArray();
                int n = b.length;
                if (n > 0 && b[n - 1] == '\r') {
                    n--;
                }
                return new String(b, 0, n, StandardCharsets.ISO_8859_1);
            }
            if (buf.size() >= MAX_LINE) {
                return null;
            }
            buf.write(c);
        }
    }

    private static byte[] readExactly(InputStream in, int n) throws IOException {
        byte[] b = new byte[n];
        int off = 0;
        while (off < n) {
            int got = in.read(b, off, n - off);
            if (got < 0) {
                return null;
            }
            off += got;
        }
        return b;
    }

    /**
     * Chunked request bodies. Go's client never uses them here -- every harness body is a sized
     * byte reader -- but a framing layer that answers "some requests" is not a framing layer.
     */
    private static byte[] readChunked(InputStream in) throws IOException {
        ByteArrayOutputStream body = new ByteArrayOutputStream();
        while (true) {
            String line = readLine(in);
            if (line == null) {
                return null;
            }
            int semi = line.indexOf(';');
            if (semi >= 0) {
                line = line.substring(0, semi);
            }
            int size;
            try {
                size = Integer.parseInt(line.trim(), 16);
            } catch (NumberFormatException e) {
                return null;
            }
            if (size < 0 || body.size() + size > MAX_BODY) {
                return null;
            }
            if (size == 0) {
                while (true) { // trailers
                    String t = readLine(in);
                    if (t == null || t.isEmpty()) {
                        break;
                    }
                }
                return body.toByteArray();
            }
            byte[] chunk = readExactly(in, size);
            if (chunk == null) {
                return null;
            }
            body.write(chunk, 0, chunk.length);
            readLine(in); // trailing CRLF
        }
    }
}
