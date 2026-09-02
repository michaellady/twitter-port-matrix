package main

import (
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// jbmcR4 is one JVM corner's R4 driver: `jbmc verify` over the compiled
// bytecode of the mutant tree.
//
// It is kept in its own file so the R4 entry in rungs.go stays a three-line
// registration. Everything specific to these corners -- what the checker reads,
// how its budget is set, why their denominators are smaller than their
// obligation counts -- is here.
//
// Both JVM corners get the SAME driver value, not two copies of one. That is
// not tidiness: `jbmc verify` resolves the obligation set from the corner name
// it is handed, so the only thing that could differ between a Kotlin
// invocation and a Java one is the argv, and the argv is identical. Two
// drivers would be two places for the budget rule to drift.
func jbmcR4() driver {
	return driver{
		Tool: "jbmc",
		Args: func(cfg Config, implName, regPath string) []string {
			// JBMC's own budget lands a minute before calibrate's, so a tree
			// the solver could not finish is reported in the tool's words
			// ("R4 UNDECIDED", and no verdict, which calibrate records as an
			// error cell) rather than as a killed subprocess with no output to
			// read.
			b := cfg.rungTimeout() - time.Minute
			if b < time.Minute {
				b = time.Minute
			}
			return []string{
				"verify", "-impl=" + implName, "-registry=" + regPath,
				"-budget=" + b.String(),
			}
		},
		Covers: jbmcReads,
	}
}

var (
	jbmcKotlinR4 = jbmcR4()
	jbmcJavaR4   = jbmcR4()
)

// jbmcReadsJVM are the source prefixes the JVM obligation entry points reach:
// dom, store and service. httpshim is absent for the same reason
// internal/httpshim is absent from Gobra's matrix on the Go corner -- it is
// trusted transport (F004), so a mutant confined to it is UNREACHED by this
// rung rather than survived (F022).
//
// One list covers both JVM corners because the two trees have the same shape
// by construction, down to the file names: impls/java/src/twitterport/store/
// Store.java and impls/kotlin/src/twitterport/store/Store.kt. The mutant
// catalogue's paths differ only in the extension, which no prefix here tests.
// TestJBMCReadsCoversBothJVMCorners pins that, so a future corner that
// reorganises its tree fails here rather than silently scoring every mutant
// unreached.
//
// This is the rung's OUTER reach. Its inner reach is narrower still: of the 15
// obligations written over these three packages on each corner, JBMC 6.11.0
// can decide 7, and the other 8 are blocked by a tool defect (F014, F034).
// That second, sharper ceiling is not expressible as a file predicate -- it is
// per obligation, not per mutant -- so `jbmc verify` reports it in its own
// verdict sentence and this predicate answers only the question calibrate asks
// it.
var jbmcReadsJVM = []string{
	"src/twitterport/dom/",
	"src/twitterport/store/",
	"src/twitterport/service/",
}

func jbmcReads(m mutants.Mutant) bool { return editsAny(m, jbmcReadsJVM) }
