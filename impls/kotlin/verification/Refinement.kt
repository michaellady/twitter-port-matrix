package twitterport.verification

import twitterport.store.Store

/**
 * The Kotlin corner's R5 (refinement) obligations, written as JBMC entry points.
 *
 * **This is not Obligations.kt one directory over.** `Obligations.kt` states FUNCTIONAL properties
 * of this implementation -- what `parseInt64` accepts, that appended ids increase, which error code
 * wins. This file states REFINEMENT clauses: the numbered rows of
 * `spec/refinement/obligations.json`, each of which says that one axis of `abs` commutes with one
 * `S_obs` transition. The two rungs must not be the same rung by construction, so the two files are
 * kept apart and `jbmc r5verify` reads only this one. A mutant that breaks a functional
 * postcondition is killed by R4; only a mutant that breaks a clause carrying an `S_obs` refinement
 * obligation is killed by R5.
 *
 * **The clause numbers are `obligations.json`'s, not new ones.** "R5 clause 11" means the same
 * sentence on this corner as on the Go corner, which is the only thing that makes a `go <- kotlin`
 * cell mean anything. `spec/refinement/clause-sites-kotlin.json` maps each number to the individual
 * `assert` below that carries it, keyed on (file, member, text) exactly as the Go corner's
 * `clause-sites.json` is.
 *
 * ## What is stateable here, and what is not
 *
 * `OBLIGATION.md` §4 gives `abs` per axis. `Store.kt`'s R5 block gives this corner's projections.
 * Three of the four axes are decidable by JBMC 6.11.0 and one is not:
 *
 *  - **log axis** (`absLogLen`, `absLogIdAt`, `absLogCreatedAtAt`, `absLogAuthorAt`) -- decidable.
 *    Longs, Ints, and an indexed `ArrayList` read, which `Store.kt` already establishes JBMC can
 *    discharge.
 *  - **users axis** (`absUserCount`, `absHasUser`) -- decidable. `HashMap.containsKey` reaches
 *    `String.equals` only after `(k = e.key) == key` fails, and handles written as literals are
 *    interned, so the reference test succeeds and the F014 defect is never reached. That is a
 *    property of the ground instances below, NOT of the clause in general: over a nondeterministic
 *    handle this axis would be undecidable too.
 *  - **clock axis** (`Store.clock()`) -- decidable.
 *  - **follows axis** (`absFollows`) -- **BLOCKED, measured**. `HashSet.contains(Edge)` compares a
 *    freshly built `Edge` whose reference test cannot succeed, so it falls through to
 *    `Edge.equals` -> `String.equals` -> F014. JBMC reports FAILURE for the claim AND FAILURE for
 *    its negation, which is not vacuity (that signature is both verifying) but plain
 *    nondeterminism. Either answer would be an artefact of the defect, so clauses 7 and 9 are in
 *    NEITHER the numerator nor the denominator -- F022's accounting, the same one `jbmc verify`
 *    applies to the eight R4 obligations F014 blocks.
 *
 * ## Every obligation is a GROUND instance, and says so
 *
 * The Go corner's clauses are universally quantified: `!(u.Handle in old(s.AbsUsers())) ==> ...`
 * holds for every handle. These are bounded instances at concrete handles. That is what a bounded
 * model checker gives, it is the same standard `Obligations.kt` already reports under, and the
 * `MATRIX.md` row must not be read as the Go corner's row. It is also why every obligation here is
 * written UNCONDITIONALLY rather than as `if (antecedent) assert(consequent)`: an implication's
 * negation canary has to refute the ANTECEDENT to be worth anything, and a ground instance whose
 * antecedent is established by its own setup has no antecedent left to refute.
 */
object Refinement {

    /** Clause 1: `abs(init) == init_S` on the users, follows, log and clock axes. */
    @JvmStatic
    fun c01_absInitIsTheEmptyState() {
        val s = Store()
        assert(s.absLogLen() == 0)
        assert(s.absUserCount() == 0)
        assert(s.absFollowCount() == 0)
        assert(s.clock() == 0L)
    }

    /** Clause 2: a fresh handle => `AbsUsers' == AbsUsers + {h}` -- added, and nothing else. */
    @JvmStatic
    fun c02_createUserAddsExactlyThatHandle() {
        val s = Store()
        val before = s.absUserCount()
        s.createUser("a")
        assert(s.absHasUser("a"))
        assert(s.absUserCount() == before + 1)
    }

    /** Clause 7: both endpoints known => `AbsFollows' == AbsFollows + {e}`. BLOCKED by F014. */
    @JvmStatic
    fun c07_addFollowAddsExactlyThatEdge() {
        val s = Store()
        val before = s.absFollowCount()
        s.addFollow("a", "b")
        assert(s.absFollows("a", "b"))
        assert(s.absFollowCount() == before + 1)
    }

    /** Clause 9: `AbsFollows' == AbsFollows - {e}`, unconditionally (F3). BLOCKED by F014. */
    @JvmStatic
    fun c09_removeFollowRemovesExactlyThatEdge() {
        val s = Store()
        s.addFollow("a", "b")
        s.removeFollow("a", "b")
        assert(!s.absFollows("a", "b"))
        assert(s.absFollowCount() == 0)
    }

    /** Clause 11: an accepted append => log length +1, and the appended tweet is last. */
    @JvmStatic
    fun c11_appendAddsExactlyOneAtTheEnd() {
        val s = Store()
        s.appendTweet("a", "x")
        val before = s.absLogLen()
        val t = s.appendTweet("a", "y")
        assert(s.absLogLen() == before + 1)
        assert(s.absLogIdAt(s.absLogLen() - 1) == t.id)
        assert(s.absLogCreatedAtAt(s.absLogLen() - 1) == t.createdAt)
        assert(s.absLogAuthorAt(s.absLogLen() - 1).compareTo("a") == 0)
    }

    /** Clause 13: the existing log prefix is never rewritten. */
    @JvmStatic
    fun c13_logPrefixNeverRewritten() {
        val s = Store()
        val t0 = s.appendTweet("a", "x")
        s.appendTweet("a", "y")
        s.tick()
        s.appendTweet("a", "z")
        // Setup, not a clause: this line states the length, which is clause 11's sentence, not
        // clause 13's. It is deliberately NOT registered as a clause site, so a failure here is
        // the `in a member carrying R5 clauses, but not on one` case the verdict has to
        // distinguish from a clause failure.
        assert(s.absLogLen() == 3)
        assert(s.absLogIdAt(0) == t0.id)
        assert(s.absLogCreatedAtAt(0) == t0.createdAt)
    }

    /** Clause 36: F7 -- `now' == now + 1`. */
    @JvmStatic
    fun c36_tickAdvancesByExactlyOne() {
        val s = Store()
        val before = s.clock()
        s.tick()
        assert(s.clock() == before + 1L)
    }
}

/**
 * The negation canaries for the clause obligations above -- F013's instrument, one per obligation.
 *
 * Each asserts the negation of the WHOLE conjunction its obligation claims, at the same reachable
 * point. Under vacuity a claim and its negation both verify, and nothing else produces that
 * signature; an injection canary cannot see it, because the injected defect is downstream of the
 * infeasible point too. A canary JBMC does not refute demotes the obligation it names to VACUOUS
 * and the run to UNDECIDED. There is no canary for clause 7 or clause 9: they are BLOCKED, so
 * nothing is claimed about them and there is nothing to audit.
 */
object RefinementCanaries {

    /** Negation of c01. */
    @JvmStatic
    fun k01_absInitIsNotEmpty() {
        val s = Store()
        assert(!(s.absLogLen() == 0 && s.absUserCount() == 0 && s.absFollowCount() == 0 && s.clock() == 0L))
    }

    /** Negation of c02. */
    @JvmStatic
    fun k02_createUserDoesNotAddThatHandle() {
        val s = Store()
        val before = s.absUserCount()
        s.createUser("a")
        assert(!(s.absHasUser("a") && s.absUserCount() == before + 1))
    }

    /** Negation of c11. */
    @JvmStatic
    fun k11_appendDoesNotAddOneAtTheEnd() {
        val s = Store()
        s.appendTweet("a", "x")
        val before = s.absLogLen()
        val t = s.appendTweet("a", "y")
        assert(!(s.absLogLen() == before + 1 &&
            s.absLogIdAt(s.absLogLen() - 1) == t.id &&
            s.absLogCreatedAtAt(s.absLogLen() - 1) == t.createdAt &&
            s.absLogAuthorAt(s.absLogLen() - 1).compareTo("a") == 0))
    }

    /** Negation of c13. */
    @JvmStatic
    fun k13_logPrefixIsRewritten() {
        val s = Store()
        val t0 = s.appendTweet("a", "x")
        s.appendTweet("a", "y")
        s.tick()
        s.appendTweet("a", "z")
        assert(!(s.absLogIdAt(0) == t0.id && s.absLogCreatedAtAt(0) == t0.createdAt))
    }

    /** Negation of c36. */
    @JvmStatic
    fun k36_tickDoesNotAdvanceByOne() {
        val s = Store()
        val before = s.clock()
        s.tick()
        assert(!(s.clock() == before + 1L))
    }
}
