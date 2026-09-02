package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

func r5Rung(t *testing.T) rung {
	t.Helper()
	sel, err := selectRungs([]string{"R5"})
	if err != nil {
		t.Fatal(err)
	}
	return sel[0]
}

// R5 is one rung with two drivers, not two rungs. The report keys cells by
// rung ID, so a second entry would double R5's aggregates -- TestOneEntryPerRungID
// pins that globally; this pins that the Kotlin corner arrived as a driver.
func TestR5DispatchesPerCorner(t *testing.T) {
	r := r5Rung(t)
	for _, want := range []struct{ impl, tool string }{{"go", "gobra"}, {"kotlin", "jbmc"}} {
		if got := r.toolFor(want.impl); got != want.tool {
			t.Errorf("R5 on %s resolves tool %q; want %q", want.impl, got, want.tool)
		}
	}
	if !r.applies("kotlin") {
		t.Error("R5 does not apply to the kotlin corner, so its cell would still be capped")
	}
	if r.applies("rust") {
		t.Error("R5 claims the rust corner; abs_rust has no body (B4/B5) and the cell is capped")
	}
	if r.applies("java") {
		t.Error("R5 claims the java corner, which has no refinement obligation file")
	}
}

// The argv is the record of which tree was verified. It has to go through the
// registry, so the tree JBMC reads is the tree calibrate's guard hashed.
func TestR5JBMCArgsGoThroughTheRegistry(t *testing.T) {
	r := r5Rung(t)
	args := r.argsFor("kotlin")(Config{}, "kotlin@limit-off-by-one", "/tmp/reg.json")
	joined := strings.Join(args, " ")
	if args[0] != "r5verify" {
		t.Errorf("R5 on kotlin invokes %q; want the r5verify subcommand", args[0])
	}
	for _, want := range []string{"-impl=kotlin@limit-off-by-one", "-registry=/tmp/reg.json", "-budget="} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q is missing %q", joined, want)
		}
	}
	// Gobra's argv must not have picked up the subcommand change.
	if got := r.argsFor("go")(Config{}, "go", "/tmp/reg.json")[0]; got != "r5verify" {
		t.Errorf("R5 on go invokes %q; want r5verify", got)
	}
}

// R4 and R5 must not be the same row on this corner either. R4 reaches dom,
// store and service; R5 reaches store alone, so a service mutant is fair game
// for R4 and UNREACHED by R5 -- not survived (F022).
func TestKotlinR5ReachIsNarrowerThanR4(t *testing.T) {
	svc := mutants.Mutant{Edits: []mutants.Edit{{File: "src/twitterport/service/Service.kt"}}}
	if !jbmcReads(svc) {
		t.Error("service is inside the Kotlin obligation set's reach; R4 reads it")
	}
	if r5KotlinReads(svc) {
		t.Error("no Kotlin refinement clause sits on the service layer; R5 does not reach it")
	}
	store := mutants.Mutant{Edits: []mutants.Edit{{File: "src/twitterport/store/Store.kt"}}}
	if !jbmcReads(store) || !r5KotlinReads(store) {
		t.Error("Store.kt carries both")
	}
	shim := mutants.Mutant{Edits: []mutants.Edit{{File: "src/twitterport/httpshim/Json.kt"}}}
	if jbmcReads(shim) || r5KotlinReads(shim) {
		t.Error("the trusted shim is reached by a proof rung; a mutant there is unreached, never survived (F004, F022)")
	}
}

// r5KotlinFiles is a copy of something tools/cmd/jbmc owns, in another main
// package that cannot import it. Re-derive it from that source so a clause
// added over a new production file fails here instead of silently making that
// file's mutants look unreached -- the same staleness TestR5FilesMatchSites
// catches for the Go corner.
func TestR5KotlinFilesMatchTheObligationSet(t *testing.T) {
	root := repoRootForRungs(t)
	b, err := os.ReadFile(filepath.Join(root, "tools", "cmd", "jbmc", "r5.go"))
	if err != nil {
		t.Skipf("tools/cmd/jbmc/r5.go not readable from here: %v", err)
	}
	src := string(b)
	i := strings.Index(src, "CoveredPaths: []string{")
	if i < 0 {
		t.Fatal("tools/cmd/jbmc/r5.go declares no CoveredPaths for the R5 corner")
	}
	rest := src[i+len("CoveredPaths: []string{"):]
	j := strings.Index(rest, "}")
	if j < 0 {
		t.Fatal("unterminated CoveredPaths literal")
	}
	want := map[string]bool{}
	for _, f := range strings.Split(rest[:j], ",") {
		f = strings.TrimSpace(strings.Trim(strings.TrimSpace(f), `"`))
		if f != "" {
			want[f] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("parsed no paths out of the R5 corner's CoveredPaths")
	}
	got := map[string]bool{}
	for _, f := range r5KotlinFiles {
		got[f] = true
	}
	for f := range want {
		if !got[f] {
			t.Errorf("%s is reached by an R5 clause obligation but is missing from r5KotlinFiles; mutants there would be scored unreached", f)
		}
	}
	for f := range got {
		if !want[f] {
			t.Errorf("r5KotlinFiles lists %s, which no R5 clause obligation reaches", f)
		}
	}
}

// calibrate reads the rung's verdict from the tool's own sentence and requires
// the exit code to agree. The Kotlin driver must speak the same two sentences
// the Go driver does, or the R5 column would mean two things.
func TestCalibrateReadsTheJBMCR5VerdictSentences(t *testing.T) {
	r := r5Rung(t)
	for _, c := range []struct {
		name   string
		out    string
		exit   int
		killed bool
		errIs  string
	}{
		{"kotlin pass", "R5 PASSED: JBMC verified 5 of 5 decidable clause obligation(s) covering R5 clause(s) 1, 2, 11, 13, 36, every one refutable in this tree; 2 clause obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [1m11.3s]\n", 0, false, ""},
		{"kotlin fail", "R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member); 2 clause obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator   [45s]\n", 1, true, ""},
		{"kotlin undecided", "R5 UNDECIDED: 1 failing goal(s) are not an own assertion of any R5 entry point, so it is not known whether a refinement obligation is among them; nothing was decided about this tree   [30s]\n", 1, false, "no R5 verdict"},
		{"pass but non-zero exit", "R5 PASSED: JBMC verified 5 of 5 decidable clause obligation(s)   [1s]\n", 1, false, "contradicts itself"},
		{"fail but zero exit", "R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [1s]\n", 0, false, "contradicts itself"},
	} {
		t.Run(c.name, func(t *testing.T) {
			killed, err := r.verdict(&toolRun{Argv: []string{"jbmc", "r5verify"}, Stdout: c.out, ExitCode: c.exit})
			if c.errIs != "" {
				if err == nil || !strings.Contains(err.Error(), c.errIs) {
					t.Fatalf("err = %v; want one containing %q", err, c.errIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if killed != c.killed {
				t.Errorf("killed = %v, want %v", killed, c.killed)
			}
		})
	}
}
