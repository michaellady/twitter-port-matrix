package twitterport.dom;

import java.util.List;

/**
 * VERIFIED CORE. One page of a timeline.
 *
 * <p>{@code nextCursor} is null exactly when nothing remains below the page (D10), which is the
 * whole meaning of the field: null means precisely "nothing remains", never "unknown".
 */
public record Page(List<Tweet> tweets, Long nextCursor) {}
