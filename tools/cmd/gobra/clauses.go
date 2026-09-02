package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A clause is one `ensures` in the Go corner's contracts, together with the
// member it sits on and the negation the canary sweep will substitute for it.
type clause struct {
	File      string // repo-relative, e.g. internal/store/memstore.go
	Pkg       string // internal/store
	StartLine int    // 1-based line of the `ensures` keyword
	EndLine   int    // last continuation line
	Member    string // the func/method the contract belongs to
	Text      string // the clause expression, continuations joined
	Kind      clauseKind
	Canary    string // the expression the canary substitutes
	CanaryWhy string // what verifying the canary would mean
}

type clauseKind string

const (
	// functional clauses are the ones a canary can interrogate.
	kindFunctional clauseKind = "functional"
	// framing clauses are permission accounting (acc(...)). Their negation is
	// not a well-formed Gobra assertion, and "nothing reaches it" is not a
	// question about them.
	kindFraming clauseKind = "framing"
	// assumedTrusted clauses sit on `// @ trusted` members. Gobra assumes
	// them rather than checking them, so they were never VERIFIED and a
	// negation canary would inject unsoundness rather than test anything.
	kindAssumedTrusted clauseKind = "assumed(trusted)"
	// assumedAbstract clauses sit on a body-less declaration -- a ghost
	// signature in a .gobra file. Same status: assumed, never checked.
	kindAssumedAbstract clauseKind = "assumed(abstract)"
)

var (
	reAnnot     = regexp.MustCompile(`^//\s*@\s?(.*)$`)
	reDirective = regexp.MustCompile(`^(ensures|requires|preserves|decreases|trusted|pure|opaque|pred|func|import|package|ghost|invariant|assert|fold|unfold|inhale|exhale|share|outline)\b`)
	reFuncDecl  = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`)
	reRecvName  = regexp.MustCompile(`^func\s+\(\s*\w+\s+\*?(\w+)\s*\)\s*([A-Za-z_]\w*)`)
)

// extractClauses parses one source file. Gobra contracts live in `// @`
// comments so the file also compiles under `go build`, which is why this is a
// line scanner rather than anything that consults the Go AST.
func extractClauses(implDir, rel string) ([]clause, error) {
	b, err := os.ReadFile(filepath.Join(implDir, rel))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	pkg := filepath.ToSlash(filepath.Dir(rel))

	var out []clause
	i := 0
	for i < len(lines) {
		if !isAnnot(lines[i]) {
			i++
			continue
		}
		// Consume one maximal annotation block.
		start := i
		for i < len(lines) && isAnnot(lines[i]) {
			i++
		}
		block := lines[start:i]

		trusted, abstract, member := blockShape(block)
		if !abstract {
			// The member is the next code line; a block that is not followed
			// by one is a stray comment and carries no obligation.
			if m := nextFunc(lines, i); m != "" {
				member = m
			} else if member == "" {
				continue
			}
		}
		for _, c := range blockClauses(block, start) {
			c.File, c.Pkg, c.Member = rel, pkg, member
			switch {
			case trusted:
				c.Kind = kindAssumedTrusted
			case abstract:
				c.Kind = kindAssumedAbstract
			case strings.HasPrefix(c.Text, "acc("):
				c.Kind = kindFraming
			default:
				c.Kind = kindFunctional
				c.Canary, c.CanaryWhy = negate(c.Text)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func isAnnot(ln string) bool { return reAnnot.MatchString(strings.TrimSpace(ln)) }

func annotBody(ln string) string {
	m := reAnnot.FindStringSubmatch(strings.TrimSpace(ln))
	if m == nil {
		return ""
	}
	return m[1]
}

// blockShape reports whether the annotation block marks its member trusted,
// whether the member is declared inside the block itself (a body-less ghost
// signature), and that member's name if so.
func blockShape(block []string) (trusted, abstract bool, member string) {
	for _, ln := range block {
		body := strings.TrimSpace(annotBody(ln))
		switch {
		case strings.HasPrefix(body, "trusted"):
			trusted = true
		case strings.HasPrefix(body, "func "):
			abstract = true
			if m := reRecvName.FindStringSubmatch(body); m != nil {
				member = "(*" + m[1] + ")." + m[2]
			} else if m := reFuncDecl.FindStringSubmatch(body); m != nil {
				member = m[1]
			}
		}
	}
	return
}

// nextFunc returns the name of the first func declaration at or after line i,
// provided nothing but blank lines and plain comments intervene.
func nextFunc(lines []string, i int) string {
	for ; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		if m := reRecvName.FindStringSubmatch(s); m != nil {
			return "(*" + m[1] + ")." + m[2]
		}
		if m := reFuncDecl.FindStringSubmatch(s); m != nil {
			return m[1]
		}
		return ""
	}
	return ""
}

// blockClauses pulls the `ensures` clauses out of one annotation block,
// joining continuation lines. A continuation is an annotation line that does
// not open a new directive.
func blockClauses(block []string, offset int) []clause {
	var out []clause
	for j := 0; j < len(block); j++ {
		body := strings.TrimSpace(annotBody(block[j]))
		if !strings.HasPrefix(body, "ensures") {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(body, "ensures"))
		end := j
		for k := j + 1; k < len(block); k++ {
			nb := strings.TrimSpace(annotBody(block[k]))
			if nb == "" || reDirective.MatchString(nb) {
				break
			}
			text += " " + nb
			end = k
		}
		out = append(out, clause{
			StartLine: offset + j + 1,
			EndLine:   offset + end + 1,
			Text:      strings.Join(strings.Fields(text), " "),
		})
		j = end
	}
	return out
}

// negate builds the canary assertion for a clause.
//
// The shape matters, and getting it wrong is the difference between a canary
// and a decoration. For a plain assertion P, the canary is !P: if both P and
// !P verify, the exit state is unreachable and P was vacuous.
//
// For an implication A ==> B the canary is NOT !(A ==> B). That is A && !B,
// which is false exactly when A is unsatisfiable -- so a vacuous clause would
// make the canary fail and be scored as live, which is backwards. The right
// question for an implication is whether its antecedent can hold at all, so
// the canary is !A. The same applies under a quantifier: for
// `forall x :: R(x) ==> B(x)` the canary is `forall x :: !R(x)`, which
// verifies exactly when the range is always empty.
func negate(text string) (string, string) {
	if prefix, rest, ok := splitQuantifier(text); ok {
		if ante, _, ok := splitImplication(rest); ok {
			return prefix + " !(" + ante + ")",
				"quantifier range is always empty -- the clause says nothing about any element"
		}
	}
	if ante, _, ok := splitImplication(text); ok {
		return "!(" + ante + ")",
			"antecedent never holds -- the clause is vacuously true"
	}
	return "!(" + text + ")",
		"the negation also verifies -- the exit state is unreachable"
}

// splitQuantifier peels one leading `forall <decls> ::`.
func splitQuantifier(s string) (prefix, rest string, ok bool) {
	if !strings.HasPrefix(s, "forall ") {
		return "", "", false
	}
	idx := indexTopLevel(s, "::")
	if idx < 0 {
		return "", "", false
	}
	return s[:idx+2], strings.TrimSpace(s[idx+2:]), true
}

// splitImplication finds the leftmost `==>` outside parentheses, braces and
// brackets, and outside any nested quantifier body.
func splitImplication(s string) (ante, cons string, ok bool) {
	idx := indexTopLevel(s, "==>")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+3:]), true
}

// indexTopLevel returns the index of tok at bracket depth zero, stopping at a
// nested quantifier's `::` so that a quantified subterm is never split.
func indexTopLevel(s, tok string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			depth--
			continue
		}
		if depth != 0 {
			continue
		}
		if tok != "::" && strings.HasPrefix(s[i:], "::") {
			// A quantifier opens here; anything after it belongs to its body.
			return -1
		}
		if strings.HasPrefix(s[i:], tok) {
			return i
		}
	}
	return -1
}

func cmdClauses(args []string) error {
	implDir, err := implDirFromArgs(args)
	if err != nil {
		return err
	}
	all, err := allClauses(implDir)
	if err != nil {
		return err
	}
	counts := map[clauseKind]int{}
	for _, c := range all {
		counts[c.Kind]++
		fmt.Printf("%-34s %-28s %-18s %s\n", c.File+":"+fmt.Sprint(c.StartLine), c.Member, c.Kind, trunc(c.Text, 90))
	}
	fmt.Printf("\n%d clauses: %d functional, %d framing, %d assumed(trusted), %d assumed(abstract)\n",
		len(all), counts[kindFunctional], counts[kindFraming],
		counts[kindAssumedTrusted], counts[kindAssumedAbstract])
	return nil
}

// contractFiles are the sources carrying the Go corner's contracts. Files
// under stubs/ are excluded on purpose: they are the trusted model of the
// standard library, not obligations about this implementation.
var contractFiles = []string{
	"internal/clock/clock.go", "internal/clock/clock.gobra",
	"internal/ids/ids.go", "internal/ids/ids.gobra",
	"internal/dom/dom.go", "internal/dom/dom.gobra",
	"internal/store/memstore.go", "internal/store/memstore.gobra",
	"internal/service/service.go", "internal/service/service.gobra",
}

func allClauses(implDir string) ([]clause, error) {
	var all []clause
	for _, f := range contractFiles {
		cs, err := extractClauses(implDir, f)
		if err != nil {
			return nil, err
		}
		all = append(all, cs...)
	}
	return all, nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func implDirFromArgs(args []string) (string, error) {
	dir := "impls/go"
	for i := 0; i < len(args); i++ {
		if args[i] == "-impl" && i+1 < len(args) {
			dir = args[i+1]
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return "", fmt.Errorf("%s does not look like the Go implementation (no go.mod)", abs)
	}
	return abs, nil
}
