package main

import (
	"path/filepath"
	"testing"
)

// PER-JUDGE ATTRIBUTION.
//
// A mutant that several rungs would notice must be credited to every one of
// them, not only to whichever ran first. The opposite -- first-judge
// attribution -- is the cheap implementation (stop the loop once something
// kills) and it produces a table that is wrong in a specific, directional way:
// every rung after the first loses exactly the kills the earlier rungs already
// took, so R1, R2, R4 and R5 all read weaker than they are, by an amount set by
// the order in `-rungs`. On this catalogue R0 kills 72 of 72, so a first-judge
// table would report every other rung as killing nothing at all.
//
// These two tests are the guard. The first is structural -- it fails if a
// short circuit is ever added to the loop -- and the second is arithmetic, over
// the aggregation that turns cells into rows.

func testJournal(t *testing.T) *journal {
	t.Helper()
	jr, err := openJournal(filepath.Join(t.TempDir(), "journal.jsonl"), false)
	if err != nil {
		t.Fatalf("opening journal: %v", err)
	}
	t.Cleanup(jr.close)
	return jr
}

// TestEveryRungRunsAfterAKill drives runRungs with a fake invocation: R0 kills,
// R1 survives, R2 kills. All three must be asked, and all three must come back
// with their own answer.
//
// Against a first-judge loop (`if c.Outcome == outcomeKilled { break }`) this
// fails on the first assertion: only R0 is invoked and only one cell returns.
func TestEveryRungRunsAfterAKill(t *testing.T) {
	rungs, err := selectRungs([]string{"R0", "R1", "R2"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	kills := map[string]bool{"R0": true, "R2": true}

	var asked []string
	cells := runRungs(rungs, "go/limit-off-by-one", "deadbeef", testJournal(t), func(r rung) Cell {
		asked = append(asked, r.ID)
		outcome := outcomeSurvived
		if kills[r.ID] {
			outcome = outcomeKilled
		}
		return Cell{Mutant: "go/limit-off-by-one", Impl: "go", Rung: r.ID, Outcome: outcome}
	})

	if len(asked) != 3 {
		t.Fatalf("the sweep asked %v; every selected rung must be asked even after one kills -- "+
			"a loop that stops at the first killing rung credits R0 and reports every later rung as blind", asked)
	}
	if len(cells) != 3 {
		t.Fatalf("got %d cells, want 3 (one per rung)", len(cells))
	}
	for i, want := range []string{"R0", "R1", "R2"} {
		if cells[i].Rung != want {
			t.Fatalf("cell %d is %s, want %s", i, cells[i].Rung, want)
		}
	}
	if cells[0].Outcome != outcomeKilled || cells[2].Outcome != outcomeKilled {
		t.Fatalf("R0=%s R2=%s: both killing rungs must record their own kill", cells[0].Outcome, cells[2].Outcome)
	}
	if cells[1].Outcome != outcomeSurvived {
		t.Fatalf("R1=%s: a rung between two kills must still record its own answer", cells[1].Outcome)
	}
}

// TestResumedCellDoesNotSkipTheOtherRungs is the same property on the resume
// path. A journalled R0 kill must not stand in for the rungs that have not run.
func TestResumedCellDoesNotSkipTheOtherRungs(t *testing.T) {
	rungs, err := selectRungs([]string{"R0", "R1", "R2"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	jr := testJournal(t)
	jr.cells[cellKey("go/limit-off-by-one", "deadbeef", "R0")] = Cell{
		Mutant: "go/limit-off-by-one", Impl: "go", TreeHash: "deadbeef", Rung: "R0", Outcome: outcomeKilled,
	}

	var asked []string
	cells := runRungs(rungs, "go/limit-off-by-one", "deadbeef", jr, func(r rung) Cell {
		asked = append(asked, r.ID)
		return Cell{Mutant: "go/limit-off-by-one", Impl: "go", Rung: r.ID, Outcome: outcomeKilled}
	})
	if len(asked) != 2 || asked[0] != "R1" || asked[1] != "R2" {
		t.Fatalf("asked %v, want [R1 R2]: the journalled R0 cell is reused and the rest still run", asked)
	}
	if len(cells) != 3 {
		t.Fatalf("got %d cells, want 3", len(cells))
	}
}

// TestKillAtTwoRungsCreditsBothRows is the table-level statement of the same
// property: one mutant killed at R0 and at R2 must be counted in BOTH rows'
// kill counts, and the row for the rung that missed it must show a survivor.
//
// Against a first-judge sweep this fails on R2: the mutant would have no R2
// cell at all, so R2's row would read 0 killed out of 1 reached rather than
// 1 of 2.
func TestKillAtTwoRungsCreditsBothRows(t *testing.T) {
	rungs, err := selectRungs([]string{"R0", "R1", "R2"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	// The cells come out of the real sweep loop rather than being written by
	// hand, so this test measures the table the sweep would actually produce.
	outcomes := map[string]map[string]string{
		// Killed by R0 and R2; R1's inputs never elicit it.
		"go/both": {"R0": outcomeKilled, "R1": outcomeUnreached, "R2": outcomeKilled},
		// Killed by R0 only; the other two saw it and passed.
		"go/r0only": {"R0": outcomeKilled, "R1": outcomeSurvived, "R2": outcomeSurvived},
	}
	run := &Run{Config: Config{Impls: []string{"go"}, Rungs: []string{"R0", "R1", "R2"}}}
	jr := testJournal(t)
	for _, key := range []string{"go/both", "go/r0only"} {
		run.Cells = append(run.Cells, runRungs(rungs, key, "deadbeef", jr, func(r rung) Cell {
			return Cell{Mutant: key, Impl: "go", TreeHash: "deadbeef", Rung: r.ID, Outcome: outcomes[key][r.ID]}
		})...)
	}
	run.Summary = summarize(run, rungs)

	want := map[string]struct{ killed, reached int }{
		"R0": {2, 2},
		"R1": {0, 1},
		"R2": {1, 2},
	}
	for _, s := range run.Summary {
		if s.Impl != "" {
			continue
		}
		w, ok := want[s.Rung]
		if !ok {
			t.Fatalf("unexpected row %s", s.Rung)
		}
		if s.Killed != w.killed || s.Reached != w.reached {
			t.Fatalf("%s: killed/reached = %d/%d, want %d/%d. A mutant several rungs kill is credited to "+
				"every one of them; only the rung that actually missed it loses the kill",
				s.Rung, s.Killed, s.Reached, w.killed, w.reached)
		}
	}
}
