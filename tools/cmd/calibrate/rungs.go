package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// A rung is one verification layer, described as data rather than as a branch.
//
// R4 (deductive proof) is an entry here, not a new code path through the
// sweep, so it arrives with the same accounting as the three empirical rungs
// and stays comparable with them. R5 (refinement) will be another entry. Both
// are per corner: a rung names the corners whose verifier it drives, and the
// other corners get a "capped" cell rather than an error or a blank.
type rung struct {
	ID    string // "R0"
	Label string // "corpus" -- the column heading
	Tool  string // binary under tools/cmd

	// Inputs names WHERE this rung's requests come from. It is not decoration:
	// it is what lets a survival be attributed to an input gap rather than to
	// the rung. R0 replays a fixed corpus; R1 and R2 draw from tracegen. A
	// mutant no corpus step distinguishes cannot be killed by R0 however good
	// R0's oracle is.
	Inputs string // "corpus" | "tracegen" | "contract"

	// Impls names the corners this rung exists for; nil means every corner.
	// A deductive rung is per verifier, and a corner without one gets a
	// "capped" cell rather than an error or a blank -- the cap is a result.
	Impls []string

	// Covers, when set, says whether the verifier READS any file the mutant
	// edits. It is the proof rung's reachability: a proof has no input
	// distribution to sample, so the analogue of "the corpus never elicits
	// it" is "the verifier never sees it" -- internal/httpshim is trusted
	// transport and no obligation is discharged over it. Without this a
	// survivor in the shim would be scored against the contract, which never
	// had a chance at it (F008's coverage denominator).
	Covers func(m mutants.Mutant) bool

	Args func(cfg Config, implName, regPath string) []string

	// Pass and Fail are the tool's OWN verdict sentences. The outcome is read
	// from these, never from the exit code alone -- GOAL.md standing rule 1.
	// The exit code is then required to agree, and a disagreement is an error
	// rather than a silent preference for one of them.
	Pass string
	Fail string

	// Launches reports how many server processes a CLEAN run of this rung
	// starts, which is what turns wall time into a comparable number. It
	// returns false when the count is not knowable from the configuration
	// alone; the count is only ever used for clean runs, because a rung that
	// stops at its first mismatch launches an unknown, smaller number.
	Launches func(cfg Config, stdout string) (int, bool)
}

var allRungs = []rung{
	{
		ID: "R0", Label: "corpus", Tool: "replay", Inputs: "corpus",
		Args: func(cfg Config, implName, regPath string) []string {
			return []string{"-impl=" + implName, "-registry=" + regPath, "-corpus=" + cfg.Corpus}
		},
		Pass: "R0 PASSED", Fail: "R0 FAILED",
		// replay launches exactly one server for the whole corpus.
		Launches: func(Config, string) (int, bool) { return 1, true },
	},
	{
		ID: "R1", Label: "diff-fuzz", Tool: "diffrun", Inputs: "tracegen",
		Args: func(cfg Config, implName, regPath string) []string {
			return []string{
				"-impls=" + implName, "-registry=" + regPath,
				"-traces=" + strconv.Itoa(cfg.R1Traces),
				"-steps=" + strconv.Itoa(cfg.R1Steps),
				"-seed=" + strconv.FormatInt(cfg.R1Seed, 10),
			}
		},
		Pass: "R1 PASSED", Fail: "R1 FAILED",
		// diffrun starts a fresh server per trace, so no state leaks between
		// traces and a failure is reproducible from its seed alone.
		Launches: func(cfg Config, _ string) (int, bool) { return cfg.R1Traces, true },
	},
	{
		ID: "R2", Label: "property", Tool: "proptest", Inputs: "tracegen",
		Args: func(cfg Config, implName, regPath string) []string {
			return []string{
				"-impls=" + implName, "-registry=" + regPath,
				"-rounds=" + strconv.Itoa(cfg.R2Rounds),
				"-setup=" + strconv.Itoa(cfg.R2Setup),
				"-seed=" + strconv.FormatInt(cfg.R2Seed, 10),
			}
		},
		Pass: "R2 PASSED", Fail: "R2 FAILED",
		// proptest starts a server per property per round. The property count
		// is not a flag, so it is read from the tool's own header rather than
		// hardcoded here -- a hardcoded 4 would go quietly stale the day a
		// fifth property is added, and every R2 cost number after that would
		// be wrong.
		Launches: func(cfg Config, stdout string) (int, bool) {
			m := reProperties.FindStringSubmatch(stdout)
			if m == nil {
				return 0, false
			}
			n, err := strconv.Atoi(m[1])
			if err != nil || n <= 0 {
				return 0, false
			}
			return n * cfg.R2Rounds, true
		},
	},
	{
		// R4 on the Go corner is Gobra over the verified core. The rung tool
		// is `gobra verify`: it lays the mutant tree out GOPATH-style, runs
		// the jar with a --packageTimeout, reads Gobra's own
		// "Gobra has found N error(s)" line, and ends with one R4 verdict
		// sentence. A run that exhausts its budget prints "R4 UNDECIDED"
		// and no verdict, which this tool records as an error cell -- a
		// proof the solver did not finish is a missing measurement, never
		// a survival (see gobraTimedOut in tools/cmd/gobra/run.go).
		//
		// This rung's verdict on a mutant is only as good as the contract
		// it discharges. Whether the clauses are refutable at all is the
		// `gobra canary` / `gobra reach` audit (F013, F021), which is
		// prior to this table, not part of it.
		ID: "R4", Label: "proof", Tool: "gobra", Inputs: "contract",
		Impls:  []string{"go"},
		Covers: gobraReads,
		Args: func(cfg Config, implName, regPath string) []string {
			// Gobra's own timeout lands a minute before calibrate's, so an
			// undecidable tree is reported in Gobra's words (UNDECIDED)
			// rather than as a killed subprocess.
			b := cfg.rungTimeout() - time.Minute
			if b < time.Minute {
				b = time.Minute
			}
			return []string{"verify", "-impl=" + implName, "-registry=" + regPath, "-budget=" + b.String()}
		},
		Pass: "R4 PASSED", Fail: "R4 FAILED",
		// No server is launched: the tree is verified, not run. Zero is a
		// known count, so the wall column is the rung's whole cost.
		Launches: func(Config, string) (int, bool) { return 0, true },
	},
}

// gobraVerified is Gobra's verification matrix, exactly as TCB.md records it
// and as verifiedPackages in tools/cmd/gobra/run.go drives it. internal/httpshim
// is deliberately absent: it is trusted transport.
var gobraVerified = []string{
	"internal/clock/",
	"internal/ids/",
	"internal/dom/",
	"internal/store/",
	"internal/service/",
}

// gobraReads reports whether any edit of the mutant lands in a package Gobra
// verifies. One covered edit is enough: the proof can notice that one.
func gobraReads(m mutants.Mutant) bool {
	for _, e := range m.Edits {
		for _, p := range gobraVerified {
			if strings.HasPrefix(e.File, p) {
				return true
			}
		}
	}
	return false
}

// applies reports whether this rung exists for the corner at all.
func (r rung) applies(impl string) bool {
	if len(r.Impls) == 0 {
		return true
	}
	for _, i := range r.Impls {
		if i == impl {
			return true
		}
	}
	return false
}

// splitRungs partitions the selected rungs into those that can run on the
// corner and those the corner caps.
func splitRungs(impl string, rungs []rung) (runnable, capped []rung) {
	for _, r := range rungs {
		if r.applies(impl) {
			runnable = append(runnable, r)
		} else {
			capped = append(capped, r)
		}
	}
	return runnable, capped
}

var reProperties = regexp.MustCompile(`properties=(\d+)`)

func selectRungs(ids []string) ([]rung, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("-rungs is empty")
	}
	byID := map[string]rung{}
	var known []string
	for _, r := range allRungs {
		byID[r.ID] = r
		known = append(known, r.ID)
	}
	var out []rung
	seen := map[string]bool{}
	for _, id := range ids {
		r, ok := byID[strings.ToUpper(id)]
		if !ok {
			return nil, fmt.Errorf("unknown rung %q; this tool drives: %s", id, strings.Join(known, ", "))
		}
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out, nil
}

// toolset holds the compiled rung binaries.
//
// They are built once. `go run` would recompile on every invocation, and with
// 36 mutants times 4 rungs that is over a hundred redundant compiles whose
// seconds would land inside the wall-time column and be reported as rung cost.
type toolset struct {
	dir  string
	bins map[string]string
	own  bool
}

var neededTools = []string{"replay", "diffrun", "proptest", "mutate", "gobra"}

func buildTools(root, binDir string) (*toolset, error) {
	ts := &toolset{bins: map[string]string{}}
	if binDir != "" {
		abs, err := filepath.Abs(binDir)
		if err != nil {
			return nil, err
		}
		ts.dir = abs
		for _, name := range neededTools {
			p := filepath.Join(abs, name)
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("-bin %s does not hold %s: %w", abs, name, err)
			}
			ts.bins[name] = p
		}
		return ts, nil
	}
	tmp, err := os.MkdirTemp("", "calibrate-bin-")
	if err != nil {
		return nil, err
	}
	ts.dir, ts.own = tmp, true
	for _, name := range neededTools {
		bin := filepath.Join(tmp, name)
		cmd := exec.Command("go", "build", "-o", bin, "./tools/cmd/"+name)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			ts.close()
			return nil, fmt.Errorf("go build ./tools/cmd/%s: %v\n%s", name, err, out)
		}
		ts.bins[name] = bin
	}
	return ts, nil
}

func (t *toolset) close() {
	if t != nil && t.own && t.dir != "" {
		_ = os.RemoveAll(t.dir)
	}
}

// exec runs one tool from the repository root and returns everything it said.
//
// Root is the working directory because the rungs take relative defaults --
// replay's corpus path, for one -- and a rung invoked from somewhere else would
// fail for a reason that has nothing to do with the mutant.
type toolRun struct {
	Argv     []string
	Stdout   string
	ExitCode int
	TimedOut bool
	Wall     time.Duration
}

func (t *toolset) run(root, name string, args []string, timeout time.Duration) (*toolRun, error) {
	bin, ok := t.bins[name]
	if !ok {
		return nil, fmt.Errorf("no binary for %q", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = root
	start := time.Now()
	out, err := cmd.CombinedOutput()
	tr := &toolRun{
		Argv:     append([]string{name}, args...),
		Stdout:   string(out),
		Wall:     time.Since(start),
		TimedOut: ctx.Err() != nil,
	}
	if err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			tr.ExitCode = ee.ExitCode()
		} else {
			return tr, err
		}
	}
	return tr, nil
}

// verdict reads the rung's own report and returns whether the mutant was
// killed.
//
// Both the sentence and the exit code have to be there and have to agree.
// Reading only the exit code is the failure GOAL.md rule 1 names; reading only
// the sentence would miss a rung that printed PASSED and then crashed on the
// way out. A disagreement is an error, and an error is a missing measurement,
// never a kill.
func (r rung) verdict(tr *toolRun) (killed bool, err error) {
	if tr.TimedOut {
		return false, fmt.Errorf("rung %s timed out; last output:\n%s", r.ID, tail(tr.Stdout, 12))
	}
	pass := countLinesWithPrefix(tr.Stdout, r.Pass)
	fail := countLinesWithPrefix(tr.Stdout, r.Fail)

	switch {
	case pass == 1 && fail == 0:
		if tr.ExitCode != 0 {
			return false, fmt.Errorf("%s printed %q but exited %d; the tool contradicts itself, so this cell has no answer:\n%s",
				r.Tool, r.Pass, tr.ExitCode, tail(tr.Stdout, 12))
		}
		return false, nil
	case fail == 1 && pass == 0:
		if tr.ExitCode == 0 {
			return false, fmt.Errorf("%s printed %q but exited 0; the tool contradicts itself, so this cell has no answer:\n%s",
				r.Tool, r.Fail, tail(tr.Stdout, 12))
		}
		return true, nil
	case pass == 0 && fail == 0:
		return false, fmt.Errorf("%s produced no %s verdict (exit %d). Nothing was measured:\n%s",
			r.Tool, r.ID, tr.ExitCode, tail(tr.Stdout, 12))
	default:
		return false, fmt.Errorf("%s produced %d pass and %d fail verdicts for %s; ambiguous:\n%s",
			r.Tool, pass, fail, r.ID, tail(tr.Stdout, 12))
	}
}

// countLinesWithPrefix counts lines that START with the sentence. Anchoring at
// the line start keeps a verdict from being read out of prose: replay's own
// help text and diffrun's mismatch dump both mention the strings.
func countLinesWithPrefix(s, prefix string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			n++
		}
	}
	return n
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = "    | " + l
	}
	return strings.Join(lines, "\n")
}

func asExitError(err error, out **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*out = ee
	}
	return ok
}
