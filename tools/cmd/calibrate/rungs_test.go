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

// R4 now has two entries -- Gobra on go, Verus on rust -- so a rung ID is a
// SET of per-corner entries. Selecting R4 must select both, each corner must
// get exactly the one entry that drives its verifier, and a corner with no
// entry at all must still get exactly one capped cell rather than one per
// entry (a cell counted twice is a denominator error, not a display bug).
func TestR4IsPerCorner(t *testing.T) {
	sel, err := selectRungs([]string{"R0", "R4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sel) != 3 {
		t.Fatalf("selecting R0,R4 gave %v; want R0 plus both R4 entries", ids(sel))
	}
	for corner, tool := range map[string]string{"go": "gobra", "rust": "verus"} {
		run, capped := splitRungs(corner, sel)
		if len(run) != 2 || len(capped) != 0 {
			t.Fatalf("%s: runnable=%v capped=%v", corner, ids(run), ids(capped))
		}
		if run[1].ID != "R4" || run[1].Tool != tool {
			t.Errorf("%s R4 is driven by %q, want %q", corner, run[1].Tool, tool)
		}
	}
	run, capped := splitRungs("kotlin", sel)
	if len(run) != 1 || run[0].ID != "R0" {
		t.Fatalf("kotlin: runnable=%v", ids(run))
	}
	if len(capped) != 1 || capped[0].ID != "R4" {
		t.Fatalf("kotlin: capped=%v; one capped cell per rung ID, not per entry", ids(capped))
	}
	// The capped cell names every corner that does have the rung, so the
	// report says what the cap is relative to.
	if got := strings.Join(capped[0].Impls, ","); got != "go,rust" {
		t.Errorf("capped R4 names impls %q; want the union go,rust", got)
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

// The table has one column per rung, not one per verifier. R4 has two entries
// now, and without collapsing them the report prints the column twice and
// every aggregate over it is doubled -- observed on the first Rust R4 gate
// run, which reported "R4 kill" twice for a sweep that measured two cells.
func TestReportRungsCollapsesPerCornerEntries(t *testing.T) {
	sel, err := selectRungs([]string{"R0", "R4"})
	if err != nil {
		t.Fatal(err)
	}
	got := ids(reportRungs(sel))
	if len(got) != 2 || got[0] != "R0" || got[1] != "R4" {
		t.Fatalf("report columns %v; want one per rung ID", got)
	}
}
