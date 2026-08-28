package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HARD REQUIREMENT 2 -- survived is not one outcome.
//
// A mutant a rung fails to kill is one of three things, and conflating them
// corrupts the table in opposite directions:
//
//	survived    the rung's own inputs DO elicit the difference, and the rung
//	            still said PASSED. A real weakness in the rung's oracle.
//	unreached   the mutant changes behaviour, but nothing in that rung's input
//	            source elicits the change. An input gap, not a rung weakness.
//	equivalent  the mutant changes no observable behaviour anywhere. Not a
//	            defect at all, so it is removed from the denominator.
//
// Score an unreached mutant as equivalent and the rung gets credit it has not
// earned; score an equivalent one as survived and the rung is blamed for a
// defect that does not exist. F009 is the worked example: `id-burned-on-reject`
// survived R0 at 54/54 and was live the whole time -- the corpus rejected a
// duplicate handle and then never registered another user, so an id burned at
// step 4 had no later allocation to be visible in.
//
// `mutate probe` runs the mutant and the original side by side over the corpus,
// over tracegen traces, and over the mutant's own declared witness, and reports
// WHICH of those tell them apart. That per-input answer is what makes the
// attribution per rung possible rather than per mutant: R0 replays the corpus,
// R1 and R2 draw from tracegen, so a mutant no corpus step distinguishes cannot
// be killed by R0 however good R0's oracle is.

// A ProbeTrace is one input source's result inside a probe.
type ProbeTrace struct {
	Name     string `json:"name"`     // "witness", "corpus", "seed=1"
	Differs  bool   `json:"differs"`  // did the mutant answer differently from the original
	AtIndex  int    `json:"at_index"` // request index of the first difference, when it differs
	Compared int    `json:"compared"` // requests compared, when it does not
}

// A ProbeRecord is the reachability verdict for one mutant.
type ProbeRecord struct {
	Mutant   string       `json:"mutant"`
	TreeHash string       `json:"tree_hash"`
	Live     bool         `json:"live"`
	Reached  []string     `json:"reached_by"`
	Traces   []ProbeTrace `json:"traces"`
	WallMS   float64      `json:"wall_ms"`
	Note     string       `json:"note,omitempty"`
}

// CorpusReaches reports whether the R0 corpus tells the mutant apart from the
// original. This one is EXACT for R0, not a sample: probe replays the same
// fixed corpus file replay does, so "the corpus does not distinguish it" and
// "R0 cannot kill it" are the same statement.
func (p ProbeRecord) CorpusReaches() bool {
	return p.reaches(func(n string) bool { return n == "corpus" })
}

// TracegenReaches reports whether any randomized trace tells the mutant apart.
//
// Unlike CorpusReaches this is a SAMPLE. probe runs a handful of traces where
// R1 runs many more, so "no probe trace reached it" is evidence that R1's input
// distribution struggles to reach it, not proof that it cannot. The report says
// so rather than presenting the two as equally solid.
func (p ProbeRecord) TracegenReaches() bool {
	return p.reaches(func(n string) bool { return strings.HasPrefix(n, "seed=") })
}

func (p ProbeRecord) reaches(match func(string) bool) bool {
	for _, n := range p.Reached {
		if match(n) {
			return true
		}
	}
	return false
}

// ReachesInputs answers the question a rung actually needs: does this rung's
// input source elicit the mutant's difference.
func (p ProbeRecord) ReachesInputs(inputs string) bool {
	switch inputs {
	case "corpus":
		return p.CorpusReaches()
	case "tracegen":
		return p.TracegenReaches()
	default:
		return p.Live
	}
}

var (
	reProbeDiffers = regexp.MustCompile(`^ {2}(\S+) +(?:DIFFERS at request|also differs, at request) (\d+)$`)
	reProbeSame    = regexp.MustCompile(`^ {2}(\S+) +no difference in (\d+) requests$`)
	reProbeSummary = regexp.MustCompile(`^live: (\d+)/(\d+) +no observable change: (\d+)$`)
	reProbeNoBuild = regexp.MustCompile(`mutant does not compile`)
)

// runProbe invokes `mutate probe` for a single mutant and reads its report.
//
// The exit code is deliberately not the signal. probe exits 1 whenever it has
// suspects, and "this mutant is equivalent" is a suspect -- which is also the
// most informative answer it can give here. Reading the code instead of the
// report would turn the single most important classification into a crash.
func runProbe(cfg Config, tools *toolset, implName, id, key, treeHash string) (*ProbeRecord, error) {
	start := time.Now()
	tr, err := tools.run(cfg.Root, "mutate", []string{
		"probe",
		"-manifest=" + cfg.Manifest,
		"-registry=" + cfg.Registry,
		"-corpus=" + cfg.Corpus,
		"-impl=" + implName,
		"-id=" + id,
		"-traces=" + strconv.Itoa(cfg.ProbeTraces),
		"-steps=" + strconv.Itoa(cfg.ProbeSteps),
	}, cfg.rungTimeout())
	if err != nil {
		return nil, err
	}
	rec, err := parseProbe(tr.Stdout)
	if err != nil {
		return nil, fmt.Errorf("%w\n%s", err, tail(tr.Stdout, 15))
	}
	rec.Mutant = key
	rec.TreeHash = treeHash
	rec.WallMS = float64(time.Since(start).Microseconds()) / 1000
	return rec, nil
}

func parseProbe(stdout string) (*ProbeRecord, error) {
	if reProbeNoBuild.MatchString(stdout) {
		return nil, fmt.Errorf("the mutant does not compile, so nothing was probed and no rung result for it means anything")
	}
	rec := &ProbeRecord{}
	sawSummary := false
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if m := reProbeDiffers.FindStringSubmatch(line); m != nil {
			at, _ := strconv.Atoi(m[2])
			rec.Traces = append(rec.Traces, ProbeTrace{Name: m[1], Differs: true, AtIndex: at})
			rec.Reached = append(rec.Reached, m[1])
			continue
		}
		if m := reProbeSame.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			rec.Traces = append(rec.Traces, ProbeTrace{Name: m[1], Compared: n})
			continue
		}
		if m := reProbeSummary.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			live, _ := strconv.Atoi(m[1])
			rec.Live = live > 0
			sawSummary = true
		}
	}
	// The summary line is required. Without it the probe did not finish, and a
	// partial probe read as "nothing reached it" would classify a live mutant
	// as equivalent -- the exact error requirement 2 exists to prevent.
	if !sawSummary {
		return nil, fmt.Errorf("mutate probe produced no `live: N/M` summary; the probe did not finish")
	}
	// The two halves of the report have to agree. probe's own summary is the
	// authority on liveness; the per-trace lines are the authority on WHICH
	// input reached it. If they disagree, neither can be used.
	if rec.Live != (len(rec.Reached) > 0) {
		return nil, fmt.Errorf("mutate probe reported live=%v but %d reaching trace(s); the report contradicts itself",
			rec.Live, len(rec.Reached))
	}
	if rec.Live && len(rec.Reached) == 1 && rec.Reached[0] == "witness" {
		rec.Note = "reached only by its declared witness: neither the corpus nor any probe trace " +
			"produces a request that tells it apart, so no rung driven by those inputs can kill it"
	}
	return rec, nil
}

func (c Config) rungTimeout() time.Duration {
	d, err := time.ParseDuration(c.RungTimeout)
	if err != nil {
		return 20 * time.Minute
	}
	return d
}
