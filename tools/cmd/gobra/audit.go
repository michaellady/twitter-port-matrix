package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// cmdAudit re-reads a canary sweep and checks that every REFUTABLE verdict is
// backed by an error Gobra reported *inside the clause's own member*.
//
// The sweep verifies a whole package, so a negated postcondition can break a
// caller in the same package instead of -- or as well as -- the method it sits
// on. `(*MemStore).Replace` calls `isMonotoneLog`, for instance. An error
// reported only at the caller would still make the sweep say REFUTABLE, and
// that would be the right answer to the wrong question: it shows the clause is
// load-bearing somewhere, not that the clause's own exit state is reachable.
//
// Anything this flags needs re-running on its own before it is believed.
func cmdAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	in := fs.String("in", "evidence/runs/gobra/negation-canaries.json", "canary results to audit")
	impl := fs.String("impl", "impls/go", "the Go implementation directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := implDirFromArgs([]string{"-impl", *impl})
	if err != nil {
		return err
	}
	var results []canaryResult
	if err := readJSON(*in, &results); err != nil {
		return err
	}

	spans := map[string]map[string][2]int{}
	var offsite, clean, other int
	for _, r := range results {
		if r.Verdict != refutable {
			other++
			continue
		}
		if _, ok := spans[r.Clause.File]; !ok {
			s, err := memberSpans(filepath.Join(implDir, r.Clause.File))
			if err != nil {
				return err
			}
			spans[r.Clause.File] = s
		}
		span, ok := spans[r.Clause.File][r.Clause.Member]
		if !ok {
			return fmt.Errorf("%s: no span for member %s", r.Clause.File, r.Clause.Member)
		}
		if anyErrorIn(r.Errors, r.Clause.File, span) {
			clean++
			continue
		}
		offsite++
		fmt.Printf("OFF-SITE  %s:%d  %s\n    clause: %s\n    member spans lines %d-%d; Gobra reported:\n",
			r.Clause.File, r.Clause.StartLine, r.Clause.Member, r.Clause.Text, span[0], span[1])
		for _, e := range r.Errors {
			fmt.Printf("      %s\n", e)
		}
	}
	fmt.Printf("\naudited %d REFUTABLE verdicts: %d backed by an error inside the clause's own\n"+
		"member, %d backed only by an error elsewhere. (%d results were not REFUTABLE.)\n",
		clean+offsite, clean, offsite, other)
	if offsite > 0 {
		return fmt.Errorf("%d REFUTABLE verdict(s) are not attributable to their own member", offsite)
	}
	return nil
}

var reErrLoc = regexp.MustCompile(`^(\S+?):(\d+):\d+ `)

func anyErrorIn(errs []string, file string, span [2]int) bool {
	base := filepath.Base(file)
	for _, e := range errs {
		m := reErrLoc.FindStringSubmatch(e)
		if m == nil {
			continue
		}
		if filepath.Base(m[1]) != base {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if n >= span[0] && n <= span[1] {
			return true
		}
	}
	return false
}

// memberSpans maps each func in a file to the line range its contract and body
// occupy: from the first line of the preceding annotation block through the
// closing brace in column 0. Gobra reports a failing postcondition at the
// method's own position, so that range is where the evidence has to land.
func memberSpans(path string) (map[string][2]int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	out := map[string][2]int{}
	for i := 0; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		var name string
		if m := reRecvName.FindStringSubmatch(s); m != nil {
			name = "(*" + m[1] + ")." + m[2]
		} else if m := reFuncDecl.FindStringSubmatch(s); m != nil {
			name = m[1]
		}
		if name == "" || !strings.HasPrefix(lines[i], "func ") {
			continue
		}
		start := i + 1
		for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "//"); j-- {
			start = j + 1
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "}" {
				end = j + 1
				break
			}
		}
		out[name] = [2]int{start, end}
	}
	// Ghost declarations live entirely inside `// @` comments and have no
	// body; span them over their annotation block.
	for i := 0; i < len(lines); i++ {
		body := strings.TrimSpace(annotBody(lines[i]))
		if !strings.HasPrefix(body, "func ") {
			continue
		}
		var name string
		if m := reRecvName.FindStringSubmatch(body); m != nil {
			name = "(*" + m[1] + ")." + m[2]
		} else if m := reFuncDecl.FindStringSubmatch(body); m != nil {
			name = m[1]
		}
		if name == "" {
			continue
		}
		start := i + 1
		for j := i - 1; j >= 0 && isAnnot(lines[j]); j-- {
			start = j + 1
		}
		out[name] = [2]int{start, i + 1}
	}
	return out, nil
}
