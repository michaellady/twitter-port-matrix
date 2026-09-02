package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// --- reading JBMC's own output ------------------------------------------

const entryO3a = "twitterport.verification.Obligations.o3a_idsStrictlyIncrease:()V"

// A transcript trimmed from a real run. The three shapes that matter are all
// here: the obligation's OWN assertion goal, a check JBMC inserted on its own
// (which must not be read as the obligation's answer), and a goal owned by a
// library method (likewise).
const transcriptVerified = `
JBMC version 6.11.0
[java::twitterport.verification.Obligations.o3a_idsStrictlyIncrease:()V.assertion.1] line 107 assertion at file Obligations.kt: SUCCESS
[java::twitterport.verification.Obligations.o3a_idsStrictlyIncrease:()V.null-pointer-exception.1] line 105 dereference failure: SUCCESS
[java::java.util.ArrayList.add:(Ljava/lang/Object;)Z.array-index-out-of-bounds.1] line 20 index: SUCCESS

** 0 of 3 failed (1 iterations)
VERIFICATION SUCCESSFUL
`

const transcriptRefuted = `
[java::twitterport.verification.Obligations.o3a_idsStrictlyIncrease:()V.assertion.1] line 107 assertion at file Obligations.kt: FAILURE
[java::java.util.HashMap.hash:(Ljava/lang/Object;)I.assertion.1] line 9 assertion: FAILURE

** 2 of 3 failed (2 iterations)
VERIFICATION FAILED
`

// The F013 shape, and the reason this tool exists: JBMC answers
// VERIFICATION SUCCESSFUL having checked nothing the obligation wrote.
const transcriptVacuous = `
[java::java.util.ArrayList.add:(Ljava/lang/Object;)Z.array-index-out-of-bounds.1] line 20 index: SUCCESS

** 0 of 1 failed (1 iterations)
VERIFICATION SUCCESSFUL
`

func TestParseJBMCReadsOnlyTheObligationsOwnAssertions(t *testing.T) {
	ob := obligation{Class: "Obligations", Fn: "o3a_idsStrictlyIncrease", Sig: "()V"}

	r := parseJBMC(transcriptVerified, ob, entryO3a)
	if r.OwnSuccess != 1 || r.OwnFailure != 0 {
		t.Fatalf("own goals: got %d ok / %d failed, want 1/0 (the null-pointer check and the ArrayList goal are not the obligation's)", r.OwnSuccess, r.OwnFailure)
	}
	if r.TotalGoals != 3 {
		t.Fatalf("total goals: got %d, want 3", r.TotalGoals)
	}
	if r.VerdictLine != "VERIFICATION SUCCESSFUL" {
		t.Fatalf("verdict line: got %q", r.VerdictLine)
	}
	if got := classifyOne(r); got != stVerified {
		t.Fatalf("classify: got %s, want VERIFIED", got)
	}

	r = parseJBMC(transcriptRefuted, ob, entryO3a)
	if r.OwnFailure != 1 {
		t.Fatalf("own failures: got %d, want 1 (the java.util.HashMap goal is a library artefact)", r.OwnFailure)
	}
	if n := r.LibFailures["java.util.HashMap"]; n != 1 {
		t.Fatalf("library failures: got %v, want one in java.util.HashMap", r.LibFailures)
	}
	if got := classifyOne(r); got != stRefuted {
		t.Fatalf("classify: got %s, want REFUTED", got)
	}
}

// A run in which JBMC says SUCCESSFUL and the obligation contributed no goal
// of its own is VACUOUS, never VERIFIED. This is F013 in one assertion.
func TestSuccessfulWithNoOwnGoalIsVacuousNotVerified(t *testing.T) {
	ob := obligation{Class: "Obligations", Fn: "o3a_idsStrictlyIncrease", Sig: "()V"}
	r := parseJBMC(transcriptVacuous, ob, entryO3a)
	if r.VerdictLine != "VERIFICATION SUCCESSFUL" {
		t.Fatalf("precondition: JBMC must have said SUCCESSFUL, got %q", r.VerdictLine)
	}
	if got := classifyOne(r); got != stVacuous {
		t.Fatalf("classify: got %s, want VACUOUS -- an obligation nothing reaches is not a proof", got)
	}
}

func TestNoGoalLinesAtAllIsUndecided(t *testing.T) {
	ob := obligation{Class: "Obligations", Fn: "o3a_idsStrictlyIncrease", Sig: "()V"}
	r := parseJBMC("jbmc: cannot open classpath entry\n", ob, entryO3a)
	if got := classifyOne(r); got != stUndecided {
		t.Fatalf("classify: got %s, want UNDECIDED", got)
	}
	r = runResult{TimedOut: true, ToolError: "JBMC exceeded its 6m0s budget"}
	if got := classifyOne(r); got != stUndecided {
		t.Fatalf("timed out: got %s, want UNDECIDED", got)
	}
}

// --- the accounting ------------------------------------------------------

func verifiedOb(fn string) obOutcome {
	return obOutcome{Fn: fn, Status: stVerified, OwnSuccess: 2}
}

func refutedCanary(fn, guards string) canaryOutcome {
	return canaryOutcome{Fn: fn, Guards: guards, Status: stRefuted}
}

func blockedSet(n int) []obligation {
	out := make([]obligation, n)
	for i := range out {
		out[i] = obligation{Fn: "blocked", Blocked: equalsReason}
	}
	return out
}

func TestPassedQuotesItsOwnCountsAndExcludesTheBlocked(t *testing.T) {
	rep := &report{
		Obs:      []obOutcome{verifiedOb("o1a"), verifiedOb("o3a")},
		Canaries: []canaryOutcome{refutedCanary("c10", "o1a"), refutedCanary("c2", "o3a")},
		Blocked:  blockedSet(8),
		Elapsed:  90 * time.Second,
	}
	if err := rep.decide(); err != nil {
		t.Fatalf("decide: %v (reasons %v)", err, rep.Reasons)
	}
	if rep.Verdict != "PASSED" {
		t.Fatalf("verdict: got %s", rep.Verdict)
	}
	// The denominator is the DECIDABLE set. The eight blocked obligations are
	// reported and counted, and are in neither the numerator nor the
	// denominator -- F022's accounting, one corner over.
	if !strings.HasPrefix(rep.Sentence, "R4 PASSED: JBMC verified 2 of 2 decidable obligation(s)") {
		t.Fatalf("sentence does not quote the decidable counts: %s", rep.Sentence)
	}
	if !strings.Contains(rep.Sentence, "8 obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator") {
		t.Fatalf("sentence does not count the blocked separately: %s", rep.Sentence)
	}
	if strings.Contains(rep.Sentence, "of 10") {
		t.Fatalf("blocked obligations leaked into the denominator: %s", rep.Sentence)
	}
}

func TestOneRefutationFailsTheTreeAndNamesIt(t *testing.T) {
	rep := &report{
		Obs: []obOutcome{
			verifiedOb("o1a"),
			{Fn: "o3b", Status: stRefuted, OwnSuccess: 1, OwnFailure: 1},
		},
		Blocked: blockedSet(8),
		Elapsed: 42 * time.Second,
	}
	err := rep.decide()
	if !errors.Is(err, errR4Failed) {
		t.Fatalf("decide: got %v, want errR4Failed (the exit code must agree with the sentence)", err)
	}
	if !strings.HasPrefix(rep.Sentence, "R4 FAILED: JBMC refuted 1 of 2 decidable obligation(s) (1 of 4 own assertion goals FAILURE): o3b") {
		t.Fatalf("sentence: %s", rep.Sentence)
	}
}

// A refutation decides the tree even when another obligation could not be
// read. A kill needs one witness; a pass needs every obligation.
func TestRefutationBeatsAnUnreadableObligation(t *testing.T) {
	rep := &report{
		Obs: []obOutcome{
			{Fn: "o1a", Status: stUndecided, Note: "JBMC exceeded its 6m0s budget"},
			{Fn: "o3b", Status: stRefuted, OwnFailure: 1},
		},
		Elapsed: time.Second,
	}
	if err := rep.decide(); !errors.Is(err, errR4Failed) {
		t.Fatalf("decide: got %v, want errR4Failed", err)
	}
}

// The budget path. UNDECIDED must print NEITHER verdict sentence, because
// calibrate counts lines by those two prefixes and a run with neither is
// recorded as an error cell rather than as a survival.
func TestUndecidedPrintsNoVerdict(t *testing.T) {
	cases := []struct {
		name string
		rep  *report
		want string
	}{
		{
			name: "an obligation timed out",
			rep: &report{
				Obs:     []obOutcome{{Fn: "o1a", Status: stUndecided, Note: "JBMC exceeded its 6m0s budget"}},
				Blocked: blockedSet(8),
			},
			want: "JBMC exceeded its 6m0s budget",
		},
		{
			name: "an obligation went vacuous",
			rep: &report{
				Obs:      []obOutcome{{Fn: "o3a", Status: stVacuous}},
				Canaries: []canaryOutcome{refutedCanary("c2", "o3a")},
				Blocked:  blockedSet(8),
			},
			want: "vacuous",
		},
		{
			name: "a canary could not be refuted",
			rep: &report{
				Obs:      []obOutcome{verifiedOb("o3a")},
				Canaries: []canaryOutcome{{Fn: "c2", Guards: "o3a", Status: stVerified}},
				Blocked:  blockedSet(8),
			},
			want: "was NOT refuted",
		},
		{
			name: "a claim no canary names",
			rep: &report{
				Obs:     []obOutcome{verifiedOb("o3a")},
				Blocked: blockedSet(8),
			},
			want: "no negation canary names this obligation",
		},
		{
			name: "nothing on the corner is decidable",
			rep: &report{
				Blocked: blockedSet(15),
			},
			want: "no obligation on this corner is decidable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rep.decide()
			if err == nil {
				t.Fatal("decide returned nil; an undecided run must not succeed")
			}
			if errors.Is(err, errR4Failed) {
				t.Fatal("an undecided run must not report a FAILED verdict either")
			}
			if tc.rep.Verdict != "UNDECIDED" {
				t.Fatalf("verdict: got %s", tc.rep.Verdict)
			}
			for _, forbidden := range []string{"R4 PASSED", "R4 FAILED"} {
				for _, line := range strings.Split(tc.rep.Sentence, "\n") {
					if strings.HasPrefix(strings.TrimSpace(line), forbidden) {
						t.Fatalf("UNDECIDED emitted a %s line calibrate would read as a verdict: %s", forbidden, line)
					}
				}
			}
			if !strings.HasPrefix(tc.rep.Sentence, "R4 UNDECIDED:") {
				t.Fatalf("sentence: %s", tc.rep.Sentence)
			}
			if !strings.Contains(strings.Join(tc.rep.Reasons, " | "), tc.want) {
				t.Fatalf("reasons %v do not mention %q", tc.rep.Reasons, tc.want)
			}
		})
	}
}

// A canary over an obligation that is itself vacuous must not rescue it: the
// reasons list has to name the vacuity, not just the canary.
func TestVacuousObligationIsReportedEvenWhenItsCanaryWasRefuted(t *testing.T) {
	rep := &report{
		Obs:      []obOutcome{{Fn: "o3a", Status: stVacuous}},
		Canaries: []canaryOutcome{refutedCanary("c2", "o3a")},
	}
	_ = rep.decide()
	if rep.Vacuous != 1 {
		t.Fatalf("vacuous count: got %d, want 1", rep.Vacuous)
	}
	if !strings.Contains(rep.Reasons[0], "o3a") {
		t.Fatalf("reasons do not name the vacuous obligation: %v", rep.Reasons)
	}
}

// The F013 signature, end to end: JBMC's own goal lines say the obligation
// VERIFIED, and its negation ALSO verified. The obligation must be COUNTED as
// vacuous, not merely mentioned -- a summary line reading "VERIFIED 3" beside
// a footnote saying those three decide nothing is the false green in a
// different font.
func TestAClaimWhoseNegationAlsoVerifiesIsCountedVacuous(t *testing.T) {
	rep := &report{
		Obs: []obOutcome{
			verifiedOb("o3a"), // JBMC: 2 own goals, both SUCCESS
			verifiedOb("o1c"),
		},
		Canaries: []canaryOutcome{
			{Fn: "c2", Guards: "o3a", Status: stVerified}, // the negation verified too
			refutedCanary("c1", "o1c"),
		},
		Blocked: blockedSet(8),
	}
	if err := rep.decide(); err == nil {
		t.Fatal("decide returned nil for a tree with a vacuous claim")
	}
	if rep.Verified != 1 || rep.Vacuous != 1 {
		t.Fatalf("counts: got VERIFIED %d VACUOUS %d, want 1 and 1", rep.Verified, rep.Vacuous)
	}
	if rep.Obs[0].Status != stVacuous {
		t.Fatalf("o3a status: got %s, want VACUOUS", rep.Obs[0].Status)
	}
	if rep.Obs[1].Status != stVerified {
		t.Fatalf("o1c status: got %s, want VERIFIED -- only the unaudited claim is demoted", rep.Obs[1].Status)
	}
}

// --- the obligation table ------------------------------------------------

// Per F013 a claim no negation canary names has not been shown refutable, and
// this rung refuses to report a verdict over one. That refusal is only useful
// if the shipped table satisfies it.
func TestEveryDecidableObligationIsGuardedByANegationCanary(t *testing.T) {
	for name, c := range corners {
		if u := c.unguarded(); len(u) > 0 {
			t.Errorf("corner %s claims %v with no negation canary; add one or mark the obligation blocked", name, u)
		}
	}
}

// The counts F014 recorded, pinned so a change to the table is deliberate.
func TestKotlinDenominatorMatchesF014(t *testing.T) {
	c := kotlinCorner
	if got, want := len(c.decidable()), 7; got != want {
		t.Errorf("decidable obligations: got %d, want %d (F014: 7 VERIFIED, 0 REFUTED, 8 BLOCKED)", got, want)
	}
	if got, want := len(c.blocked()), 8; got != want {
		t.Errorf("blocked obligations: got %d, want %d", got, want)
	}
	// Every blocked obligation names one of the three recorded reasons. A
	// free-text reason is a reason nobody can audit.
	for _, o := range c.blocked() {
		switch o.Blocked {
		case equalsReason, getBytesReason, satReason:
		default:
			t.Errorf("%s is blocked for an unrecorded reason: %q", o.Fn, o.Blocked)
		}
	}
	// A canary is never in the denominator, whatever else it is.
	for _, o := range c.Obligations {
		if o.Canary && o.Decidable() {
			t.Errorf("%s is a canary and would be counted as an obligation", o.Fn)
		}
	}
}

func TestCornerForAcceptsAMutantEntryName(t *testing.T) {
	c, err := cornerFor("kotlin@tick-goes-backwards")
	if err != nil {
		t.Fatalf("cornerFor: %v", err)
	}
	if c.Name != "kotlin" {
		t.Fatalf("corner: got %s", c.Name)
	}
	if _, err := cornerFor("java"); err == nil {
		t.Fatal("expected an error for a corner with no obligation set; a row over an empty denominator is worse than no row")
	}
}
