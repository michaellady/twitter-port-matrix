package main

import (
	"regexp"
	"strings"
	"testing"
)

// EVERY RATE CARRIES ITS DENOMINATOR.
//
// F008 is that volume is not coverage and F009 that a rung cannot kill what it
// cannot reach; F022 measured the consequence -- 4 of 18 Go mutants edit a file
// no obligation covers, so R4's ceiling on that corner is 14 of 18 before a
// clause is written. A bare percentage cannot express any of that. "100%" is
// the same six characters whether it stands over 14 reached cells or over 18,
// and the two are different claims about the rung.
//
// These tests pin the fraction into the report and into results.json.

// TestRateCarriesItsDenominator covers the arithmetic, including the case a
// bare percentage gets wrong: nothing reached is not a 0% kill rate.
func TestRateCarriesItsDenominator(t *testing.T) {
	cases := []struct {
		num, den int
		want     string
	}{
		{14, 14, "14/14 = 100%"},
		{14, 18, "14/18 = 78%"},
		{0, 18, "0/18 = 0%"},
		{0, 0, "0/0 = n/a"},
		{7, 17, "7/17 = 41%"},
	}
	for _, c := range cases {
		if got := rate(c.num, c.den); got != c.want {
			t.Fatalf("rate(%d, %d) = %q, want %q", c.num, c.den, got, c.want)
		}
	}
}

// proofRun is one rung's worth of the shape F022 describes: 18 live mutants on
// the Go corner, 4 of them in code the verifier never reads.
func proofRun(t *testing.T) (*Run, []rung) {
	t.Helper()
	rungs, err := selectRungs([]string{"R4"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	run := &Run{Config: Config{Impls: []string{"go"}, Rungs: []string{"R4"}}}
	for i := 0; i < 18; i++ {
		outcome := outcomeKilled
		if i >= 14 {
			outcome = outcomeUnreached
		}
		run.Cells = append(run.Cells, Cell{
			Mutant: "go/m" + string(rune('a'+i)), Impl: "go", Rung: "R4", Outcome: outcome,
		})
	}
	run.Summary = summarize(run, rungs)
	run.Warnings = warnings(run, rungs)
	return run, rungs
}

// TestReportPrintsRatesWithDenominators is the F022 row rendered. The report
// must say 14/14 and 14/18, not 100% and 78%.
//
// Against the pre-change renderer this fails: the kill table printed
// "  100%    78%" with nothing anywhere in the document saying what 100% was a
// percentage of.
func TestReportPrintsRatesWithDenominators(t *testing.T) {
	run, rungs := proofRun(t)
	out := renderReport(run, rungs)

	for _, want := range []string{"14/14 = 100%", "14/18 = 78%"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the report does not contain %q; a rate without its denominator is not readable:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "DENOMINATORS") {
		t.Fatalf("the report has no DENOMINATORS section:\n%s", out)
	}
	// The exclusion has to be counted AND explained, per rung.
	if !strings.Contains(out, "18 cell(s) measured, 14 in the killed/reached denominator, 4 excluded") {
		t.Fatalf("the report does not state how many cells were excluded from R4's denominator:\n%s", out)
	}
	if !strings.Contains(out, "F022") {
		t.Fatalf("R4's excluded cells are not explained (no reason given for the unreached ones):\n%s", out)
	}
}

// TestSummaryCarriesTheDenominator checks the same fact in the machine-readable
// half. A consumer reading results.json must not have to re-derive `reached`
// from the outcome counts, because the whole failure mode is a rate travelling
// without its divisor.
func TestSummaryCarriesTheDenominator(t *testing.T) {
	run, _ := proofRun(t)
	s := run.Summary[0]
	if s.Cells != 18 || s.Reached != 14 || s.Killed != 14 {
		t.Fatalf("cells/reached/killed = %d/%d/%d, want 18/14/14", s.Cells, s.Reached, s.Killed)
	}
	if s.Reached != s.Live-s.Unreached {
		t.Fatalf("reached (%d) must equal live - unreached (%d - %d); equivalent mutants are already outside live",
			s.Reached, s.Live, s.Unreached)
	}
	if len(s.Excluded) != 1 || s.Excluded[0].Outcome != outcomeUnreached || s.Excluded[0].Count != 4 {
		t.Fatalf("excluded = %+v, want one unreached entry of 4", s.Excluded)
	}
	if s.Excluded[0].Reason == "" {
		t.Fatalf("an exclusion with no reason is a blank cell with extra steps")
	}
}

// TestZeroReachedIsNotZeroPercent is the case a bare percentage reports
// backwards. A rung that reached nothing has no kill rate; the pre-change
// renderer printed 0% for it, which is the same string it prints for a rung
// that saw every mutant and killed none.
func TestZeroReachedIsNotZeroPercent(t *testing.T) {
	rungs, err := selectRungs([]string{"R4"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	run := &Run{
		Config: Config{Impls: []string{"go"}, Rungs: []string{"R4"}},
		Cells: []Cell{
			{Mutant: "go/shim-a", Impl: "go", Rung: "R4", Outcome: outcomeUnreached},
			{Mutant: "go/shim-b", Impl: "go", Rung: "R4", Outcome: outcomeUnreached},
		},
	}
	run.Summary = summarize(run, rungs)
	run.Warnings = warnings(run, rungs)
	out := renderReport(run, rungs)

	if !strings.Contains(out, "0/0 = n/a") {
		t.Fatalf("a rung with nothing reached must report n/a, not a rate:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "R4 ") {
			continue
		}
		// killed/live is a real 0 of 2 here and stays; what must not appear is
		// a percentage standing on its own. Strip every well-formed rate and
		// nothing with a % in it may be left.
		if rest := reRate.ReplaceAllString(line, ""); strings.Contains(rest, "%") {
			t.Fatalf("the kill table carries a percentage with no denominator beside it: %q", line)
		}
	}
}

// reRate matches a rate that carries its own arithmetic: "14/18 = 78%" or the
// zero-denominator form "0/0 = n/a".
var reRate = regexp.MustCompile(`\d+/\d+ = (?:\d+%|n/a)`)

// TestExclusionsAccountForEveryCell is the arithmetic invariant behind the
// section: nothing may quietly leave a denominator. If a new outcome word is
// added, it either lands in `reached` or it is named here.
func TestExclusionsAccountForEveryCell(t *testing.T) {
	rungs, err := selectRungs([]string{"R4"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	run := &Run{
		Config: Config{Impls: []string{"go", "rust"}, Rungs: []string{"R4"}},
		Cells: []Cell{
			{Mutant: "go/a", Impl: "go", Rung: "R4", Outcome: outcomeKilled},
			{Mutant: "go/b", Impl: "go", Rung: "R4", Outcome: outcomeSurvived},
			{Mutant: "go/c", Impl: "go", Rung: "R4", Outcome: outcomeUnreached},
			{Mutant: "go/d", Impl: "go", Rung: "R4", Outcome: outcomeEquivalent},
			{Mutant: "go/e", Impl: "go", Rung: "R4", Outcome: outcomeUnclassified},
			{Mutant: "go/f", Impl: "go", Rung: "R4", Outcome: outcomeError},
			{Mutant: "rust/a", Impl: "rust", Rung: "R4", Outcome: outcomeCapped},
		},
	}
	run.Summary = summarize(run, rungs)

	for _, s := range run.Summary {
		sum := 0
		for _, e := range s.Excluded {
			sum += e.Count
			if e.Reason == "" {
				t.Fatalf("%s/%s: %s is excluded with no reason", s.Rung, s.Impl, e.Outcome)
			}
		}
		if s.Cells != s.Reached+sum {
			t.Fatalf("%s/%s: %d cells, %d reached, %d explained away -- %d cells left a denominator "+
				"with nothing saying why", s.Rung, s.Impl, s.Cells, s.Reached, sum, s.Cells-s.Reached-sum)
		}
	}
}

// TestByCornerRatesCarryTheirDenominator: the per-corner rows are what the
// twelve-cell matrix is built from, and a capped corner must not read as a
// corner that scored zero.
func TestByCornerRatesCarryTheirDenominator(t *testing.T) {
	rungs, err := selectRungs([]string{"R4"})
	if err != nil {
		t.Fatalf("selectRungs: %v", err)
	}
	run := &Run{
		Config: Config{Impls: []string{"go", "rust"}, Rungs: []string{"R4"}},
		Cells: []Cell{
			{Mutant: "go/a", Impl: "go", Rung: "R4", Outcome: outcomeKilled},
			{Mutant: "go/b", Impl: "go", Rung: "R4", Outcome: outcomeUnreached},
			{Mutant: "rust/a", Impl: "rust", Rung: "R4", Outcome: outcomeCapped},
			{Mutant: "rust/b", Impl: "rust", Rung: "R4", Outcome: outcomeCapped},
		},
	}
	run.Summary = summarize(run, rungs)
	run.Warnings = warnings(run, rungs)
	out := renderReport(run, rungs)

	if !strings.Contains(out, "BY CORNER") {
		t.Fatalf("no BY CORNER section:\n%s", out)
	}
	var goRow, rustRow string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "go       R4") {
			goRow = line
		}
		if strings.HasPrefix(line, "rust     R4") {
			rustRow = line
		}
	}
	if !strings.Contains(goRow, "1/1 = 100%") {
		t.Fatalf("go's R4 row does not carry its denominator: %q", goRow)
	}
	if !strings.Contains(rustRow, "0/0 = n/a") {
		t.Fatalf("a corner whose cells are all capped must read n/a, not a kill rate: %q", rustRow)
	}
}
