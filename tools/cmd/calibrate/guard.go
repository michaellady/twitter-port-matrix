package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// applyTimeout bounds materialisation. It is generous because a cold Rust tree
// clone is hundreds of megabytes, and short enough that a wedged copy does not
// stall a 36-mutant sweep indefinitely.
const applyTimeout = 10 * time.Minute

// HARD REQUIREMENT 1 -- assert that the mutant was the thing exercised.
//
// `mutate apply` writes a registry naming BOTH the mutant (`go@<id>`) and the
// untouched original (`go`), deliberately: a rung run against a mutant is only
// interpretable next to the same rung run against the original. The cost of
// that is a name one character away from the mutant which resolves to the
// pristine implementation, passes every rung cleanly, and produces output
// indistinguishable from the mutant surviving.
//
// That is not hypothetical. It happened while confirming F009 and it looked
// exactly like the mutant surviving. Made inside this tool it would report
// every mutant a survivor and every rung worthless, with no error anywhere --
// the sweep would simply have measured the unmutated implementation N times
// and printed a table of zeroes that reads as a finding about the rungs.
//
// So the name is not trusted, and neither is the registry. The directory the
// rung is ABOUT to resolve is read off disk, hashed, and compared against the
// mutant tree's content address before the rung is allowed to run.

// A guardResult is the evidence that a specific rung invocation was pointed at
// mutated bytes. It is recorded per cell so the claim is auditable after the
// fact rather than being a property of code nobody re-reads.
type guardResult struct {
	ImplName   string `json:"impl_name"`
	Dir        string `json:"dir"`
	TreeHash   string `json:"tree_hash"`
	SourceHash string `json:"source_hash"`
	BaselineIs string `json:"baseline_dir"`
	Checks     int    `json:"checks_passed"`
}

// checkMutantSelected refuses every way a rung invocation could end up
// measuring something other than the mutant.
//
// Each check is a distinct failure mode, and they are ordered cheapest first so
// the message names the earliest thing that is wrong:
//
//  1. the name lacks the `@<id>` suffix -- the F009 slip exactly;
//  2. the name is not in the registry at all;
//  3. it resolves to the same directory as the baseline entry;
//  4. it resolves INSIDE impls/, which would mean an implementation was
//     mutated in place rather than copied;
//  5. the bytes on disk do not hash to the mutant tree's content address, or
//     do hash to the ORIGINAL's. Checks 1-4 are all about names and paths;
//     only this one reads the code the server will actually run.
func checkMutantSelected(root, implName, regPath string, res *mutants.Result) (*guardResult, error) {
	want := res.Mutant.Impl + "@" + res.Mutant.ID

	// 1 -- the suffix. Checked against the mutant's own identity rather than
	// against a bare strings.Contains("@"): "go@" or "go@typo" would satisfy
	// the loose form and resolve to nothing, or worse, to something else.
	if implName != want {
		return nil, fmt.Errorf(
			"refusing to run: implementation name %q is not the mutant.\n"+
				"  The registry `mutate apply` writes also names the UNMUTATED corner %q.\n"+
				"  Selecting that name tests the original, passes cleanly, and is\n"+
				"  indistinguishable in the output from the mutant surviving (F009).\n"+
				"  Expected %q.",
			implName, res.Mutant.Impl, want)
	}

	reg, err := implrun.LoadRegistry(regPath)
	if err != nil {
		return nil, fmt.Errorf("reading the mutant registry %s: %w", regPath, err)
	}

	// 2 -- present at all.
	spec, err := reg.Get(implName)
	if err != nil {
		return nil, fmt.Errorf("refusing to run: %w", err)
	}

	// 3 -- distinct from the baseline. If `mutate apply` ever wrote the same
	// directory for both entries, checks 1 and 2 would both pass while the
	// rung ran the original.
	base, err := reg.Get(res.Mutant.Impl)
	if err != nil {
		return nil, fmt.Errorf("refusing to run: the mutant registry has no baseline entry %q to compare against: %w",
			res.Mutant.Impl, err)
	}
	mutDir, err := filepath.Abs(spec.Dir)
	if err != nil {
		return nil, err
	}
	baseDir, err := filepath.Abs(base.Dir)
	if err != nil {
		return nil, err
	}
	if mutDir == baseDir {
		return nil, fmt.Errorf(
			"refusing to run: %q and the baseline %q both resolve to %s, so the rung would test the original",
			implName, res.Mutant.Impl, mutDir)
	}

	// 4 -- outside impls/. A mutant tree under impls/ would mean the injector
	// wrote to the thing it is measuring, which is the one failure this whole
	// rig is built to avoid.
	implsRoot, err := filepath.Abs(filepath.Join(root, "impls"))
	if err != nil {
		return nil, err
	}
	if within(mutDir, implsRoot) {
		return nil, fmt.Errorf(
			"refusing to run: the mutant tree %s is inside %s. A mutant is a COPY; a tree under\n"+
				"  impls/ means an implementation was written to, and every later measurement is suspect",
			mutDir, implsRoot)
	}

	// 5 -- the bytes. Everything above is names and paths; this reads what the
	// server will run and checks it against the content address `mutate apply`
	// recorded, and against the original's, so neither "wrong tree" nor
	// "unmutated tree at the right path" can get through.
	onDisk, err := mutants.LoadTree(mutDir)
	if err != nil {
		return nil, fmt.Errorf("refusing to run: cannot read the tree at %s: %w", mutDir, err)
	}
	got := onDisk.Hash()
	if got == res.SourceHash {
		return nil, fmt.Errorf(
			"refusing to run: the tree at %s hashes to the UNMUTATED source (%s).\n"+
				"  The mutation did not land. A rung run against this would report a survivor\n"+
				"  for a defect that was never injected",
			mutDir, short(res.SourceHash))
	}
	if got != res.TreeHash {
		return nil, fmt.Errorf(
			"refusing to run: the tree at %s does not match the materialised mutant.\n"+
				"  on disk  %s\n  expected %s\n"+
				"  Something rewrote the tree between `mutate apply` and now",
			mutDir, got, res.TreeHash)
	}

	return &guardResult{
		ImplName:   implName,
		Dir:        mutDir,
		TreeHash:   got,
		SourceHash: res.SourceHash,
		BaselineIs: baseDir,
		Checks:     5,
	}, nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func short(h string) string {
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// applyMutant materialises one mutant by running `mutate apply`, and reads back
// the provenance record it wrote.
//
// The real `mutate apply` is used rather than calling mutants.Materialize
// directly, for one reason: Materialize does not write the two-entry registry,
// and the two-entry registry IS the hazard requirement 1 exists to catch.
// Bypassing it would make the guard a test of code no calibration run
// executes.
func applyMutant(cfg Config, tools *toolset, m mutants.Mutant, outDir string) (*mutants.Result, string, *toolRun, error) {
	tr, err := tools.run(cfg.Root, "mutate", []string{
		"apply",
		"-manifest=" + cfg.Manifest,
		"-registry=" + cfg.Registry,
		"-impl=" + m.Impl,
		"-id=" + m.ID,
		"-out=" + outDir,
		"-force",
	}, applyTimeout)
	if err != nil {
		return nil, "", tr, err
	}
	if tr.ExitCode != 0 {
		return nil, "", tr, fmt.Errorf("mutate apply exited %d:\n%s", tr.ExitCode, tail(tr.Stdout, 12))
	}
	b, err := os.ReadFile(filepath.Join(outDir, "mutant.json"))
	if err != nil {
		return nil, "", tr, fmt.Errorf("mutate apply reported success but wrote no provenance record: %w", err)
	}
	var res mutants.Result
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, "", tr, fmt.Errorf("reading %s/mutant.json: %w", outDir, err)
	}
	if res.Mutant.ID != m.ID || res.Mutant.Impl != m.Impl {
		return nil, "", tr, fmt.Errorf("mutate apply materialised %s but was asked for %s", res.Mutant.Key(), m.Key())
	}
	return &res, filepath.Join(outDir, "registry.json"), tr, nil
}
