package main

import (
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// jbmcKotlinR4 is the Kotlin corner's R4 driver: `jbmc verify` over the
// compiled bytecode of the mutant tree.
//
// It is kept in its own file so the R4 entry in rungs.go stays a three-line
// registration. Everything specific to this corner -- what the checker reads,
// how its budget is set, why its denominator is smaller than its obligation
// count -- is here.
var jbmcKotlinR4 = driver{
	Tool: "jbmc",
	Args: func(cfg Config, implName, regPath string) []string {
		// JBMC's own budget lands a minute before calibrate's, so a tree the
		// solver could not finish is reported in the tool's words
		// ("R4 UNDECIDED", and no verdict, which calibrate records as an error
		// cell) rather than as a killed subprocess with no output to read.
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

// jbmcReadsKotlin are the source prefixes the Kotlin obligation entry points
// reach: dom, store and service. httpshim is absent for the same reason
// internal/httpshim is absent from Gobra's matrix on the Go corner -- it is
// trusted transport (F004), so a mutant confined to it is UNREACHED by this
// rung rather than survived (F022).
//
// This is the rung's OUTER reach. Its inner reach is narrower still: of the 15
// obligations written over these three packages, JBMC 6.11.0 can decide 7, and
// the other 8 are blocked by a tool defect (F014). That second, sharper
// ceiling is not expressible as a file predicate -- it is per obligation, not
// per mutant -- so `jbmc verify` reports it in its own verdict sentence and
// this predicate answers only the question calibrate asks it.
var jbmcReadsKotlin = []string{
	"src/twitterport/dom/",
	"src/twitterport/store/",
	"src/twitterport/service/",
}

func jbmcReads(m mutants.Mutant) bool { return editsAny(m, jbmcReadsKotlin) }
