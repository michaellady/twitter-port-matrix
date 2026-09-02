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
