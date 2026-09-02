// Command calibrate is the repository's deliverable: the per-rung mutation
// kill table.
//
// For every mutant in the catalogue and every enabled rung it answers two
// questions -- did the rung notice, and what did noticing cost -- and it does
// so by RUNNING the real rungs as subprocesses and reading their own verdict
// sentences. Nothing here reimplements a rung. A calibration that measured a
// reimplementation would be measuring itself.
//
// Three things about this tool are load-bearing, and each exists because
// getting it wrong produces a table that is wrong while looking fine.
//
//  1. IT ASSERTS THAT THE MUTANT WAS THE THING EXERCISED. `mutate apply` emits
//     a registry naming BOTH the mutant (`go@<id>`) and the untouched original
//     (`go`). Pointing a rung at `go` runs the ORIGINAL, passes cleanly, and is
//     byte-for-byte indistinguishable in the output from the mutant surviving.
//     That slip happened for real while confirming F009. Made here it would
//     report every mutant a survivor and every rung worthless, silently, having
//     measured the unmutated implementation N times. See guard.go: the name
//     must carry the `@<id>` suffix, and the directory the rung will actually
//     resolve is content-addressed and compared against the mutant tree hash
//     before any rung is allowed to run. `-guard-demo` shows both the false
//     green and the refusal.
//
//  2. SURVIVED IS NOT ONE OUTCOME. A mutant a rung fails to kill is one of
//     three things, and they mean opposite corrections to the table:
//
//     survived    the rung's own inputs DO elicit the defect and the rung
//     still said PASSED. A real weakness in the rung.
//     unreached   the mutant changes behaviour, but nothing in that rung's
//     input source elicits the change. An input gap, not a rung
//     weakness -- F009's `id-burned-on-reject` exactly.
//     equivalent  the mutant changes no observable behaviour at all. Not a
//     defect; excluded from the denominator, because counting it
//     would record a survivor at every rung and understate all of
//     them.
//
//     `mutate probe` settles which, by running the mutant and the original side
//     by side over the corpus, over tracegen traces, and over the mutant's own
//     declared witness. Reachability is attributed per rung by input source:
//     R0 is driven by the corpus, R1 and R2 by tracegen.
//
//  3. COST IS REPORTED TWICE, LABELLED. Raw wall time mixes what the rung costs
//     with what the language costs: R1 relaunches a server per trace and R2 per
//     property-round, so a rung's seconds are dominated by process startup, and
//     Go and Rust do not start at the same speed. The launch floor is therefore
//     MEASURED per corner and the table reports both raw wall and
//     wall - launches*floor. Neither is "the" cost; the report says which is
//     which.
//
// The run is resumable. A full sweep is 36 mutants times 4 rungs, and a crash
// two hours in must not restart from zero. Every completed cell is appended to
// a journal keyed by the mutant's CONTENT ADDRESS, not its name -- so a resume
// after the source drifted under a mutant re-runs that mutant instead of
// silently reusing a measurement of different code.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// Config is the whole of what a run was asked to do. It is written into the
// results file verbatim: a kill table without the parameters that produced it
// is not reproducible, and an unreproducible number is an anecdote.
type Config struct {
	Root         string   `json:"root"`
	Manifest     string   `json:"manifest"`
	Registry     string   `json:"registry"`
	Corpus       string   `json:"corpus"`
	Impls        []string `json:"impls"`
	IDs          []string `json:"ids,omitempty"`
	Family       string   `json:"family,omitempty"`
	Rungs        []string `json:"rungs"`
	R1Traces     int      `json:"r1_traces"`
	R1Steps      int      `json:"r1_steps"`
	R1Seed       int64    `json:"r1_seed"`
	R2Rounds     int      `json:"r2_rounds"`
	R2Setup      int      `json:"r2_setup"`
	R2Seed       int64    `json:"r2_seed"`
	ProbeMode    string   `json:"probe_mode"`
	ProbeTraces  int      `json:"probe_traces"`
	ProbeSteps   int      `json:"probe_steps"`
	FloorSamples int      `json:"floor_samples"`
	RungTimeout  string   `json:"rung_timeout"`
}

func main() {
	var (
		root      = flag.String("root", ".", "repository root; rung subprocesses run from here")
		manifest  = flag.String("manifest", "tools/cmd/mutate/mutants.json", "mutant catalogue")
		registry  = flag.String("registry", "impls/registry.json", "implementation registry")
		corpus    = flag.String("corpus", "generated/conformance.jsonl", "R0 corpus")
		implsFlag = flag.String("impls", "go,rust", "corners to sweep, comma-separated")
		idsFlag   = flag.String("ids", "", "restrict to these mutant ids, comma-separated")
		family    = flag.String("family", "", "restrict to one mutant family")
		rungsFlag = flag.String("rungs", "R0,R1,R2", "rungs to run, comma-separated; R0,R1 is a valid partial sweep. R4 is opt-in and per corner: Gobra on go, Verus on rust, JBMC on kotlin (~1-2 min per mutant); R5 is Gobra on go")
		out       = flag.String("out", "", "result directory (default evidence/runs/calibration/<stamp>)")
		resume    = flag.Bool("resume", false, "reuse the journal already in -out and skip cells it records")

		r1Traces = flag.Int("r1-traces", 20, "R1 traces per mutant")
		r1Steps  = flag.Int("r1-steps", 200, "R1 requests per trace")
		r1Seed   = flag.Int64("r1-seed", 1, "R1 base seed")
		r2Rounds = flag.Int("r2-rounds", 6, "R2 rounds per property")
		r2Setup  = flag.Int("r2-setup", 40, "R2 requests used to reach each random state")
		r2Seed   = flag.Int64("r2-seed", 1, "R2 base seed")

		probeMode   = flag.String("probe", "any", "when to run `mutate probe`: any (a mutant that survived any rung), all (survived every rung), never")
		probeTraces = flag.Int("probe-traces", 4, "randomized traces per probe, after the corpus")
		probeSteps  = flag.Int("probe-steps", 120, "requests per probe trace")

		floorSamples = flag.Int("floor-samples", 3, "build+launch samples used to measure each corner's process floor; 0 skips it")
		rungTimeout  = flag.Duration("rung-timeout", 20*time.Minute, "kill a rung subprocess after this long")
		keepTrees    = flag.Bool("keep-trees", false, "keep each mutant's scratch tree instead of removing it")
		binDir       = flag.String("bin", "", "directory holding prebuilt rung binaries; default builds them into a scratch dir")

		guardDemo = flag.String("guard-demo", "", "prove the requirement-1 guard fires: takes impl/id, e.g. go/cursor-inclusive, and exits")
	)
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		die("resolving -root: %v", err)
	}

	cfg := Config{
		Root:         absRoot,
		Manifest:     *manifest,
		Registry:     *registry,
		Corpus:       *corpus,
		Impls:        splitList(*implsFlag),
		IDs:          splitList(*idsFlag),
		Family:       *family,
		Rungs:        splitList(*rungsFlag),
		R1Traces:     *r1Traces,
		R1Steps:      *r1Steps,
		R1Seed:       *r1Seed,
		R2Rounds:     *r2Rounds,
		R2Setup:      *r2Setup,
		R2Seed:       *r2Seed,
		ProbeMode:    *probeMode,
		ProbeTraces:  *probeTraces,
		ProbeSteps:   *probeSteps,
		FloorSamples: *floorSamples,
		RungTimeout:  rungTimeout.String(),
	}
	switch cfg.ProbeMode {
	case "any", "all", "never":
	default:
		die("-probe must be any, all or never; got %q", cfg.ProbeMode)
	}

	selected, err := selectRungs(cfg.Rungs)
	if err != nil {
		die("%v", err)
	}

	man, err := mutants.Load(underRoot(absRoot, cfg.Manifest))
	if err != nil {
		die("loading manifest: %v", err)
	}

	tools, err := buildTools(absRoot, *binDir)
	if err != nil {
		die("building the rung binaries: %v", err)
	}
	defer tools.close()

	if *guardDemo != "" {
		os.Exit(runGuardDemo(cfg, man, tools, *guardDemo))
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(absRoot, "evidence", "runs", "calibration",
			time.Now().UTC().Format("20060102-150405"))
	} else if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(absRoot, outDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		die("creating %s: %v", outDir, err)
	}

	sel := selectMutants(man, cfg)
	if len(sel) == 0 {
		die("no mutants match impls=%v ids=%v family=%q", cfg.Impls, cfg.IDs, cfg.Family)
	}

	jr, err := openJournal(filepath.Join(outDir, "journal.jsonl"), *resume)
	if err != nil {
		die("%v", err)
	}
	defer jr.close()

	// One entry per rung ID: a rung dispatches to a per-corner driver
	// (Gobra on go, Verus on rust, JBMC on kotlin), so the table already has
	// one row and one column per ID and needs no collapsing step.
	reported := selected
	fmt.Printf("calibrate: %d mutant(s) x %d rung(s) = %d cells\n", len(sel), len(reported), len(sel)*len(reported))
	fmt.Printf("           rungs=%s  out=%s  resume=%v\n", strings.Join(cfg.Rungs, ","), outDir, *resume)
	if *resume {
		fmt.Printf("           journal holds %d reusable cell(s) and %d probe(s)\n", len(jr.cells), len(jr.probes))
	}
	fmt.Println(strings.Repeat("=", 78))

	run := &Run{
		Tool:      "calibrate",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Config:    cfg,
		Floors:    map[string]Floor{},
	}

	// The floor is measured before any mutant runs, against the UNMUTATED
	// corner. It is the cost of one build-check plus one process launch plus
	// one health wait -- exactly what each rung pays per server it starts --
	// and it is what makes "R1 took 90s" separable into tool cost and language
	// cost. Measured, not assumed: Go and Rust do not launch at the same speed
	// and neither number is guessable.
	if cfg.FloorSamples > 0 {
		for _, impl := range implsOf(sel) {
			if f, ok := jr.floor(impl); ok {
				run.Floors[impl] = f
				fmt.Printf("  floor %-6s %6.0f ms/launch  (from journal)\n", impl, f.LaunchMS)
				continue
			}
			f, err := measureFloor(absRoot, cfg.Registry, impl, cfg.FloorSamples)
			if err != nil {
				fmt.Printf("  floor %-6s UNAVAILABLE: %v\n", impl, err)
				continue
			}
			run.Floors[impl] = f
			jr.appendFloor(f)
			fmt.Printf("  floor %-6s %6.0f ms/launch  (%d samples, spread %.0f-%.0f ms)\n",
				impl, f.LaunchMS, f.Samples, f.MinMS, f.MaxMS)
		}
		fmt.Println(strings.Repeat("-", 78))
	}

	for _, m := range sel {
		cells, probe, setup := calibrateOne(cfg, tools, m, selected, jr, *keepTrees)
		run.Cells = append(run.Cells, cells...)
		if probe != nil {
			run.Probes = append(run.Probes, *probe)
		}
		if setup != nil {
			run.Setups = append(run.Setups, *setup)
		}
	}

	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	run.Summary = summarize(run, reported)
	run.Warnings = warnings(run, reported)

	body := renderReport(run, reported)
	fmt.Print("\n" + body)

	if err := os.WriteFile(filepath.Join(outDir, "report.txt"), []byte(body), 0o644); err != nil {
		die("writing report.txt: %v", err)
	}
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		die("encoding results: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "results.json"), append(b, '\n'), 0o644); err != nil {
		die("writing results.json: %v", err)
	}
	fmt.Printf("\nwritten: %s/report.txt\n         %s/results.json\n         %s/journal.jsonl\n",
		outDir, outDir, outDir)

	// A cell that errored means a number is missing, not that a rung was
	// green. Exiting 0 on a partially-failed sweep would let a caller record
	// an incomplete table as a complete one.
	for _, c := range run.Cells {
		if c.Outcome == outcomeError {
			os.Exit(1)
		}
	}
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "calibrate: "+format+"\n", a...)
	os.Exit(1)
}

// underRoot resolves a configured path against the repository root, leaving an
// already-absolute path alone.
//
// Rung subprocesses run with the root as their working directory, so a relative
// path means the same thing to them and to this process -- but only if this
// process joins it rather than resolving it against its own cwd. An absolute
// path has to survive untouched, because pointing -manifest or -corpus at a
// scratch file outside the repository is how a narrowed rung gets calibrated.
func underRoot(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// selectMutants applies the corner / id / family filters. It goes through
// Manifest.Select once per corner so the result keeps catalogue order, which
// groups by corner and family and makes the report read stably across runs.
func selectMutants(man *mutants.Manifest, cfg Config) []mutants.Mutant {
	want := map[string]bool{}
	for _, id := range cfg.IDs {
		want[id] = true
	}
	var out []mutants.Mutant
	for _, impl := range cfg.Impls {
		for _, m := range man.Select(impl, "", cfg.Family) {
			if len(want) > 0 && !want[m.ID] {
				continue
			}
			out = append(out, m)
		}
	}
	return out
}

func implsOf(ms []mutants.Mutant) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		if !seen[m.Impl] {
			seen[m.Impl] = true
			out = append(out, m.Impl)
		}
	}
	return out
}
