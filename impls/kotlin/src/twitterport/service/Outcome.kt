package twitterport.service

/**
 * VERIFIED CORE. Either a value or one of the contract's error codes.
 *
 * The core speaks the observable error vocabulary directly. A translation table in the HTTP shim
 * would be one more place for the vocabulary to drift out from under a proof.
 *
 * **Named `Outcome`, not `Result`.** Kotlin ships `kotlin.Result` in the default-imported
 * `kotlin` package, so a `twitterport.service.Result` would shadow a stdlib type of the same name
 * at every use site -- legal, and resolved by the explicit import, but the reader cannot tell
 * which one is meant without checking the imports. The Java corner had no such collision and could
 * call it `Result`. This is a small, real, language-specific cost, recorded rather than absorbed.
 *
 * A sealed hierarchy rather than a nullable-error field: a `when (outcome)` over `Ok`/`Err` is
 * checked for exhaustiveness by the compiler, so adding a third case would break every call site
 * at compile time instead of falling through one of them at runtime. That is a genuine Kotlin
 * advantage over the Java corner's `isErr()`/`error()`/`value()` triple, in which nothing stops a
 * caller reading `value()` on an error.
 */
sealed interface Outcome<out T> {

    /** A success carrying [value]. */
    data class Ok<out T>(val value: T) : Outcome<T>

    /** A rejection carrying one of the contract's error codes. */
    data class Err(val code: String) : Outcome<Nothing>
}
