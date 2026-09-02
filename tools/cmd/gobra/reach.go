package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// cmdReach answers the question F013 actually turns on: is the state an
// obligation talks about reachable at all?
//
// The method is one `ensures false` per member. If Gobra verifies it, no
// execution reaches that member's exit, and *every* postcondition on it is
// vacuously true -- which is exactly what happened to the six Kotlin store
// obligations, where an undischargeable checkcast made everything downstream
// infeasible.
//
// This is coarser than the per-clause negation sweep and strictly cheaper:
// one invocation per member instead of one per clause, and it terminates,
// which the quantified per-clause canaries do not always do. It cannot see a
// clause made vacuous by an unsatisfiable antecedent on a member whose exit is
// otherwise reachable -- that is what `canary` is for. Run both; where a
// canary times out, this is the part of the answer that still stands.
func cmdReach(args []string) error {
	fs := flag.NewFlagSet("reach", flag.ContinueOnError)
	impl := fs.String("impl", "impls/go", "the Go implementation directory")
	jobs := fs.Int("jobs", 2, "parallel Gobra invocations")
	budget := fs.Duration("timeout", 6*time.Minute, "time budget per probe")
	out := fs.String("out", "", "write the result set here as JSON")
	only := fs.String("only", "", "restrict to members whose file or name contains this")
	isolate := fs.Bool("isolate", false,
		"elide the member's functional postconditions, leaving only `ensures false`. "+
			"Sound: `false` implies each of them, so nothing about the exit state changes -- "+
			"the solver is simply not asked to prove them alongside it")
	fs.Var(extraArgsFlag{}, "gobra-arg", "extra argument passed to every Gobra invocation (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := implDirFromArgs([]string{"-impl", *impl})
	if err != nil {
		return err
	}
	all, err := allClauses(implDir)
	if err != nil {
		return err
	}

	// One probe per member that carries at least one obligation Gobra checks.
	type member struct {
		file, pkg, name string
		clauses         int
	}
	seen := map[string]*member{}
	var order []string
	for _, c := range all {
		if c.Kind != kindFunctional && c.Kind != kindFraming {
			continue // assumed clauses are not checked, so reachability is moot
		}
		if *only != "" && !strings.Contains(c.File, *only) && !strings.Contains(c.Member, *only) {
			continue
		}
		k := c.File + "\x00" + c.Member
		if seen[k] == nil {
			seen[k] = &member{file: c.File, pkg: c.Pkg, name: c.Member}
			order = append(order, k)
		}
		seen[k].clauses++
	}
	sort.Strings(order)
	fmt.Fprintf(os.Stderr, "reachability probes: %d members carrying %d checked clauses, %d workers\n",
		len(order), len(all), *jobs)

	type probe struct {
		File     string  `json:"file"`
		Member   string  `json:"member"`
		Clauses  int     `json:"clauses_on_member"`
		Verdict  string  `json:"verdict"`
		Detail   string  `json:"detail,omitempty"`
		ElapsedS float64 `json:"elapsed_s"`
	}
	probes := make([]probe, len(order))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *jobs)
	var n int
	var mu sync.Mutex
	for i, k := range order {
		m := seen[k]
		wg.Add(1)
		go func(i int, m *member) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p := probe{File: m.file, Member: m.name, Clauses: m.clauses}
			var elided []clause
			if *isolate {
				elided = functionalEnsuresOn(all, m.file, m.name)
			}
			v, detail, el := probeReachable(implDir, m.file, m.pkg, m.name, elided, *budget)
			p.Verdict, p.Detail, p.ElapsedS = v, detail, el.Seconds()
			probes[i] = p
			mu.Lock()
			n++
			fmt.Fprintf(os.Stderr, "  [%2d/%2d] %-14s %s %s\n", n, len(order), v, m.file, m.name)
			mu.Unlock()
		}(i, m)
	}
	wg.Wait()

	fmt.Printf("\n%-34s %-26s %-8s %s\n", "FILE", "MEMBER", "CLAUSES", "EXIT REACHABLE?")
	dead := 0
	for _, p := range probes {
		if p.Verdict == "UNREACHABLE" {
			dead++
		}
		fmt.Printf("%-34s %-26s %-8d %s\n", p.File, p.Member, p.Clauses, p.Verdict)
	}
	fmt.Printf("\n%d members probed; %d have an unreachable exit.\n", len(probes), dead)
	if dead > 0 {
		fmt.Println("\nEvery obligation on an unreachable exit is vacuously true. These are the")
		fmt.Println("F013 shape and must not be counted as verified:")
		for _, p := range probes {
			if p.Verdict == "UNREACHABLE" {
				fmt.Printf("  %s %s — %d clause(s) vacuous\n", p.File, p.Member, p.Clauses)
			}
		}
	}
	if *out != "" {
		b, err := json.MarshalIndent(probes, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nfull results: %s\n", *out)
	}
	if dead > 0 {
		return fmt.Errorf("%d member(s) have an unreachable exit", dead)
	}
	return nil
}

// probeReachable appends `ensures false` to one member and runs its package.
//
//	Gobra rejects it  -> the exit is reachable; obligations there are about
//	                     something
//	Gobra accepts it  -> nothing reaches the exit; every obligation on the
//	                     member is vacuous
func probeReachable(implDir, file, pkg, member string, elided []clause, budget time.Duration) (string, string, time.Duration) {
	ws, err := newWorkspace(implDir)
	if err != nil {
		return "ERROR", err.Error(), 0
	}
	defer ws.close()
	if len(elided) > 0 {
		if err := elide(filepath.Join(ws.module, file), elided); err != nil {
			return "ERROR", err.Error(), 0
		}
	}
	if err := appendEnsuresFalse(filepath.Join(ws.module, file), member); err != nil {
		return "ERROR", err.Error(), 0
	}
	res, err := runGobra(ws, []string{pkg}, "", budget)
	if errors.Is(err, errTimeout) {
		return "TIMEOUT", "no verdict within " + budget.String(), res.Elapsed
	}
	if err != nil {
		return "ERROR", err.Error(), res.Elapsed
	}
	if res.Total == 0 {
		return "UNREACHABLE", "`ensures false` verified", res.Elapsed
	}
	var first string
	if len(res.Errors) > 0 {
		first = fmt.Sprintf("%s:%d %s", res.Errors[0].File, res.Errors[0].Line, res.Errors[0].Message)
	}
	return "reachable", first, res.Elapsed
}

// functionalEnsuresOn returns every functional `ensures` on one member.
func functionalEnsuresOn(all []clause, file, member string) []clause {
	var out []clause
	for _, c := range all {
		if c.File == file && c.Member == member && c.Kind == kindFunctional {
			out = append(out, c)
		}
	}
	return out
}

// appendEnsuresFalse adds `// @ ensures false` to the end of a member's
// contract block, immediately above its declaration.
func appendEnsuresFalse(path, member string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, ln := range lines {
		s := strings.TrimSpace(ln)
		var name string
		if m := reRecvName.FindStringSubmatch(s); m != nil {
			name = "(*" + m[1] + ")." + m[2]
		} else if m := reFuncDecl.FindStringSubmatch(s); m != nil {
			name = m[1]
		}
		if name != member || !strings.HasPrefix(ln, "func ") {
			continue
		}
		out := append([]string{}, lines[:i]...)
		out = append(out, "// @ ensures false")
		out = append(out, lines[i:]...)
		return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
	}
	return fmt.Errorf("%s: member %s not found", path, member)
}
