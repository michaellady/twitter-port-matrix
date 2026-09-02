package main

import (
	"strings"
	"testing"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

func r4Rung(t *testing.T) rung {
	t.Helper()
	rs, err := selectRungs([]string{"R4"})
	if err != nil || len(rs) != 1 {
		t.Fatalf("selectRungs(R4): %v", err)
	}
	return rs[0]
}

// R4 is ONE rung driven by two verifiers, not two rungs. The corner decides
// which binary runs; the column heading and the verdict sentence do not change.
func TestR4DispatchesPerCorner(t *testing.T) {
	r := r4Rung(t)
	for _, impl := range []string{"go", "kotlin"} {
		if !r.applies(impl) {
			t.Fatalf("R4 does not apply to %s", impl)
		}
	}
	if r.applies("rust") {
		t.Skip("another lane has added the Verus driver; this test only pins the two corners it knows")
	}
	if got := r.toolFor("go"); got != "gobra" {
		t.Errorf("tool for go: got %q, want gobra", got)
	}
	if got := r.toolFor("kotlin"); got != "jbmc" {
		t.Errorf("tool for kotlin: got %q, want jbmc", got)
	}
	// A corner with no driver falls back to the rung's own fields, so the
	// rungs that have only ever had one verifier are untouched.
	r0 := allRungs[0]
	if got := r0.toolFor("kotlin"); got != r0.Tool {
		t.Errorf("R0 tool for kotlin: got %q, want the rung's own %q", got, r0.Tool)
	}
}

// The rung must resolve the SAME tree calibrate's guard hashed, which means
// going through the registry by entry name rather than by directory.
func TestJBMCArgsGoThroughTheRegistry(t *testing.T) {
	r := r4Rung(t)
	cfg := Config{RungTimeout: (20 * time.Minute).String()}
	args := r.argsFor("kotlin")(cfg, "kotlin@tick-goes-backwards", "/tmp/reg.json")
	joined := strings.Join(args, " ")
	for _, want := range []string{"verify", "-impl=kotlin@tick-goes-backwards", "-registry=/tmp/reg.json", "-budget="} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %v is missing %q", args, want)
		}
	}
	// The verifier's own budget has to land before calibrate kills the
	// subprocess, or an undecidable tree is reported as a dead process with no
	// output instead of in the tool's own words.
	var budget time.Duration
	for _, a := range args {
		if strings.HasPrefix(a, "-budget=") {
			d, err := time.ParseDuration(strings.TrimPrefix(a, "-budget="))
			if err != nil {
				t.Fatalf("budget %q: %v", a, err)
			}
			budget = d
		}
	}
	if budget >= cfg.rungTimeout() {
		t.Errorf("jbmc budget %s does not land before calibrate's %s timeout", budget, cfg.rungTimeout())
	}
}

// F022's accounting, one corner over: a mutant confined to the trusted
// transport shim is UNREACHED by the proof rung, never survived. Scoring those
// as survivors reads a rung weakness where there is only a scope boundary.
func TestJBMCReadsExcludesTheTrustedShim(t *testing.T) {
	cases := []struct {
		file string
		want bool
	}{
		{"src/twitterport/store/Store.kt", true},
		{"src/twitterport/service/Service.kt", true},
		{"src/twitterport/dom/Dom.kt", true},
		{"src/twitterport/httpshim/Json.kt", false},
		{"src/twitterport/httpshim/Server.kt", false},
	}
	for _, tc := range cases {
		m := mutants.Mutant{Impl: "kotlin", ID: "x", Edits: []mutants.Edit{{File: tc.file}}}
		if got := jbmcReads(m); got != tc.want {
			t.Errorf("jbmcReads(%s): got %v, want %v", tc.file, got, tc.want)
		}
	}
}

// The whole catalogue, so the ceiling this rung has before a single obligation
// is written is a number rather than an impression.
func TestKotlinCoverageCeilingIsMeasured(t *testing.T) {
	man, err := mutants.Load("../mutate/mutants.json")
	if err != nil {
		t.Skipf("catalogue not readable from here: %v", err)
	}
	total, covered := 0, 0
	for _, m := range man.Mutants {
		if m.Impl != "kotlin" {
			continue
		}
		total++
		if jbmcReads(m) {
			covered++
		}
	}
	if total == 0 {
		t.Fatal("no kotlin mutants in the catalogue")
	}
	if covered != total-2 {
		t.Errorf("kotlin mutants covered by JBMC: got %d of %d, want %d (the two httpshim mutants are unreached)", covered, total, total-2)
	}
}

// The wire between the two tools: calibrate reads the sentence jbmc prints.
// These are verbatim from a real run, so a change to either side that the
// other does not know about fails here rather than in a sweep.
func TestCalibrateReadsTheJBMCVerdictSentences(t *testing.T) {
	r := r4Rung(t)
	pass := "R4 PASSED: JBMC verified 7 of 7 decidable obligation(s) (0 of 11 own assertion goals FAILURE), every one refutable in this tree; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [1m52.5s]"
	fail := "R4 FAILED: JBMC refuted 2 of 7 decidable obligation(s) (2 of 11 own assertion goals FAILURE): o3b_createdAtNonDecreasing, o3c_lemmaOverThreeAppends; 8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [1m5.0s]"
	undecided := "R4 UNDECIDED: 1 of 7 decidable obligation(s) could not be read (o3a_idsStrictlyIncrease: JBMC exceeded its 6m0s budget); nothing was decided about this tree   [6m2.0s]"

	killed, err := r.verdict(&toolRun{Stdout: pass, ExitCode: 0})
	if err != nil || killed {
		t.Errorf("PASSED sentence: killed=%v err=%v", killed, err)
	}
	killed, err = r.verdict(&toolRun{Stdout: fail, ExitCode: 1})
	if err != nil || !killed {
		t.Errorf("FAILED sentence: killed=%v err=%v", killed, err)
	}
	// An undecided run carries no verdict, so calibrate records an error cell.
	// It must never be read as a survival: a proof the solver did not finish
	// is a missing measurement.
	if _, err := r.verdict(&toolRun{Stdout: undecided, ExitCode: 1}); err == nil {
		t.Error("UNDECIDED was read as an answer; it must be an error cell")
	}
	// And the two halves must agree, per standing rule 1.
	if _, err := r.verdict(&toolRun{Stdout: fail, ExitCode: 0}); err == nil {
		t.Error("a FAILED sentence with exit 0 must be an error, not a kill")
	}
	if _, err := r.verdict(&toolRun{Stdout: pass, ExitCode: 1}); err == nil {
		t.Error("a PASSED sentence with exit 1 must be an error, not a pass")
	}
}
