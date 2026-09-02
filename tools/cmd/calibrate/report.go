package main

import (
	"fmt"
	"sort"
	"strings"
)

// A Run is the whole machine-readable result. The human table is rendered from
// exactly this structure, so the two cannot drift: anything the table claims is
// in the JSON, and anything in the JSON was measured.
type Run struct {
	Tool       string           `json:"tool"`
	StartedAt  string           `json:"started_at"`
	FinishedAt string           `json:"finished_at"`
	Config     Config           `json:"config"`
	Floors     map[string]Floor `json:"launch_floors"`
	Setups     []Setup          `json:"setups,omitempty"`
	Cells      []Cell           `json:"cells"`
	Probes     []ProbeRecord    `json:"probes,omitempty"`
	Summary    []RungSummary    `json:"summary"`
	Warnings   []string         `json:"warnings,omitempty"`
}

// A RungSummary is one row of the kill table.
//
// The denominator is stated twice on purpose. "Reachable" divides by the
// mutants this rung's inputs can actually elicit, which is the honest measure
// of the rung's ORACLE. "Live" divides by every mutant that changes behaviour
// at all, which is the honest measure of the rung AS CONFIGURED -- inputs
// included. A rung with a perfect oracle and a blind corpus scores 100% on the
// first and poorly on the second, and both facts are worth having. Equivalent
// mutants appear in neither.
type RungSummary struct {
	Rung         string  `json:"rung"`
	Label        string  `json:"label"`
	Inputs       string  `json:"inputs"`
	Impl         string  `json:"impl,omitempty"` // empty means "all corners"
	Live         int     `json:"live"`
	Killed       int     `json:"killed"`
	Survived     int     `json:"survived"`
	Unreached    int     `json:"unreached"`
	Equivalent   int     `json:"equivalent"`
	Unclassified int     `json:"unclassified"`
	Errors       int     `json:"errors"`
	Capped       int     `json:"capped,omitempty"` // cells the corner has no verifier for; in no denominator
	KillReach    float64 `json:"kill_rate_reachable"`
	KillLive     float64 `json:"kill_rate_live"`
	WallMS       float64 `json:"wall_ms_total"`
	KilledWallMS float64 `json:"wall_ms_mean_killed"`
	CleanWallMS  float64 `json:"wall_ms_mean_clean"`
	ToolMS       float64 `json:"tool_ms_mean_clean"`
	ToolSamples  int     `json:"tool_samples"`
	Launches     int     `json:"launches_per_clean_run"`
}

// HasTool distinguishes "no tool figure was derivable" from "the tool figure
// came out at or below zero". Collapsing the two would hide the more
// interesting case: a rung whose own cost is smaller than the process floor is
// a rung that IS process startup, and that is a result, not a missing value.
func (s RungSummary) HasTool() bool { return s.ToolSamples > 0 }

func summarize(run *Run, rungs []rung) []RungSummary {
	// Fill in the tool-cost figure now that the floors are known. It is stored
	// on the cell rather than only on the summary so results.json can be
	// re-aggregated without re-deriving the correction.
	for i := range run.Cells {
		c := &run.Cells[i]
		f, ok := run.Floors[c.Impl]
		if !ok || c.Launches == 0 || c.Outcome == outcomeKilled || c.Outcome == outcomeError {
			continue
		}
		c.ToolMS = c.WallMS - float64(c.Launches)*f.LaunchMS
		c.ToolMeasured = true
	}

	var out []RungSummary
	for _, r := range rungs {
		out = append(out, aggregate(run, r, ""))
	}
	// Per-corner rows follow the totals. The deliverable is eventually per
	// port direction, and a port's claim is capped by its weaker end, so a
	// combined number that hides one corner being worse is the wrong shape.
	for _, impl := range run.Config.Impls {
		for _, r := range rungs {
			s := aggregate(run, r, impl)
			if s.Live+s.Equivalent+s.Errors+s.Unclassified+s.Capped > 0 {
				out = append(out, s)
			}
		}
	}
	return out
}

func aggregate(run *Run, r rung, impl string) RungSummary {
	s := RungSummary{Rung: r.ID, Label: r.Label, Inputs: r.Inputs, Impl: impl}
	var killedWall, cleanWall, toolWall float64
	var killedN, cleanN, toolN int

	for _, c := range run.Cells {
		if c.Rung != r.ID {
			continue
		}
		if impl != "" && c.Impl != impl {
			continue
		}
		s.WallMS += c.WallMS
		switch c.Outcome {
		case outcomeKilled:
			s.Killed++
			s.Live++
			killedWall += c.WallMS
			killedN++
		case outcomeSurvived:
			s.Survived++
			s.Live++
		case outcomeUnreached:
			s.Unreached++
			s.Live++
		case outcomeEquivalent:
			s.Equivalent++
		case outcomeUnclassified:
			s.Unclassified++
		case outcomeError:
			s.Errors++
		case outcomeCapped:
			// Not run, not timed, not in any denominator.
			s.Capped++
			continue
		}
		if c.Outcome != outcomeKilled && c.Outcome != outcomeError {
			cleanWall += c.WallMS
			cleanN++
			if c.ToolMeasured {
				toolWall += c.ToolMS
				toolN++
				s.Launches = c.Launches
			}
		}
	}
	if d := s.Killed + s.Survived; d > 0 {
		s.KillReach = float64(s.Killed) / float64(d)
	}
	if s.Live > 0 {
		s.KillLive = float64(s.Killed) / float64(s.Live)
	}
	if killedN > 0 {
		s.KilledWallMS = killedWall / float64(killedN)
	}
	if cleanN > 0 {
		s.CleanWallMS = cleanWall / float64(cleanN)
	}
	s.ToolSamples = toolN
	if toolN > 0 {
		s.ToolMS = toolWall / float64(toolN)
	}
	return s
}

// warnings names the numbers a reader should not accept at face value.
//
// A rung that kills nothing and a rung that kills everything are both
// suspicious, and for the same underlying reason: neither one discriminates. A
// 0% rung may be misconfigured, pointed at the wrong tree, or reading a verdict
// this tool misparses. A 100% rung may be genuinely strong, or the mutant
// catalogue may be too easy for it -- which is a statement about the
// catalogue, not a licence for the rung.
func warnings(run *Run, rungs []rung) []string {
	var w []string
	for _, s := range run.Summary {
		if s.Impl != "" {
			continue
		}
		switch {
		case s.Live > 0 && s.Killed == 0:
			w = append(w, fmt.Sprintf(
				"%s killed NOTHING (0/%d live mutants). A rung that never fires has not been shown to be able to fire; "+
					"check it against a known-bad canary before reading this row as evidence about the mutants.", s.Rung, s.Live))
		case s.Live > 0 && s.Killed == s.Live:
			w = append(w, fmt.Sprintf(
				"%s killed EVERYTHING (%d/%d). Worth reading as a statement about the catalogue as much as the rung: "+
					"a mutant set this rung finds trivial cannot distinguish it from a stronger one.", s.Rung, s.Killed, s.Live))
		}
		if s.Equivalent > 0 {
			w = append(w, fmt.Sprintf(
				"%s: %d mutant(s) changed no observable behaviour and were excluded from the denominator. "+
					"They should be given a witness or dropped from the catalogue -- left in, they make every rung look weaker.", s.Rung, s.Equivalent))
		}
		if s.Unclassified > 0 {
			w = append(w, fmt.Sprintf(
				"%s: %d survival(s) were never probed (-probe=%s), so it is not known whether they are rung weaknesses, "+
					"input gaps, or non-defects. These are not evidence about the rung.", s.Rung, s.Unclassified, run.Config.ProbeMode))
		}
		if s.Errors > 0 {
			w = append(w, fmt.Sprintf("%s: %d cell(s) errored. Those are missing measurements, not passes.", s.Rung, s.Errors))
		}
		if s.Capped > 0 {
			w = append(w, fmt.Sprintf(
				"%s: %d cell(s) are capped -- the corner has no verifier this rung drives. They are in no denominator; "+
					"the row measures only the corners that reach the rung.", s.Rung, s.Capped))
		}
		if s.Unreached > 0 && s.Inputs == "tracegen" {
			w = append(w, fmt.Sprintf(
				"%s: %d mutant(s) recorded as unreached on SAMPLED evidence. probe ran %d trace(s) where this rung ran %d; "+
					"unreached-by-tracegen is weaker evidence than unreached-by-corpus, which is exact.",
				s.Rung, s.Unreached, run.Config.ProbeTraces, run.Config.R1Traces))
		}
	}
	if len(run.Floors) == 0 && len(run.Cells) > 0 {
		w = append(w, "no launch floor was measured, so the wall column mixes rung cost with process-startup cost and the corners are not comparable.")
	}
	for _, s := range run.Summary {
		if s.Impl == "" || !s.HasTool() || s.ToolMS > 0 {
			continue
		}
		w = append(w, fmt.Sprintf(
			"%s on %s: the process floor (%d launches x measured floor) accounts for the whole clean wall time -- "+
				"the corrected figure is %.1fs, at or below zero. This rung's seconds on this corner are process startup, "+
				"not rung work, so comparing its wall time against another corner's compares the two languages' launch costs. "+
				"Compare launch counts instead.",
			s.Rung, s.Impl, s.Launches, s.ToolMS/1000))
	}
	return w
}

func renderReport(run *Run, rungs []rung) string {
	var b strings.Builder
	line := strings.Repeat("=", 78)

	fmt.Fprintf(&b, "CALIBRATION -- per-rung mutation kill table\n%s\n", line)
	fmt.Fprintf(&b, "corners   %s\n", strings.Join(run.Config.Impls, ", "))
	fmt.Fprintf(&b, "rungs     %s\n", strings.Join(run.Config.Rungs, ", "))
	fmt.Fprintf(&b, "R1        %d traces x %d steps, seed %d\n", run.Config.R1Traces, run.Config.R1Steps, run.Config.R1Seed)
	fmt.Fprintf(&b, "R2        %d rounds x %d setup requests, seed %d\n", run.Config.R2Rounds, run.Config.R2Setup, run.Config.R2Seed)
	fmt.Fprintf(&b, "probe     %s (%d traces x %d steps, plus the corpus and each mutant's witness)\n",
		run.Config.ProbeMode, run.Config.ProbeTraces, run.Config.ProbeSteps)
	fmt.Fprintf(&b, "window    %s .. %s\n", run.StartedAt, run.FinishedAt)

	fmt.Fprintf(&b, "\n%s\nKILL TABLE\n%s\n", line, line)
	fmt.Fprintf(&b, "%-14s %6s %7s %9s %10s %6s %7s %7s %8s\n",
		"rung", "live", "killed", "survived", "unreached", "equiv", "kill%", "kill%", "wall")
	fmt.Fprintf(&b, "%-14s %6s %7s %9s %10s %6s %7s %7s %8s\n",
		"", "", "", "", "", "", "reach", "live", "")
	for _, s := range run.Summary {
		if s.Impl != "" {
			continue
		}
		fmt.Fprintf(&b, "%-14s %6d %7d %9d %10d %6d %6.0f%% %6.0f%% %7.0fs\n",
			s.Rung+" "+s.Label, s.Live, s.Killed, s.Survived, s.Unreached, s.Equivalent,
			100*s.KillReach, 100*s.KillLive, s.WallMS/1000)
	}
	fmt.Fprint(&b, "\n  live       mutants that change observable behaviour. Equivalent ones are excluded\n")
	fmt.Fprint(&b, "             from every denominator: no rung can kill them, so counting them would\n")
	fmt.Fprint(&b, "             understate every rung for a reason that is not about the rung.\n")
	fmt.Fprint(&b, "  survived   the rung's own inputs DO elicit the difference and it passed anyway.\n")
	fmt.Fprint(&b, "             A gap in the rung.\n")
	fmt.Fprint(&b, "  unreached  live, but nothing in this rung's input source elicits the difference.\n")
	fmt.Fprint(&b, "             A gap in the inputs. F009 is the worked example. For a proof rung the\n")
	fmt.Fprint(&b, "             input source is the contract: unreached means the verifier reads none\n")
	fmt.Fprint(&b, "             of the files the mutant edits.\n")
	fmt.Fprint(&b, "  capped     the corner has no verifier for this rung. Not a measurement; shown\n")
	fmt.Fprint(&b, "             per mutant and per corner, in no denominator.\n")
	fmt.Fprint(&b, "  kill%reach killed / (killed + survived) -- the rung's oracle.\n")
	fmt.Fprint(&b, "  kill%live  killed / every live mutant -- the rung as configured, inputs included.\n")

	if len(run.Config.Impls) > 1 {
		fmt.Fprintf(&b, "\n%s\nBY CORNER\n%s\n", line, line)
		fmt.Fprintf(&b, "%-8s %-14s %6s %7s %9s %10s %6s %8s\n",
			"corner", "rung", "live", "killed", "survived", "unreached", "equiv", "wall")
		for _, s := range run.Summary {
			if s.Impl == "" {
				continue
			}
			fmt.Fprintf(&b, "%-8s %-14s %6d %7d %9d %10d %6d %7.0fs\n",
				s.Impl, s.Rung+" "+s.Label, s.Live, s.Killed, s.Survived, s.Unreached, s.Equivalent, s.WallMS/1000)
		}
	}

	b.WriteString(renderCost(run))
	b.WriteString(renderMutants(run, rungs))
	b.WriteString(renderProbes(run))

	fmt.Fprintf(&b, "\n%s\nREAD WITH CARE\n%s\n", line, line)
	if len(run.Warnings) == 0 {
		fmt.Fprint(&b, "  nothing flagged.\n")
	}
	for i, w := range run.Warnings {
		fmt.Fprintf(&b, "%2d. %s\n", i+1, wrap(w, 74, "    "))
	}
	return b.String()
}

// renderCost is requirement 3's answer, stated rather than implied.
func renderCost(run *Run) string {
	var b strings.Builder
	line := strings.Repeat("=", 78)
	fmt.Fprintf(&b, "\n%s\nCOST -- and which cost it is\n%s\n", line, line)
	fmt.Fprint(&b, "Every rung relaunches the implementation: R0 once for the whole corpus, R1 once\n")
	fmt.Fprint(&b, "per trace, R2 once per property-round. A launch is a warm build check plus a\n")
	fmt.Fprint(&b, "process start plus a health wait, and that price differs by language. So raw\n")
	fmt.Fprint(&b, "seconds mix what the RUNG costs with what the LANGUAGE costs. Both are below.\n\n")

	if len(run.Floors) == 0 {
		fmt.Fprint(&b, "  no floor measured; only raw wall time is available.\n")
	} else {
		fmt.Fprintf(&b, "  %-8s %12s %14s %8s %16s\n", "corner", "floor/launch", "of which build", "samples", "spread")
		var impls []string
		for k := range run.Floors {
			impls = append(impls, k)
		}
		sort.Strings(impls)
		for _, k := range impls {
			f := run.Floors[k]
			fmt.Fprintf(&b, "  %-8s %9.0f ms %11.0f ms %8d %8.0f-%.0f ms\n",
				k, f.LaunchMS, f.BuildMS, f.Samples, f.MinMS, f.MaxMS)
		}
	}

	fmt.Fprintf(&b, "\n  %-8s %-14s %9s %11s %11s %11s\n",
		"corner", "rung", "launches", "wall killed", "wall clean", "tool clean")
	for _, s := range run.Summary {
		if s.Impl == "" {
			continue
		}
		launches := "-"
		if s.Launches > 0 {
			launches = fmt.Sprintf("%d", s.Launches)
		}
		tool := "-"
		if s.HasTool() {
			tool = fmt.Sprintf("%.1fs", s.ToolMS/1000)
			if s.ToolMS <= 0 {
				// Not hidden and not clamped. A non-positive figure means the
				// rung's own work costs no more than the launches it makes,
				// which is the single most useful thing this column can say.
				tool += " *"
			}
		}
		fmt.Fprintf(&b, "  %-8s %-14s %9s %10s %10s %10s\n",
			s.Impl, s.Rung+" "+s.Label, launches,
			secs(s.KilledWallMS), secs(s.CleanWallMS), tool)
	}
	fmt.Fprint(&b, "\n  wall killed  mean wall for a mutant this rung caught. NOT comparable with the\n")
	fmt.Fprint(&b, "               clean column, and not reliably smaller either. R1 stops at its\n")
	fmt.Fprint(&b, "               first mismatch -- then shrinks, and every shrink attempt is\n")
	fmt.Fprint(&b, "               another server launch, so at low trace counts a kill can cost\n")
	fmt.Fprint(&b, "               several times a clean pass. The launch count of a kill depends\n")
	fmt.Fprint(&b, "               on the defect, so it is not corrected and no tool figure is\n")
	fmt.Fprint(&b, "               derived from it.\n")
	fmt.Fprint(&b, "  wall clean   mean wall for a full pass -- the worst case, and what a sweep of\n")
	fmt.Fprint(&b, "               a correct implementation actually costs.\n")
	fmt.Fprint(&b, "  tool clean   wall clean minus launches x measured floor. The rung's own cost\n")
	fmt.Fprint(&b, "               with process startup removed. Reported only where the launch\n")
	fmt.Fprint(&b, "               count is known from the configuration -- never inferred.\n")
	fmt.Fprint(&b, "  *            the correction came out at or below zero. Not an error and not\n")
	fmt.Fprint(&b, "               hidden: it means this rung's own work costs no more than the\n")
	fmt.Fprint(&b, "               servers it launches, so at this configuration the rung IS process\n")
	fmt.Fprint(&b, "               startup. Compare rungs by their launch counts, not their seconds.\n")
	fmt.Fprint(&b, "  Mutant setup (materialise + warm the build cache) is charged to no rung; see\n")
	fmt.Fprint(&b, "  the setups block in results.json.\n")
	return b.String()
}

func secs(ms float64) string {
	if ms <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1fs", ms/1000)
}

func renderMutants(run *Run, rungs []rung) string {
	var b strings.Builder
	line := strings.Repeat("=", 78)
	fmt.Fprintf(&b, "\n%s\nPER MUTANT\n%s\n", line, line)

	byMutant := map[string]map[string]Cell{}
	var order []string
	for _, c := range run.Cells {
		if _, ok := byMutant[c.Mutant]; !ok {
			byMutant[c.Mutant] = map[string]Cell{}
			order = append(order, c.Mutant)
		}
		byMutant[c.Mutant][c.Rung] = c
	}

	fmt.Fprintf(&b, "%-34s", "mutant")
	for _, r := range rungs {
		fmt.Fprintf(&b, " %-11s", r.ID)
	}
	fmt.Fprint(&b, "\n")
	for _, name := range order {
		fmt.Fprintf(&b, "%-34s", name)
		for _, r := range rungs {
			c, ok := byMutant[name][r.ID]
			if !ok {
				fmt.Fprintf(&b, " %-11s", "-")
				continue
			}
			fmt.Fprintf(&b, " %-11s", mark(c.Outcome))
		}
		fmt.Fprint(&b, "\n")
	}
	return b.String()
}

func mark(outcome string) string {
	switch outcome {
	case outcomeKilled:
		return "kill"
	case outcomeSurvived:
		return "SURVIVED"
	case outcomeUnreached:
		return "unreached"
	case outcomeEquivalent:
		return "equivalent"
	case outcomeUnclassified:
		return "survived?"
	case outcomeCapped:
		return "capped"
	default:
		return "ERROR"
	}
}

func renderProbes(run *Run) string {
	if len(run.Probes) == 0 {
		return ""
	}
	var b strings.Builder
	line := strings.Repeat("=", 78)
	fmt.Fprintf(&b, "\n%s\nREACHABILITY -- why each survivor survived\n%s\n", line, line)
	for _, p := range run.Probes {
		verdict := "EQUIVALENT"
		if p.Live {
			verdict = "live"
		}
		fmt.Fprintf(&b, "  %-30s %s\n", p.Mutant, verdict)
		for _, t := range p.Traces {
			if t.Differs {
				fmt.Fprintf(&b, "      %-10s differs at request %d\n", t.Name, t.AtIndex)
			} else {
				fmt.Fprintf(&b, "      %-10s no difference in %d requests\n", t.Name, t.Compared)
			}
		}
		if p.Note != "" {
			fmt.Fprintf(&b, "      %s\n", wrap(p.Note, 66, "      "))
		}
	}
	return b.String()
}

func wrap(s string, width int, indent string) string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for _, w := range words {
		if cur == "" {
			cur = w
			continue
		}
		if len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n"+indent)
}
