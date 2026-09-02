package main

import (
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

func TestR4AppliesToGoOnly(t *testing.T) {
	sel, err := selectRungs([]string{"R0", "R4"})
	if err != nil {
		t.Fatal(err)
	}
	run, capped := splitRungs("rust", sel)
	if len(run) != 1 || run[0].ID != "R0" || len(capped) != 1 || capped[0].ID != "R4" {
		t.Fatalf("rust: runnable=%v capped=%v", ids(run), ids(capped))
	}
	run, capped = splitRungs("go", sel)
	if len(run) != 2 || len(capped) != 0 {
		t.Fatalf("go: runnable=%v capped=%v", ids(run), ids(capped))
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
