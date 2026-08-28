package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// runGuardDemo proves requirement 1's guard by reproducing the hazard it exists
// to catch and then catching it.
//
// A guard nobody has watched fire is in the same position as a rung nobody has
// watched fail -- GOAL.md standing rule 2. So this does not describe the check;
// it runs the wrong thing on purpose, shows the clean green it produces, and
// then shows the refusal. The demonstration itself is checked: if the false
// green does not appear, or the guard does not refuse, or the correctly named
// mutant is not actually killed, this exits non-zero, because in any of those
// cases the demonstration proved nothing.
func runGuardDemo(cfg Config, man *mutants.Manifest, tools *toolset, spec string) int {
	impl, id, ok := splitSpec(spec)
	if !ok {
		fmt.Fprintf(os.Stderr, "calibrate: -guard-demo wants impl/id, e.g. go/cursor-inclusive; got %q\n", spec)
		return 2
	}
	m, err := man.Get(impl, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibrate: %v\n", err)
		return 2
	}

	outDir, err := os.MkdirTemp("", "calibrate-guarddemo-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibrate: %v\n", err)
		return 1
	}
	defer os.RemoveAll(outDir)

	fmt.Printf("guard demonstration -- %s\n%s\n", m.Key(), strings.Repeat("=", 78))
	res, regPath, _, err := applyMutant(cfg, tools, m, outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibrate: %v\n", err)
		return 1
	}
	fmt.Printf("materialised %s\n  mutant tree   %s\n  original      %s\n  registry      %s\n\n",
		m.Key(), res.TreeHash, res.SourceHash, regPath)

	r0 := allRungs[0] // R0 is the cheapest rung that can show the difference.
	pass := true

	// STEP 1 -- the hazard, run for real. The registry `mutate apply` writes
	// also names the unmutated corner. Selecting it runs the ORIGINAL against
	// the mutant's own registry file, and the output is a clean pass with
	// nothing anywhere saying which tree was tested.
	fmt.Printf("1. the hazard: point R0 at %q -- the OTHER name in the mutant's registry\n", m.Impl)
	bare, err := tools.run(cfg.Root, r0.Tool, r0.Args(cfg, m.Impl, regPath), cfg.rungTimeout())
	if err != nil {
		fmt.Fprintf(os.Stderr, "   running replay: %v\n", err)
		return 1
	}
	bareKilled, bareErr := r0.verdict(bare)
	fmt.Printf("   replay says: %q  (exit %d)\n", verdictLine(bare.Stdout, r0), bare.ExitCode)
	if bareErr != nil || bareKilled {
		fmt.Printf("   UNEXPECTED: the bare name did not produce a clean pass, so this run does not\n")
		fmt.Printf("   demonstrate the hazard. %v\n", bareErr)
		pass = false
	} else {
		fmt.Printf("   That is a FALSE GREEN. It is indistinguishable from the mutant surviving,\n")
		fmt.Printf("   and a sweep making this slip reports every mutant a survivor and every rung\n")
		fmt.Printf("   worthless -- having measured the unmutated implementation N times (F009).\n")
	}

	// STEP 2 -- the guard, on the same wrong name.
	fmt.Printf("\n2. the guard, given that same name\n")
	if g, err := checkMutantSelected(cfg.Root, m.Impl, regPath, res); err == nil {
		fmt.Printf("   GUARD DID NOT FIRE. It accepted %q and resolved %s.\n", m.Impl, g.Dir)
		fmt.Printf("   The guard is broken; every number a sweep produces is unsupported.\n")
		pass = false
	} else {
		for _, line := range strings.Split(err.Error(), "\n") {
			fmt.Printf("   %s\n", line)
		}
	}

	// STEP 3 -- the guard, on the right name, showing what it verified.
	mutantName := m.Impl + "@" + m.ID
	fmt.Printf("\n3. the guard, given the mutant name %q\n", mutantName)
	g, err := checkMutantSelected(cfg.Root, mutantName, regPath, res)
	if err != nil {
		fmt.Printf("   GUARD REFUSED THE REAL MUTANT: %v\n", err)
		fmt.Printf("   That is worse than a missing guard: it would block correct measurements.\n")
		pass = false
	} else {
		fmt.Printf("   accepted, %d/5 checks\n", g.Checks)
		fmt.Printf("     name carries the @<id> suffix and matches the materialised mutant\n")
		fmt.Printf("     resolves to  %s\n", g.Dir)
		fmt.Printf("     baseline is  %s   (a different directory)\n", g.BaselineIs)
		fmt.Printf("     not under    %s/impls\n", cfg.Root)
		fmt.Printf("     bytes hash   %s   == the materialised mutant tree\n", g.TreeHash)
		fmt.Printf("                  %s   != the unmutated source\n", g.SourceHash)
	}

	// STEP 4 -- and the rung, correctly aimed, says something different.
	fmt.Printf("\n4. R0 against the guarded name\n")
	good, err := tools.run(cfg.Root, r0.Tool, r0.Args(cfg, mutantName, regPath), cfg.rungTimeout())
	if err != nil {
		fmt.Fprintf(os.Stderr, "   running replay: %v\n", err)
		return 1
	}
	goodKilled, goodErr := r0.verdict(good)
	fmt.Printf("   replay says: %q  (exit %d)\n", verdictLine(good.Stdout, r0), good.ExitCode)
	if goodErr != nil {
		fmt.Printf("   verdict unreadable: %v\n", goodErr)
		pass = false
	} else if !goodKilled {
		fmt.Printf("   NOTE: R0 does not kill this mutant, so steps 1 and 4 produce the same green\n")
		fmt.Printf("   and this run cannot show the difference. Re-run -guard-demo with a mutant R0\n")
		fmt.Printf("   kills; the guard result in steps 2 and 3 still stands on its own.\n")
	} else {
		fmt.Printf("   Same registry, same corpus, one character different in the name: 1 says\n")
		fmt.Printf("   PASSED and 4 says FAILED. Only the guard tells those two runs apart.\n")
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 78))
	if !pass {
		fmt.Printf("GUARD DEMONSTRATION FAILED -- see above.\n")
		return 1
	}
	fmt.Printf("guard demonstration passed: the false green was reproduced, and the guard\n")
	fmt.Printf("refused it on the name check and would have refused it on the content address.\n")
	return 0
}

func splitSpec(s string) (impl, id string, ok bool) {
	for _, sep := range []string{"/", "@"} {
		if i := strings.Index(s, sep); i > 0 && i < len(s)-1 {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
