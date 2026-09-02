package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// verdict is what a negation canary established about one clause.
type verdict string

const (
	// refutable: Gobra rejected the negation. The clause constrains reachable
	// code -- it is a real obligation.
	refutable verdict = "REFUTABLE"
	// vacuous: Gobra verified the negation as well. Both a claim and its
	// opposite hold, which is only possible when nothing reaches the
	// obligation. This is finding F013's signature.
	vacuous verdict = "VACUOUS"
	// illFormed: the canary did not type-check, so it asked no question.
	// Reported rather than silently counted as either answer.
	illFormed verdict = "ILL-FORMED"
)

type canaryResult struct {
	Clause   clause        `json:"clause"`
	Verdict  verdict       `json:"verdict"`
	Errors   []string      `json:"errors,omitempty"`
	Elapsed  time.Duration `json:"-"`
	ElapsedS float64       `json:"elapsed_s"`
}

func cmdCanary(args []string) error {
	fs := flag.NewFlagSet("canary", flag.ContinueOnError)
	impl := fs.String("impl", "impls/go", "the Go implementation directory")
	jobs := fs.Int("jobs", 2, "parallel Gobra invocations")
	only := fs.String("only", "", "restrict to clauses whose file:line or member contains this")
	out := fs.String("out", "", "write the full result set here as JSON")
	skipSelf := fs.Bool("skip-selftest", false, "do not run the sweep's own canary first")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := implDirFromArgs([]string{"-impl", *impl})
	if err != nil {
		return err
	}
	if !*skipSelf {
		if err := selfTest(implDir); err != nil {
			return err
		}
	}
	all, err := allClauses(implDir)
	if err != nil {
		return err
	}

	var todo []clause
	for _, c := range all {
		if c.Kind != kindFunctional {
			continue
		}
		if *only != "" && !matches(c, *only) {
			continue
		}
		todo = append(todo, c)
	}
	fmt.Fprintf(os.Stderr, "negation canaries: %d functional clauses, %d workers\n", len(todo), *jobs)

	results := make([]canaryResult, len(todo))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *jobs)
	var done int
	var mu sync.Mutex
	for i, c := range todo {
		wg.Add(1)
		go func(i int, c clause) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := runCanary(implDir, c)
			results[i] = r
			mu.Lock()
			done++
			fmt.Fprintf(os.Stderr, "  [%3d/%3d] %-12s %s:%d %s\n",
				done, len(todo), r.Verdict, c.File, c.StartLine, trunc(c.Text, 60))
			mu.Unlock()
		}(i, c)
	}
	wg.Wait()

	report(results)
	if *out != "" {
		b, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nfull results: %s\n", *out)
	}
	for _, r := range results {
		if r.Verdict == vacuous {
			// A vacuous obligation is a false green. Exit non-zero so a
			// caller that does look at exit codes is not misled either.
			return fmt.Errorf("%d clause(s) are vacuous", countVerdict(results, vacuous))
		}
	}
	return nil
}

func matches(c clause, pat string) bool {
	return strings.Contains(fmt.Sprintf("%s:%d", c.File, c.StartLine), pat) ||
		strings.Contains(c.Member, pat) || strings.Contains(c.Pkg, pat)
}

// runCanary substitutes one clause's negation and asks Gobra to prove it.
//
// Only the clause's own package is verified. That keeps each run cheap and
// keeps the question local: a negated postcondition that also broke a caller
// in another package would report an error there and be scored as refutable
// for the wrong reason.
func runCanary(implDir string, c clause) canaryResult {
	r := canaryResult{Clause: c}
	ws, err := newWorkspace(implDir)
	if err != nil {
		r.Verdict, r.Errors = illFormed, []string{err.Error()}
		return r
	}
	defer ws.close()

	if err := substitute(filepath.Join(ws.module, c.File), c); err != nil {
		r.Verdict, r.Errors = illFormed, []string{err.Error()}
		return r
	}
	res, err := runGobra(ws, []string{c.Pkg}, "")
	if err != nil {
		r.Verdict, r.Errors = illFormed, []string{err.Error()}
		return r
	}
	r.Elapsed = res.Elapsed
	r.ElapsedS = res.Elapsed.Seconds()
	for _, e := range res.Errors {
		r.Errors = append(r.Errors, fmt.Sprintf("%s:%d:%d %s", e.File, e.Line, e.Col, e.Message))
	}
	switch {
	case res.Total == 0:
		r.Verdict = vacuous
	case hasTypeError(res.Errors):
		r.Verdict = illFormed
	default:
		r.Verdict = refutable
	}
	return r
}

// hasTypeError distinguishes a canary Gobra could not parse or type-check
// from one it understood and rejected. Only the second is a result.
func hasTypeError(errs []gobraError) bool {
	for _, e := range errs {
		m := e.Message
		if strings.Contains(m, "might not hold") || strings.Contains(m, "might fail") ||
			strings.Contains(m, "Assert") || strings.Contains(m, "Postcondition") {
			return false
		}
	}
	return len(errs) > 0
}

// substitute rewrites the clause in place, keeping the file's line count
// exactly so that Gobra's reported positions still line up with the original.
func substitute(path string, c clause) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if c.EndLine > len(lines) {
		return fmt.Errorf("%s: clause runs past end of file", path)
	}
	first := lines[c.StartLine-1]
	indent := first[:len(first)-len(strings.TrimLeft(first, " \t"))]
	lines[c.StartLine-1] = indent + "// @ ensures " + c.Canary
	// Continuation lines become plain comments: still there, no longer
	// annotations, so the line numbering is untouched.
	for i := c.StartLine; i < c.EndLine; i++ {
		lines[i] = indent + "// (negation canary: continuation elided)"
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func countVerdict(rs []canaryResult, v verdict) int {
	n := 0
	for _, r := range rs {
		if r.Verdict == v {
			n++
		}
	}
	return n
}

func report(rs []canaryResult) {
	sorted := append([]canaryResult(nil), rs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Clause.File != sorted[j].Clause.File {
			return sorted[i].Clause.File < sorted[j].Clause.File
		}
		return sorted[i].Clause.StartLine < sorted[j].Clause.StartLine
	})
	fmt.Printf("\n%-34s %-26s %-11s %s\n", "SITE", "MEMBER", "VERDICT", "CLAUSE")
	for _, r := range sorted {
		fmt.Printf("%-34s %-26s %-11s %s\n",
			fmt.Sprintf("%s:%d", r.Clause.File, r.Clause.StartLine),
			r.Clause.Member, r.Verdict, trunc(r.Clause.Text, 80))
	}
	fmt.Printf("\n%d clauses: %d refutable, %d VACUOUS, %d ill-formed\n",
		len(rs), countVerdict(rs, refutable), countVerdict(rs, vacuous), countVerdict(rs, illFormed))
	if n := countVerdict(rs, vacuous); n > 0 {
		fmt.Printf("\nVACUOUS -- both the clause and its negation verify, so nothing reaches it:\n")
		for _, r := range sorted {
			if r.Verdict == vacuous {
				fmt.Printf("  %s:%d  %s\n      %s\n      canary: %s\n      means:  %s\n",
					r.Clause.File, r.Clause.StartLine, r.Clause.Member,
					r.Clause.Text, r.Clause.Canary, r.Clause.CanaryWhy)
			}
		}
	}
}
