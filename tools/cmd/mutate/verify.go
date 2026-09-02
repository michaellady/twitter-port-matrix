package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// cmdVerify is the guard against the quiet failure mode of a mutation rig.
//
// Two things can rot without anybody noticing:
//
//   - an anchor stops matching, because the implementation was edited under
//     the manifest. The mutant then injects nothing, the rungs all pass, and
//     the kill table records a defect that every rung caught. Every rung's
//     measured kill rate goes UP for a defect that never existed.
//   - a mutant stops compiling. That one is loud when you look, and silent
//     when a workflow reads an exit code from a wrapper instead.
//
// Both are checked for every mutant, and both are reported per mutant rather
// than as a single pass/fail, because "35 of 36" needs to say which one.
func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	manifestPath := fs.String("manifest", defaultManifest, "mutant manifest")
	registryPath := fs.String("registry", defaultRegistry, "implementation registry")
	implName := fs.String("impl", "", "restrict to one corner")
	id := fs.String("id", "", "restrict to one mutant id")
	family := fs.String("family", "", "restrict to one family")
	build := fs.Bool("build", true, "also compile every mutant (slow; -build=false checks anchors only)")
	keep := fs.Bool("keep", false, "keep the scratch trees instead of removing them")
	_ = fs.Parse(args)

	man, reg := loadAll(*manifestPath, *registryPath)
	sel := man.Select(*implName, *id, *family)
	if len(sel) == 0 {
		die("no mutants match impl=%q id=%q family=%q", *implName, *id, *family)
	}

	fmt.Printf("mutate verify: %d mutant(s)  [anchors%s]\n", len(sel), map[bool]string{true: " + compile", false: " only"}[*build])
	fmt.Printf("               manifest=%s registry=%s\n", *manifestPath, *registryPath)
	fmt.Println(rule())

	trees := map[string]*mutants.Tree{}
	baselineOK := map[string]bool{}
	var anchorFail, compileFail int
	var failures []string

	for _, m := range sel {
		spec, err := reg.Get(m.Impl)
		if err != nil {
			die("%v", err)
		}
		if _, ok := trees[m.Impl]; !ok {
			t, err := mutants.LoadTree(spec.Dir)
			if err != nil {
				die("reading %s: %v", spec.Dir, err)
			}
			trees[m.Impl] = t
			fmt.Printf("\n%s  source %s  base %s\n", m.Impl, spec.Dir, short(t.Hash()))
			if *build {
				baselineOK[m.Impl] = checkBaseline(m.Impl, spec)
			}
		}

		// Anchors first: no point compiling a tree the mutation never reached.
		_, edits, err := mutants.Apply(trees[m.Impl], m)
		if err != nil {
			anchorFail++
			failures = append(failures, m.Key())
			fmt.Printf("  DRIFT  %-38s\n", m.Key())
			for _, line := range strings.Split(err.Error(), "\n") {
				fmt.Printf("         %s\n", line)
			}
			continue
		}
		sites := make([]string, 0, len(edits))
		for _, e := range edits {
			sites = append(sites, fmt.Sprintf("%s:%d", e.File, e.Line))
		}

		if !*build {
			fmt.Printf("  ok     %-38s %s\n", m.Key(), strings.Join(sites, " "))
			continue
		}
		if !compileMutant(m, spec, sites, *keep) {
			compileFail++
			failures = append(failures, m.Key())
		}
	}

	fmt.Println("\n" + rule())
	fmt.Printf("anchors: %d/%d match exactly one site\n", len(sel)-anchorFail, len(sel))
	if *build {
		fmt.Printf("compile: %d/%d build clean\n", len(sel)-anchorFail-compileFail, len(sel))
		for impl, ok := range baselineOK {
			if !ok {
				fmt.Printf("NOTE: the UNMUTATED %s corner does not build; every failure above is suspect\n", impl)
			}
		}
	}
	if len(failures) > 0 {
		fmt.Printf("\nverify FAILED: %d mutant(s): %s\n", len(failures), strings.Join(failures, ", "))
		if anchorFail > 0 {
			fmt.Println("A drifted anchor injects nothing. Until it is repointed, that mutant would")
			fmt.Println("be recorded as killed by every rung, inflating the whole table.")
		}
		return 1
	}
	if !*build {
		// Say exactly what was checked. "verify PASSED" over an anchors-only
		// run must not read as if the mutants were compiled.
		fmt.Println("\nverify PASSED (anchors only): every anchor matches exactly one site.")
		fmt.Println("Nothing was compiled and nothing was run; re-run without -build=false.")
		return 0
	}
	fmt.Println("\nverify PASSED: every anchor matches one site; every mutant compiles")
	fmt.Println("(compiling proves the mutant is a legal program, NOT that it changes")
	fmt.Println(" behaviour -- run `mutate probe` for that)")
	return 0
}

// compileMutant materialises one mutant, compiles it, and cleans up before
// returning.
//
// It is a function rather than the body of the loop so the scratch tree is
// released per mutant. Deferring the cleanup to the end of a 36-mutant run
// keeps 36 copies of a corner alive at once; on the Rust corner each of those
// carries a rebuilt target/ directory.
func compileMutant(m mutants.Mutant, spec implrun.Spec, sites []string, keep bool) bool {
	start := time.Now()
	out, err := os.MkdirTemp("", "mutate-verify-")
	if err != nil {
		die("scratch dir: %v", err)
	}
	if !keep {
		defer os.RemoveAll(out)
	}
	res, err := mutants.Materialize(spec.Dir, m, out)
	if err != nil {
		fmt.Printf("  FAIL   %-38s materialise: %v\n", m.Key(), err)
		return false
	}
	mspec := spec
	mspec.Dir = res.TreeDir
	b, err := implrun.Compile(mspec)
	if b != nil {
		defer b.Close()
	}
	if err != nil {
		fmt.Printf("  FAIL   %-38s does not compile\n", m.Key())
		fmt.Printf("         %v\n", err)
		for _, line := range tailLines(b.Output, 12) {
			fmt.Printf("         | %s\n", line)
		}
		if keep {
			fmt.Printf("         tree kept at %s\n", res.TreeDir)
		}
		return false
	}
	// F031: the implementation compiling is not the same question as the
	// proof rung being able to build this tree. Ask the second one too.
	oblOut, ran, oerr := implrun.CompileObligations(mspec)
	if oerr != nil {
		fmt.Printf("  FAIL   %-38s implementation compiles, OBLIGATIONS do not\n", m.Key())
		fmt.Printf("         %v\n", oerr)
		for _, line := range tailLines(oblOut, 12) {
			fmt.Printf("         | %s\n", line)
		}
		fmt.Printf("         The proof rung compiles the obligations against the tree under test,\n")
		fmt.Printf("         so this mutant would reach it as an ERROR cell -- a missing\n")
		fmt.Printf("         measurement, not a kill and not a survival (F031).\n")
		if keep {
			fmt.Printf("         tree kept at %s\n", res.TreeDir)
		}
		return false
	}
	note := ""
	if ran {
		note = " +obl"
	}
	fmt.Printf("  ok     %-38s %-34s %s  %.1fs%s\n",
		m.Key(), strings.Join(sites, " "), short(res.TreeHash), time.Since(start).Seconds(), note)
	return true
}

// checkBaseline compiles the unmutated corner, so a broken toolchain shows up
// once as itself instead of N times as a mutant failure.
func checkBaseline(name string, spec implrun.Spec) bool {
	start := time.Now()
	b, err := implrun.Compile(spec)
	if b != nil {
		defer b.Close()
	}
	if err != nil {
		fmt.Printf("       BASELINE DOES NOT BUILD: %v\n", err)
		for _, line := range tailLines(b.Output, 12) {
			fmt.Printf("       | %s\n", line)
		}
		return false
	}
	fmt.Printf("       baseline builds clean (%.1fs)\n", time.Since(start).Seconds())
	return true
}

func tailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
