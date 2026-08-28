package twitterport.dom;

/**
 * VERIFIED CORE. A single follow edge, {@code from} follows {@code to}.
 *
 * <p>Deliberately a flat value in one flat set, not a nested {@code Map<String, Set<String>>}.
 * Finding F004 records why: six of the Go store's ten {@code // @ trusted} shims existed only
 * because the containers were nested, and flattening deleted them outright. The same shape is
 * carried here so the Java corner presents a verifier with the same small surface.
 */
public record Edge(String from, String to) {}
