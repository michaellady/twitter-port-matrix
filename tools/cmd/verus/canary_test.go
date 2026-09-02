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
// F016, F027): obligations used to sit almost entirely on hand-written
// functions inside `#[cfg(verus_only)] mod verus_proof`, over `external_body`
// shims, with nothing tying them to the code that ships.
//
// **This test used to assert exactly one shipped block, `Follow::new`.** It was
// written as a tripwire for the repair, and on 2026-09-02 the repair landed:
// `crates/ids`, `crates/clock` and `crates/store` had their state lifted out of
// `Mutex` / `RwLock` into plain owned value types, and their contracts moved
// onto the shipped functions (F041). The tripwire fired, which is what it was
// for; it is re-armed here against the NEW truth, so the next crate to be
// lifted -- `crates/service` is the one left -- trips it in turn.
func TestShippedClausesCoverTheLiftedCrates(t *testing.T) {
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
	shipped, twin, _, assumed := splitBlocks(blocks)

	// Every crate whose state has been lifted out of its lock must carry at
	// least one contract on a function that ships. A crate dropping off this
	// list is a regression, not a refactor.
	byCrate := map[string][]string{}
	for _, b := range shipped {
		byCrate[b.Crate] = append(byCrate[b.Crate], b.Func)
	}
	for _, want := range []string{"domain", "ids", "clock", "store"} {
		if len(byCrate[want]) == 0 {
			t.Fatalf("crate %s has no shipped ensures block; the lift regressed. shipped = %v", want, byCrate)
		}
	}

	// `crates/service` is the corner's remaining twin holdout: its state is
	// three `Arc`-shared sub-stores plus a write mutex, and lifting it is the
	// next move (F041). When it happens this assertion fires -- deliberately.
	if len(byCrate["service"]) != 0 {
		t.Fatalf("crates/service now has shipped ensures blocks (%v). That is the good news: "+
			"update this test, ASSURANCE.md's Rust rows and F041 together", byCrate["service"])
	}

	// The domain contract that was the corner's ONLY one is still there and
	// still five clauses. It is the control for everything above.
	var follow *clauseBlock
	for _, b := range shipped {
		if b.Func == "new" && strings.Contains(b.Rel, "crates/domain") {
			follow = b
		}
	}
	if follow == nil {
		t.Fatal("Follow::new lost its shipped ensures block")
	}
	if len(follow.Clauses) != 5 {
		t.Fatalf("Follow::new carries %d clause(s), want 5", len(follow.Clauses))
	}

	// Every shipped clause must have a canary. A shipped clause the negator
	// refuses is a clause the R4 row is not licensed to count.
	for _, b := range shipped {
		for _, c := range b.Clauses {
			if _, err := negate(c.Text); err != nil {
				t.Fatalf("no canary for shipped clause %q on %s: %v", c.Text, b.Func, err)
			}
		}
	}

	if len(twin) == 0 {
		t.Fatal("no twin blocks found; the parser is not seeing verus_proof modules and the shipped count is inflated")
	}

	// Shipped must now outnumber twin. Before the lift it was 5 against 57.
	ns, nt := countClauses(shipped), countClauses(twin)
	if ns <= nt {
		t.Fatalf("shipped clauses %d, twin clauses %d; the lift moved the majority onto shipped functions and this asserts it stayed there", ns, nt)
	}

	// The two `obeys_key_model` axioms in crates/store carry `ensures` and are
	// discharged by `admit()`. They must be classified assumed, never shipped:
	// sweeping an admitted postcondition returns VACUOUS as a tautology, which
	// reads as a finding and is not one. F042.
	if len(assumed) == 0 {
		t.Fatal("no assumed blocks found; crates/store's two admitted key-model axioms must be classified assumed, not shipped")
	}
	for _, b := range assumed {
		for _, sb := range shipped {
			if sb == b {
				t.Fatalf("%s:%s is both shipped and assumed", b.Rel, b.Func)
			}
		}
	}
}

// A `broadcast proof fn` signature must be recognised as a signature. When it
// was not, its `ensures` block was attributed to whatever `fn` the scanner had
// seen last -- in the real tree, `impl Display for StoreError`'s `fmt`, sixty
// lines away in a different impl. F042.
func TestFnNameReadsVerusModifiers(t *testing.T) {
	for sig, want := range map[string]string{
		"pub broadcast proof fn axiom_string_obeys_key_model()": "axiom_string_obeys_key_model",
		"pub open spec fn wf(&self) -> bool":                    "wf",
		"pub closed spec fn count(g: &Generator) -> int":        "count",
		"proof fn lemma_x()":                                    "lemma_x",
		"pub fn next(&mut self) -> (out: i64)":                  "next",
	} {
		got, ok := fnName(sig)
		if !ok || got != want {
			t.Fatalf("fnName(%q) = %q, %v; want %q", sig, got, ok, want)
		}
	}
	if !isGhostSignature("pub broadcast proof fn axiom_x()") {
		t.Fatal("a broadcast proof fn must classify as ghost")
	}
	if !isGhostSignature("pub open spec fn wf(&self) -> bool") {
		t.Fatal("an open spec fn must classify as ghost")
	}
	if isGhostSignature("pub fn next(&mut self) -> (out: i64)") {
		t.Fatal("an exec fn must not classify as ghost")
	}
}

// An `admit()` body makes every postcondition provable. Its clauses are
// assumed, and the sweep must exclude them by name rather than report a
// VACUOUS verdict that is a tautology about the body rather than a fact about
// the code. F042.
func TestBodyIsAdmittedSpotsAdmittedAndUnimplementedBodies(t *testing.T) {
	admitted := []string{
		"pub broadcast proof fn a()",
		"    ensures",
		"        p(),",
		"{",
		"    admit();",
		"}",
	}
	if !bodyIsAdmitted(admitted, 0) {
		t.Fatal("an admit() body was not recognised as assumed")
	}
	unimpl := []string{
		"pub closed spec fn f() -> int {",
		"    unimplemented!()",
		"}",
	}
	if !bodyIsAdmitted(unimpl, 0) {
		t.Fatal("an unimplemented!() body was not recognised as assumed")
	}
	real := []string{
		"pub fn next(&mut self) -> (out: i64)",
		"    ensures",
		"        out >= 1,",
		"{",
		"    self.value = self.value + 1;",
		"    self.value",
		"}",
	}
	if bodyIsAdmitted(real, 0) {
		t.Fatal("a real body was misread as assumed")
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
