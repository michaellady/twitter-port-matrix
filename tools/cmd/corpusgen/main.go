// Command corpusgen generates the conformance corpus from S_obs, and can
// replay the legacy hand-written corpus to surface spec/corpus drift.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	mode := flag.String("mode", "", "legacy-diff | emit")
	legacy := flag.String("legacy", "", "path to the legacy conformance.jsonl")
	out := flag.String("out", "", "output path for emit")
	flag.Parse()

	var err error
	switch *mode {
	case "legacy-diff":
		if *legacy == "" {
			err = fmt.Errorf("-legacy is required")
		} else {
			err = legacyDiff(*legacy)
		}
	case "emit":
		if *out == "" {
			err = fmt.Errorf("-out is required")
		} else {
			err = emit(*out)
		}
	default:
		err = fmt.Errorf("-mode must be legacy-diff or emit")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpusgen: %v\n", err)
		os.Exit(1)
	}
}
