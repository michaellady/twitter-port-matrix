package twitterport.dom;

/**
 * VERIFIED CORE. A registered user.
 *
 * <p>Ids are allocated from 1 in registration order (D2). Handles are the identity used everywhere
 * in the API; ids are returned but never accepted as input.
 */
public record User(String handle, long id) {}
