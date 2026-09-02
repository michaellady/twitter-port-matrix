package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Isolation must change exactly one thing: what the solver is asked to prove
// at the exit. If it also moved a line, Gobra's reported positions would stop
// matching the shipped file and every verdict would be attributed to the wrong
// clause; if it also removed an `invariant` or a `requires`, the exit state
// itself would change and the vacuity question would no longer be the same
// question. Both are silent failures that look like a faster sweep.
func TestElideKeepsEverythingButTheEnsures(t *testing.T) {
	const src = `package store

// @ requires acc(s.LockP())
// @ ensures acc(s.LockP())
// @ ensures len(out) <= limit
// @ ensures forall a int :: 0 <= a && a < len(out) ==>
// @            out[a].ID < cursor
// @ ensures s.AbsLogLen() == old(s.AbsLogLen())
func (s *MemStore) HomeTimeline() (out []int) {
	// @ invariant 0 <= n && n <= limit
	return nil
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "memstore.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// The two-line quantified clause and the framing-by-abs clause; NOT the
	// `acc(...)` one and NOT `len(out) <= limit`, which stands in for the
	// clause under audit.
	elided := []clause{
		{StartLine: 6, EndLine: 7},
		{StartLine: 8, EndLine: 8},
	}
	if err := elide(path, elided); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(strings.Split(string(b), "\n")), len(strings.Split(src, "\n")); got != want {
		t.Fatalf("line count changed: %d -> %d; Gobra positions would no longer line up", want, got)
	}
	for _, keep := range []string{
		"// @ requires acc(s.LockP())",
		"// @ ensures acc(s.LockP())",
		"// @ ensures len(out) <= limit",
		"// @ invariant 0 <= n && n <= limit",
	} {
		if !strings.Contains(string(b), keep) {
			t.Errorf("isolation removed %q, which it must not touch", keep)
		}
	}
	for _, gone := range []string{"forall a int", "AbsLogLen"} {
		if strings.Contains(string(b), gone) {
			t.Errorf("isolation left %q in place", gone)
		}
	}
}

// otherEnsuresOn picks the siblings to elide. Picking up a clause from another
// member, or a framing clause, would change a contract the run is not asking
// about.
func TestOtherEnsuresOnStaysOnTheMember(t *testing.T) {
	target := clause{File: "a.go", Member: "M", Kind: kindFunctional, StartLine: 10}
	all := []clause{
		target,
		{File: "a.go", Member: "M", Kind: kindFunctional, StartLine: 11},
		{File: "a.go", Member: "M", Kind: kindFraming, StartLine: 12},
		{File: "a.go", Member: "N", Kind: kindFunctional, StartLine: 30},
		{File: "b.go", Member: "M", Kind: kindFunctional, StartLine: 10},
	}
	got := otherEnsuresOn(all, target)
	if len(got) != 1 || got[0].StartLine != 11 {
		t.Fatalf("got %+v, want exactly the functional sibling at line 11", got)
	}
}

// A timeout has to be quotable in Gobra's words, and the `0 error(s)` line has
// to travel with it -- that pairing is the whole reason a timed-out run is not
// read as a pass.
func TestTerminationLinesQuoteGobraNotTheTool(t *testing.T) {
	raw := strings.Join([]string{
		"Verifying package /x - store",
		"The verification of package /x - store got terminated after 720 seconds",
		"The verification of member /x.store.HomeTimeline(string) did not terminate",
		"Gobra has found 0 error(s)",
		"The verification of 1 members timed out",
	}, "\n")
	got := terminationLines(raw)
	if len(got) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(got), strings.Join(got, "\n"))
	}
	if !strings.Contains(strings.Join(got, "\n"), "Gobra has found 0 error(s)") {
		t.Error("the `0 error(s)` line must travel with the termination lines")
	}
}

// A hand-written canary that has stopped binding is worse than none: the sweep
// falls back to the derived spelling, which on this member does not terminate,
// and the clause silently returns to UNAUDITED with nothing saying why. So
// every entry must match exactly one clause of the shipped contracts.
func TestHandCanariesAllBind(t *testing.T) {
	all, err := allClauses("../../../impls/go")
	if err != nil {
		t.Fatalf("allClauses (this is applyHandCanaries failing, and that is the point): %v", err)
	}
	got := 0
	for _, c := range all {
		if c.CanaryEquivalence == "" {
			continue
		}
		got++
		if c.Canary == "" || c.CanaryWhy == "" {
			t.Errorf("%s:%d hand canary is incomplete", c.File, c.StartLine)
		}
	}
	if got != len(handCanaries) {
		t.Fatalf("%d clauses carry a hand canary, want %d", got, len(handCanaries))
	}
}

// The shape a verdict was reached in has to travel with it, or a reader has a
// status and no way to tell which run produced it.
func TestCanaryModeNamesTheShape(t *testing.T) {
	plain := clause{}
	hand := clause{CanaryEquivalence: "len(out) <= 0 is len(out) == 0"}
	if got := canaryMode(plain, false); got != "" {
		t.Errorf("default shape should record no mode, got %q", got)
	}
	if got := canaryMode(plain, true); got != "isolated" {
		t.Errorf("got %q, want isolated", got)
	}
	if got := canaryMode(hand, true); got != "isolated hand-written canary" {
		t.Errorf("got %q, want both", got)
	}
}

// The stack trace Gobra prints for a malformed --packageTimeout contains the
// frame `packageTimeoutDuration`. Nothing in it is a termination line, and a
// looser match would score a crashed run as a timed-out one.
func TestTerminationLinesIgnoresTheStackTrace(t *testing.T) {
	raw := "java.lang.NumberFormatException\n\tat viper.gobra.frontend.Config.packageTimeoutDuration(Config.scala:1)"
	if got := terminationLines(raw); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
	if gobraTimedOut(raw) {
		t.Error("a stack trace is not a timeout")
	}
}
