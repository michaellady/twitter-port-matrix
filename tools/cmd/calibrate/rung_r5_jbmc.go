package main

import (
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// jbmcKotlinR5 is the Kotlin corner's R5 driver: `jbmc r5verify` over the
// compiled bytecode of the mutant tree.
//
// R5 is the same corner read a SECOND way, exactly as it is on the Go corner
// where `gobra verify` and `gobra r5verify` are the same Gobra run asked two
// different questions. Here the two questions are asked of two different
// obligation files -- Obligations.kt for R4, Refinement.kt for R5 -- because
// JBMC's unit of work is an entry point rather than a package, so "run it
// again and read it differently" is literally "run the other entry points".
//
// That difference has one consequence worth stating: on this corner R5 CANNOT
// inherit an R4 kill, because it never runs an R4 obligation. On the Go corner
// the separation is achieved by attribution (an error in a member carrying no
// R5 clause is an R4 kill and not an R5 one); here it is achieved by
// construction. Both rows answer the same question, and this one has a
// stronger guarantee that it is not the R4 row wearing R5's name.
//
// What R5 on this corner is NOT: the Go corner's R5. Its clauses are ground
// instances inside an unwinding bound, three of the four abs axes are
// decidable and the follows axis is not (F014), and the numbers it reports
// exclude the blocked clauses from both the numerator and the denominator per
// F022. spec/refinement/clause-sites-kotlin.json carries that caveat next to
// the sites themselves so a reader of the cell reaches it.
var jbmcKotlinR5 = driver{
	Tool: "jbmc",
	Args: func(cfg Config, implName, regPath string) []string {
		// JBMC's own budget lands a minute before calibrate's, so a tree the
		// solver could not finish is reported in the tool's words
		// ("R5 UNDECIDED", and no verdict, which calibrate records as an error
		// cell) rather than as a killed subprocess with no output to read.
		b := cfg.rungTimeout() - time.Minute
		if b < time.Minute {
			b = time.Minute
		}
		return []string{
			"r5verify", "-impl=" + implName, "-registry=" + regPath,
			"-budget=" + b.String(),
		}
	},
	Covers: r5KotlinReads,
}

// r5KotlinFiles are the production files the Kotlin R5 entry points reach.
//
// It is ONE file, against R4's three, and that is the property that keeps the
// two rows from being the same row on this corner: every clause in
// Refinement.kt is a store-layer clause, so a mutant in service/Service.kt or
// dom/Dom.kt is fair game for R4 and UNREACHED by R5. The Go corner's r5Files
// is narrower than gobraVerified for the same reason.
//
// TestR5KotlinFilesMatchSites re-derives the reach from the obligation set so
// this list cannot go stale silently the day a service-layer clause is added.
var r5KotlinFiles = []string{
	"src/twitterport/store/Store.kt",
}

func r5KotlinReads(m mutants.Mutant) bool { return editsAny(m, r5KotlinFiles) }
