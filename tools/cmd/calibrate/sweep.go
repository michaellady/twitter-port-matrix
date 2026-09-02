package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

// A Setup records what one mutant cost to prepare. It is kept out of every
// rung's wall time on purpose: materialising a tree and warming its build cache
// is the price of running ANY rung against that mutant, so charging it to
// whichever rung happened to go first would make R0 look expensive on Rust and
// cheap on Go for reasons that have nothing to do with R0.
type Setup struct {
	Mutant      string  `json:"mutant"`
	TreeHash    string  `json:"tree_hash"`
	ApplyMS     float64 `json:"apply_ms"`
	WarmBuildMS float64 `json:"warm_build_ms"`
	CopyMode    string  `json:"copy_mode"`
}

// calibrateOne measures every enabled rung against one mutant.
//
// The order is not arbitrary. The mutant is materialised, then the guard
// establishes that the directory the rungs will resolve holds mutated bytes,
// then the tree's build cache is warmed, and only then does any rung run and
// any clock start.
func calibrateOne(cfg Config, tools *toolset, m mutants.Mutant, selected []rung, jr *journal, keepTree bool) ([]Cell, *ProbeRecord, *Setup) {
	key := m.Key()

	// A rung the corner has no verifier for is a capped cell, decided from
	// the configuration alone: nothing is materialised or run for it, and it
	// is appended after the measured cells so the report's per-mutant row
	// still shows every selected rung.
	rungs, capped := splitRungs(m.Impl, selected)
	cappedCells := make([]Cell, 0, len(capped))
	for _, r := range capped {
		cappedCells = append(cappedCells, cappedCell(m, r))
	}
	if len(rungs) == 0 {
		fmt.Printf("\n%-28s capped: no selected rung exists for corner %s\n", key, m.Impl)
		return cappedCells, nil, nil
	}
	cells, probe, setup := calibrateRunnable(cfg, tools, m, rungs, jr, keepTree)
	return append(cells, cappedCells...), probe, setup
}

func cappedCell(m mutants.Mutant, r rung) Cell {
	return Cell{
		Mutant: m.Key(), Impl: m.Impl, ID: m.ID, Family: m.Family,
		Rung: r.ID, Outcome: outcomeCapped,
		Detail: fmt.Sprintf("corner %s has no %s rung (%s exists for %s); not a measurement and in no denominator",
			m.Impl, r.ID, r.Label, strings.Join(r.Impls, ", ")),
	}
}

func calibrateRunnable(cfg Config, tools *toolset, m mutants.Mutant, rungs []rung, jr *journal, keepTree bool) ([]Cell, *ProbeRecord, *Setup) {
	key := m.Key()

	// The planned content address is computed BEFORE anything is written, from
	// the current source plus the manifest. That is what makes resumption safe:
	// if another agent has edited the corner since the last run, the address
	// differs, the journalled cells do not match, and this mutant is measured
	// again instead of a stale number being reported as fresh.
	srcDir, err := implDir(cfg, m.Impl)
	if err != nil {
		return []Cell{errCell(m, "", rungs, err)}, nil, nil
	}
	_, planned, err := mutants.PlanHash(srcDir, m)
	if err != nil {
		return []Cell{errCell(m, "", rungs, fmt.Errorf("planning the mutant: %w", err))}, nil, nil
	}

	// If every cell for this exact tree is already journalled, the mutant does
	// not need to be materialised at all -- which is where most of the time
	// saved by resuming actually comes from on the Rust corner.
	if cells, probe, ok := reuse(cfg, m, planned, rungs, jr); ok {
		fmt.Printf("\n%-28s %s  (resumed, %d cell(s) from journal)\n", key, short(planned), len(cells))
		return cells, probe, nil
	}

	outDir, err := os.MkdirTemp("", "calibrate-mutant-")
	if err != nil {
		return []Cell{errCell(m, planned, rungs, err)}, nil, nil
	}
	if !keepTree {
		defer os.RemoveAll(outDir)
	}

	res, regPath, applyRun, err := applyMutant(cfg, tools, m, outDir)
	if err != nil {
		return []Cell{errCell(m, planned, rungs, fmt.Errorf("materialising: %w", err))}, nil, nil
	}
	setup := Setup{Mutant: key, TreeHash: res.TreeHash, ApplyMS: msOf(applyRun), CopyMode: res.CopyMode}

	implName := m.Impl + "@" + m.ID
	guard, err := checkMutantSelected(cfg.Root, implName, regPath, res)
	if err != nil {
		return []Cell{errCell(m, res.TreeHash, rungs, err)}, nil, nil
	}

	fmt.Printf("\n%-28s %s  %s\n", key, short(res.TreeHash), m.Family)
	fmt.Printf("  guard    %d/5 checks: %s -> %s (baseline %s is a different tree)\n",
		guard.Checks, implName, guard.Dir, m.Impl)

	// Warm the mutant tree's build cache once, before any rung runs. The first
	// build inside a freshly cloned Rust tree is a real compile (~4s); leaving
	// it inside the first rung's wall time would put a one-off cost into a
	// per-rung number and it would look like that rung is slow.
	warm := warmBuild(cfg, res.TreeDir, m.Impl)
	setup.WarmBuildMS = warm
	fmt.Printf("  setup    apply %.1fs (%s copy) + warm build %.1fs -- charged to no rung\n",
		setup.ApplyMS/1000, res.CopyMode, warm/1000)

	cells := runRungs(rungs, key, res.TreeHash, jr, func(r rung) Cell {
		return runRung(cfg, tools, m, r, implName, regPath, res.TreeHash, guard)
	})

	probe := maybeProbe(cfg, tools, m, res.TreeHash, cells, jr)
	classify(m, cells, probe, rungs)
	return cells, probe, &setup
}

// runRungs measures EVERY selected rung against one materialised mutant.
//
// PER-JUDGE ATTRIBUTION, and that is why this loop has no early exit. A mutant
// R0 kills is still handed to R1, R2, R4 and R5, and every rung that would
// have killed it records its own kill in its own cell. Stopping at the first
// killing rung -- first-judge attribution -- would be cheaper and would
// produce a table that understates every rung after the first by an amount
// determined by nothing but the order the rungs happen to run in. The later
// rows would read "adds nothing over the cheaper rung", which would be a
// statement about this loop, not about those rungs.
//
// The invocation is injected so the loop's shape is testable without
// materialising a tree or launching a verifier: TestEveryRungRunsAfterAKill
// fails the moment a `break` or a kill short circuit appears here.
func runRungs(rungs []rung, key, treeHash string, jr *journal, run func(rung) Cell) []Cell {
	var cells []Cell
	for _, r := range rungs {
		if c, ok := jr.cell(key, treeHash, r.ID); ok {
			fmt.Printf("  %-3s      %-10s %6.1fs  (resumed)\n", r.ID, c.Outcome, c.WallMS/1000)
			cells = append(cells, c)
			continue
		}
		c := run(r)
		jr.appendCell(c)
		cells = append(cells, c)
	}
	return cells
}

// runRung invokes one rung and reads its answer. The outcome recorded here is
// deliberately RAW -- killed, or "survived?" -- because whether a survival is a
// rung weakness, an input gap, or a non-defect is not knowable until the mutant
// has been probed. Journalling the raw answer means a resumed run reclassifies
// from the same evidence rather than inheriting a verdict.
func runRung(cfg Config, tools *toolset, m mutants.Mutant, r rung, implName, regPath, treeHash string, guard *guardResult) Cell {
	c := Cell{
		Mutant: m.Key(), Impl: m.Impl, ID: m.ID, Family: m.Family,
		TreeHash: treeHash, Rung: r.ID, Guard: guard,
	}
	tr, err := tools.run(cfg.Root, r.Tool, r.Args(cfg, implName, regPath), cfg.rungTimeout())
	if err != nil {
		c.Outcome, c.Error = outcomeError, err.Error()
		fmt.Printf("  %-3s      ERROR      %s\n", r.ID, err)
		return c
	}
	c.WallMS = msOf(tr)
	c.Verdict = verdictLine(tr.Stdout, r)

	killed, err := r.verdict(tr)
	if err != nil {
		c.Outcome, c.Error = outcomeError, err.Error()
		fmt.Printf("  %-3s      ERROR      %s\n", r.ID, firstLineOf(err.Error()))
		return c
	}
	if killed {
		c.Outcome = outcomeKilled
		// No tool-cost figure for a kill. The rung stopped at its first
		// mismatch and then, in R1's case, shrank the failing trace -- and
		// every shrink attempt is another server launch. The launch count of a
		// kill therefore depends on the defect, not on the configuration, and
		// correcting a number nobody counted would invent one. Measured: an R1
		// kill at 4 traces cost 20.3s where the four traces themselves account
		// for well under a quarter of it.
		fmt.Printf("  %-3s      killed     %6.1fs  %s\n", r.ID, c.WallMS/1000, trunc1(c.Verdict))
		return c
	}
	c.Outcome = outcomeUnclassified
	if n, ok := r.Launches(cfg, tr.Stdout); ok {
		c.Launches = n
	}
	fmt.Printf("  %-3s      survived   %6.1fs  %s\n", r.ID, c.WallMS/1000, trunc1(c.Verdict))
	return c
}

// maybeProbe runs `mutate probe` when a survival needs explaining.
//
// It is not run for every mutant because it is expensive -- it builds the
// mutant and the original and drives both over five traces -- and a mutant
// every rung killed needs no explanation. The default fires on ANY survival
// rather than only on a mutant that survived everything, because the
// interesting question is per rung: a mutant R1 kills and R0 does not is
// exactly the case where "was R0 blind or was the corpus short" has an answer
// worth having, and F009 is that case.
func maybeProbe(cfg Config, tools *toolset, m mutants.Mutant, treeHash string, cells []Cell, jr *journal) *ProbeRecord {
	if cfg.ProbeMode == "never" {
		return nil
	}
	survived, measured := 0, 0
	for _, c := range cells {
		if c.Outcome == outcomeError {
			continue
		}
		measured++
		if c.Outcome == outcomeUnclassified {
			survived++
		}
	}
	switch cfg.ProbeMode {
	case "any":
		if survived == 0 {
			return nil
		}
	case "all":
		if measured == 0 || survived != measured {
			return nil
		}
	}
	if p, ok := jr.probe(m.Key(), treeHash); ok {
		fmt.Printf("  probe    %s  (resumed)\n", reachSummary(p))
		return &p
	}
	p, err := runProbe(cfg, tools, m.Impl, m.ID, m.Key(), treeHash)
	if err != nil {
		fmt.Printf("  probe    UNAVAILABLE: %s\n", firstLineOf(err.Error()))
		return nil
	}
	jr.appendProbe(*p)
	fmt.Printf("  probe    %s  (%.1fs)\n", reachSummary(*p), p.WallMS/1000)
	return p
}

// classify turns raw survivals into the three-way outcome. It is a pure
// function of the cell and the probe, which is what lets a resumed run produce
// the same table as an uninterrupted one.
func classify(m mutants.Mutant, cells []Cell, p *ProbeRecord, rungs []rung) {
	byID := map[string]rung{}
	for _, r := range rungs {
		byID[r.ID] = r
	}
	for i := range cells {
		c := &cells[i]
		if c.Outcome != outcomeUnclassified {
			continue
		}
		r := byID[c.Rung]
		// A proof rung's reach is decided from the verification matrix, not
		// from a probe: either the verifier reads a file the mutant edits or
		// it does not. Liveness still comes from the probe, because a
		// survivor nobody can tell from the original is equivalent whatever
		// the verifier read.
		covered := true
		if r.Covers != nil {
			covered = r.Covers(m)
		}
		if p == nil {
			c.Detail = "not probed, so this survival is unexplained: it may be a rung weakness, an input gap, or a mutant with no observable effect"
			if !covered {
				c.Detail = "not probed; the verifier reads none of the files this mutant edits, so if it is live it is unreached by the contract, not survived"
			}
			continue
		}
		switch {
		case !p.Live:
			c.Outcome = outcomeEquivalent
			c.Detail = "no probe input tells the mutant apart from the original, including its own declared witness; not counted against any rung"
		case !covered:
			c.Outcome = outcomeUnreached
			c.Detail = fmt.Sprintf("live (reached by %s), but the verifier reads none of the files this mutant edits (%s); no obligation covers it",
				strings.Join(p.Reached, ", "), editedFiles(m))
		case r.Covers != nil:
			c.Outcome = outcomeSurvived
			c.Detail = fmt.Sprintf("live (reached by %s) and inside the verified core (%s), yet the proof passed: the contract does not constrain the mutated behaviour",
				strings.Join(p.Reached, ", "), editedFiles(m))
		case p.ReachesInputs(r.Inputs):
			c.Outcome = outcomeSurvived
			c.Detail = fmt.Sprintf("live and reached by this rung's inputs (%s), yet the rung passed: a gap in the rung, not in the inputs",
				strings.Join(p.Reached, ", "))
		default:
			c.Outcome = outcomeUnreached
			c.Detail = fmt.Sprintf("live, but nothing in this rung's input source (%s) elicits the difference; reached only by %s",
				r.Inputs, strings.Join(p.Reached, ", "))
		}
	}
}

func editedFiles(m mutants.Mutant) string {
	seen := map[string]bool{}
	var out []string
	for _, e := range m.Edits {
		if !seen[e.File] {
			seen[e.File] = true
			out = append(out, e.File)
		}
	}
	return strings.Join(out, ", ")
}

// reuse returns the journalled cells for a mutant when every enabled rung is
// already recorded at this exact tree hash.
func reuse(cfg Config, m mutants.Mutant, treeHash string, rungs []rung, jr *journal) ([]Cell, *ProbeRecord, bool) {
	var cells []Cell
	for _, r := range rungs {
		c, ok := jr.cell(m.Key(), treeHash, r.ID)
		if !ok {
			return nil, nil, false
		}
		cells = append(cells, c)
	}
	var probe *ProbeRecord
	if p, ok := jr.probe(m.Key(), treeHash); ok {
		probe = &p
	}
	// A survivor with no journalled probe is not resumable: the classification
	// would be missing, and reporting it as "survived" on absent evidence is
	// the mistake requirement 2 exists to prevent.
	if probe == nil && cfg.ProbeMode != "never" {
		for _, c := range cells {
			if c.Outcome == outcomeUnclassified {
				return nil, nil, false
			}
		}
	}
	classify(m, cells, probe, rungs)
	return cells, probe, true
}

func warmBuild(cfg Config, treeDir, impl string) float64 {
	spec, err := implSpec(cfg, impl)
	if err != nil {
		return 0
	}
	spec.Dir = treeDir
	start := time.Now()
	b, err := implrun.Compile(spec)
	if b != nil {
		b.Close()
	}
	if err != nil {
		// Not fatal here. A mutant that does not compile is a real outcome and
		// the rung about to run will say so in its own words, which is the
		// output that belongs in the record.
		return 0
	}
	return msSince(start)
}

func implSpec(cfg Config, impl string) (implrun.Spec, error) {
	reg, err := implrun.LoadRegistry(underRoot(cfg.Root, cfg.Registry))
	if err != nil {
		return implrun.Spec{}, err
	}
	spec, err := reg.Get(impl)
	if err != nil {
		return implrun.Spec{}, err
	}
	if !filepath.IsAbs(spec.Dir) {
		spec.Dir = filepath.Join(cfg.Root, spec.Dir)
	}
	return spec, nil
}

func implDir(cfg Config, impl string) (string, error) {
	spec, err := implSpec(cfg, impl)
	if err != nil {
		return "", err
	}
	return spec.Dir, nil
}

func errCell(m mutants.Mutant, treeHash string, rungs []rung, err error) Cell {
	id := "-"
	if len(rungs) > 0 {
		id = rungs[0].ID
	}
	fmt.Printf("\n%-28s ERROR  %s\n", m.Key(), firstLineOf(err.Error()))
	return Cell{
		Mutant: m.Key(), Impl: m.Impl, ID: m.ID, Family: m.Family,
		TreeHash: treeHash, Rung: id, Outcome: outcomeError, Error: err.Error(),
	}
}

// verdictLine pulls the rung's own summary sentence out of its output so the
// record carries what the tool said, not this tool's paraphrase of it.
func verdictLine(stdout string, r rung) string {
	for _, line := range strings.Split(stdout, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, r.Pass) || strings.HasPrefix(t, r.Fail) {
			return t
		}
	}
	return ""
}

func reachSummary(p ProbeRecord) string {
	if !p.Live {
		return "EQUIVALENT -- no input tells it apart from the original"
	}
	return "live, reached by " + strings.Join(p.Reached, ", ")
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func trunc1(s string) string {
	const n = 58
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func msOf(tr *toolRun) float64 {
	if tr == nil {
		return 0
	}
	return float64(tr.Wall.Microseconds()) / 1000
}
