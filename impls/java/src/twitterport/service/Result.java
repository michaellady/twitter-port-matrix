package twitterport.service;

/**
 * VERIFIED CORE. Either a value or one of the contract's error codes.
 *
 * <p>The core speaks the observable error vocabulary directly. A translation table in the HTTP
 * shim would be one more place for the vocabulary to drift out from under a proof.
 */
public final class Result<T> {

    private final String error;
    private final T value;

    private Result(String error, T value) {
        this.error = error;
        this.value = value;
    }

    public static <T> Result<T> ok(T value) {
        return new Result<>(null, value);
    }

    public static <T> Result<T> err(String code) {
        return new Result<>(code, null);
    }

    public boolean isErr() {
        return error != null;
    }

    /** The contract error code, or null when this is a success. */
    public String error() {
        return error;
    }

    public T value() {
        return value;
    }
}
