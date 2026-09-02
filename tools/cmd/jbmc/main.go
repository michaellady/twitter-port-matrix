// Command jbmc drives the JVM corners' bounded-proof rung (R4) over JBMC.
//
// It is the Kotlin/Java counterpart of tools/cmd/gobra, and it exists for the
// same reason: "JBMC said VERIFICATION SUCCESSFUL" is not by itself evidence.
// Per GOAL.md standing rule 1 the verdict is read from the tool's own goal
// lines, never from its exit status -- JBMC exits non-zero for a refuted
// property AND for a parse error AND for an entry point that does not resolve,
// so the status cannot tell the answer from the accident.
//
// # Why this rung needs three outcomes and not two
//
// Two findings make the JVM corners different from the Go corner, and both are
// about a green that means nothing:
//
//   - F014: JBMC 6.11.0 verifies `"abc".equals("abc")` as FALSE. It is a tool
//     defect, reproduced in plain javac output, and it blocks every obligation
//     that reduces to string equality. On the Kotlin corner that is 8 of 15.
//     An obligation JBMC cannot decide must land in NEITHER the numerator nor
//     the denominator of a kill rate -- exactly the way F022 treats a mutant
//     confined to the trusted shim as *unreached* rather than *survived*.
//     Counting a tool defect as a survival reads like a proof result and is a
//     tool-defect result.
//
//   - F013: six Kotlin obligations once reported VERIFIED because an
//     undischargeable erased checkcast made everything after it infeasible.
//     CBMC assumes a failed check held for the rest of the trace, so a
//     VERIFIED that nothing reaches is vacuous -- and an injection canary
//     cannot see it, because the injected defect is downstream of the
//     infeasible point too. Only a NEGATION canary can: under vacuity a claim
//     and its negation both verify, and nothing else produces that signature.
//
// So every obligation this rung claims carries at least one negation canary in
// the same tree, and a claim whose canary cannot be refuted is reported
// VACUOUS and decides nothing. See verdict.go for the accounting.
//
// Subcommands:
//
//	verify   compile the corner and its obligations, run JBMC over the
//	         decidable obligations, print JBMC's own goal counts, and end with
//	         the R4 PASSED / R4 FAILED line calibrate reads (or R4 UNDECIDED,
//	         and no verdict, when the budget ran out or a claim went vacuous)
//	list     print the obligation table for a corner and what each is blocked by
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
	case "list":
		err = cmdList(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if errors.Is(err, errR4Failed) {
		// The verdict line is already on stdout; the exit code is its
		// required counterpart, and calibrate insists the two agree.
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jbmc: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: jbmc <command> [flags]

  verify  compile impls/<corner> together with its obligations, run JBMC over
          every DECIDABLE obligation, and report the rung's verdict. The last
          line is "R4 PASSED" or "R4 FAILED" (exit 1), or "R4 UNDECIDED" -- and
          no verdict at all -- when the -budget ran out, when a claimed
          obligation turned out vacuous, or when nothing decidable was left.
          With -registry, -impl is an entry name such as kotlin@<mutant-id>,
          which is how calibrate points it at a mutant tree.
  list    print the obligation table: which obligations this rung decides,
          which are blocked by a recorded JBMC limit, and which canary guards
          each claim.

Requires kotlinc (Kotlin corner), javac (Java corner), jbmc and a JDK that
still ships lib/modules. JBMC's core-models.jar is located automatically.
`)
}
