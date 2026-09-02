package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file extracts the Rust corner's `ensures` clauses and builds the
// NEGATION CANARY for each one.
//
// Why a negation canary and not the injection canary the R4 rung already has:
// an injection canary asks "if I break the code, does the gate notice". Put to
// a vacuous obligation that question is ill-posed -- an obligation nothing
// reaches verifies over the broken code too, and over the infeasible point the
// injection created. Only a NEGATION canary can see it: under vacuity a claim
// AND its negation both verify, and nothing else produces that signature.
// F013 is the finding; tools/cmd/gobra and tools/cmd/jbmc are the instruments
// on the other two corners. Until this file existed the Rust corner had no
// instrument at all, and GOAL.md's own rule -- "a proof-backed result counts
// only if the obligation was shown non-vacuous" -- was not being met by the
// row `calibrate` prints.
//
// THE CANARY SHAPE IS THE PART THAT IS EASY TO GET WRONG. For a clause
// `A ==> B`, the canary is `!A`, NOT `!(A ==> B)`. The latter is `A && !B`,
// which is unsatisfiable exactly when A is unsatisfiable -- so a verifier
// refutes it precisely when the clause is VACUOUS, and a vacuous clause scores
// as live. The canary asks whether the antecedent is reachable at all, and the
// verifier refuting `!A` is what says it is.

// A clauseBlock is one `ensures` list on one function.
//
// The whole list is tracked, not just individual clauses, because the canary
// REPLACES the list rather than adding to it. Leaving the original clauses in
// place would mean a FAILED verdict could come from any of them, and the run
// would prove nothing about the canary -- the attribution problem
// tools/cmd/gobra/audit.go exists to solve on the Go corner.
type clauseBlock struct {
	Crate string
	File  string // absolute
	Rel   string // relative to the implementation root, for display
	Func  string

	HeadLine  int // 0-based index of the `ensures` line
	FirstLine int // 0-based index of the first clause line
	LastLine  int // 0-based index of the last clause line, inclusive
	Indent    string

	Clauses []clause
	Twin    bool // inside a #[cfg(verus_only)] mod verus_proof block
	// Ghost is set for a `proof fn` or `spec fn`. Its `ensures` is a lemma
	// about the ghost world, not an obligation on code that runs, so a
	// negation canary over it does not say anything about the shipped row.
	Ghost bool
	// Assumed is set for a function carrying `#[verifier::external_body]` or
	// whose body is `admit()`. Its postcondition is ASSUMED, so the canary
	// would report VACUOUS for the trivial reason that Verus proves anything
	// there -- a verdict that looks like a finding and is a tautology. These
	// are counted and named, never swept and never folded into the shipped
	// number. See F042.
	Assumed bool
}

// A clause is one `ensures` obligation.
type clause struct {
	Block *clauseBlock
	Text  string // the clause, comma stripped, newlines collapsed to spaces
	Line  int    // 0-based index of its first line, for display
}

// Key names the clause in the checkpoint and in the report. It carries the
// function as well as the file and the text: framing clauses repeat verbatim
// across members, and a (file, text) key silently collides between two
// obligations whose verdicts may differ. That collision was a live bug in the
// Go corner's sweep and cost a withdrawn number; it is not being rebuilt here.
func (c clause) Key() string {
	return c.Block.Rel + "\x00" + c.Block.Func + "\x00" + c.Text
}

// extractClauses reads every verify-enabled crate and returns its `ensures`
// blocks, marked as shipped or twin.
//
// The shipped/twin split is not cosmetic. F012 and F016 established that most
// Verus obligations on this corner sit on hand-written functions inside
// `#[cfg(verus_only)] mod verus_proof`, over `external_body` shims, with
// nothing mechanically tying them to the code that ships. A canary on a twin
// measures the twin. This function re-derives that split from the tree instead
// of restating it from a finding, so it cannot go stale.
func extractClauses(implDir string, crates []crate) ([]*clauseBlock, error) {
	var out []*clauseBlock
	for _, c := range crates {
		dir := filepath.Join(implDir, c.Rel, "src")
		err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() || !strings.HasSuffix(p, ".rs") {
				return nil
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			rel, _ := filepath.Rel(implDir, p)
			out = append(out, parseEnsures(string(b), c.Name, p, rel)...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", c.Rel, err)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rel != out[j].Rel {
			return out[i].Rel < out[j].Rel
		}
		return out[i].HeadLine < out[j].HeadLine
	})
	return out, nil
}

// parseEnsures is a line scanner, not a Rust parser.
//
// It handles the one shape this corner actually uses -- `ensures` alone on a
// line, followed by comma-terminated clauses, followed by the opening brace of
// the body -- and it is backed by a test that re-reads the real tree, so a
// shape it cannot read shows up as a missing clause in the test rather than as
// a silently smaller denominator.
func parseEnsures(src, crateName, abs, rel string) []*clauseBlock {
	lines := strings.Split(src, "\n")
	var out []*clauseBlock
	twinDepth := -1 // brace depth at which `mod verus_proof` opened
	depth := 0
	fn := ""
	ghost := false
	assumed := false
	// pendingAssumed carries an `#[verifier::external_body]` (or a nearby
	// `admit()`) forward to the next signature line it applies to.
	pendingAssumed := false

	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		t := strings.TrimSpace(ln)

		if strings.HasPrefix(t, "mod verus_proof") {
			twinDepth = depth
		}
		if strings.Contains(t, "external_body") {
			pendingAssumed = true
		}
		if f, ok := fnName(t); ok {
			fn = f
			ghost = isGhostSignature(t)
			assumed = pendingAssumed || bodyIsAdmitted(lines, i)
			pendingAssumed = false
		}

		if t == "ensures" {
			blk := &clauseBlock{
				Crate: crateName, File: abs, Rel: rel, Func: fn,
				HeadLine: i, Twin: twinDepth >= 0,
				Ghost: ghost, Assumed: assumed,
			}
			// Clauses run until the line that opens the body. `requires`
			// always precedes `ensures` in Verus, so nothing but clauses and
			// the brace can follow.
			j := i + 1
			var cur []string
			curStart := -1
			for ; j < len(lines); j++ {
				ct := strings.TrimSpace(lines[j])
				if ct == "" {
					continue
				}
				if strings.HasPrefix(ct, "{") || strings.HasPrefix(ct, "decreases") || strings.HasPrefix(ct, "invariant") {
					break
				}
				if curStart < 0 {
					curStart = j
					blk.Indent = leadingSpace(lines[j])
				}
				cur = append(cur, ct)
				if strings.HasSuffix(ct, ",") {
					text := strings.TrimSuffix(strings.Join(cur, " "), ",")
					blk.Clauses = append(blk.Clauses, clause{Block: blk, Text: strings.TrimSpace(text), Line: curStart})
					cur, curStart = nil, -1
				}
			}
			if len(cur) > 0 && curStart >= 0 {
				// A final clause with no trailing comma is legal Rust.
				blk.Clauses = append(blk.Clauses, clause{Block: blk, Text: strings.TrimSpace(strings.Join(cur, " ")), Line: curStart})
			}
			if len(blk.Clauses) > 0 {
				blk.FirstLine = blk.Clauses[0].Line
				blk.LastLine = j - 1
				// Trim trailing blank lines out of the replaced range.
				for blk.LastLine > blk.FirstLine && strings.TrimSpace(lines[blk.LastLine]) == "" {
					blk.LastLine--
				}
				out = append(out, blk)
			}
			i = j - 1
			continue
		}

		depth += strings.Count(ln, "{") - strings.Count(ln, "}")
		if twinDepth >= 0 && depth <= twinDepth {
			twinDepth = -1
		}
	}
	return out
}

func leadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// fnName pulls the name out of a function signature line.
//
// The modifier list is not decoration. Verus signatures carry `spec fn`,
// `proof fn`, `open spec fn`, `closed spec fn` and `broadcast proof fn`, and a
// prefix this function does not recognise makes it return the PREVIOUS
// function's name -- so the block gets attributed to whatever `fn` was seen
// last. That happened: two `broadcast proof fn` axioms in `crates/store` were
// reported against `impl Display for StoreError`'s `fmt`, sixty lines away and
// in a different impl. F042.
func fnName(t string) (string, bool) {
	idx := strings.Index(t, "fn ")
	if idx < 0 {
		return "", false
	}
	// Only a signature, not `fn` inside a comment or a type.
	if idx > 0 {
		prefix := t[:idx]
		ok := false
		for _, m := range []string{"pub ", "unsafe ", "const ", "async ", "spec ", "proof ", "exec ", "broadcast "} {
			if strings.HasSuffix(prefix, m) {
				ok = true
				break
			}
		}
		if !ok && strings.TrimSpace(prefix) != "" {
			return "", false
		}
	}
	rest := t[idx+3:]
	end := strings.IndexAny(rest, "(<")
	if end <= 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:end]), true
}

// errUnsupportedShape marks a clause whose negation this file will not guess.
//
// Returning an error is the point. A canary built from a shape the negator did
// not actually understand produces a verdict that looks like a measurement,
// and a wrong canary reports a live clause as vacuous or the reverse. An
// ILL-FORMED cell that names the shape is worth more than a number nobody can
// check.
type errUnsupportedShape struct{ text, why string }

func (e *errUnsupportedShape) Error() string {
	return fmt.Sprintf("cannot build a negation canary for %q: %s", e.text, e.why)
}

// negate builds the negation canary for one clause.
//
//	A ==> B      ->  !(A)      the antecedent is what has to be reachable
//	C            ->  !(C)
//
// A quantified clause is refused rather than guessed: `forall x :: R(x) ==>
// B(x)` needs `forall x :: !R(x)`, which is a different rewrite from the
// propositional one, and this corner has no quantified shipped clause to test
// it against today. When one appears, the test that re-reads the tree will
// fail on the ILL-FORMED cell, which is the intended way to find out.
func negate(text string) (string, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", &errUnsupportedShape{t, "empty clause"}
	}
	if strings.Contains(t, "forall") || strings.Contains(t, "exists") {
		return "", &errUnsupportedShape{t, "quantified; the canary is the negated MATRIX under the same quantifier, not the negation of the whole clause"}
	}
	if i := indexTopLevel(t, "==>"); i >= 0 {
		ant := strings.TrimSpace(t[:i])
		if ant == "" {
			return "", &errUnsupportedShape{t, "implication with an empty antecedent"}
		}
		return "!(" + ant + ")", nil
	}
	return "!(" + t + ")", nil
}

// indexTopLevel finds sep outside any bracket. Verus clauses carry generics
// and calls -- `result->Ok_0.from@ == from@`, `s.contains(x)` -- so a naive
// strings.Index would split inside one.
func indexTopLevel(s, sep string) int {
	depth := 0
	for i := 0; i+len(sep) <= len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			depth--
			continue
		}
		if depth == 0 && strings.HasPrefix(s[i:], sep) {
			return i
		}
	}
	return -1
}

// spliceCanary rewrites the file so the block's ENTIRE ensures list is the one
// canary clause, and returns the original bytes so the caller can restore.
//
// extra is spliced in ahead of `ensures` and is how the self-test injects
// `requires false,`: with a false precondition every postcondition is
// vacuously provable, so a sweep that cannot report VACUOUS there is not
// measuring anything and says so instead of continuing.
func spliceCanary(blk *clauseBlock, canary string, extra []string) (original []byte, err error) {
	original, err = os.ReadFile(blk.File)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(original), "\n")
	if blk.LastLine >= len(lines) {
		return nil, fmt.Errorf("%s: block ends at line %d but the file has %d", blk.Rel, blk.LastLine+1, len(lines))
	}
	head := leadingSpace(lines[blk.HeadLine])

	var repl []string
	for _, e := range extra {
		repl = append(repl, head+e)
	}
	repl = append(repl, head+"ensures", blk.Indent+canary+",")

	out := append([]string{}, lines[:blk.HeadLine]...)
	out = append(out, repl...)
	out = append(out, lines[blk.LastLine+1:]...)
	if err := os.WriteFile(blk.File, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		return nil, err
	}
	return original, nil
}

// isGhostSignature reports whether a signature line declares a `spec fn` or a
// `proof fn` -- ghost items, which do not run. Their `ensures` is a lemma, and
// a negation canary over a lemma measures the lemma.
func isGhostSignature(t string) bool {
	i := strings.Index(t, "fn ")
	if i < 0 {
		return false
	}
	head := t[:i]
	return strings.Contains(head, "spec ") || strings.Contains(head, "proof ")
}

// bodyIsAdmitted reports whether the function opening at or just after `sig`
// has `admit();` as its body. An admitted body makes every postcondition
// provable, so its clauses are assumed, not proved, and the sweep must say so
// rather than sweeping them.
//
// The scan is bounded: it looks only as far as the opening brace plus a few
// lines, because an `admit()` deeper inside a real body is a different animal
// (a hole in a proof) and is not what this flag is for.
func bodyIsAdmitted(lines []string, sig int) bool {
	for j := sig; j < len(lines) && j < sig+40; j++ {
		t := strings.TrimSpace(lines[j])
		if t == "{" || strings.HasSuffix(t, "{") {
			for k := j + 1; k < len(lines) && k < j+4; k++ {
				b := strings.TrimSpace(lines[k])
				if b == "" || strings.HasPrefix(b, "//") {
					continue
				}
				return strings.HasPrefix(b, "admit()") || strings.HasPrefix(b, "unimplemented!()")
			}
			return false
		}
	}
	return false
}
