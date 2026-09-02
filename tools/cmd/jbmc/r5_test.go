package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The transcript is JBMC's own, copied verbatim from a run of
// `twitterport.verification.Refinement.c13_logPrefixNeverRewritten`. Everything
// this rung claims is read out of lines shaped like these, so the parser is
// tested against them rather than against a paraphrase.
const r5Transcript = `** Results:
[java::twitterport.store.Store.<init>:()V.null-pointer-exception.1] line 37 Null pointer check: SUCCESS
[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.1] line 122 assertion at file twitterport/verification/Refinement.kt line 122 function java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V bytecode-index 40: SUCCESS
[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.2] line 123 assertion at file twitterport/verification/Refinement.kt line 123 function java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V bytecode-index 62: FAILURE
[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.3] line 124 assertion at file twitterport/verification/Refinement.kt line 124 function java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V bytecode-index 84: SUCCESS
VERIFICATION FAILED`

const r5Entry = "twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V"

// The join key is the LINE NUMBER, which the R4 parser throws away. If this
// stops being read, every attribution below silently becomes a guess.
func TestR5GoalsCarryTheirLineNumber(t *testing.T) {
	goals := parseR5Goals(r5Transcript, r5Entry)
	if len(goals) != 4 {
		t.Fatalf("parsed %d goals, want 4: %+v", len(goals), goals)
	}
	own, lines := 0, map[int]string{}
	for _, g := range goals {
		if g.Own {
			own++
			lines[g.Line] = g.Status
		}
	}
	if own != 3 {
		t.Errorf("own assertion goals = %d, want 3 (the null-pointer check in Store is not one)", own)
	}
	for line, want := range map[int]string{122: "SUCCESS", 123: "FAILURE", 124: "SUCCESS"} {
		if lines[line] != want {
			t.Errorf("line %d: got %q, want %q", line, lines[line], want)
		}
	}
}

// testR5Index mirrors the real Refinement.kt shape: c13 carries clause 13 at
// lines 123-124 and a setup assert at 122 that is deliberately NOT a site;
// cXX carries no clause at all.
func testR5Index() *r5KotlinIdx {
	s13a := r5KotlinSite{Member: "c13_logPrefixNeverRewritten", Text: "s.absLogIdAt(0) == t0.id", StartLine: 123, EndLine: 123, N: []int{13}}
	s13b := r5KotlinSite{Member: "c13_logPrefixNeverRewritten", Text: "s.absLogCreatedAtAt(0) == t0.createdAt", StartLine: 124, EndLine: 124, N: []int{13}}
	return &r5KotlinIdx{
		file:     "verification/Refinement.kt",
		sites:    map[string]r5KotlinSite{"a": s13a, "b": s13b},
		byMember: map[string][]r5KotlinSite{"c13_logPrefixNeverRewritten": {s13a, s13b}},
		clauses:  map[string][]int{"c13_logPrefixNeverRewritten": {13}},
	}
}

// The four answers attribution can give, and only the last one is undecided.
// This is the Kotlin twin of gobra's TestAttribute, deliberately case for case.
func TestR5Attribute(t *testing.T) {
	x := testR5Index()

	// 1. on a registered clause site.
	var a r5Attributions
	x.attribute("c13_logPrefixNeverRewritten", []r5Goal{{Line: 123, Status: "FAILURE", Own: true}}, &a)
	if len(a.OnClause) != 1 || len(a.InMember) != 0 || len(a.OffR5) != 0 || len(a.Unplaceable) != 0 {
		t.Fatalf("on-clause: %s", r5Counts(a))
	}
	if !strings.Contains(a.OnClause[0].what, "R5 clause 13 FAILED") {
		t.Errorf("attribution does not name the clause: %q", a.OnClause[0].what)
	}

	// 2. an assert inside an R5 member that is not itself a site: the setup
	//    line. NOT undecided -- the member's proof did not complete, which is
	//    the same standard R4 applies.
	a = r5Attributions{}
	x.attribute("c13_logPrefixNeverRewritten", []r5Goal{{Line: 122, Status: "FAILURE", Own: true}}, &a)
	if len(a.InMember) != 1 || len(a.OnClause) != 0 || len(a.Unplaceable) != 0 {
		t.Fatalf("sibling assert on an R5 member: %s", r5Counts(a))
	}

	// 3. a member carrying no R5 clause: R4's kill, not R5's.
	a = r5Attributions{}
	x.attribute("cXX_notAClause", []r5Goal{{Line: 900, Status: "FAILURE", Own: true}}, &a)
	if len(a.OffR5) != 1 || len(a.InMember) != 0 {
		t.Fatalf("member with no R5 clause: %s", r5Counts(a))
	}

	// 4. a failing goal that is not an own assertion: a library goal, a
	//    null-pointer check, an uncaught exception. The one honestly
	//    undecidable case.
	a = r5Attributions{}
	x.attribute("c13_logPrefixNeverRewritten", []r5Goal{
		{Owner: "java.util.HashMap.hash:(Ljava/lang/Object;)I", Kind: "assertion.1", Status: "FAILURE"},
	}, &a)
	if len(a.Unplaceable) != 1 || len(a.OnClause)+len(a.InMember)+len(a.OffR5) != 0 {
		t.Fatalf("unplaceable goal: %s", r5Counts(a))
	}

	// A SUCCESS is never attributed anywhere.
	a = r5Attributions{}
	x.attribute("c13_logPrefixNeverRewritten", []r5Goal{{Line: 123, Status: "SUCCESS", Own: true}}, &a)
	if len(a.lines()) != 0 {
		t.Fatalf("a SUCCESS was attributed: %s", r5Counts(a))
	}
}

// R5 must not be credited with an R4 kill that never touched a refinement
// clause, or the two rows are the same row. On this corner the separation is
// structural -- r5verify never runs an R4 obligation -- but the attribution
// must not undo it if one is ever added to Refinement.kt.
func TestR5DoesNotInheritR4Kills(t *testing.T) {
	x := testR5Index()
	var a r5Attributions
	x.attribute("cXX_notAClause", []r5Goal{{Line: 900, Status: "FAILURE", Own: true}}, &a)
	if len(a.OnClause)+len(a.InMember) > 0 {
		t.Fatalf("a failure in a member with no R5 clause was credited to R5: %s", r5Counts(a))
	}
	rep := &r5Report{Obs: []r5Outcome{{Fn: "cXX_notAClause", Status: stRefuted}}, Attr: a}
	if err := rep.decide(); err == nil {
		t.Fatal("want an undecided/verdict error, got nil")
	}
	if strings.HasPrefix(rep.Sentence, "R5 FAILED") {
		t.Errorf("R5 claimed a kill it did not earn: %q", rep.Sentence)
	}
}

// The verdict sentence is what calibrate reads. A FAILED that does not start
// the line with "R5 FAILED", or an UNDECIDED that starts with either prefix,
// is scored as the wrong outcome for every mutant.
func TestR5VerdictSentences(t *testing.T) {
	// A kill on the clause itself.
	rep := &r5Report{
		Obs:  []r5Outcome{{Fn: "c36_tickAdvancesByExactlyOne", Status: stRefuted}},
		Attr: r5Attributions{OnClause: []r5Attribution{{"Refinement.kt:133 c36", "R5 clause 36 FAILED"}}},
	}
	if err := rep.decide(); err != errR5Failed {
		t.Fatalf("want errR5Failed, got %v", err)
	}
	if !strings.HasPrefix(rep.Sentence, "R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)") {
		t.Errorf("FAILED sentence: %q", rep.Sentence)
	}

	// The one undecidable case: a failure on no clause of any entry point.
	rep = &r5Report{
		Obs:      []r5Outcome{{Fn: "c11", Status: stVerified, OwnSuccess: 4}},
		Canaries: []canaryOutcome{{Fn: "k11", Guards: "c11", Status: stRefuted}},
		Attr:     r5Attributions{Unplaceable: []r5Attribution{{"java.util.HashMap assertion.1", "not an own assertion goal"}}},
	}
	if err := rep.decide(); err == nil {
		t.Fatal("want an undecided error, got nil")
	}
	assertNoVerdictPrefix(t, rep.Sentence)

	// A claim whose negation canary was not refuted is VACUOUS, and a run with
	// a vacuous claim decides nothing (F013).
	rep = &r5Report{
		Obs:      []r5Outcome{{Fn: "c11", Status: stVerified, OwnSuccess: 4}},
		Canaries: []canaryOutcome{{Fn: "k11", Guards: "c11", Status: stVerified}},
	}
	if err := rep.decide(); err == nil {
		t.Fatal("want an undecided error for an unrefuted canary, got nil")
	}
	assertNoVerdictPrefix(t, rep.Sentence)
	if !strings.Contains(strings.Join(rep.Reasons, " "), "was NOT refuted") {
		t.Errorf("the vacuity reason is not reported: %v", rep.Reasons)
	}

	// A claim no canary names at all is demoted for the same reason: it has
	// never been asked.
	rep = &r5Report{Obs: []r5Outcome{{Fn: "c11", Status: stVerified, OwnSuccess: 4}}}
	if err := rep.decide(); err == nil {
		t.Fatal("want an undecided error for an unguarded claim, got nil")
	}
	assertNoVerdictPrefix(t, rep.Sentence)

	// A clean tree.
	rep = &r5Report{
		Obs:      []r5Outcome{{Fn: "c11", Status: stVerified, OwnSuccess: 4}},
		Canaries: []canaryOutcome{{Fn: "k11", Guards: "c11", Status: stRefuted}},
		Blocked:  []obligation{{Fn: "c07"}, {Fn: "c09"}},
		Clauses:  []int{11},
		Elapsed:  time.Second,
	}
	if err := rep.decide(); err != nil {
		t.Fatalf("clean tree: %v", err)
	}
	if !strings.HasPrefix(rep.Sentence, "R5 PASSED: JBMC verified 1 of 1 decidable clause obligation(s) covering R5 clause(s) 11") {
		t.Errorf("PASSED sentence: %q", rep.Sentence)
	}
	// F022: the blocked clauses are quoted, and they are in no denominator.
	if !strings.Contains(rep.Sentence, "2 clause obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator") {
		t.Errorf("the blocked accounting is missing from the verdict: %q", rep.Sentence)
	}
	if strings.Contains(rep.Sentence, "1 of 3") {
		t.Errorf("a blocked clause entered the denominator: %q", rep.Sentence)
	}
}

func assertNoVerdictPrefix(t *testing.T, s string) {
	t.Helper()
	if strings.HasPrefix(s, "R5 PASSED") || strings.HasPrefix(s, "R5 FAILED") {
		t.Errorf("an undecided run printed a verdict prefix, which calibrate would score as an outcome: %q", s)
	}
	if !strings.HasPrefix(s, "R5 UNDECIDED") {
		t.Errorf("undecided sentence: %q", s)
	}
}

// A multi-line assert is reported by JBMC at the line its expression CLOSES
// on, not the line it opens on -- measured on RefinementCanaries.k11. So a
// site's span is a range, and a parser that recorded only the opening line
// would put the goal outside every span and report UNDECIDED for a real kill.
func TestParseKotlinAssertsSpansMultipleLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "X.kt")
	src := "object X {\n" + // 1
		"    fun a() {\n" + // 2
		"        assert(p == 1)\n" + // 3
		"    }\n" + // 4
		"    fun b() {\n" + // 5
		"        assert(!(p == 1 &&\n" + // 6
		"            q == 2))\n" + // 7
		"    }\n}\n" // 8
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseKotlinAsserts(path)
	if err != nil {
		t.Fatal(err)
	}
	if span, ok := got["a\x00p == 1"]; !ok || span != [2]int{3, 3} {
		t.Errorf("single-line assert: got %v ok=%v, want [3 3]", span, ok)
	}
	if span, ok := got["b\x00!(p == 1 && q == 2)"]; !ok || span != [2]int{6, 7} {
		t.Errorf("multi-line assert: got %v ok=%v, want [6 7]", span, ok)
	}
}

// Every site recorded in clause-sites-kotlin.json must resolve in the shipped
// tree. A site that does not resolve turns a real R5 kill into "no refinement
// clause failed", which is the quietest way this rung could be wrong.
func TestEveryKotlinR5SiteResolvesInTheTree(t *testing.T) {
	root := r5RepoRoot(t)
	sitesPath := filepath.Join(root, "spec", "refinement", "clause-sites-kotlin.json")
	var sites struct {
		Clauses map[string]struct {
			Sites []struct{ File, Member, Text string } `json:"sites"`
		} `json:"clauses"`
	}
	b, err := os.ReadFile(sitesPath)
	if err != nil {
		t.Skipf("clause-sites-kotlin.json not readable from here: %v", err)
	}
	if err := json.Unmarshal(b, &sites); err != nil {
		t.Fatal(err)
	}
	found, err := parseKotlinAsserts(filepath.Join(root, "impls", "kotlin", kotlinR5Corner.R5File))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for num, cl := range sites.Clauses {
		for _, s := range cl.Sites {
			if _, ok := found[s.Member+"\x00"+s.Text]; !ok {
				t.Errorf("clause %s site not found in the tree: %s  %q", num, s.Member, s.Text)
				continue
			}
			n++
		}
	}
	if n == 0 {
		t.Fatal("no site resolved at all")
	}
}

// The clause numbers are obligations.json's, not a second numbering. A number
// outside its range would make "R5 clause 11" mean two different sentences on
// the two corners, which is exactly what a cross-corner cell cannot survive.
func TestKotlinR5ClauseNumbersAreObligationsJSONs(t *testing.T) {
	root := r5RepoRoot(t)
	var ob struct {
		Clauses []struct {
			Clause string `json:"clause"`
		} `json:"clauses"`
	}
	b, err := os.ReadFile(filepath.Join(root, "spec", "refinement", "obligations.json"))
	if err != nil {
		t.Skipf("obligations.json not readable from here: %v", err)
	}
	if err := json.Unmarshal(b, &ob); err != nil {
		t.Fatal(err)
	}
	var sites struct {
		Clauses map[string]json.RawMessage `json:"clauses"`
	}
	b, err = os.ReadFile(filepath.Join(root, "spec", "refinement", "clause-sites-kotlin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &sites); err != nil {
		t.Fatal(err)
	}
	for k := range sites.Clauses {
		n, err := strconv.Atoi(k)
		if err != nil || n < 1 || n > len(ob.Clauses) {
			t.Errorf("clause key %q is not a 1-based index into obligations.json's %d clauses", k, len(ob.Clauses))
		}
	}
}

// F013, one rung over: this rung refuses to report a verdict over a clause no
// negation canary names, so the refusal must be reachable from the obligation
// table alone.
func TestEveryDecidableR5ClauseIsGuarded(t *testing.T) {
	if u := kotlinR5Corner.unguarded(); len(u) > 0 {
		t.Errorf("R5 claims %d clause obligation(s) no canary guards: %s", len(u), strings.Join(u, ", "))
	}
	// And the check itself has to be able to fail.
	broken := kotlinR5Corner
	broken.Obligations = append([]obligation{{Class: "Refinement", Fn: "c99_unguarded", Sig: "()V"}}, broken.Obligations...)
	if len(broken.unguarded()) != 1 {
		t.Error("the unguarded check does not notice an unguarded clause obligation")
	}
}

// R5's reach on this corner must be narrower than R4's, or the two rows are
// the same row. R4 reaches dom, store and service; R5 reaches store only.
func TestKotlinR5ReachIsNarrowerThanR4(t *testing.T) {
	if len(kotlinR5Corner.CoveredPaths) != 1 || kotlinR5Corner.CoveredPaths[0] != "src/twitterport/store/Store.kt" {
		t.Errorf("R5 reach = %v; the clause obligations call Store and nothing else", kotlinR5Corner.CoveredPaths)
	}
	for _, p := range kotlinCorner.CoveredPaths {
		if strings.HasPrefix(kotlinR5Corner.CoveredPaths[0], p) {
			return // R5's file lies inside one of R4's prefixes, as it must
		}
	}
	t.Errorf("R5's reach %v is not inside R4's %v", kotlinR5Corner.CoveredPaths, kotlinCorner.CoveredPaths)
}

func r5RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "spec")); err == nil {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("repository root not found from the test's working directory")
	return ""
}

func r5Counts(a r5Attributions) string {
	return "onClause=" + strconv.Itoa(len(a.OnClause)) +
		" inMember=" + strconv.Itoa(len(a.InMember)) +
		" offR5=" + strconv.Itoa(len(a.OffR5)) +
		" unplaceable=" + strconv.Itoa(len(a.Unplaceable))
}

// An obligation that THROWS is not a pass and is not a clause failure. JBMC
// reports it on a numerically-named goal that the R4 parser's pattern does not
// match at all; if this rung inherited that pattern, a tree where every clause
// obligation died on an exception would present as "nothing failed anywhere".
func TestUncaughtExceptionGoalIsReadAndIsUnplaceable(t *testing.T) {
	const out = `** Results:
[java::twitterport.verification.Refinement.c11_appendAddsExactlyOneAtTheEnd:()V.1] line 100 no uncaught exception: FAILURE
VERIFICATION FAILED`
	goals := parseR5Goals(out, "twitterport.verification.Refinement.c11_appendAddsExactlyOneAtTheEnd:()V")
	if len(goals) != 1 {
		t.Fatalf("the uncaught-exception goal was not read at all: %+v", goals)
	}
	if goals[0].Own {
		t.Error("an uncaught-exception goal was counted as an own assertion goal, which would attribute it to a clause")
	}
	x := testR5Index()
	var a r5Attributions
	x.attribute("c11_appendAddsExactlyOneAtTheEnd", goals, &a)
	if len(a.Unplaceable) != 1 || len(a.OnClause)+len(a.InMember) != 0 {
		t.Fatalf("uncaught exception: %s", r5Counts(a))
	}
	rep := &r5Report{Obs: []r5Outcome{{Fn: "c11_appendAddsExactlyOneAtTheEnd", Status: stVacuous}}, Attr: a}
	if err := rep.decide(); err == nil {
		t.Fatal("want an undecided error, got nil")
	}
	assertNoVerdictPrefix(t, rep.Sentence)
}
