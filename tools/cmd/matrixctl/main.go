// Command matrixctl drives the port matrix: environment checks, spec gates,
// verification, differential runs, and calibration.
//
// Every gate reads the underlying tool's own output. No gate is decided by a
// wrapper's exit code, and no gate can be satisfied by a check that is
// incapable of failing -- the spec gate runs a known-bad canary and treats a
// canary that passes as a hard failure.
package main

import (
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
	case "doctor":
		err = doctor()
	case "spec":
		if len(os.Args) < 3 || os.Args[2] != "check" {
			err = fmt.Errorf("usage: matrixctl spec check")
		} else {
			err = specCheck()
		}
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nmatrixctl: FAILED: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `matrixctl -- port matrix driver

  doctor        check the toolchain, the vendored spec, and the isolation rules
  spec check    regenerate the corpus, model-check twitter.tla, run the S_obs
                link check, and prove the link check can fail
`)
}
