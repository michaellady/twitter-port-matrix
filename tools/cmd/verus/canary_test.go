package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The canary shape is the whole instrument. For `A ==> B` the canary is `!A`.
// The tempting alternative, `!(A ==> B)`, is `A && !B` -- unsatisfiable
// exactly when A is unsatisfiable -- so a verifier refutes it precisely when
// the clause is VACUOUS and every vacuous clause scores as live. That is the
// failure this test exists to prevent, and it is not hypothetical: it is the
// mistake F013 was written about.
func TestNegateOfAnImplicationIsTheNegatedAntecedent(t *testing.T) {
	got, err := negate("from@ == to@ ==> result is Err")
	if err != nil {
		t.Fatalf("negate: %v", err)
	}
	if want := "!(from@ == to@)"; got != want {
		t.Fatalf("canary = %q, want %q", got, want)
	}
	if strings.Contains(got, "==>") {
		t.Fatalf("canary %q still carries the implication; it negates the whole clause instead of the antecedent", got)
	}
}

func TestNegateOfANonImplicationIsTheWholeClause(t *testing.T) {
	got, err := negate("result is Ok")
	if err != nil {
		t.Fatalf("negate: %v", err)
	}
	if want := "!(result is Ok)"; got != want {
		t.Fatalf("canary = %q, want %q", got, want)
	}
}

// A quantified clause needs the matrix negated UNDER the quantifier
// (`forall x :: R(x) ==> B(x)` -> `forall x :: !R(x)`), which is a different
// rewrite. This corner has no quantified shipped clause to test that against,
// so the negator refuses instead of guessing: an ILL-FORMED cell that names
// the shape is worth more than a verdict built on a canary nobody checked.
func TestNegateRefusesQuantifiedClauses(t *testing.T) {
	for _, c := range []string{
		"forall|i: int| 0 <= i < s.len() ==> s[i] > 0",
		"exists|x: int| p(x)",
	} {
		if _, err := negate(c); err == nil {
			t.Fatalf("negate(%q) succeeded; a guessed canary under a quantifier reports vacuity backwards", c)
		}
	}
}

// An `==>` inside brackets is not the top-level connective. Splitting there
// builds a canary out of half an expression, which does not compile, and an
// ILL-FORMED cell looks like a tooling problem rather than the parse bug it is.
func TestIndexTopLevelIgnoresBracketedOperators(t *testing.T) {
	s := "f(a ==> b) == c"
	if i := indexTopLevel(s, "==>"); i != -1 {
		t.Fatalf("indexTopLevel found a top-level ==> at %d in %q; it is inside the call", i, s)
	}
	s2 := "f(a ==> b) ==> c"
	i := indexTopLevel(s2, "==>")
	if i < 0 || strings.TrimSpace(s2[:i]) != "f(a ==> b)" {
		t.Fatalf("indexTopLevel split %q at %d; antecedent should be the whole call", s2, i)
	}
}

// The checkpoint and report key must separate two clauses that read the same.
// On the Go corner a (file, text) key collided across ten members sharing a
// framing clause, a resumed sweep reused one member's verdict for another's,
// and the resulting number had to be withdrawn. A key that cannot tell a
// TIMEOUT from a REFUTABLE cannot be trusted to tell a VACUOUS from one
// either, so the function is part of this instrument, not a detail.
func TestClauseKeySeparatesTwoFunctionsSharingClauseText(t *testing.T) {
	a := &clauseBlock{Rel: "crates/store/src/lib.rs", Func: "put"}
	b := &clauseBlock{Rel: "crates/store/src/lib.rs", Func: "get"}
	text := "self.len() == old(self).len()"
	if (clause{Block: a, Text: text}).Key() == (clause{Block: b, Text: text}).Key() {
		t.Fatal("two functions sharing a framing clause collide on the same key")
	}
}

// The shipped/twin split is the Rust corner's central structural fact (F012,
// F016, F027): most obligations sit on hand-written functions inside
// `#[cfg(verus_only)] mod verus_proof`, over `external_body` shims, with
// nothing tying them to the code that ships. This test re-derives the split
// from the real tree so the claim cannot go stale, and so a clause moving out
// of a twin -- the repair everyone wants -- shows up as a failure here rather
// than as a quietly larger sweep.
func TestShippedClausesAreExactlyFollowNew(t *testing.T) {
	root := repoRoot(t)
	implDir := filepath.Join(root, "impls", "rust")
	crates, err := verifyEnabledCrates(implDir)
	if err != nil {
		t.Skipf("no Rust tree here: %v", err)
	}
	blocks, err := extractClauses(implDir, crates)
	if err != nil {
		t.Fatalf("extractClauses: %v", err)
	}
	shipped, twin := splitBlocks(blocks)
	if len(shipped) != 1 {
		var names []string
		for _, b := range shipped {
			names = append(names, b.Rel+":"+b.Func)
		}
		t.Fatalf("shipped ensures blocks = %d (%s); the Rust corner has exactly one, Follow::new in crates/domain. "+
			"If a contract moved onto a shipped function this is the good news -- update the test and F027 together", len(shipped), strings.Join(names, ", "))
	}
	b := shipped[0]
	if b.Func != "new" || !strings.Contains(b.Rel, "crates/domain") {
		t.Fatalf("the one shipped block is %s:%s, want new in crates/domain", b.Rel, b.Func)
	}
	if len(b.Clauses) != 5 {
		t.Fatalf("Follow::new carries %d clause(s), want 5", len(b.Clauses))
	}
	for _, c := range b.Clauses {
		if _, err := negate(c.Text); err != nil {
			t.Fatalf("no canary for shipped clause %q: %v", c.Text, err)
		}
	}
	if len(twin) == 0 {
		t.Fatal("no twin blocks found; the parser is not seeing verus_proof modules and the shipped count is inflated")
	}
}

// The canary REPLACES the clause list rather than joining it. Left in place,
// the original clauses can fail on their own and the run would attribute their
// failure to the canary -- the sweep would report "refutable" for a clause it
// never tested.
func TestSpliceReplacesTheWholeEnsuresList(t *testing.T) {
	src := `verus! {
    impl Follow {
        pub fn new(from: String, to: String) -> (result: R)
            ensures
                from@ == to@ ==> result is Err,
                from@ != to@ ==> result is Ok,
        {
            body
        }
    }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.rs")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks := parseEnsures(src, "domain", path, "crates/domain/src/lib.rs")
	if len(blocks) != 1 || len(blocks[0].Clauses) != 2 {
		t.Fatalf("parsed %d block(s); want 1 with 2 clauses", len(blocks))
	}
	if _, err := spliceCanary(blocks[0], "!(from@ == to@)", nil); err != nil {
		t.Fatalf("splice: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "result is Ok") {
		t.Fatalf("the other clause survived the splice:\n%s", got)
	}
	if !strings.Contains(got, "!(from@ == to@),") {
		t.Fatalf("canary missing from the spliced file:\n%s", got)
	}
	if !strings.Contains(got, "body") || !strings.Contains(got, "pub fn new") {
		t.Fatalf("splice damaged the function:\n%s", got)
	}
}

// The self-test's injection point has to land ahead of `ensures`, or the file
// does not compile and every clause comes back ILL-FORMED -- which would read
// as "the sweep could not decide" rather than "the sweep is broken".
func TestSpliceInjectsExtraAheadOfEnsures(t *testing.T) {
	src := "    fn f() -> (r: bool)\n        ensures\n            r == true,\n    {\n        true\n    }\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.rs")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks := parseEnsures(src, "c", path, "c/lib.rs")
	if len(blocks) != 1 {
		t.Fatalf("parsed %d blocks", len(blocks))
	}
	if _, err := spliceCanary(blocks[0], "!(r)", []string{"requires false,"}); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	got := string(out)
	ri, ei := strings.Index(got, "requires false,"), strings.Index(got, "ensures")
	if ri < 0 || ei < 0 || ri > ei {
		t.Fatalf("requires must precede ensures:\n%s", got)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("repository root not found")
	return ""
}
