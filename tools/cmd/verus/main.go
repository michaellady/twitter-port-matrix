// Command verus drives the Rust corner's deductive rung (R4).
//
// It is the `cargo-verus` counterpart of tools/cmd/gobra, and it exists for
// the same reason: `calibrate` must read a verdict SENTENCE, never an exit
// code (GOAL.md standing rule 1), and the sentence must carry the verifier's
// own words rather than a paraphrase.
//
// Three things about Verus made this more than a wrapper, and each is
// implemented and commented where it bites:
//
//  1. Verus is invoked through cargo, so it is CACHED. A second run over an
//     unchanged tree prints no `verification results::` line at all and exits
//     0 -- which reads as a clean pass to anything that only counts errors.
//     See touchSources and the "reported" accounting in run.go.
//  2. Verus prints one `verification results::` line PER CRATE, plus one for
//     vstd on a cold tree. The lines are attributed to crates by the
//     `Checking <name> <ver> (<dir>)` line that precedes them, which is only
//     reliable under --jobs 1.
//  3. A crate that fails verification fails to COMPILE, so every crate
//     downstream of it is never checked. A FAILED verdict is therefore
//     partial by construction, and says so.
//
// What an R4 verdict on this corner MEANS is a separate question from whether
// the plumbing works, and the answer is not the same as on the Go corner.
// F012 and F016 established that most Verus obligations here are on
// hand-written twins inside `#[cfg(verus_only)] mod verus_proof`, not on the
// shipped functions; exactly one property (F4, `domain::Follow::new`) is
// proved unconditionally about shipped code. F024 records what that does to
// this rung's row. Read it before quoting a Rust R4 kill rate.
//
// Subcommands:
//
//	verify   run cargo-verus over the verify-enabled crates of one tree,
//	         print Verus's own result lines verbatim, and end with the
//	         R4 PASSED / R4 FAILED / R4 UNDECIDED line calibrate reads
//	crates   list the crates the tree marks verify-enabled, which is the
//	         Rust corner's verification matrix
//	canary   negation-canary sweep: for every `ensures` clause on a shipped
//	         function, ask Verus to prove the NEGATED ANTECEDENT. A clause
//	         whose negated antecedent Verus proves is vacuous -- it verifies
//	         because nothing reaches it -- and its contribution to an R4 kill
//	         is not a kill (F013). Until this existed the Rust row had no
//	         vacuity instrument and GOAL.md's rule for trusting it was unmet
package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "crates":
		err = cmdCrates(os.Args[2:])
	case "canary":
		err = cmdCanary(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if errors.Is(err, errR4Failed) {
		// The verdict line is already on stdout; the exit code is its
		// required counterpart, and nothing else needs saying.
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "verus: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: verus <command> [flags]

  verify  run "cargo-verus verus verify" over one implementation tree and
          report Verus's own "verification results:: N verified, M errors"
          lines. The last line is "R4 PASSED" or "R4 FAILED" (exit 1), or
          "R4 UNDECIDED" when the -budget ran out. With -registry, -impl is
          an entry name such as rust@<mutant-id>, which is how calibrate
          points it at a mutant tree
  crates  list the crates carrying [package.metadata.verus] verify = true --
          the Rust corner's verification matrix, read from the tree rather
          than hardcoded
  canary  negation-canary sweep over every "ensures" clause on a shipped
          function: replace the clause list with the negated antecedent and
          ask Verus to prove it. Proved means the antecedent is unreachable
          and the obligation is VACUOUS; refuted means it is REFUTABLE and
          the obligation is load-bearing. Self-tests itself first by splicing
          a false precondition and requiring the answer VACUOUS

Requires cargo-verus and its sibling rust_verify/z3 (CARGO_VERUS or VERUS_PATH
override the location; otherwise cargo-verus or verus on PATH, otherwise
/opt/verus/verus-x86-linux/), a cargo toolchain, and rustc at the exact
version Verus was built against.
`)
}
