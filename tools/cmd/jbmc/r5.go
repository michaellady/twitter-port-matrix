package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cmdR5Verify is R5 as a `calibrate` rung on the Kotlin corner: run JBMC over
// the REFINEMENT clause obligations and decide whether a refinement clause is
// what broke.
//
// It is the JVM counterpart of `gobra r5verify`, and it answers the same
// narrower question R5 asks everywhere: R4 asks "does the tree still verify",
// R5 asks "did a clause carrying an S_obs refinement obligation stop holding".
// Crediting R5 with every R4 kill would make the two rows identical by
// construction and the second one worthless.
//
// # Why the join works here, measured rather than assumed
//
// Gobra reports a failing postcondition at the postcondition's own line, which
// is what makes per-clause attribution possible on the Go corner. JBMC's own
// goal lines carry the same information and one field more -- the entry point:
//
//	[java::twitterport.verification.Refinement.c13_logPrefixNeverRewritten:()V.assertion.2]
//	    line 123 assertion at file twitterport/verification/Refinement.kt line 123
//	    function java::...c13_logPrefixNeverRewritten:()V bytecode-index 62: SUCCESS
//
// So each goal names (entry point, assertion index, file, line), and one
// `assert` in Refinement.kt is one goal. That is strictly finer than what the
// Gobra join gets, and it is why this rung exists at all: the crux was never
// "can a rung be written", it was whether JBMC's output can be read per clause.
// It can. evidence/findings/F046 records the transcript.
//
// # Two things this rung must NOT do
//
//  1. Report a verdict over a clause JBMC cannot decide. The follows axis of
//    `abs` reduces to `HashSet.contains(Edge)` -> `Edge.equals` -> the F014
//    String.equals defect, and JBMC answers FAILURE for the claim AND FAILURE
//    for its negation. Either answer is an artefact of the defect, so clauses 7
//    and 9 are in NEITHER the numerator nor the denominator -- F022's
//    accounting, the same one `jbmc verify` applies to R4's eight blocked
//    obligations. The count is quoted in the verdict sentence rather than left
//    to be inferred.
//
//  2. Read a green as a proof. Every clause claimed here carries a negation
//    canary in the same tree; a canary JBMC does not refute demotes the clause
//    it names to VACUOUS and the run to UNDECIDED (F013).
//
// # The one honestly undecidable case
//
// A FAILURE goal that is not an own assertion of an R5 entry point -- a library
// goal, a null-pointer or array-bounds check inside the tree, an uncaught
// exception -- cannot be placed on any clause. It says nothing either way about
// the refinement obligations, and guessing would throw away the whole point of
// the rung, so the run prints R5 UNDECIDED and no verdict. calibrate records
// that as an error cell: a missing measurement, never a survival.
//
// An assert that IS inside an R5 entry point but is not itself a registered
// clause site is a different case and is NOT undecided: it is reported as
// "elsewhere in its member" and counts as a kill, exactly as `gobra r5verify`
// treats a loop invariant inside a member whose contract carries R5 clauses.
// See evidence/findings/F044 -- the comment on the R5 entry in
// tools/cmd/calibrate/rungs.go described this case as UNDECIDED, which the Go
// tool it describes has not done since its first run.
func cmdR5Verify(args []string) error {
	fs := flag.NewFlagSet("r5verify", flag.ContinueOnError)
	impl := fs.String("impl", "kotlin", "corner to verify; a registry entry name (kotlin, or kotlin@<id>) or, without -registry, a directory")
	cornerName := fs.String("corner", "", "R5 obligation set to use; defaults to the corner -impl names")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry and the directory is read from it")
	sitesPath := fs.String("sites", "spec/refinement/clause-sites-kotlin.json", "the R5 clause sites to attribute failures to")
	budget := fs.Duration("budget", 20*time.Minute, "time budget for the WHOLE run; exhausting it prints R5 UNDECIDED and no verdict, never a pass")
	obBudget := fs.Duration("ob-budget", 6*time.Minute, "time budget for one JBMC invocation")
	unwind := fs.Int("unwind", 30, "JBMC loop unwinding bound")
	strLen := fs.Int("string-length", 3, "JBMC --max-nondet-string-length")
	work := fs.String("work", filepath.Join(os.TempDir(), "jbmc-r5-rung"), "scratch directory; the extracted java.util is cached here between runs")
	jdk := fs.String("jdk", "", "JDK home containing lib/modules and bin/jimage (auto-detected when empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	start := time.Now()
	deadline := start.Add(*budget)

	which := *cornerName
	if which == "" {
		which = *impl
	}
	c, err := r5CornerFor(which)
	if err != nil {
		return err
	}
	implDir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}
	if u := c.unguarded(); len(u) > 0 {
		return fmt.Errorf("R5 corner %s claims %d clause obligation(s) no negation canary guards (%s); per F013 this rung will not report a verdict over them",
			c.Name, len(u), strings.Join(u, ", "))
	}

	// The clause spans come from the TREE UNDER TEST, never from impls/kotlin.
	// Mutants do not edit verification/, but reading the pristine copy would
	// make that a property of the mutant catalogue rather than of this tool,
	// and the Go rung learned the same lesson the expensive way.
	idx, err := r5KotlinIndex(implDir, c, *sitesPath)
	if err != nil {
		return err
	}
	if len(idx.sites) == 0 {
		return fmt.Errorf("%s carries none of the %d R5 clause sites in %s; nothing to attribute a failure to",
			implDir, idx.declared, *sitesPath)
	}

	if err := os.MkdirAll(*work, 0o755); err != nil {
		return err
	}
	tc, err := findToolchain(*jdk, *work)
	if err != nil {
		return err
	}

	fmt.Printf("jbmc: refinement rung (R5) over the %s corner\n", c.Name)
	fmt.Printf("        tree    %s\n", implDir)
	fmt.Printf("        jbmc    %s\n", firstLine(capture(tc.JBMC, "--version")))
	fmt.Printf("        kotlinc %s\n", firstLine(capture(tc.Compile, "-version")))
	fmt.Printf("        bounds  --unwind %d --max-nondet-string-length %d, budget %s (%s per obligation)\n",
		*unwind, *strLen, *budget, *obBudget)

	classes := filepath.Join(*work, "classes-"+sanitise(*impl))
	compileBudget := time.Until(deadline)
	if compileBudget > 5*time.Minute {
		compileBudget = 5 * time.Minute
	}
	cel, err := compileCorner(tc, c, implDir, classes, compileBudget)
	if err != nil {
		if errors.Is(err, errTimeout) {
			fmt.Println(r5Undecided("the corner did not compile inside the budget", time.Since(start)))
			return err
		}
		return err
	}
	fmt.Printf("        compile %s in %s\n", strings.Join(c.SrcDirs, " + "), cel.Round(1e8))
	cp := strings.Join([]string{classes, tc.Stdlib, tc.Models, tc.JavaUtil}, ":")

	fmt.Printf("R5 sites: %d of %d clause(s) in %s carry a Kotlin assertion; %d site(s) located in this tree\n",
		idx.withSites, idx.declared, filepath.Base(*sitesPath), len(idx.sites))
	fmt.Println(strings.Repeat("=", 100))

	rep := &r5Report{Corner: c.Name, Blocked: c.blocked()}

	fmt.Printf("%-40s %-10s %-24s %-9s %s\n", "clause obligation", "verdict", "own assertion goals", "wall", "JBMC's own line")
	fmt.Println(strings.Repeat("-", 100))
	for _, ob := range c.decidable() {
		left := time.Until(deadline)
		if left <= 0 {
			fmt.Println()
			fmt.Println(r5Undecided("the run budget was exhausted before "+ob.Fn, time.Since(start)))
			return fmt.Errorf("%w: budget exhausted at %s", errTimeout, ob.Fn)
		}
		r := runOne(tc, cp, c.Pkg, ob, *unwind, *strLen, minDur(left, *obBudget))
		goals := parseR5Goals(r.Raw, ob.Entry(c.Pkg))
		st := classifyOne(r)
		rep.Obs = append(rep.Obs, r5Outcome{Fn: ob.Fn, Status: st, OwnSuccess: r.OwnSuccess, OwnFailure: r.OwnFailure, Note: r.ToolError})
		if st != stUndecided {
			idx.attribute(ob.Fn, goals, &rep.Attr)
		}
		printRow(ob.Fn, string(st), r)
	}

	// The canaries, but only when nothing was refuted: a refutation already
	// decides the tree, and the canary sweep exists to protect a PASS.
	refuted := 0
	for _, o := range rep.Obs {
		if o.Status == stRefuted {
			refuted++
		}
	}
	if refuted == 0 {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Println("negation canaries (F013): each MUST be refuted, or the clause it names is vacuous")
		for _, o := range rep.Obs {
			if o.Status == stUndecided {
				continue
			}
			for _, k := range c.canariesFor(o.Fn) {
				left := time.Until(deadline)
				if left <= 0 {
					fmt.Println()
					fmt.Println(r5Undecided("the run budget was exhausted before canary "+k.Fn, time.Since(start)))
					return fmt.Errorf("%w: budget exhausted at %s", errTimeout, k.Fn)
				}
				r := runOne(tc, cp, c.Pkg, k, *unwind, *strLen, minDur(left, *obBudget))
				st := classifyOne(r)
				rep.Canaries = append(rep.Canaries, canaryOutcome{Fn: k.Fn, Guards: k.Guards, Status: st})
				label := string(st)
				if st == stRefuted {
					label = "refuted-ok"
				}
				printRow(k.Fn, label, r)
			}
		}
	} else {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Printf("negation canaries not run: %d clause obligation(s) were refuted, which decides the tree on its own\n", refuted)
	}

	rep.Elapsed = time.Since(start)
	rep.Clauses = idx.clausesOf(c)
	err = rep.decide()

	fmt.Println(strings.Repeat("=", 100))
	for _, l := range rep.Attr.lines() {
		fmt.Printf("  %-52s %s\n", l.where, l.what)
	}
	fmt.Printf("decidable %d clause obligation(s) covering R5 clause(s) %s\n", len(rep.Obs), joinIntsR5(rep.Clauses))
	fmt.Printf("blocked   %d   (recorded JBMC 6.11.0 limits; in neither the numerator nor the denominator)\n", len(rep.Blocked))
	for _, b := range c.blockedReasons() {
		fmt.Printf("    %s\n", b)
	}
	for _, r := range rep.Reasons {
		fmt.Printf("  ! %s\n", r)
	}
	fmt.Println()
	fmt.Println(rep.Sentence)

	if errors.Is(err, errR5Failed) {
		return errR5Failed
	}
	return err
}

// errR5Failed is the exit-code half of an R5 FAILED verdict, the same contract
// errR4Failed carries for R4: the sentence is on stdout and calibrate requires
// the exit code to agree with it.
var errR5Failed = errors.New("R5 FAILED")

// --- the report ---------------------------------------------------------

type r5Outcome struct {
	Fn         string
	Status     status
	OwnSuccess int
	OwnFailure int
	Note       string
}

type r5Report struct {
	Corner   string
	Obs      []r5Outcome
	Canaries []canaryOutcome
	Blocked  []obligation
	Clauses  []int
	Attr     r5Attributions
	Elapsed  time.Duration

	Verdict   string
	Sentence  string
	Reasons   []string
	Verified  int
	Refuted   int
	Vacuous   int
	Undecided int
}

// decide turns the per-clause answers into the one sentence calibrate reads.
//
// The order matches gobra r5verify's, because the two corners' R5 cells are
// compared against each other and a rung whose corners answer in a different
// order is a rung whose column means two things:
//
//  1. A refutation that lands on a refinement clause is a kill and needs one
//     witness.
//  2. A failure this tool cannot place on any clause is UNDECIDED -- the one
//     honestly undecidable case.
//  3. A pass needs every claimed clause to have been decided AND shown
//     refutable in this tree (F013).
func (rep *r5Report) decide() error {
	den := len(rep.Obs)
	blockedNote := fmt.Sprintf("%d clause obligation(s) blocked by a recorded JBMC 6.11.0 limit (F014), in no denominator", len(rep.Blocked))

	// 1. A refutation on a refinement clause.
	on, in := len(rep.Attr.OnClause), len(rep.Attr.InMember)
	if on+in > 0 {
		for _, o := range rep.Obs {
			if o.Status == stRefuted {
				rep.Refuted++
			}
		}
		rep.Verdict = "FAILED"
		rep.Sentence = fmt.Sprintf(
			"R5 FAILED: %d of %d failing obligation(s) hit a refinement clause (%d on the clause itself, %d elsewhere in its member); %s   [%s]",
			on+in, on+in+len(rep.Attr.OffR5)+len(rep.Attr.Unplaceable), on, in, blockedNote, rep.Elapsed.Round(1e8))
		return errR5Failed
	}

	// 2. The one honestly undecidable case.
	if n := len(rep.Attr.Unplaceable); n > 0 {
		rep.Verdict = "UNDECIDED"
		rep.Reasons = append(rep.Reasons, fmt.Sprintf(
			"%d failing goal(s) are not an own assertion of any R5 entry point, so it is not known whether a refinement obligation is among them", n))
		rep.Sentence = rep.undecidedSentence()
		return errors.New("R5 UNDECIDED: " + rep.Reasons[0])
	}

	if den == 0 {
		rep.Verdict = "UNDECIDED"
		rep.Reasons = append(rep.Reasons, "no R5 clause on this corner is decidable by JBMC; every one carries a recorded tool limit")
		rep.Sentence = rep.undecidedSentence()
		return errors.New("R5 UNDECIDED: nothing decidable")
	}

	// 3. Demote every claim the canary sweep did not earn.
	guarded := map[string]int{}
	blind := map[string][]string{}
	for _, k := range rep.Canaries {
		guarded[k.Guards]++
		if k.Status != stRefuted {
			blind[k.Guards] = append(blind[k.Guards], fmt.Sprintf(
				"%s guards %s and was NOT refuted (%s); under vacuity a claim and its negation both verify, so %s decides nothing (F013)",
				k.Fn, k.Guards, k.Status, k.Guards))
		}
	}
	for i := range rep.Obs {
		o := &rep.Obs[i]
		switch {
		case o.Status == stUndecided:
			rep.Reasons = append(rep.Reasons, o.Fn+": "+o.Note)
		case o.Status == stVacuous:
			rep.Reasons = append(rep.Reasons,
				o.Fn+": JBMC reported no assertion goal of its own; nothing reaches the clause, so its SUCCESS is vacuous (F013)")
		case guarded[o.Fn] == 0:
			o.Status = stVacuous
			rep.Reasons = append(rep.Reasons,
				o.Fn+": no negation canary names this clause, so its VERIFIED has not been shown refutable (F013)")
		case len(blind[o.Fn]) > 0:
			o.Status = stVacuous
			rep.Reasons = append(rep.Reasons, blind[o.Fn]...)
		}
	}
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
		rep.Sentence = rep.undecidedSentence()
		return errors.New("R5 UNDECIDED: " + rep.Reasons[0])
	}

	rep.Verdict = "PASSED"
	rep.Sentence = fmt.Sprintf(
		"R5 PASSED: JBMC verified %d of %d decidable clause obligation(s) covering R5 clause(s) %s, every one refutable in this tree; %s   [%s]",
		rep.Verified, den, joinIntsR5(rep.Clauses), blockedNote, rep.Elapsed.Round(1e8))
	return nil
}

// undecidedSentence deliberately begins with neither "R5 PASSED" nor
// "R5 FAILED": calibrate counts lines by those prefixes and an undecided run
// must produce neither, so the cell is an error rather than a survival.
func (rep *r5Report) undecidedSentence() string {
	reason := "nothing was decided"
	if len(rep.Reasons) > 0 {
		reason = rep.Reasons[0]
	}
	return fmt.Sprintf("R5 UNDECIDED: %s; nothing was decided about this tree   [%s]", reason, rep.Elapsed.Round(1e8))
}

func r5Undecided(reason string, elapsed time.Duration) string {
	return fmt.Sprintf("R5 UNDECIDED: %s; nothing was decided about this tree   [%s]", reason, elapsed.Round(1e8))
}

// --- goal parsing -------------------------------------------------------

// r5GoalLine matches one JBMC result line and keeps the LINE NUMBER, which the
// R4 parser discards. The line number is the join key: one `assert` in
// Refinement.kt is one goal, reported at that assert's own line.
//
// The kind alternation ends in `|[0-9]+` where R4's parser stops at `[a-z-]+`,
// and the difference is load-bearing rather than cosmetic. JBMC numbers two
// goal kinds without naming them, and one of them is
//
//	[java::...c13_logPrefixNeverRewritten:()V.1] line 118 no uncaught exception: FAILURE
//
// which is what an obligation that THROWS looks like. Under R4's pattern that
// line is not a goal at all, so a tree where every clause obligation died on an
// exception would present as "no failing goal anywhere" -- and this rung would
// have to decide between calling that a pass and calling it vacuous, from a
// transcript that had already said what happened. It is neither: it is a
// failure that sits on no clause, so it is read and reported as one.
var r5GoalLine = regexp.MustCompile(`^\[java::([^\]]+?)\.([a-z-]+(?:\.[0-9]+)?|[0-9]+)\](?: line ([0-9]+))? .*: (SUCCESS|FAILURE)$`)

type r5Goal struct {
	Owner  string
	Kind   string
	Line   int
	Status string
	Own    bool // an assertion goal belonging to the entry point under test
}

func parseR5Goals(out, entry string) []r5Goal {
	var goals []r5Goal
	ownPrefix := "java::" + entry
	for _, line := range strings.Split(out, "\n") {
		m := r5GoalLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[3])
		g := r5Goal{Owner: m[1], Kind: m[2], Line: n, Status: m[4]}
		g.Own = "java::"+g.Owner == ownPrefix && strings.HasPrefix(g.Kind, "assertion")
		goals = append(goals, g)
	}
	return goals
}

// --- the clause index ---------------------------------------------------

// r5KotlinSite is one registered `assert` carrying one or more R5 clauses.
type r5KotlinSite struct {
	Member    string
	Text      string
	StartLine int
	EndLine   int
	N         []int
}

type r5KotlinIdx struct {
	declared  int
	withSites int
	file      string
	sites     map[string]r5KotlinSite // keyed on member \x00 text
	byMember  map[string][]r5KotlinSite
	clauses   map[string][]int // member -> the R5 clauses its asserts carry
}

// r5Attributions is one run's failing goals, sorted into the four answers that
// matter -- the same four gobra r5verify sorts Gobra's errors into.
type r5Attributions struct {
	OnClause    []r5Attribution
	InMember    []r5Attribution
	OffR5       []r5Attribution
	Unplaceable []r5Attribution
}

type r5Attribution struct {
	where string
	what  string
}

func (a r5Attributions) lines() []r5Attribution {
	var out []r5Attribution
	out = append(out, a.OnClause...)
	out = append(out, a.InMember...)
	out = append(out, a.OffR5...)
	out = append(out, a.Unplaceable...)
	return out
}

// attribute assigns each FAILING goal of one entry point to a clause if it
// sits on one, to the member if it does not, and to nothing at all if it is
// not an own assertion goal.
func (x *r5KotlinIdx) attribute(member string, goals []r5Goal, a *r5Attributions) {
	for _, g := range goals {
		if g.Status != "FAILURE" {
			continue
		}
		if !g.Own {
			// A library goal, a null-pointer or array-bounds check, an
			// uncaught exception. It sits on no clause and says nothing
			// either way about the refinement obligations.
			a.Unplaceable = append(a.Unplaceable, r5Attribution{
				shortOwner(g.Owner) + " " + g.Kind,
				"FAILURE on no clause of any R5 entry point: not an own assertion goal"})
			continue
		}
		where := fmt.Sprintf("%s:%d %s", filepath.Base(x.file), g.Line, member)
		if s, ok := x.siteAt(member, g.Line); ok {
			a.OnClause = append(a.OnClause, r5Attribution{where,
				fmt.Sprintf("R5 clause %s FAILED: %s", joinIntsR5(s.N), truncR5(s.Text, 60))})
			continue
		}
		if ns, ok := x.clauses[member]; ok && len(ns) > 0 {
			a.InMember = append(a.InMember, r5Attribution{where,
				fmt.Sprintf("not a registered clause site, but in a member carrying R5 clause %s", joinIntsR5(ns))})
			continue
		}
		a.OffR5 = append(a.OffR5, r5Attribution{where, "member carries no R5 clause"})
	}
}

func (x *r5KotlinIdx) siteAt(member string, line int) (r5KotlinSite, bool) {
	for _, s := range x.byMember[member] {
		if line >= s.StartLine && line <= s.EndLine {
			return s, true
		}
	}
	return r5KotlinSite{}, false
}

// clausesOf lists every R5 clause number the corner's DECIDABLE obligations
// carry, so the verdict sentence can name them instead of a bare count.
func (x *r5KotlinIdx) clausesOf(c corner) []int {
	seen := map[int]bool{}
	for _, ob := range c.decidable() {
		for _, n := range x.clauses[ob.Fn] {
			seen[n] = true
		}
	}
	var out []int
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// r5KotlinIndex parses the obligation file FROM THE TREE UNDER TEST and joins
// its asserts against the recorded sites on (file, member, text) -- the same
// key spec/refinement/clause-sites.json uses for the Go corner.
func r5KotlinIndex(implDir string, c corner, sitesPath string) (*r5KotlinIdx, error) {
	var sites struct {
		Clauses map[string]struct {
			EvidenceKind string `json:"evidence_kind"`
			Sites        []struct {
				File   string `json:"file"`
				Member string `json:"member"`
				Text   string `json:"text"`
			} `json:"sites"`
		} `json:"clauses"`
	}
	if err := readJSONFile(sitesPath, &sites); err != nil {
		return nil, err
	}
	rel := c.R5File
	found, err := parseKotlinAsserts(filepath.Join(implDir, rel))
	if err != nil {
		return nil, err
	}
	idx := &r5KotlinIdx{
		file:     rel,
		sites:    map[string]r5KotlinSite{},
		byMember: map[string][]r5KotlinSite{},
		clauses:  map[string][]int{},
	}
	for n, cl := range sites.Clauses {
		idx.declared++
		if len(cl.Sites) == 0 {
			continue
		}
		idx.withSites++
		num, _ := strconv.Atoi(n)
		for _, s := range cl.Sites {
			key := s.Member + "\x00" + s.Text
			got, ok := found[key]
			if !ok {
				// Reported rather than dropped: left silent, a site recorded
				// but absent from this tree would turn a real R5 kill into
				// "no refinement clause failed".
				fmt.Printf("  note: R5 clause %d site not found in this tree: %s %s\n", num, s.File, s.Member)
				continue
			}
			site := idx.sites[key]
			site.Member, site.Text = s.Member, s.Text
			site.StartLine, site.EndLine = got[0], got[1]
			if !containsIntR5(site.N, num) {
				site.N = append(site.N, num)
				sort.Ints(site.N)
			}
			idx.sites[key] = site
			if !containsIntR5(idx.clauses[s.Member], num) {
				idx.clauses[s.Member] = append(idx.clauses[s.Member], num)
				sort.Ints(idx.clauses[s.Member])
			}
		}
	}
	for _, s := range idx.sites {
		idx.byMember[s.Member] = append(idx.byMember[s.Member], s)
	}
	return idx, nil
}

var (
	reKotlinFun    = regexp.MustCompile(`^\s*fun\s+([A-Za-z0-9_]+)\s*\(`)
	reKotlinAssert = regexp.MustCompile(`^\s*assert\(`)
)

// parseKotlinAsserts returns, for every `assert(...)` in the file, the
// [firstLine, lastLine] span of the statement, keyed on member \x00 text where
// text is the assert's argument with runs of whitespace collapsed.
//
// The span is a range rather than a line because JBMC reports a multi-line
// assert at its LAST line -- measured, not assumed: RefinementCanaries.k11's
// assert opens at one line and its goal comes back at the line the expression
// closes on. A single-line assert has StartLine == EndLine and the range is
// exactly the line.
func parseKotlinAsserts(path string) (map[string][2]int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	out := map[string][2]int{}
	member := ""
	for i := 0; i < len(lines); i++ {
		if m := reKotlinFun.FindStringSubmatch(lines[i]); m != nil {
			member = m[1]
			continue
		}
		if member == "" || !reKotlinAssert.MatchString(lines[i]) {
			continue
		}
		// Join continuation lines until the assert's parentheses balance.
		text, depth, start := "", 0, i
		for ; i < len(lines); i++ {
			text += " " + strings.TrimSpace(lines[i])
			depth += strings.Count(lines[i], "(") - strings.Count(lines[i], ")")
			if depth <= 0 {
				break
			}
		}
		inner := strings.TrimSpace(text)
		inner = strings.TrimPrefix(inner, "assert(")
		inner = strings.TrimSuffix(inner, ")")
		out[member+"\x00"+strings.Join(strings.Fields(inner), " ")] = [2]int{start + 1, i + 1}
	}
	return out, nil
}

// --- the R5 corner ------------------------------------------------------

// followsAxisReason is the measured reason clauses 7 and 9 are in neither the
// numerator nor the denominator on this corner.
//
// It is a DIFFERENT signature from R4's blocked obligations and worth stating
// separately: those are blocked because JBMC answers FAILURE for a true claim.
// Here JBMC answers FAILURE for the claim AND FAILURE for its negation, which
// is not vacuity -- that signature is both VERIFYING -- but plain
// nondeterminism, because `HashSet.contains` falls through to the broken
// intrinsic and returns an unconstrained boolean.
const followsAxisReason = "the follows axis of abs is undecidable: HashSet.contains(Edge) reduces to Edge.equals -> String.equals (F014), and JBMC answers FAILURE for the claim AND for its negation"

// kotlinR5Corner is the Kotlin corner's R5 obligation set. It is deliberately
// NOT the R4 set one directory over: Obligations.kt states functional
// properties of this implementation, Refinement.kt states clauses of
// spec/refinement/obligations.json, and a rung that read both would be R4
// wearing R5's name.
var kotlinR5Corner = corner{
	Name:    "kotlin",
	SrcDirs: []string{"src", "verification"},
	Pkg:     "twitterport.verification",
	R5File:  "verification/Refinement.kt",
	Obligations: []obligation{
		{Class: "Refinement", Fn: "c01_absInitIsTheEmptyState", Sig: "()V"},
		{Class: "Refinement", Fn: "c02_createUserAddsExactlyThatHandle", Sig: "()V"},
		{Class: "Refinement", Fn: "c07_addFollowAddsExactlyThatEdge", Sig: "()V", Blocked: followsAxisReason},
		{Class: "Refinement", Fn: "c09_removeFollowRemovesExactlyThatEdge", Sig: "()V", Blocked: followsAxisReason},
		{Class: "Refinement", Fn: "c11_appendAddsExactlyOneAtTheEnd", Sig: "()V"},
		{Class: "Refinement", Fn: "c13_logPrefixNeverRewritten", Sig: "()V"},
		{Class: "Refinement", Fn: "c36_tickAdvancesByExactlyOne", Sig: "()V"},

		{Class: "RefinementCanaries", Fn: "k01_absInitIsNotEmpty", Sig: "()V", Canary: true, Guards: "c01_absInitIsTheEmptyState"},
		{Class: "RefinementCanaries", Fn: "k02_createUserDoesNotAddThatHandle", Sig: "()V", Canary: true, Guards: "c02_createUserAddsExactlyThatHandle"},
		{Class: "RefinementCanaries", Fn: "k11_appendDoesNotAddOneAtTheEnd", Sig: "()V", Canary: true, Guards: "c11_appendAddsExactlyOneAtTheEnd"},
		{Class: "RefinementCanaries", Fn: "k13_logPrefixIsRewritten", Sig: "()V", Canary: true, Guards: "c13_logPrefixNeverRewritten"},
		{Class: "RefinementCanaries", Fn: "k36_tickDoesNotAdvanceByOne", Sig: "()V", Canary: true, Guards: "c36_tickAdvancesByExactlyOne"},
	},
	// Every R5 entry point reaches exactly one production file. That is
	// NARROWER than the R4 set's dom + store + service, which is the property
	// that keeps R4 and R5 from being the same row: a mutant in
	// service/Service.kt is fair game for R4 and unreached by R5.
	CoveredPaths: []string{"src/twitterport/store/Store.kt"},
}

var r5Corners = map[string]corner{kotlinR5Corner.Name: kotlinR5Corner}

func r5CornerFor(name string) (corner, error) {
	base := name
	if i := strings.IndexByte(base, '@'); i >= 0 {
		base = base[:i]
	}
	c, ok := r5Corners[base]
	if !ok {
		var names []string
		for k := range r5Corners {
			names = append(names, k)
		}
		sort.Strings(names)
		return corner{}, fmt.Errorf("no R5 clause obligation set for corner %q; this rung drives: %s", base, strings.Join(names, ", "))
	}
	return c, nil
}

// --- small helpers ------------------------------------------------------

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func containsIntR5(ns []int, n int) bool {
	for _, x := range ns {
		if x == n {
			return true
		}
	}
	return false
}

func joinIntsR5(ns []int) string {
	if len(ns) == 0 {
		return "(none)"
	}
	var s []string
	for _, n := range ns {
		s = append(s, strconv.Itoa(n))
	}
	return strings.Join(s, ", ")
}

func truncR5(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
