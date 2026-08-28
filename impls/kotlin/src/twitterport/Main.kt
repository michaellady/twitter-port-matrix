package twitterport

import twitterport.httpshim.Server

/**
 * Entry point for the Kotlin corner of the port matrix.
 *
 * Zero dependencies beyond `kotlin-stdlib` and zero build tool: the whole implementation is plain
 * Kotlin compiled by `kotlinc` straight to a self-contained jar. See the registry entry in
 * `impls/registry.json` for the exact build and run commands.
 *
 * Listen address comes from `ADDR` ("host:port"), falling back to `PORT` and then to 8080, so the
 * replay harness can hand it a free port without this process choosing one.
 */
fun main() {
    var host = "127.0.0.1"
    var port = 8080

    val addr = System.getenv("ADDR")
    if (!addr.isNullOrEmpty()) {
        val colon = addr.lastIndexOf(':')
        require(colon >= 0) { "ADDR must be host:port, got: $addr" }
        val h = addr.substring(0, colon)
        if (h.isNotEmpty()) {
            host = h
        }
        port = addr.substring(colon + 1).toInt()
    } else {
        val p = System.getenv("PORT")
        if (!p.isNullOrEmpty()) {
            port = p.toInt()
        }
    }

    Server.start(host, port)
}
