package main

import "testing"

// The negation logic is the whole sweep. Get the shape wrong and every clause
// still comes back REFUTABLE -- which is the answer that looks like success,
// so nothing downstream would notice. These cases pin the three shapes and, in
// particular, that an implication is canaried on its antecedent rather than on
// the whole clause.
func TestNegate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{{
		name: "plain assertion is negated whole",
		in:   "s.AbsUsers() == old(s.AbsUsers())",
		want: "!(s.AbsUsers() == old(s.AbsUsers()))",
	}, {
		// !(A ==> B) is A && !B, which is false exactly when A is
		// unsatisfiable -- so it would score a vacuous clause as live.
		name: "implication is canaried on its antecedent",
		in:   "(u.Handle in old(s.AbsUsers())) ==> s.AbsUsers() == old(s.AbsUsers())",
		want: "!((u.Handle in old(s.AbsUsers())))",
	}, {
		name: "quantified implication asks whether the range is ever non-empty",
		in:   "forall k int :: 0 <= k && k < old(s.AbsLogLen()) ==> s.AbsLogAt(k) == old(s.AbsLogAt(k))",
		want: "forall k int :: !(0 <= k && k < old(s.AbsLogLen()))",
	}, {
		name: "an implication whose consequent is quantified splits at the outer arrow",
		in:   "cursor > 0 ==> forall a int :: 0 <= a && a < len(out) ==> out[a].ID < cursor",
		want: "!(cursor > 0)",
	}, {
		// The arrow here is inside the quantifier body, not a top-level
		// implication of the clause; splitting on it would negate the wrong
		// thing entirely.
		name: "an arrow inside a nested quantifier is not the top-level one",
		in:   "ok ==> forall i, j int :: 0 <= i && i < j ==> xs[i].ID < xs[j].ID",
		want: "!(ok)",
	}, {
		name: "a disjunction has no antecedent to isolate",
		in:   "s.AbsLogLen() == old(s.AbsLogLen()) || s.AbsLogLen() == old(s.AbsLogLen()) + 1",
		want: "!(s.AbsLogLen() == old(s.AbsLogLen()) || s.AbsLogLen() == old(s.AbsLogLen()) + 1)",
	}, {
		name: "an arrow inside brackets is not top level",
		in:   "result == (a ==> b)",
		want: "!(result == (a ==> b))",
	}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why := negate(c.in)
			if got != c.want {
				t.Errorf("negate(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
			if why == "" {
				t.Error("negate returned no explanation of what verifying would mean")
			}
		})
	}
}

func TestBlockClauses(t *testing.T) {
	block := []string{
		"// @ requires acc(s.LockP())",
		"// @ ensures acc(s.LockP())",
		"// @ ensures !(u.Handle in old(s.AbsUsers())) ==>",
		"// @            s.AbsUsers() == old(s.AbsUsers()) union set[string]{u.Handle}",
		"// @ ensures s.AbsLogLen() == old(s.AbsLogLen())",
	}
	got := blockClauses(block, 0)
	if len(got) != 3 {
		t.Fatalf("got %d clauses, want 3: %+v", len(got), got)
	}
	want := "!(u.Handle in old(s.AbsUsers())) ==> s.AbsUsers() == old(s.AbsUsers()) union set[string]{u.Handle}"
	if got[1].Text != want {
		t.Errorf("continuation not joined:\n got %q\nwant %q", got[1].Text, want)
	}
	// The joined clause must still report the line span it came from, or the
	// canary would blank out the wrong lines.
	if got[1].StartLine != 3 || got[1].EndLine != 4 {
		t.Errorf("span = %d-%d, want 3-4", got[1].StartLine, got[1].EndLine)
	}
}

// A checkpoint key built from (file, text) collides: the same framing clause is
// written verbatim on many members. Resuming a sweep then reuses one member's
// verdict for another member's clause, which is how a run came to report two
// timed-out HomeTimeline clauses as refutable.
func TestKeyDistinguishesMembersWithIdenticalClauseText(t *testing.T) {
	same := "s.AbsLogLen() == old(s.AbsLogLen())"
	a := clause{File: "internal/store/memstore.go", Member: "(*MemStore).PutUser", Text: same}
	b := clause{File: "internal/store/memstore.go", Member: "(*MemStore).HomeTimeline", Text: same}
	if key(a) == key(b) {
		t.Fatalf("key collides across members: %q", key(a))
	}
	c := clause{File: "internal/store/memstore.go", Member: "(*MemStore).PutUser", Text: same}
	if key(a) != key(c) {
		t.Errorf("key is not stable for the same clause: %q vs %q", key(a), key(c))
	}
}

// The real contract inventory must contain such a collision, or the test above
// is guarding against nothing. This reads the shipped sources rather than a
// fixture so that it keeps meaning something as the contracts change.
func TestRealClausesContainRepeatedText(t *testing.T) {
	all, err := allClauses("../../../impls/go")
	if err != nil {
		t.Skipf("implementation not readable from here: %v", err)
	}
	byFileText := map[string]map[string]bool{}
	for _, c := range all {
		k := c.File + "\x00" + c.Text
		if byFileText[k] == nil {
			byFileText[k] = map[string]bool{}
		}
		byFileText[k][c.Member] = true
	}
	worst := 0
	for _, members := range byFileText {
		if len(members) > worst {
			worst = len(members)
		}
	}
	if worst < 2 {
		t.Fatal("no clause text is shared across members; the collision guard is now vacuous")
	}
	t.Logf("most-repeated clause text appears on %d distinct members", worst)
}

// Gobra reports a --packageTimeout with "0 error(s)" and never says "timeout".
// Reading the error count without this check scores a hung proof as verified.
func TestGobraTimedOutRecognisesGobrasOwnWording(t *testing.T) {
	raw := `INFO viper.gobra.Gobra - Verifying package /x/internal/store - store [06:21:29]
ERROR viper.gobra.Gobra - The verification of package /x/internal/store - store got terminated after 600 seconds
ERROR viper.gobra.Gobra - The verification of member /x/internal/store - store.isMonotoneLog([]dom.Tweet) did not terminate
INFO viper.gobra.Gobra - Gobra has found 0 error(s)
INFO viper.gobra.Gobra - The verification of 1 members timed out`
	if !gobraTimedOut(raw) {
		t.Fatal("a timed-out package that reports 0 error(s) was not recognised as a timeout")
	}
	clean := `INFO viper.gobra.Gobra - Verifying package /x/internal/store - store [06:21:29]
INFO viper.gobra.Gobra - Gobra found no errors
INFO viper.gobra.Gobra - Gobra has found 0 error(s)`
	if gobraTimedOut(clean) {
		t.Fatal("a clean run was scored as a timeout")
	}
}
