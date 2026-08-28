package twitterport;

import twitterport.httpshim.Server;

/**
 * Entry point for the Java corner of the port matrix.
 *
 * <p>Zero dependencies and zero build tool: the whole implementation is plain JDK 17 compiled by
 * {@code javac}, serving on {@code com.sun.net.httpserver.HttpServer}. See the registry entry in
 * {@code impls/registry.json} for the exact build and run commands.
 *
 * <p>Listen address comes from {@code ADDR} ("host:port"), falling back to {@code PORT} and then to
 * 8080, so the replay harness can hand it a free port without this process choosing one.
 */
public final class Main {

    private Main() {}

    public static void main(String[] args) throws Exception {
        String host = "127.0.0.1";
        int port = 8080;

        String addr = System.getenv("ADDR");
        if (addr != null && !addr.isEmpty()) {
            int colon = addr.lastIndexOf(':');
            if (colon < 0) {
                throw new IllegalArgumentException("ADDR must be host:port, got: " + addr);
            }
            String h = addr.substring(0, colon);
            if (!h.isEmpty()) {
                host = h;
            }
            port = Integer.parseInt(addr.substring(colon + 1));
        } else {
            String p = System.getenv("PORT");
            if (p != null && !p.isEmpty()) {
                port = Integer.parseInt(p);
            }
        }

        Server.start(host, port);
    }
}
