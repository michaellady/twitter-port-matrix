package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

func r4() rung {
	for _, r := range allRungs {
		if r.ID == "R4" {
			return r
		}
	}
	panic("no R4 rung")
}

// The R4 verdict is read from `gobra verify`'s last line and the exit code has
// to agree with it. Every disagreement is an error cell, never a kill and never
// a survival -- standing rule 1 applied to a proof.
func TestR4Verdict(t *testing.T) {
	r := r4()
	passed := "Gobra's own verdict lines:\n  TOTAL      Gobra has found 0 error(s)   [59s]\nR4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [59s]\n"
	failed := "  TOTAL      Gobra has found 1 error(s)   [61s]\n    memstore.go:88:2 Postcondition might not hold.\nR4 FAILED: Gobra has found 1 error(s) over 5 package(s)   [61s]\n"
	undecided := "Gobra's own report of the termination:\n    | ... got terminated after 60 seconds\nR4 UNDECIDED: Gobra exceeded its 1m0s budget (run 1 of 1); nothing was decided about this tree\n"

	cases := []struct {
		name   string
		out    string
		exit   int
		killed bool
		err    string
	}{
		{"pass agrees", passed, 0, false, ""},
		{"fail agrees", failed, 1, true, ""},
		{"pass but exit 1", passed, 1, false, "contradicts itself"},
		{"fail but exit 0", failed, 0, false, "contradicts itself"},
		{"undecided is no verdict", undecided, 1, false, "no R4 verdict"},
		{"nothing at all", "gobra: reading registry: no such file\n", 1, false, "no R4 verdict"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			killed, err := r.verdict(&toolRun{Stdout: c.out, ExitCode: c.exit})
			if c.err == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if killed != c.killed {
					t.Fatalf("killed=%v, want %v", killed, c.killed)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("want an error containing %q, got %v (killed=%v)", c.err, err, killed)
			}
		})
	}
	if l := verdictLine(failed, r); !strings.HasPrefix(l, "R4 FAILED: Gobra has found 1 error(s)") {
		t.Errorf("verdict line not carried verbatim: %q", l)
	}
}

// A timed-out subprocess is an error cell too, whichever half of the verdict
// it managed to print before calibrate killed it.
func TestR4TimedOutIsError(t *testing.T) {
	_, err := r4().verdict(&toolRun{Stdout: "R4 PASSED: ...\n", ExitCode: 0, TimedOut: true})
	if err == nil {
		t.Fatal("a timed-out rung was scored")
	}
}

// R4 is one rung with one column, dispatching to a different verifier per
// corner: Gobra on go, Verus on rust, JBMC on both JVM corners.
//
// Java was capped here until impls/java carried an obligation set. It was
// never capped for want of a verifier -- JBMC reads its bytecode, and F014's
// own repros are javac output -- but for want of anything to run, and a row
// over an empty denominator is worse than a capped cell. The obligation set is
// written and measured (F034), so the corner is on the rung and its cell is a
// measurement rather than a cap.
func TestR4IsPerCorner(t *testing.T) {
	sel, err := selectRungs([]string{"R0", "R4"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(sel); len(got) != 2 {
		t.Fatalf("selecting R0,R4 gave %v; want one entry per rung ID", got)
	}
	for corner, tool := range map[string]string{"go": "gobra", "rust": "verus", "kotlin": "jbmc", "java": "jbmc"} {
		run, capped := splitRungs(corner, sel)
		if len(run) != 2 || len(capped) != 0 {
			t.Fatalf("%s: runnable=%v capped=%v; want R0 and R4 both runnable", corner, ids(run), ids(capped))
		}
		if got := run[1].toolFor(corner); got != tool {
			t.Errorf("%s R4 is driven by %q, want %q", corner, got, tool)
		}
	}
	// A corner nobody has registered is still capped, and that is the state
	// the vocabulary exists for: capped is "this rung was never asked", not
	// "this rung found nothing".
	run, capped := splitRungs("elixir", sel)
	if len(run) != 1 || run[0].ID != "R0" {
		t.Fatalf("unregistered corner: runnable=%v; want R0 only", ids(run))
	}
	if len(capped) != 1 || capped[0].ID != "R4" {
		t.Fatalf("unregistered corner: capped=%v; want R4 capped", ids(capped))
	}
}

// Coverage is decided from the files a mutant edits against the packages
// Gobra verifies. A mutant confined to the trusted shim cannot be reached by
// any obligation, and scoring its survival against the contract would blame
// the proof for code it never read.
func TestGobraReads(t *testing.T) {
	shim := mutants.Mutant{Edits: []mutants.Edit{{File: "internal/httpshim/shim.go"}}}
	core := mutants.Mutant{Edits: []mutants.Edit{{File: "internal/store/memstore.go"}}}
	both := mutants.Mutant{Edits: []mutants.Edit{{File: "internal/httpshim/shim.go"}, {File: "internal/service/service.go"}}}
	if gobraReads(shim) {
		t.Error("httpshim is trusted transport; Gobra does not read it")
	}
	if !gobraReads(core) || !gobraReads(both) {
		t.Error("an edit inside the verified core is covered")
	}
}

func TestClassifyProofRung(t *testing.T) {
	r := r4()
	live := &ProbeRecord{Live: true, Reached: []string{"corpus", "witness"}}
	shim := mutants.Mutant{Impl: "go", ID: "x", Edits: []mutants.Edit{{File: "internal/httpshim/shim.go"}}}
	core := mutants.Mutant{Impl: "go", ID: "y", Edits: []mutants.Edit{{File: "internal/store/memstore.go"}}}

	cells := []Cell{{Rung: "R4", Outcome: outcomeUnclassified}}
	classify(shim, cells, live, []rung{r})
	if cells[0].Outcome != outcomeUnreached {
		t.Errorf("live shim survivor: got %s, want %s (%s)", cells[0].Outcome, outcomeUnreached, cells[0].Detail)
	}

	cells = []Cell{{Rung: "R4", Outcome: outcomeUnclassified}}
	classify(core, cells, live, []rung{r})
	if cells[0].Outcome != outcomeSurvived {
		t.Errorf("live core survivor: got %s, want %s (%s)", cells[0].Outcome, outcomeSurvived, cells[0].Detail)
	}

	cells = []Cell{{Rung: "R4", Outcome: outcomeUnclassified}}
	classify(core, cells, &ProbeRecord{Live: false}, []rung{r})
	if cells[0].Outcome != outcomeEquivalent {
		t.Errorf("equivalent mutant: got %s, want %s", cells[0].Outcome, outcomeEquivalent)
	}

	cells = []Cell{{Rung: "R4", Outcome: outcomeUnclassified}}
	classify(shim, cells, nil, []rung{r})
	if cells[0].Outcome != outcomeUnclassified {
		t.Errorf("unprobed survivor must stay unclassified, got %s", cells[0].Outcome)
	}
}

func ids(rs []rung) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func r5() rung {
	for _, r := range allRungs {
		if r.ID == "R5" {
			return r
		}
	}
	panic("no R5 rung")
}

// R5's reach is narrower than R4's, and the list in rungs.go is a copy of
// something the spec owns. Re-derive it here so a site added to a new file
// fails this test instead of silently making that file's mutants look
// unreached.
func TestR5FilesMatchSites(t *testing.T) {
	var sites struct {
		Clauses map[string]struct {
			Sites []struct {
				File string `json:"file"`
			} `json:"sites"`
		} `json:"clauses"`
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "spec", "refinement", "clause-sites.json"))
	if err != nil {
		t.Skipf("clause-sites.json not readable from here: %v", err)
	}
	if err := json.Unmarshal(b, &sites); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, cl := range sites.Clauses {
		for _, s := range cl.Sites {
			want[s.File] = true
		}
	}
	got := map[string]bool{}
	for _, f := range r5Files {
		got[f] = true
	}
	for f := range want {
		if !got[f] {
			t.Errorf("%s carries an R5 site but is missing from r5Files; mutants there would be scored unreached", f)
		}
	}
	for f := range got {
		if !want[f] {
			t.Errorf("r5Files lists %s, which carries no R5 site", f)
		}
	}
}

// R4 and R5 must not be the same row by construction. A mutant inside the
// verified core but in a file no refinement clause sits on is fair game for
// R4 and unreached by R5.
func TestR5ReachIsNarrowerThanR4(t *testing.T) {
	dom := mutants.Mutant{Edits: []mutants.Edit{{File: "internal/dom/dom.go"}}}
	if !gobraReads(dom) {
		t.Error("internal/dom is inside the verified core; R4 reads it")
	}
	if r5Reads(dom) {
		t.Error("internal/dom carries no R5 clause site; R5 does not reach it")
	}
	store := mutants.Mutant{Edits: []mutants.Edit{{File: "internal/store/memstore.go"}}}
	if !gobraReads(store) || !r5Reads(store) {
		t.Error("memstore carries both")
	}
}

func TestR5Verdict(t *testing.T) {
	r := r5()
	cases := []struct {
		name   string
		out    string
		exit   int
		killed bool
		err    string
	}{
		{"clean", "R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m35.5s]\n", 0, false, ""},
		{"failed elsewhere is not an R5 kill",
			"R5 PASSED: 1 failing obligation(s), none of them a refinement clause   [1m2s]\n", 0, false, ""},
		{"refinement clause failed",
			"R5 FAILED: 1 of 1 failing obligation(s) are refinement clauses   [1m2s]\n", 1, true, ""},
		{"undecided is no verdict",
			"R5 UNDECIDED: no refinement clause failed, but 1 failing clause(s) sit on member(s)\n", 1, false, "no R5 verdict"},
		{"pass but exit 1",
			"R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [1m2s]\n", 1, false, "contradicts itself"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			killed, err := r.verdict(&toolRun{Stdout: c.out, ExitCode: c.exit})
			if c.err == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if killed != c.killed {
					t.Fatalf("killed=%v, want %v", killed, c.killed)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Fatalf("want error containing %q, got %v", c.err, err)
			}
		})
	}
}

// Coverage on the Rust corner, decided the same way as on the Go corner: the
// files a mutant edits against the crates Verus is asked to verify.
// crates/server is the trusted transport shim, so a mutant confined to it is
// unreached rather than survived (the F022 argument, restated on this corner).
func TestVerusReads(t *testing.T) {
	shim := mutants.Mutant{Edits: []mutants.Edit{{File: "crates/server/src/handlers.rs"}}}
	core := mutants.Mutant{Edits: []mutants.Edit{{File: "crates/store/src/lib.rs"}}}
	both := mutants.Mutant{Edits: []mutants.Edit{{File: "crates/server/src/handlers.rs"}, {File: "crates/service/src/lib.rs"}}}
	if verusReads(shim) {
		t.Error("crates/server is trusted transport; Verus is not asked to verify it")
	}
	if !verusReads(core) || !verusReads(both) {
		t.Error("an edit inside a verify-enabled crate is covered")
	}
}

// verusVerified is a hardcoded copy of the Rust corner's verification matrix,
// and the matrix itself lives in the crate manifests. Re-derive it here so the
// copy cannot go stale silently -- the same guard TestR5FilesMatchSites gives
// the R5 file list. If a crate gains or loses
// "[package.metadata.verus] verify = true", this fails and the R4 coverage
// denominator is corrected rather than quietly wrong.
func TestVerusCratesMatchTheTree(t *testing.T) {
	root := filepath.Join("..", "..", "..", "impls", "rust", "crates")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("impls/rust not readable from here: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "Cargo.toml"))
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.Index(s, "[package.metadata.verus]")
		if i < 0 {
			continue
		}
		section := s[i:]
		if j := strings.Index(section[1:], "\n["); j >= 0 {
			section = section[:j+1]
		}
		if strings.Contains(section, "verify = true") {
			got["crates/"+e.Name()+"/"] = true
		}
	}
	want := map[string]bool{}
	for _, c := range verusVerified {
		want[c] = true
	}
	for c := range want {
		if !got[c] {
			t.Errorf("%s is listed in verusVerified but is no longer verify-enabled", c)
		}
	}
	for c := range got {
		if !want[c] {
			t.Errorf("%s is verify-enabled in the tree but missing from verusVerified; R4's coverage denominator is too small", c)
		}
	}
}

// The table has one column per rung, not one per verifier.
//
// This started as a bug: a first design gave R4 one entry per verifier, and
// the report printed the R4 column twice and doubled every aggregate over it
// -- observed on a Rust R4 gate run that reported "R4 kill" twice for a sweep
// measuring two cells. The merged design makes that unrepresentable: one entry
// per rung ID, dispatching to a per-corner driver. This test pins the property
// so a future per-verifier entry cannot quietly reintroduce the doubling.
func TestOneEntryPerRungID(t *testing.T) {
	sel, err := selectRungs([]string{"R0", "R4", "R5"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, r := range sel {
		seen[r.ID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("rung %s has %d entries; the report keys cells by ID, so >1 doubles its aggregates", id, n)
		}
	}
	if got := ids(sel); len(got) != 3 {
		t.Fatalf("selected %v; want exactly R0, R4, R5", got)
	}
}

// R4 dispatches to a different verifier per corner, and every corner it claims
// must actually have one -- an Impls entry with no driver would run Gobra
// against a tree Gobra cannot read.
func TestEveryR4CornerHasADriver(t *testing.T) {
	sel, err := selectRungs([]string{"R4"})
	if err != nil {
		t.Fatal(err)
	}
	r := sel[0]
	for _, impl := range r.Impls {
		if tool := r.toolFor(impl); tool == "" {
			t.Errorf("R4 claims corner %s but resolves no tool for it", impl)
		}
	}
	for _, want := range []struct{ impl, tool string }{
		{"go", "gobra"}, {"rust", "verus"}, {"kotlin", "jbmc"}, {"java", "jbmc"},
	} {
		if got := r.toolFor(want.impl); got != want.tool {
			t.Errorf("R4 on %s resolves tool %q; want %q", want.impl, got, want.tool)
		}
	}
}

// TestEveryProofRungCornerHasAVacuityInstrument is the gate F030 found
// missing.
//
// GOAL.md: "A rung's kill verdict on a proof-backed row counts only if the
// obligation that noticed the mutant was itself shown non-vacuous. `gobra
// canary` / `gobra reach` is the instrument; the equivalents for Verus and
// JBMC must exist before their rows are trusted."
//
// Nothing enforced that. The Rust corner shipped an R4 row whose driver had no
// vacuity instrument of any kind -- an INJECTION canary was described in the
// loop log with the words the rule uses, and no gate could tell the difference
// (F013 is the finding about that substitution; F030 is it happening a second
// time). This test is the cheap check that would have caught it, and it is
// what a fourth corner's driver will have to pass.
//
// It is deliberately a SOURCE-level check. A vacuity instrument is not
// something a rung entry can declare -- declaring it is exactly what the Rust
// corner effectively did -- so the test goes and looks for one in the tool the
// rung actually invokes.
func TestEveryProofRungCornerHasAVacuityInstrument(t *testing.T) {
	root := repoRootForRungs(t)

	// The tools each proof-backed rung invokes, per corner.
	tools := map[string]bool{}
	for _, r := range allRungs {
		if r.Inputs != "contract" {
			continue
		}
		for _, impl := range r.Impls {
			tools[r.toolFor(impl)] = true
		}
	}
	if len(tools) == 0 {
		t.Fatal("no contract-backed rung found; this test is looking at the wrong field")
	}

	for tool := range tools {
		dir := filepath.Join(root, "tools", "cmd", tool)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("rung invokes %q but %s is not readable: %v", tool, dir, err)
		}
		found := false
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			// The instrument's signature: it has a NAME for the verdict it
			// exists to produce -- the string literal, not the word. The
			// first version of this test looked for the bare word and passed
			// on a driver that only mentioned vacuity in a doc comment, which
			// is the same "declaring it counts as having it" mistake the test
			// is here to catch.
			if strings.Contains(string(b), `"VACUOUS"`) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tools/cmd/%s drives a proof-backed rung but has no vacuity instrument: "+
				"no source file declares a \"VACUOUS\" verdict. An injection canary is not this -- it asks whether the gate "+
				"notices broken code, which a vacuous obligation passes too (F013, F030). "+
				"The corner's R4 row is not trusted until a negation canary exists here.", tool)
		}
	}
}

func repoRootForRungs(t *testing.T) string {
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

// TestNoConflictMarkersInTrackedFiles is a gate against an integration mistake
// this repository has now actually made.
//
// Merging the Java branch, three files conflicted. Two were checked for markers
// by name, resolved, and `git add -A` staged the third with its markers intact.
// `go build`, `go vet` and `go test ./tools/...` all passed, because the file
// was ASSURANCE.md and nothing compiles markdown. The unresolved conflict sat
// in the ceiling table -- the document this project treats as the source no
// cell may contradict -- and was found only because a later merge conflicted in
// the same region.
//
// The lesson is not "grep more carefully next time". It is that the evidence
// files carry the claims and nothing was checking them, so this walks the whole
// tree rather than a list of files someone has to remember to extend.
func TestNoConflictMarkersInTrackedFiles(t *testing.T) {
	root := repoRootForRungs(t)
	// Built by concatenation so this file does not match itself.
	markers := []string{"<<<<" + "<<< ", ">>>>" + ">>> ", "====" + "===\n"}

	skipDir := map[string]bool{".git": true, ".claude": true, "target": true, "node_modules": true}
	var bad []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // an unreadable path is not this test's business
		}
		if fi.IsDir() {
			if skipDir[fi.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".md", ".go", ".json", ".rs", ".kt", ".java", ".tla", ".toml", ".yaml", ".yml", ".sh":
		default:
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		for _, ln := range strings.Split(string(b), "\n") {
			for _, m := range markers {
				if strings.HasPrefix(ln+"\n", m) {
					bad = append(bad, rel+": "+ln)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		t.Errorf("unresolved conflict marker: %s", b)
	}
}
