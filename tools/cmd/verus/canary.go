package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The verdicts a canary sweep can return.
//
// Only REFUTABLE licenses anything. A clause the verifier could not refute the
// antecedent of is VACUOUS: it verifies because nothing reaches it, and the
// kill it appears to contribute to an R4 row is not a kill. The other two are
// absences, and they are deliberately NOT folded into either -- an ILL-FORMED
// or TIMEOUT cell recorded as REFUTABLE would be the F016 mistake (a VERIFIED
// count that includes obligations nobody audited) rebuilt on a third corner.
const (
	vREFUTABLE = "REFUTABLE"
	vVACUOUS   = "VACUOUS"
	vILLFORMED = "ILL-FORMED"
	vTIMEOUT   = "TIMEOUT"
)

type canaryResult struct {
	Clause  clause
	Canary  string
	Verdict string
	Note    string
	Elapsed time.Duration
}

func cmdCanary(args []string) error {
	fs := flag.NewFlagSet("canary", flag.ContinueOnError)
	impl := fs.String("impl", "impls/rust", "the Rust implementation directory; with -registry, a registry entry name instead")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry")
	budget := fs.Duration("budget", 15*time.Minute, "time budget per Verus run; exhausting it is TIMEOUT, never a pass and never a refutation")
	twins := fs.Bool("twins", false, "also sweep clauses inside #[cfg(verus_only)] mod verus_proof; off by default because a canary on a twin measures the twin, not the shipped function (F012, F016)")
	listOnly := fs.Bool("list", false, "print the clauses and the canary each would get, and run nothing")
	skipSelfTest := fs.Bool("skip-selftest", false, "skip the self-test that proves this sweep can report VACUOUS. Only for a rerun that already showed it in the same session")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}
	crates, err := verifyEnabledCrates(implDir)
	if err != nil {
		return err
	}
	blocks, err := extractClauses(implDir, crates)
	if err != nil {
		return err
	}

	shipped, twin := splitBlocks(blocks)
	fmt.Printf("Verus negation canary over %s\n", implDir)
	fmt.Printf("  verify-enabled crates: %s\n", strings.Join(crateNames(crates), ", "))
	fmt.Printf("  ensures blocks: %d on shipped functions, %d inside #[cfg(verus_only)] mod verus_proof\n",
		len(shipped), len(twin))
	fmt.Printf("  clauses:        %d shipped, %d twin\n", countClauses(shipped), countClauses(twin))

	target := shipped
	if *twins {
		target = blocks
	}
	if len(target) == 0 {
		return fmt.Errorf("no ensures block to sweep: every clause in this tree is inside a verus_proof twin, and -twins was not given")
	}

	var todo []clause
	for _, b := range target {
		todo = append(todo, b.Clauses...)
	}
	fmt.Printf("  sweeping:       %d clause(s)\n\n", len(todo))

	if *listOnly {
		for _, c := range todo {
			can, nerr := negate(c.Text)
			if nerr != nil {
				fmt.Printf("  %s:%d %s\n      clause %s\n      canary ILL-FORMED: %v\n", c.Block.Rel, c.Line+1, c.Block.Func, c.Text, nerr)
				continue
			}
			fmt.Printf("  %s:%d %s\n      clause %s\n      canary %s\n", c.Block.Rel, c.Line+1, c.Block.Func, c.Text, can)
		}
		return nil
	}

	// Everything runs in a copy. The sweep rewrites source files, and a run
	// killed between the splice and the restore would leave the tree the rest
	// of this rig measures in a mutated state.
	ws, cleanup, err := copyTree(implDir)
	if err != nil {
		return err
	}
	defer cleanup()
	fmt.Printf("workspace: %s (a copy; the tree at %s is never written to)\n\n", ws, implDir)

	wsBlocks, err := extractClauses(ws, crates)
	if err != nil {
		return err
	}
	byKey := map[string]clause{}
	for _, b := range wsBlocks {
		for _, c := range b.Clauses {
			byKey[c.Key()] = c
		}
	}

	// Baseline. Without it a sweep cannot tell "the canary was refuted" from
	// "this tree never verified in the first place".
	fmt.Println("baseline (unmodified copy) --")
	base, berr := runVerus(ws, crates, *budget)
	if berr != nil {
		return fmt.Errorf("baseline run failed, so no canary verdict on this tree can be attributed: %w", berr)
	}
	bl, _, verr := base.verdict()
	if verr != nil {
		return fmt.Errorf("baseline produced no verdict, so nothing downstream is attributable: %w", verr)
	}
	fmt.Printf("  %s\n", bl)
	if base.Errors > 0 {
		return fmt.Errorf("baseline is not clean (%d error(s)); a canary sweep over a tree that already fails proves nothing", base.Errors)
	}
	fmt.Println()

	if !*skipSelfTest {
		if err := selfTest(ws, crates, byKey, todo, *budget); err != nil {
			return err
		}
		fmt.Println()
	}

	fmt.Println("sweep --")
	var results []canaryResult
	for i, c := range todo {
		wc, ok := byKey[c.Key()]
		if !ok {
			results = append(results, canaryResult{Clause: c, Verdict: vILLFORMED, Note: "clause not found in the workspace copy"})
			continue
		}
		r := runCanary(ws, crates, wc, nil, *budget)
		results = append(results, r)
		fmt.Printf("  [%2d/%2d] %-10s %6s  %s:%d %s\n", i+1, len(todo), r.Verdict,
			r.Elapsed.Round(time.Second), c.Block.Rel, c.Line+1, short(c.Text, 60))
		if r.Note != "" {
			fmt.Printf("           %s\n", r.Note)
		}
	}

	fmt.Println()
	return report(results)
}

// selfTest proves the sweep can report VACUOUS before any of its verdicts are
// believed.
//
// It takes a real clause, splices `requires false,` ahead of the canary, and
// requires the answer to be VACUOUS. Under a false precondition every
// postcondition is vacuously provable, so a sweep that still says REFUTABLE
// there is not measuring vacuity at all and every VACUOUS it fails to report
// downstream is a false green. This is standing rule 2 -- show the gate can
// fail before trusting it -- applied to the gate itself.
func selfTest(ws string, crates []crate, byKey map[string]clause, todo []clause, budget time.Duration) error {
	var probe clause
	var canary string
	for _, c := range todo {
		if _, err := negate(c.Text); err != nil {
			continue
		}
		wc, ok := byKey[c.Key()]
		if !ok {
			continue
		}
		probe, canary = wc, c.Text
		break
	}
	if canary == "" {
		return fmt.Errorf("no clause with a well-formed canary to self-test on; refusing to report a sweep whose VACUOUS path was never exercised")
	}
	fmt.Printf("self-test (does this sweep report VACUOUS when it should?) --\n")
	fmt.Printf("  probe %s:%d %s\n", probe.Block.Rel, probe.Line+1, short(probe.Text, 60))
	fmt.Printf("  with `requires false,` spliced in, every postcondition is vacuously provable,\n  so the canary MUST come back VACUOUS\n")
	r := runCanary(ws, crates, probe, []string{"requires false,"}, budget)
	fmt.Printf("  -> %s [%s]\n", r.Verdict, r.Elapsed.Round(time.Second))
	if r.Note != "" {
		fmt.Printf("     %s\n", r.Note)
	}
	if r.Verdict != vVACUOUS {
		return fmt.Errorf("SELF-TEST FAILED: expected VACUOUS under `requires false`, got %s. "+
			"This sweep cannot see a vacuous obligation, so none of its verdicts mean anything and none are reported", r.Verdict)
	}
	fmt.Println("  self-test PASSED: the sweep reports VACUOUS when the obligation is unreachable")
	return nil
}

// runCanary splices one canary, runs Verus, restores the file, and classifies.
//
// Attribution is by CRATE. The canary replaces the clause list of one function
// in one crate, so an error anywhere else is not about the canary: a splice
// that broke the syntax stops the crate compiling and it reports no
// `verification results::` line at all, and a `requires false` self-test can
// break a downstream caller while the canary's own crate verifies cleanly.
// Reading a whole-tree error count instead of the crate's own would score both
// of those as a refutation.
func runCanary(ws string, crates []crate, c clause, extra []string, budget time.Duration) canaryResult {
	res := canaryResult{Clause: c}
	canary, err := negate(c.Text)
	if err != nil {
		res.Verdict, res.Note = vILLFORMED, err.Error()
		return res
	}
	res.Canary = canary

	original, err := spliceCanary(c.Block, canary, extra)
	if err != nil {
		res.Verdict, res.Note = vILLFORMED, "splice failed: "+err.Error()
		return res
	}
	defer func() { _ = os.WriteFile(c.Block.File, original, 0o644) }()

	r, rerr := runVerus(ws, crates, budget)
	if r != nil {
		res.Elapsed = r.Elapsed
	}
	if errors.Is(rerr, errTimeout) {
		res.Verdict = vTIMEOUT
		res.Note = fmt.Sprintf("Verus did not finish inside %s; nothing was decided about this clause", budget)
		return res
	}
	if rerr != nil {
		res.Verdict, res.Note = vILLFORMED, rerr.Error()
		return res
	}

	var own *crateResult
	for i := range r.Reported {
		if r.Reported[i].Crate == c.Block.Crate {
			own = &r.Reported[i]
		}
	}
	switch {
	case own == nil:
		res.Verdict = vILLFORMED
		res.Note = fmt.Sprintf("crate %s reported no `verification results::` line under the canary (the splice most likely did not compile); "+
			"no verdict is attributable. Reported: %s", c.Block.Crate, describe(r))
	case own.Errors > 0:
		res.Verdict = vREFUTABLE
		res.Note = fmt.Sprintf("%s: verification results:: %d verified, %d errors -- Verus refuted the negated antecedent, so it is reachable",
			c.Block.Crate, own.Verified, own.Errors)
	default:
		res.Verdict = vVACUOUS
		res.Note = fmt.Sprintf("%s: verification results:: %d verified, 0 errors -- Verus PROVED the negated antecedent, so the clause's antecedent is unsatisfiable and the obligation is discharged over nothing",
			c.Block.Crate, own.Verified)
	}
	return res
}

func report(rs []canaryResult) error {
	n := map[string]int{}
	for _, r := range rs {
		n[r.Verdict]++
	}
	fmt.Printf("canary sweep: %d clause(s)   REFUTABLE %d   VACUOUS %d   ILL-FORMED %d   TIMEOUT %d\n",
		len(rs), n[vREFUTABLE], n[vVACUOUS], n[vILLFORMED], n[vTIMEOUT])

	if n[vVACUOUS] > 0 {
		fmt.Println()
		fmt.Println("VACUOUS clauses -- these verify because nothing reaches them (F013):")
		for _, r := range rs {
			if r.Verdict == vVACUOUS {
				fmt.Printf("  %s:%d %s\n      %s\n", r.Clause.Block.Rel, r.Clause.Line+1, r.Clause.Block.Func, r.Clause.Text)
			}
		}
	}
	undecided := n[vILLFORMED] + n[vTIMEOUT]
	if undecided > 0 {
		fmt.Printf("\n%d clause(s) undecided; they are in NEITHER the refutable count nor the vacuous one.\n", undecided)
	}
	fmt.Println()
	switch {
	case n[vVACUOUS] > 0:
		fmt.Printf("R4 obligations on this corner are NOT all non-vacuous: %d of %d are discharged over unreachable antecedents.\n", n[vVACUOUS], len(rs))
	case n[vREFUTABLE] == len(rs):
		fmt.Printf("every one of the %d shipped clause(s) is refutable; the R4 row on this corner is not measuring vacuous obligations.\n", len(rs))
	default:
		fmt.Printf("%d of %d clause(s) shown refutable; the rest were not decided, so the row is licensed only to that extent.\n", n[vREFUTABLE], len(rs))
	}
	return nil
}

func splitBlocks(bs []*clauseBlock) (shipped, twin []*clauseBlock) {
	for _, b := range bs {
		if b.Twin {
			twin = append(twin, b)
		} else {
			shipped = append(shipped, b)
		}
	}
	return shipped, twin
}

func countClauses(bs []*clauseBlock) int {
	n := 0
	for _, b := range bs {
		n += len(b.Clauses)
	}
	return n
}

func crateNames(cs []crate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func describe(r *result) string {
	if len(r.Reported) == 0 {
		return "none"
	}
	var parts []string
	for _, c := range r.Reported {
		parts = append(parts, fmt.Sprintf("%s %dv/%de", c.Crate, c.Verified, c.Errors))
	}
	return strings.Join(parts, ", ")
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// copyTree copies the implementation into a temp directory, skipping build
// output. The Rust tree is a few hundred kilobytes of source, so the copy is
// free; the first Verus run inside it pays a cold dependency build once and
// every run after that is warm.
func copyTree(src string) (string, func(), error) {
	dst, err := os.MkdirTemp("", "verus-canary-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dst) }
	err = filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		if fi.IsDir() {
			if fi.Name() == "target" || fi.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		return copyFile(p, filepath.Join(dst, rel), fi.Mode())
	})
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dst, cleanup, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
