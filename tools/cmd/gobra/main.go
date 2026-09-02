// Command gobra drives the Go corner's deductive rungs (R4 and the
// Gobra-backed part of R5) and audits the results for vacuity.
//
// It exists because "Gobra found no errors" is not by itself evidence. Per
// GOAL.md standing rule 1 the verdict is read from the tool's own output, and
// per finding F013 a VERIFIED obligation is worth nothing until a *negation*
// canary shows the verifier is able to refute its opposite. An obligation
// whose negation also verifies is vacuous: nothing reaches it.
//
// Subcommands:
//
//	verify   run Gobra over the five verified packages, print its verdict
//	         lines verbatim, report the Viper member counts, and end with
//	         the R4 PASSED / R4 FAILED line calibrate reads
//	clauses  list the specification clauses the canary sweep will target
//	canary   negate each clause in turn and report refutable / VACUOUS
//	r5       per-clause status for the R5 refinement obligation
//	r5verify R5 as a calibrate rung: verify one tree and report whether a
//	         REFINEMENT clause is what failed, as opposed to any obligation
//	reach    per-member `ensures false` probe: is the exit reachable at all?
//	audit    check each REFUTABLE verdict is backed by an error in its own member
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
	case "clauses":
		err = cmdClauses(os.Args[2:])
	case "canary":
		err = cmdCanary(os.Args[2:])
	case "r5":
		err = cmdR5(os.Args[2:])
	case "r5verify":
		err = cmdR5Verify(os.Args[2:])
	case "reach":
		err = cmdReach(os.Args[2:])
	case "audit":
		err = cmdAudit(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if errors.Is(err, errR4Failed) || errors.Is(err, errR5Undecided) {
		// The verdict line is already on stdout; the exit code is its
		// required counterpart, and nothing else needs saying.
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gobra: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: gobra <command> [flags]

  verify    run Gobra over internal/{clock,ids,dom,store,service} and report
            its own verdict lines plus the Viper member counts. The last line
            is "R4 PASSED" or "R4 FAILED" (exit 1), or "R4 UNDECIDED" when the
            -budget ran out. With -registry, -impl is an entry name such as
            go@<mutant-id>, which is how calibrate points it at a mutant tree
  clauses   list the ensures clauses, classified (functional / framing /
            assumed-because-trusted)
  canary    negation-canary sweep: negate each functional clause in turn and
            record whether Gobra can refute it
  r5        per-clause status for the 42 R5 clauses, joined from the canary
            results rather than from what obligations.json records
  r5verify  R5 as a calibrate rung. Verifies one tree (-registry + -impl
            go@<mutant-id> points it at a mutant) and attributes each failing
            obligation to a clause by line. Ends with "R5 FAILED" only when a
            refinement clause is among them, "R5 PASSED" when none is, and
            "R5 UNDECIDED" when the answer cannot be read off the run
  reach     probe each member with "ensures false". If it verifies, nothing
            reaches that exit and every obligation on it is vacuous -- the
            F013 shape. Cheaper and more complete than the per-clause sweep,
            and it terminates where quantified canaries sometimes do not
  audit     re-read a sweep and check every REFUTABLE verdict is backed by an
            error Gobra reported inside the clause's own member

Requires a Gobra fat jar (GOBRA_JAR, default /opt/gobra/gobra.jar), a JVM and
z3 on PATH.
`)
}
