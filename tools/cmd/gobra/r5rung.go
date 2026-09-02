package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cmdR5Verify is R5 as a `calibrate` rung: run Gobra over one tree and decide
// whether the REFINEMENT obligations are what broke.
//
// R4 asks "does the package verify". R5 asks a narrower question, and the
// difference is the whole point of having both rows: a mutant that breaks some
// functional postcondition is killed by the proof rung, but only a mutant that
// breaks a clause carrying an S_obs refinement obligation is killed by the
// refinement rung. Crediting R5 with every R4 kill would make the two rows
// identical by construction and the second one worthless.
//
// The join works because Gobra reports a failing postcondition AT THE
// POSTCONDITION'S OWN LINE:
//
//	internal/clock/clock.go:26:9 Postcondition might not hold.
//
// where line 26 is the `// @ ensures l != nil` itself. So an error line inside
// a clause's span identifies the clause exactly. The clause is then matched
// against spec/refinement/clause-sites.json on (file, member, text) -- the same
// key `gobra r5` joins on -- to find which R5 obligation, if any, it carries.
//
// A clause is not the only thing that can fail, and the first run of this rung
// got that wrong. Two memstore mutants failed at a LOOP INVARIANT inside
// (*MemStore).HomeTimeline:
//
//	internal/store/memstore.go:580 Loop invariant might not be established.
//
// which sits in no `ensures` span at all, so both cells came back UNDECIDED.
// Those invariants are not incidental -- one of them is commented "R5 (no
// fabrication)" in the source -- they are how HomeTimeline's refinement
// postconditions are proved. So attribution is by clause AND by member: an
// error anywhere inside a member whose contract carries R5 sites means that
// member's proof did not complete, and the refinement clauses on it are no
// longer discharged.
//
// That is the same standard R4 already applies. A proof rung's kill is "this
// tree can no longer be verified", not "the postcondition is provably false",
// and holding R5 to a stricter standard than R4 would make the two rows
// incomparable -- which is the one thing the kill table must not do.
//
// THE SPANS ARE READ FROM THE TREE UNDER TEST, never from impls/go. A mutant
// inserts and deletes lines, so a span taken from the pristine source lands on
// the wrong clause in the mutant and the attribution is silently wrong.
func cmdR5Verify(args []string) error {
	fs := flag.NewFlagSet("r5verify", flag.ContinueOnError)
	impl := fs.String("impl", "impls/go", "the Go implementation directory; with -registry, a registry entry name instead")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry (go, or go@<id> from `mutate apply`)")
	sitesPath := fs.String("sites", "spec/refinement/clause-sites.json", "the R5 clause sites to attribute failures to")
	budget := fs.Duration("budget", 20*time.Minute, "time budget; a run that exceeds it is UNDECIDED, never a pass")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}

	// The clause spans and the R5 site set both come from the tree under test.
	spans, err := r5Spans(implDir, *sitesPath)
	if err != nil {
		return err
	}
	if len(spans.sites) == 0 {
		return fmt.Errorf("%s carries none of the %d R5 clause sites in %s; nothing to attribute a failure to",
			implDir, spans.declared, *sitesPath)
	}

	ws, err := newWorkspace(implDir)
	if err != nil {
		return err
	}
	defer ws.close()

	res, err := runGobra(ws, verifiedPackages, "", *budget)
	if errors.Is(err, errTimeout) {
		fmt.Printf("R5 UNDECIDED: Gobra exceeded its %s budget; nothing was decided about this tree\n", *budget)
		if res != nil {
			fmt.Println(tailLines(res.Raw, 6))
		}
		return err
	}
	if err != nil {
		return err
	}
	if res.Total < 0 {
		return fmt.Errorf("gobra printed no `Gobra has found N error(s)` total; no verdict:\n%s", tailLines(res.Raw, 12))
	}

	fmt.Printf("R5 sites: %d of %d clause(s) in %s carry a Gobra postcondition; %d site(s) located in this tree\n",
		spans.withSites, spans.declared, filepath.Base(*sitesPath), len(spans.sites))
	fmt.Printf("Gobra has found %d error(s)   [%s]\n", res.Total, res.Elapsed.Round(1e8))

	a := spans.attribute(res.Errors)
	for _, l := range append(append([]attribution{}, a.onClause...), append(a.inMember, a.offR5...)...) {
		fmt.Printf("  %-46s %s\n", l.where, l.what)
	}
	for _, e := range a.unlocated {
		fmt.Printf("  %-46s %s\n", e.File+":"+strconv.Itoa(e.Line),
			"error inside no member of any contract file: "+e.Message)
	}

	switch {
	case len(a.onClause)+len(a.inMember) > 0:
		fmt.Println(r5RungVerdict(true, len(a.onClause), len(a.inMember), len(a.offR5), res.Total, res.Elapsed))
		return errR4Failed
	case res.Total == 0:
		fmt.Println(r5RungVerdict(false, 0, 0, 0, 0, res.Elapsed))
		return nil
	case len(a.unlocated) > 0:
		// The one case that stays undecided. An error this tool cannot place
		// in any member says nothing either way about the refinement
		// obligations, and guessing would be the whole point of the rung
		// thrown away.
		fmt.Printf("R5 UNDECIDED: %d error(s) could not be placed in any member, so it is not known\n"+
			"              whether a refinement obligation is among them\n", len(a.unlocated))
		return errR5Undecided
	default:
		fmt.Println(r5RungVerdict(false, 0, 0, len(a.offR5), res.Total, res.Elapsed))
		return nil
	}
}

// errR5Undecided is neither a kill nor a survival. calibrate records it as an
// error cell: a missing measurement.
var errR5Undecided = errors.New("R5 UNDECIDED")

// r5RungVerdict separates the two ways a refinement obligation stops being
// discharged, because they are not equally direct evidence and the record
// should say which one it saw.
func r5RungVerdict(failed bool, onClause, inMember, off, total int, elapsed time.Duration) string {
	if failed {
		return fmt.Sprintf("R5 FAILED: %d of %d failing obligation(s) hit a refinement clause (%d on the clause itself, %d elsewhere in its member)   [%s]",
			onClause+inMember, total, onClause, inMember, elapsed.Round(1e8))
	}
	if off > 0 {
		return fmt.Sprintf("R5 PASSED: %d failing obligation(s), none in a member carrying a refinement clause   [%s]",
			off, elapsed.Round(1e8))
	}
	return fmt.Sprintf("R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [%s]",
		elapsed.Round(1e8))
}

// --- the join ----------------------------------------------------------

type r5Site struct {
	clause        // the parsed clause, carrying its span in THIS tree
	N      []int  // the R5 obligation numbers it carries
	Member string // repeated for convenience in messages
}

type r5Index struct {
	declared  int // clauses in clause-sites.json
	withSites int // of those, the ones that name a Gobra postcondition
	sites     map[string]r5Site
	all       []clause // every clause in the tree, for span lookup
	// members maps "<file>\x00<member>" to the R5 clause numbers its contract
	// carries, and spans maps a file to every member's line range in THIS
	// tree. Together they place an error that is not on a clause at all.
	members map[string][]int
	spans   map[string]map[string][2]int
}

type attribution struct {
	where string
	what  string
}

// attributions is one run's errors, sorted into the four answers that matter.
type attributions struct {
	onClause  []attribution // on an R5 clause's own line
	inMember  []attribution // elsewhere in a member whose contract carries R5 clauses
	offR5     []attribution // in a member carrying no R5 clause
	unlocated []gobraError  // in no member of any contract file
}

// r5Spans parses the tree's clauses and joins them against the recorded R5
// sites on (file, member, text) -- exactly the key `gobra r5` uses.
func r5Spans(implDir, sitesPath string) (*r5Index, error) {
	var sites struct {
		Clauses map[string]struct {
			Sites []struct {
				File   string `json:"file"`
				Member string `json:"member"`
				Text   string `json:"text"`
			} `json:"sites"`
		} `json:"clauses"`
	}
	if err := readJSON(sitesPath, &sites); err != nil {
		return nil, err
	}
	all, err := allClauses(implDir)
	if err != nil {
		return nil, err
	}
	byKey := map[string]clause{}
	for _, c := range all {
		byKey[c.File+"\x00"+c.Member+"\x00"+c.Text] = c
	}

	idx := &r5Index{
		sites:   map[string]r5Site{},
		all:     all,
		members: map[string][]int{},
		spans:   map[string]map[string][2]int{},
	}
	// Member spans come from the tree under test. A mutant inserts and
	// deletes lines, so a span taken from impls/go lands on the wrong member
	// here and the attribution would be confidently wrong.
	for _, f := range contractFiles {
		sp, err := memberSpans(filepath.Join(implDir, f))
		if err != nil {
			return nil, err
		}
		idx.spans[f] = sp
	}
	for n, cl := range sites.Clauses {
		idx.declared++
		if len(cl.Sites) == 0 {
			continue
		}
		idx.withSites++
		num, _ := strconv.Atoi(n)
		for _, s := range cl.Sites {
			key := s.File + "\x00" + s.Member + "\x00" + s.Text
			c, ok := byKey[key]
			if !ok {
				// The site is recorded but not present in this tree. Left
				// silent it would turn a real R5 kill into "no R5 clause
				// failed", so it is reported rather than dropped.
				fmt.Printf("  note: R5 clause %d site not found in this tree: %s %s\n", num, s.File, s.Member)
				continue
			}
			site := idx.sites[key]
			site.clause, site.Member = c, c.Member
			site.N = append(site.N, num)
			sort.Ints(site.N)
			idx.sites[key] = site
			mk := c.File + "\x00" + c.Member
			if !containsInt(idx.members[mk], num) {
				idx.members[mk] = append(idx.members[mk], num)
				sort.Ints(idx.members[mk])
			}
		}
	}
	return idx, nil
}

// attribute assigns each Gobra error to a clause if it sits on one, and
// otherwise to the member whose span contains it.
func (x *r5Index) attribute(errs []gobraError) attributions {
	var a attributions
	for _, e := range errs {
		if c, ok := x.clauseAt(e); ok {
			where := fmt.Sprintf("%s:%d %s", c.File, e.Line, c.Member)
			if s, ok := x.sites[c.File+"\x00"+c.Member+"\x00"+c.Text]; ok {
				a.onClause = append(a.onClause, attribution{where,
					fmt.Sprintf("R5 clause %s FAILED: %s", joinInts(s.N), trunc(c.Text, 70))})
				continue
			}
			// A non-R5 clause on a member that carries R5 clauses: Gobra
			// reports one failing postcondition per member, so those clauses
			// were not established either.
			if ns, ok := x.members[c.File+"\x00"+c.Member]; ok {
				a.inMember = append(a.inMember, attribution{where,
					fmt.Sprintf("not an R5 clause, but on a member carrying R5 clause %s: %s", joinInts(ns), trunc(c.Text, 50))})
				continue
			}
			a.offR5 = append(a.offR5, attribution{where, "not an R5 clause: " + trunc(c.Text, 70)})
			continue
		}
		file, member, ok := x.memberAt(e)
		if !ok {
			a.unlocated = append(a.unlocated, e)
			continue
		}
		where := fmt.Sprintf("%s:%d %s", file, e.Line, member)
		if ns, ok := x.members[file+"\x00"+member]; ok {
			a.inMember = append(a.inMember, attribution{where,
				fmt.Sprintf("proof of a member carrying R5 clause %s did not complete: %s", joinInts(ns), trunc(e.Message, 46))})
			continue
		}
		a.offR5 = append(a.offR5, attribution{where, "member carries no R5 clause: " + trunc(e.Message, 46)})
	}
	return a
}

// memberAt finds the member whose span contains the error, for errors that sit
// on no clause: a loop invariant, an assertion, a precondition at a call site.
func (x *r5Index) memberAt(e gobraError) (file, member string, ok bool) {
	base := filepath.Base(e.File)
	for f, byMember := range x.spans {
		if filepath.Base(f) != base {
			continue
		}
		for m, span := range byMember {
			if e.Line >= span[0] && e.Line <= span[1] {
				return f, m, true
			}
		}
	}
	return "", "", false
}

// clauseAt finds the clause whose span contains the error's line. Gobra
// reports the file by basename in some positions and by path in others, so the
// comparison is on the basename.
func (x *r5Index) clauseAt(e gobraError) (clause, bool) {
	base := filepath.Base(e.File)
	for _, c := range x.all {
		if filepath.Base(c.File) != base {
			continue
		}
		if e.Line >= c.StartLine && e.Line <= c.EndLine {
			return c, true
		}
	}
	return clause{}, false
}

func containsInt(ns []int, n int) bool {
	for _, x := range ns {
		if x == n {
			return true
		}
	}
	return false
}

func joinInts(ns []int) string {
	var s []string
	for _, n := range ns {
		s = append(s, strconv.Itoa(n))
	}
	return strings.Join(s, ", ")
}
