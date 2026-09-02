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
		out[name] = [2]int{start, bodyEnd(lines, i)}
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
		// Stop the scan-back at the PREVIOUS ghost declaration, not just at
		// the top of the comment block. Three ghost members share one block in
		// internal/dom/dom.go -- ErrorMem, IsDuplicableMem and Duplicate --
		// and without this every one of them starts at the block's first line,
		// so all three claim the same lines and an error inside the block is
		// attributed to whichever the map happens to yield (F040).
		start := i + 1
		for j := i - 1; j >= 0 && isAnnot(lines[j]); j-- {
			if isGhostDecl(lines[j]) {
				break
			}
			start = j + 1
		}
		out[name] = [2]int{start, i + 1}
	}
	return out, nil
}

// bodyEnd returns the line AFTER the closing brace of the function whose
// signature starts at line funcLine, by counting braces.
//
// F040 is why this is not a scan for the first line equal to "}". A one-line
// method -- `func (e invalidHandleError) Error() string { return "..." }`, of
// which internal/dom/dom.go has three -- closes its body on its own line and
// never produces a bare "}". The old scan therefore ran past it and stopped at
// the next function's closing brace, so `Error` claimed every line up to the
// end of ValidHandle. Gobra reported a failing loop invariant at dom.go:206,
// inside ValidHandle, and the R5 rung attributed it to `(*invalidHandleError).Error`
// at line 115. Which member a line resolves to decided nothing that has been
// published, but it decides the R5 column, and "the answer depends on map
// iteration order" is not a property an instrument may have.
func bodyEnd(lines []string, funcLine int) int {
	depth, opened := 0, false
	for j := funcLine; j < len(lines); j++ {
		for _, ch := range codeOnly(lines[j]) {
			switch ch {
			case '{':
				depth++
				opened = true
			case '}':
				depth--
			}
		}
		if opened && depth <= 0 {
			return j + 1
		}
	}
	return len(lines)
}

// codeOnly strips line comments and the contents of string and rune literals,
// so a brace inside either is not counted as structure. Gobra annotations are
// comments and carry braces of their own -- `// @ pred (e T) ErrorMem() { true }`
// -- and counting those would end a body in the middle of its own doc block.
func codeOnly(s string) string {
	var b strings.Builder
	const (
		plain = iota
		inStr
		inRaw
		inRune
	)
	state := plain
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {
		case inStr:
			if c == '\\' {
				i++
			} else if c == '"' {
				state = plain
			}
		case inRaw:
			if c == '`' {
				state = plain
			}
		case inRune:
			if c == '\\' {
				i++
			} else if c == '\'' {
				state = plain
			}
		default:
			switch {
			case c == '/' && i+1 < len(s) && s[i+1] == '/':
				return b.String()
			case c == '"':
				state = inStr
			case c == '`':
				state = inRaw
			case c == '\'':
				state = inRune
			default:
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// isGhostDecl reports whether an annotation line declares a ghost member.
func isGhostDecl(line string) bool {
	body := strings.TrimSpace(annotBody(line))
	return strings.HasPrefix(body, "func ") || strings.HasPrefix(body, "pred ")
}
