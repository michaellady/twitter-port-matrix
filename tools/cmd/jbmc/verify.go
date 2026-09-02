package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
)

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	impl := fs.String("impl", "kotlin", "corner to verify; a registry entry name (kotlin, or kotlin@<id> from `mutate apply`) or, without -registry, a directory")
	// A canary tree lives outside impls/ under whatever name the run gave it,
	// so the obligation set cannot always be read off the directory. Naming it
	// explicitly is what lets a canary be the SAME instrument pointed at a
	// deliberately broken copy, which is the only way rule 2's "shown to fail"
	// means anything.
	cornerName := fs.String("corner", "", "obligation set to use; defaults to the corner -impl names")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry and the directory is read from it, so the tree JBMC reads is the tree calibrate's guard hashed")
	budget := fs.Duration("budget", 25*time.Minute, "time budget for the WHOLE run; exhausting it prints R4 UNDECIDED and no verdict, never a pass")
	obBudget := fs.Duration("ob-budget", 6*time.Minute, "time budget for one JBMC invocation")
	unwind := fs.Int("unwind", 30, "JBMC loop unwinding bound")
	strLen := fs.Int("string-length", 3, "JBMC --max-nondet-string-length")
	work := fs.String("work", filepath.Join(os.TempDir(), "jbmc-rung"), "scratch directory; the extracted java.util is cached here between runs")
	jdk := fs.String("jdk", "", "JDK home containing lib/modules and bin/jimage (auto-detected when empty)")
	only := fs.String("only", "", "run only obligations whose name contains this substring (diagnostics; the verdict then covers only those)")
	auditBlocked := fs.Bool("audit-blocked", false, "also run the obligations carrying a recorded JBMC limit, to check the recorded reason has not gone stale. They still decide nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	start := time.Now()
	deadline := start.Add(*budget)

	which := *cornerName
	if which == "" {
		which = *impl
	}
	c, err := cornerFor(which)
	if err != nil {
		return err
	}
	implDir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}
	if u := c.unguarded(); len(u) > 0 {
		// Refused rather than warned: this rung's whole claim is that its
		// VERIFIED verdicts are refutable, and an obligation no canary names
		// has not been shown to be.
		return fmt.Errorf("corner %s claims %d obligation(s) no negation canary guards (%s); per F013 this rung will not report a verdict over them",
			c.Name, len(u), strings.Join(u, ", "))
	}

	if err := os.MkdirAll(*work, 0o755); err != nil {
		return err
	}
	tc, err := findToolchain(*jdk, *work)
	if err != nil {
		return err
	}

	fmt.Printf("jbmc: bounded proof rung over the %s corner\n", c.Name)
	fmt.Printf("        tree    %s\n", implDir)
	fmt.Printf("        jbmc    %s\n", firstLine(capture(tc.JBMC, "--version")))
	fmt.Printf("        kotlinc %s\n", firstLine(capture(tc.Compile, "-version")))
	fmt.Printf("        bounds  --unwind %d --max-nondet-string-length %d, budget %s (%s per obligation)\n",
		*unwind, *strLen, *budget, *obBudget)

	classes := filepath.Join(*work, "classes-"+sanitise(*impl))
	compileBudget := time.Until(deadline)
	if compileBudget > 5*time.Minute {
		compileBudget = 5 * time.Minute
	}
	cel, err := compileCorner(tc, c, implDir, classes, compileBudget)
	if err != nil {
		if errors.Is(err, errTimeout) {
			fmt.Println(undecided("the corner did not compile inside the budget", time.Since(start)))
			return err
		}
		return err
	}
	fmt.Printf("        compile %s in %s\n", strings.Join(c.SrcDirs, " + "), cel.Round(1e8))
	cp := strings.Join([]string{classes, tc.Stdlib, tc.Models, tc.JavaUtil}, ":")
	fmt.Println(strings.Repeat("=", 100))

	rep := &report{Corner: c.Name, Blocked: c.blocked()}

	// 1. The decidable obligations. These are the only runs whose answer can
	//    enter a kill rate.
	fmt.Printf("%-34s %-10s %-24s %-9s %s\n", "obligation", "verdict", "own assertion goals", "wall", "JBMC's own line")
	fmt.Println(strings.Repeat("-", 100))
	for _, ob := range c.decidable() {
		if *only != "" && !strings.Contains(ob.Fn, *only) {
			// A skipped obligation is recorded, not dropped. Dropping it would
			// shrink the denominator and let a one-obligation diagnostic run
			// print "1 of 1 verified", which reads exactly like a complete
			// pass. An obligation nobody ran is an obligation nobody decided.
			rep.Obs = append(rep.Obs, obOutcome{Fn: ob.Fn, Status: stUndecided,
				Note: "not run: excluded by -only=" + *only})
			continue
		}
		left := time.Until(deadline)
		if left <= 0 {
			fmt.Println()
			fmt.Println(undecided("the run budget was exhausted before "+ob.Fn, time.Since(start)))
			return fmt.Errorf("%w: budget exhausted at %s", errTimeout, ob.Fn)
		}
		r := runOne(tc, cp, c.Pkg, ob, *unwind, *strLen, minDur(left, *obBudget))
		st := classifyOne(r)
		rep.Obs = append(rep.Obs, obOutcome{
			Fn: ob.Fn, Status: st, OwnSuccess: r.OwnSuccess, OwnFailure: r.OwnFailure,
			Note: r.ToolError,
		})
		printRow(ob.Fn, string(st), r)
	}

	// 2. The negation canaries, but only when nothing was refuted. A
	//    refutation already decides the tree, and the canary sweep exists to
	//    protect a PASS -- running it after a FAIL would spend minutes on a
	//    question whose answer cannot change the verdict.
	refuted := 0
	for _, o := range rep.Obs {
		if o.Status == stRefuted {
			refuted++
		}
	}
	if refuted == 0 {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Println("negation canaries (F013): each MUST be refuted, or the obligation it names is vacuous")
		for _, o := range rep.Obs {
			if o.Status == stUndecided {
				continue // nothing was claimed about it, so nothing needs auditing
			}
			for _, k := range c.canariesFor(o.Fn) {
				left := time.Until(deadline)
				if left <= 0 {
					fmt.Println()
					fmt.Println(undecided("the run budget was exhausted before canary "+k.Fn, time.Since(start)))
					return fmt.Errorf("%w: budget exhausted at %s", errTimeout, k.Fn)
				}
				r := runOne(tc, cp, c.Pkg, k, *unwind, *strLen, minDur(left, *obBudget))
				st := classifyOne(r)
				rep.Canaries = append(rep.Canaries, canaryOutcome{Fn: k.Fn, Guards: k.Guards, Status: st})
				label := string(st)
				if st == stRefuted {
					label = "refuted-ok"
				}
				printRow(k.Fn, label, r)
			}
		}
	} else {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Printf("negation canaries not run: %d obligation(s) were refuted, which decides the tree on its own\n", refuted)
	}

	// 3. The blocked obligations, only when explicitly asked. They decide
	//    nothing either way; running them is an audit of the recorded reason,
	//    not a measurement of the tree. Two of them exhaust memory, which is
	//    why the rung never launches them by default.
	if *auditBlocked {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Println("blocked obligations (F014) -- run only to check the recorded reason; in no denominator")
		for _, ob := range c.blocked() {
			if *only != "" && !strings.Contains(ob.Fn, *only) {
				continue
			}
			left := time.Until(deadline)
			if left <= 0 {
				break
			}
			r := runOne(tc, cp, c.Pkg, ob, *unwind, *strLen, minDur(left, *obBudget))
			printRow(ob.Fn, "BLOCKED/"+string(classifyOne(r)), r)
		}
	}

	rep.Elapsed = time.Since(start)
	err = rep.decide()

	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("decidable %d   VERIFIED %d   REFUTED %d   VACUOUS %d   UNDECIDED %d\n",
		len(rep.Obs), rep.Verified, rep.Refuted, rep.Vacuous, rep.Undecided)
	fmt.Printf("blocked   %d   (recorded JBMC 6.11.0 limits; in neither the numerator nor the denominator)\n", len(rep.Blocked))
	for _, b := range c.blockedReasons() {
		fmt.Printf("    %s\n", b)
	}
	for _, r := range rep.Reasons {
		fmt.Printf("  ! %s\n", r)
	}
	fmt.Println()
	fmt.Println(rep.Sentence)

	if errors.Is(err, errR4Failed) {
		return errR4Failed
	}
	return err
}

func printRow(name, verdict string, r runResult) {
	own := fmt.Sprintf("%d ok, %d failed", r.OwnSuccess, r.OwnFailure)
	if r.ToolError != "" {
		own = r.ToolError
	}
	line := r.VerdictLine
	if len(r.LibFailures) > 0 {
		var ks []string
		for k := range r.LibFailures {
			ks = append(ks, k)
		}
		line += "  (library goals failed in " + strings.Join(ks, ", ") + ")"
	}
	fmt.Printf("%-34s %-10s %-24s %-9s %s\n", name, verdict, own, r.Elapsed.Round(1e8), line)
}

// undecided is the no-verdict sentence. It must not begin with "R4 PASSED" or
// "R4 FAILED": calibrate counts lines by those prefixes and records a run with
// neither as an error cell, which is what an undecided proof is.
func undecided(reason string, elapsed time.Duration) string {
	return fmt.Sprintf("R4 UNDECIDED: %s; nothing was decided about this tree   [%s]", reason, elapsed.Round(1e8))
}

func (c corner) blockedReasons() []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range c.blocked() {
		if seen[o.Blocked] {
			continue
		}
		seen[o.Blocked] = true
		var names []string
		for _, p := range c.blocked() {
			if p.Blocked == o.Blocked {
				names = append(names, p.Fn)
			}
		}
		out = append(out, fmt.Sprintf("%s\n        %s", o.Blocked, strings.Join(names, ", ")))
	}
	return out
}

// resolveImplDir turns -impl into a directory. With -registry it goes through
// the same registry the other rungs use, so the tree JBMC reads is the tree
// calibrate's guard hashed -- resolved by name, exactly as replay and diffrun
// resolve theirs.
func resolveImplDir(impl, registry string) (string, error) {
	if registry == "" {
		dir := impl
		if !strings.ContainsAny(impl, "/\\") {
			dir = filepath.Join("impls", impl)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("no implementation directory at %s: %w", abs, err)
		}
		return abs, nil
	}
	reg, err := implrun.LoadRegistry(registry)
	if err != nil {
		return "", fmt.Errorf("reading registry %s: %w", registry, err)
	}
	spec, err := reg.Get(impl)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(spec.Dir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func sanitise(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '@' || r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, s)
}
