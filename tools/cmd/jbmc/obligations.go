package main

import (
	"fmt"
	"sort"
	"strings"
)

// The three reasons an obligation over a JVM corner cannot be discharged by
// JBMC 6.11.0. Each was reduced to a two-line repro and each reproduces
// identically in plain Java compiled by javac, so none of them is a cost the
// language imposes -- see evidence/findings/F014.
//
// They are constants rather than free text because the rung's denominator is
// defined by them: an obligation carrying one of these reasons is excluded
// from the numerator AND the denominator, and a reason nobody can name is a
// reason nobody can audit.
const (
	// `assert "abc".equals("abc")` is reported FAILURE. So is `a.equals(a)` on
	// a single reference. compareTo, startsWith, isEmpty, length and charAt are
	// all fine, so the defect is localised to the org.cprover.CProverString
	// .equals intrinsic that the model's equals delegates to. Note the
	// direction: the defect produces a spurious FAILURE, so an obligation
	// blocked by it must not be counted as a KILL either.
	equalsReason = "JBMC's String.equals is unsound (`assert \"abc\".equals(\"abc\")` is FAILURE, F014); every visibility and error-code comparison is undecidable"

	// String.getBytes(Charset) dispatches on Charset.name() compared with that
	// same broken intrinsic, so it falls through to an opaque stub returning an
	// array of unconstrained length. validHandle and validText measure UTF-8
	// bytes -- which is what makes them byte-exact against a Go reference
	// machine -- so both, and every service path that starts with validation,
	// are outside what this checker can say anything about.
	getBytesReason = "JBMC's String.getBytes(Charset) is nondeterministic (F014); the UTF-8 byte-length predicates are unreachable"

	// A plain scalability wall: the SAT instance for a nondeterministic limit
	// over a four-entry log exhausts memory (11 GB RSS observed). Not a
	// modelling gap -- and the reason these two are never even launched by the
	// rung, which shares a four-CPU machine with the other corners' sweeps.
	satReason = "JBMC exhausts memory on this instance (\"SAT checker ran out of memory\"); a bounded-model-checking scalability wall, not a modelling gap"
)

// An obligation is one JBMC entry point.
type obligation struct {
	Class string // "Obligations" or "Canaries"
	Fn    string // method name
	Sig   string // JVM descriptor

	// Blocked records a KNOWN reason this checker cannot discharge this
	// obligation. A blocked obligation is not run by the rung at all: whatever
	// JBMC answered about it would be an artefact of the defect named here,
	// and an artefact must not become a kill (a spurious FAILURE) or a
	// survival (a spurious SUCCESS). It is in neither the numerator nor the
	// denominator -- the same accounting F022 applies to a mutant confined to
	// the trusted shim.
	Blocked string

	// Canary marks a known-bad entry point that MUST be refuted. Per F013 this
	// is the only instrument that can see a vacuous VERIFIED: an injection
	// canary asks "if I break the code does the gate notice", which is
	// ill-posed when the break is downstream of an infeasible point, while a
	// negation canary asks "can the verifier refute the opposite", which
	// nothing but a reachable obligation can answer.
	Canary bool

	// Guards names the obligation this canary is the negation of. Every
	// obligation the rung CLAIMS must be named by at least one canary, or the
	// claim is unaudited and the rung refuses to make it.
	Guards string
}

// Decidable reports whether this rung is allowed to read an answer about the
// obligation at all.
func (o obligation) Decidable() bool { return o.Blocked == "" && !o.Canary }

// Entry is the JBMC --function argument.
func (o obligation) Entry(pkg string) string {
	return pkg + "." + o.Class + "." + o.Fn + ":" + o.Sig
}

// The two source compilers this rung drives. A JVM corner is not identified by
// its language for anything except this: JBMC reads bytecode, so the corner's
// language matters exactly once, at the point where source becomes class files.
const (
	compilerKotlinc = "kotlinc"
	compilerJavac   = "javac"
)

// A corner is one implementation whose bytecode this rung checks.
type corner struct {
	Name string // registry entry name: "kotlin"

	// Compiler is the binary that turns this corner's sources into the
	// bytecode JBMC reads. Empty means kotlinc, so the Kotlin corner's entry
	// is unchanged by the Java corner's arrival.
	Compiler string

	// SrcDirs are relative to the implementation directory; both are compiled
	// together so the obligations link against the corner's own classes rather
	// than a copy.
	SrcDirs []string

	Pkg          string // the package the obligation classes live in
	Obligations  []obligation
	CoveredPaths []string // source prefixes the obligation entry points reach

	// R5File is the obligation source file, relative to the implementation
	// directory, that `r5verify` reads clause spans out of. It is empty for
	// the R4 set, which needs no per-assert line join: R4's unit of answer is
	// the whole entry point, R5's is the individual clause.
	R5File string
}

// kotlinCorner mirrors impls/kotlin/verification/Obligations.kt and
// Canaries.kt. The blocked reasons are the ones F014 measured; the rung does
// not re-derive them per run, and `jbmc verify -audit-blocked` is what checks
// they have not gone stale.
var kotlinCorner = corner{
	Name:     "kotlin",
	Compiler: compilerKotlinc,
	SrcDirs:  []string{"src", "verification"},
	Pkg:      "twitterport.verification",
	Obligations: []obligation{
		{Class: "Obligations", Fn: "o1a_oneCharAcceptSet", Sig: "(Ljava/lang/String;)V"},
		{Class: "Obligations", Fn: "o1b_twoCharAcceptSet", Sig: "(Ljava/lang/String;)V"},
		{Class: "Obligations", Fn: "o1c_emptyAndBareSignRejected", Sig: "()V"},

		{Class: "Obligations", Fn: "o2a_emptyIsInvalid", Sig: "()V", Blocked: getBytesReason},
		{Class: "Obligations", Fn: "o2b_goodHandleIsValid", Sig: "()V", Blocked: getBytesReason},

		{Class: "Obligations", Fn: "o3a_idsStrictlyIncrease", Sig: "()V"},
		{Class: "Obligations", Fn: "o3b_createdAtNonDecreasing", Sig: "(Z)V"},
		{Class: "Obligations", Fn: "o3c_lemmaOverThreeAppends", Sig: "(ZZ)V"},

		{Class: "Obligations", Fn: "o4a_pageRespectsLimit", Sig: "(I)V", Blocked: satReason},
		{Class: "Obligations", Fn: "o4b_cursorNullMeansExhausted", Sig: "()V", Blocked: equalsReason},
		{Class: "Obligations", Fn: "o4c_pageIsNewestFirst", Sig: "()V", Blocked: equalsReason},

		{Class: "Obligations", Fn: "o5a_unknownBeatsSelfFollow", Sig: "()V", Blocked: getBytesReason},
		{Class: "Obligations", Fn: "o5b_knownSelfFollowIsForbidden", Sig: "()V", Blocked: getBytesReason},
		{Class: "Obligations", Fn: "o5c_syntaxBeatsExistence", Sig: "()V"},
		{Class: "Obligations", Fn: "o5d_rejectionBurnsNoId", Sig: "()V", Blocked: satReason},

		{Class: "Canaries", Fn: "c1_bareSignIsANumber", Sig: "()V", Canary: true, Guards: "o1c_emptyAndBareSignRejected"},
		{Class: "Canaries", Fn: "c2_idsDoNotIncrease", Sig: "()V", Canary: true, Guards: "o3a_idsStrictlyIncrease"},
		{Class: "Canaries", Fn: "c3_clockCanDecrease", Sig: "()V", Canary: true, Guards: "o3b_createdAtNonDecreasing"},
		{Class: "Canaries", Fn: "c4_pageMayExceedLimit", Sig: "()V", Canary: true, Guards: "o4a_pageRespectsLimit"},
		{Class: "Canaries", Fn: "c5_timelineIsOldestFirst", Sig: "()V", Canary: true, Guards: "o4c_pageIsNewestFirst"},
		{Class: "Canaries", Fn: "c6_domIsReachable", Sig: "()V", Canary: true, Guards: "o2a_emptyIsInvalid"},
		{Class: "Canaries", Fn: "c7_storeIsReachable", Sig: "()V", Canary: true, Guards: "o3a_idsStrictlyIncrease"},
		{Class: "Canaries", Fn: "c8_serviceIsReachable", Sig: "()V", Canary: true, Guards: "o5c_syntaxBeatsExistence"},
		{Class: "Canaries", Fn: "c9_syntaxDoesNotBeatExistence", Sig: "()V", Canary: true, Guards: "o5c_syntaxBeatsExistence"},

		// Added with this rung. Before them o1a, o1b and o3c were CLAIMED
		// VERIFIED with no negation canary naming them, which is exactly the
		// state F013 says cannot be trusted: the claim was never shown
		// refutable, so a vacuous one would have read as a proof. A rung that
		// reports a kill rate over unaudited claims is reporting a number it
		// has not earned.
		{Class: "Canaries", Fn: "c10_nonDigitIsANumber", Sig: "()V", Canary: true, Guards: "o1a_oneCharAcceptSet"},
		{Class: "Canaries", Fn: "c11_signThenSignIsANumber", Sig: "()V", Canary: true, Guards: "o1b_twoCharAcceptSet"},
		{Class: "Canaries", Fn: "c12_thirdAppendDoesNotIncrease", Sig: "()V", Canary: true, Guards: "o3c_lemmaOverThreeAppends"},
	},
	// The obligation entry points reach dom, store and service. They do not
	// reach httpshim: it is trusted transport on this corner exactly as it is
	// on the Go corner (F004), so a mutant confined to it is UNREACHED by this
	// rung rather than survived -- F022's accounting, one corner over.
	CoveredPaths: []string{
		"src/twitterport/dom/",
		"src/twitterport/store/",
		"src/twitterport/service/",
	},
}

// javaCorner mirrors impls/java/verification/twitterport/verification/
// Obligations.java and Canaries.java. It is the TWIN of kotlinCorner: the same
// fifteen obligations, in the same five groups, stating the same properties
// over this corner's own classes.
//
// # Why the twin, and what it measures
//
// F014 established that the operative limit on the Kotlin corner is a JBMC
// defect rather than a language cost, reduced each blocker to a two-line repro
// in plain javac output, and concluded that "this wall is shared with the Java
// corner". That conclusion was an inference: the Java corner had no obligation
// set, so nothing had ever been run against it. evidence/MATRIX.md capped six
// of the twelve R4 cells on exactly that absence.
//
// Writing the twin turns the inference into a measurement, and the measurement
// came back agreeing: 7 decidable and 8 blocked, obligation for obligation the
// same 7 and the same 8 as Kotlin's. That is a stronger result than a new
// number would have been -- the wall is a property of the checker, confirmed
// on the language F014's own repros were written in.
//
// # Every Blocked reason below was measured on THIS corner
//
// None of them is copied from the Kotlin table. Each was run under
// --unwind 30 --max-nondet-string-length 3 and classified from JBMC's own goal
// lines; the transcripts are in evidence/runs/calibration/java-obligation-probe/.
// Two of the reasons are sharper here than the Kotlin entry states them, and
// the sharpening is in F034.
var javaCorner = corner{
	Name:     "java",
	Compiler: compilerJavac,
	SrcDirs:  []string{"src", "verification"},
	Pkg:      "twitterport.verification",
	Obligations: []obligation{
		{Class: "Obligations", Fn: "o1a_oneCharAcceptSet", Sig: "(Ljava/lang/String;)V"},
		{Class: "Obligations", Fn: "o1b_twoCharAcceptSet", Sig: "(Ljava/lang/String;)V"},
		{Class: "Obligations", Fn: "o1c_emptyAndBareSignRejected", Sig: "()V"},

		{Class: "Obligations", Fn: "o2a_emptyIsInvalid", Sig: "()V", Blocked: getBytesReason},
		{Class: "Obligations", Fn: "o2b_goodHandleIsValid", Sig: "()V", Blocked: getBytesReason},

		{Class: "Obligations", Fn: "o3a_idsStrictlyIncrease", Sig: "()V"},
		{Class: "Obligations", Fn: "o3b_createdAtNonDecreasing", Sig: "(Z)V"},
		{Class: "Obligations", Fn: "o3c_lemmaOverThreeAppends", Sig: "(ZZ)V"},

		{Class: "Obligations", Fn: "o4a_pageRespectsLimit", Sig: "(I)V", Blocked: satReason},
		{Class: "Obligations", Fn: "o4b_cursorNullMeansExhausted", Sig: "()V", Blocked: equalsReason},
		{Class: "Obligations", Fn: "o4c_pageIsNewestFirst", Sig: "()V", Blocked: equalsReason},

		{Class: "Obligations", Fn: "o5a_unknownBeatsSelfFollow", Sig: "()V", Blocked: getBytesReason},
		{Class: "Obligations", Fn: "o5b_knownSelfFollowIsForbidden", Sig: "()V", Blocked: getBytesReason},
		{Class: "Obligations", Fn: "o5c_syntaxBeatsExistence", Sig: "()V"},
		{Class: "Obligations", Fn: "o5d_rejectionBurnsNoId", Sig: "()V", Blocked: satReason},

		// EVERY obligation above is named by at least one canary below -- the
		// blocked ones included. The Kotlin corner guards only the obligations
		// somebody suspected, which is how F025 happened: three claims went
		// unaudited for two months inside the one gate built to catch exactly
		// that, because the gate was indexed by canary rather than by claim.
		// Indexing by claim costs five extra canaries and makes the property
		// structural instead of remembered. It also means that if a blocked
		// obligation ever becomes decidable -- a JBMC fix, a different bound --
		// it can be claimed the same day it is measured rather than claimed
		// unaudited.
		{Class: "Canaries", Fn: "c1_bareSignIsANumber", Sig: "()V", Canary: true, Guards: "o1c_emptyAndBareSignRejected"},
		{Class: "Canaries", Fn: "c2_idsDoNotIncrease", Sig: "()V", Canary: true, Guards: "o3a_idsStrictlyIncrease"},
		{Class: "Canaries", Fn: "c3_clockCanDecrease", Sig: "()V", Canary: true, Guards: "o3b_createdAtNonDecreasing"},
		{Class: "Canaries", Fn: "c4_pageMayExceedLimit", Sig: "()V", Canary: true, Guards: "o4a_pageRespectsLimit"},
		{Class: "Canaries", Fn: "c5_timelineIsOldestFirst", Sig: "()V", Canary: true, Guards: "o4c_pageIsNewestFirst"},
		{Class: "Canaries", Fn: "c6_domIsReachable", Sig: "()V", Canary: true, Guards: "o2a_emptyIsInvalid"},
		{Class: "Canaries", Fn: "c7_storeIsReachable", Sig: "()V", Canary: true, Guards: "o3a_idsStrictlyIncrease"},
		{Class: "Canaries", Fn: "c8_serviceIsReachable", Sig: "()V", Canary: true, Guards: "o5c_syntaxBeatsExistence"},
		{Class: "Canaries", Fn: "c9_syntaxDoesNotBeatExistence", Sig: "()V", Canary: true, Guards: "o5c_syntaxBeatsExistence"},
		{Class: "Canaries", Fn: "c10_nonDigitIsANumber", Sig: "()V", Canary: true, Guards: "o1a_oneCharAcceptSet"},
		{Class: "Canaries", Fn: "c11_signThenSignIsANumber", Sig: "()V", Canary: true, Guards: "o1b_twoCharAcceptSet"},
		{Class: "Canaries", Fn: "c12_thirdAppendDoesNotIncrease", Sig: "()V", Canary: true, Guards: "o3c_lemmaOverThreeAppends"},
		{Class: "Canaries", Fn: "c13_cursorEmittedWhenExhausted", Sig: "()V", Canary: true, Guards: "o4b_cursorNullMeansExhausted"},
		{Class: "Canaries", Fn: "c14_goodHandleIsInvalid", Sig: "()V", Canary: true, Guards: "o2b_goodHandleIsValid"},
		{Class: "Canaries", Fn: "c15_emptyTextIsValid", Sig: "()V", Canary: true, Guards: "o2a_emptyIsInvalid"},
		{Class: "Canaries", Fn: "c16_selfFollowBeatsUnknown", Sig: "()V", Canary: true, Guards: "o5a_unknownBeatsSelfFollow"},
		{Class: "Canaries", Fn: "c17_knownSelfFollowIsAllowed", Sig: "()V", Canary: true, Guards: "o5b_knownSelfFollowIsForbidden"},
		{Class: "Canaries", Fn: "c18_rejectionBurnsAnId", Sig: "()V", Canary: true, Guards: "o5d_rejectionBurnsNoId"},
	},
	// Same three packages the Kotlin obligations reach, and httpshim is absent
	// for the same reason: it is trusted transport (F004), so a mutant confined
	// to it is UNREACHED by this rung rather than survived (F022).
	CoveredPaths: []string{
		"src/twitterport/dom/",
		"src/twitterport/store/",
		"src/twitterport/service/",
	},
}

// corners is every JVM corner this rung can drive.
//
// The Java corner was deliberately absent until impls/java had an obligation
// set: registering an R4 entry for a corner with no obligations would produce a
// row over an empty denominator, which is worse than the capped cell calibrate
// gives a corner with no rung at all. It now has one, written as the twin of
// the Kotlin corner's, and measured rather than assumed.
var corners = map[string]corner{
	kotlinCorner.Name: kotlinCorner,
	javaCorner.Name:   javaCorner,
}

func cornerFor(name string) (corner, error) {
	// A mutant entry is "<corner>@<mutant-id>"; the obligations are the
	// corner's regardless of which mutant is applied.
	base := name
	if i := strings.IndexByte(base, '@'); i >= 0 {
		base = base[:i]
	}
	c, ok := corners[base]
	if !ok {
		var names []string
		for k := range corners {
			names = append(names, k)
		}
		sort.Strings(names)
		return corner{}, fmt.Errorf("no JBMC obligation set for corner %q; this rung drives: %s", base, strings.Join(names, ", "))
	}
	return c, nil
}

// decidable is the obligation set the rung reads an answer from.
func (c corner) decidable() []obligation {
	var out []obligation
	for _, o := range c.Obligations {
		if o.Decidable() {
			out = append(out, o)
		}
	}
	return out
}

// blocked is the obligation set the rung refuses to read an answer from.
func (c corner) blocked() []obligation {
	var out []obligation
	for _, o := range c.Obligations {
		if o.Blocked != "" {
			out = append(out, o)
		}
	}
	return out
}

// canariesFor returns the negation canaries naming this obligation.
func (c corner) canariesFor(fn string) []obligation {
	var out []obligation
	for _, o := range c.Obligations {
		if o.Canary && o.Guards == fn {
			out = append(out, o)
		}
	}
	return out
}

// unguarded lists decidable obligations no canary names. It must be empty:
// per F013 a claim nothing can refute is not a claim this rung is willing to
// make, and TestEveryDecidableObligationIsGuarded pins that.
func (c corner) unguarded() []string {
	var out []string
	for _, o := range c.decidable() {
		if len(c.canariesFor(o.Fn)) == 0 {
			out = append(out, o.Fn)
		}
	}
	return out
}

func cmdList(args []string) error {
	name := "kotlin"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
	}
	c, err := cornerFor(name)
	if err != nil {
		return err
	}
	fmt.Printf("corner %s -- %d obligation(s), %d decidable, %d blocked by a recorded JBMC limit\n\n",
		c.Name, len(c.Obligations)-len(c.canaries()), len(c.decidable()), len(c.blocked()))
	fmt.Printf("%-34s %-10s %s\n", "obligation", "status", "guarded by / blocked by")
	fmt.Println(strings.Repeat("-", 96))
	for _, o := range c.Obligations {
		if o.Canary {
			continue
		}
		if o.Blocked != "" {
			fmt.Printf("%-34s %-10s %s\n", o.Fn, "BLOCKED", o.Blocked)
			continue
		}
		var g []string
		for _, k := range c.canariesFor(o.Fn) {
			g = append(g, k.Fn)
		}
		fmt.Printf("%-34s %-10s %s\n", o.Fn, "decidable", strings.Join(g, ", "))
	}
	if u := c.unguarded(); len(u) > 0 {
		fmt.Printf("\nUNGUARDED (F013): %s\n", strings.Join(u, ", "))
	}
	return nil
}

func (c corner) canaries() []obligation {
	var out []obligation
	for _, o := range c.Obligations {
		if o.Canary {
			out = append(out, o)
		}
	}
	return out
}
