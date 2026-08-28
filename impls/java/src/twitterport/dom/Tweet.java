package twitterport.dom;

/**
 * VERIFIED CORE. One entry of the append-ordered tweet log.
 *
 * <p>{@code text} is observable but model-irrelevant (D1): the projection used by {@code tlclink}
 * drops it before checking a trace against {@code twitter.tla}.
 */
public record Tweet(long id, String author, String text, long createdAt) {}
