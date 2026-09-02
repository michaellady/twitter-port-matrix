package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// errR4Failed is the exit-code half of a FAILED verdict. The verdict itself is
// the "R4 FAILED" line on stdout; calibrate reads that line and then requires
// the exit code to agree with it, so both have to be produced together. Same
// contract as tools/cmd/gobra.
var errR4Failed = errors.New("R4 FAILED")

// A status is what JBMC's own goal lines say about ONE obligation.
//
// There are five, not two, and the extra three are the whole reason this rung
// is delicate:
//
//	VERIFIED   own assertion goals exist and every one is SUCCESS
//	REFUTED    at least one own assertion goal is FAILURE
//	VACUOUS    JBMC reported NO own assertion goal, or the obligation's
//	           negation canary could not be refuted. Either way nothing
//	           reaches the claim and its SUCCESS means nothing (F013)
//	BLOCKED    a recorded JBMC 6.11.0 defect makes any answer an artefact of
//	           the defect rather than a fact about the port (F014)
//	UNDECIDED  the run timed out or JBMC produced nothing to read
//
// Only VERIFIED and REFUTED enter the kill accounting. BLOCKED and VACUOUS are
// in neither the numerator nor the denominator, exactly as F022 puts a mutant
// confined to the trusted shim in neither.
type status string

const (
	stVerified  status = "VERIFIED"
	stRefuted   status = "REFUTED"
	stVacuous   status = "VACUOUS"
	stBlocked   status = "BLOCKED"
	stUndecided status = "UNDECIDED"
)

// classifyOne reads one JBMC invocation. It never looks at an exit status and
// it never treats "no assertion goal at all" as a pass: JBMC answers
// VERIFICATION SUCCESSFUL when a --function name does not resolve to anything
// it can check, and that answer looks exactly like a proof.
func classifyOne(r runResult) status {
	switch {
	case r.TimedOut, r.ToolError != "":
		return stUndecided
	case r.OwnSuccess == 0 && r.OwnFailure == 0:
		return stVacuous
	case r.OwnFailure > 0:
		return stRefuted
	default:
		return stVerified
	}
}

// obOutcome is one decidable obligation's answer.
type obOutcome struct {
	Fn         string
	Status     status
	OwnSuccess int
	OwnFailure int
	Note       string
}

// canaryOutcome is one negation canary's answer. A canary that is NOT refuted
// names an obligation nothing reaches.
type canaryOutcome struct {
	Fn     string
	Guards string
	Status status
}

// report is the whole accounting for one tree, computed as a pure function of
// the runs so it can be unit-tested without a solver.
type report struct {
	Corner   string
	Obs      []obOutcome
	Canaries []canaryOutcome
	Blocked  []obligation
	Elapsed  time.Duration

	// Filled in by decide.
	Verdict    string // "PASSED" | "FAILED" | "UNDECIDED"
	Sentence   string
	Reasons    []string
	Verified   int
	Refuted    int
	Vacuous    int
	Undecided  int
	OwnGoals   int
	FailedGoal int
}

// decide turns the per-obligation answers into the one sentence calibrate
// reads.
//
// The order of the tests is load-bearing:
//
//  1. A refutation is a kill and needs exactly one witness, so it is decided
//     first and is not weakened by another obligation being undecidable.
//  2. A pass needs EVERY decidable obligation to have been decided, so a
//     vacuous or timed-out obligation makes the run UNDECIDED. Per GOAL.md and
//     tools/cmd/gobra, UNDECIDED prints no verdict and calibrate records an
//     error cell -- never a survival.
//  3. A pass also needs every claim to have been shown refutable in THIS tree.
//     F013's failure mode is a claim nothing reaches, and its only instrument
//     is the negation canary, so an unrefuted canary demotes the claim it
//     guards to VACUOUS and the run to UNDECIDED.
func (rep *report) decide() error {
	for _, o := range rep.Obs {
		rep.OwnGoals += o.OwnSuccess + o.OwnFailure
		rep.FailedGoal += o.OwnFailure
	}
	blockedNote := fmt.Sprintf("%d obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator", len(rep.Blocked))
	den := len(rep.Obs)

	// 1. A refutation is a kill and needs exactly one witness.
	var refuted []string
	for _, o := range rep.Obs {
		if o.Status == stRefuted {
			refuted = append(refuted, o.Fn)
		}
	}
	if len(refuted) > 0 {
		rep.Refuted = len(refuted)
		rep.Verified = den - rep.Refuted
		sort.Strings(refuted)
		rep.Verdict = "FAILED"
		rep.Sentence = fmt.Sprintf(
			"R4 FAILED: JBMC refuted %d of %d decidable obligation(s) (%d of %d own assertion goals FAILURE): %s; %s   [%s]",
			rep.Refuted, den, rep.FailedGoal, rep.OwnGoals, strings.Join(refuted, ", "), blockedNote, rep.Elapsed.Round(1e8))
		return errR4Failed
	}

	if den == 0 {
		rep.Verdict = "UNDECIDED"
		rep.Reasons = append(rep.Reasons, "no obligation on this corner is decidable by JBMC; every one carries a recorded tool limit")
		rep.Sentence = rep.undecidedSentence(0)
		return errors.New("R4 UNDECIDED: nothing decidable")
	}

	// 2. Demote every claim the canary sweep did not earn. An obligation whose
	//    negation ALSO verifies is not verified: that signature is produced by
	//    nothing but an unreachable claim (F013), and reading its SUCCESS as a
	//    proof is the deepest false green this project has recorded. An
	//    obligation no canary names is demoted for the same reason -- it has
	//    simply never been asked.
	//
	//    The demotion happens BEFORE the counts, so the summary line says
	//    VERIFIED for exactly the obligations the verdict claims.
	guarded := map[string]int{}
	blind := map[string][]string{}
	for _, c := range rep.Canaries {
		guarded[c.Guards]++
		if c.Status != stRefuted {
			blind[c.Guards] = append(blind[c.Guards], fmt.Sprintf(
				"%s guards %s and was NOT refuted (%s); under vacuity a claim and its negation both verify, so %s decides nothing (F013)",
				c.Fn, c.Guards, c.Status, c.Guards))
		}
	}
	for i := range rep.Obs {
		o := &rep.Obs[i]
		switch {
		case o.Status == stUndecided:
			rep.Reasons = append(rep.Reasons, o.Fn+": "+o.Note)
		case o.Status == stVacuous:
			rep.Reasons = append(rep.Reasons,
				o.Fn+": JBMC reported no assertion goal of its own; nothing reaches the claim, so its SUCCESS is vacuous (F013)")
		case guarded[o.Fn] == 0:
			o.Status = stVacuous
			rep.Reasons = append(rep.Reasons,
				o.Fn+": no negation canary names this obligation, so its VERIFIED has not been shown refutable (F013)")
		case len(blind[o.Fn]) > 0:
			o.Status = stVacuous
			rep.Reasons = append(rep.Reasons, blind[o.Fn]...)
		}
	}

	// 3. Count what survived the demotion.
	for _, o := range rep.Obs {
		switch o.Status {
		case stVerified:
			rep.Verified++
		case stVacuous:
			rep.Vacuous++
		case stUndecided:
			rep.Undecided++
		}
	}

	if rep.Verified != den {
		rep.Verdict = "UNDECIDED"
		rep.Sentence = rep.undecidedSentence(rep.Vacuous + rep.Undecided)
		return errors.New("R4 UNDECIDED: " + rep.Reasons[0])
	}

	rep.Verdict = "PASSED"
	rep.Sentence = fmt.Sprintf(
		"R4 PASSED: JBMC verified %d of %d decidable obligation(s) (%d of %d own assertion goals FAILURE), every one refutable in this tree; %s   [%s]",
		rep.Verified, den, rep.FailedGoal, rep.OwnGoals, blockedNote, rep.Elapsed.Round(1e8))
	return nil
}

// undecidedSentence deliberately does NOT start with "R4 PASSED" or
// "R4 FAILED". calibrate counts lines by those prefixes, and an UNDECIDED run
// must produce neither, so the cell is recorded as an error rather than as a
// survival.
func (rep *report) undecidedSentence(n int) string {
	return fmt.Sprintf("R4 UNDECIDED: %d of %d decidable obligation(s) could not be read (%s); nothing was decided about this tree   [%s]",
		n, len(rep.Obs), rep.Reasons[0], rep.Elapsed.Round(1e8))
}
