package main

import "testing"

// Gobra reports a failing postcondition at the postcondition's own line, which
// is what makes per-clause attribution possible. But a clause is not the only
// thing that can fail: the first run of this rung scored two memstore mutants
// UNDECIDED because what failed was a LOOP INVARIANT inside a member whose
// contract carries R5 clauses. These are the four answers attribution can now
// give, and only the last one is undecided.
func TestAttribute(t *testing.T) {
	x := testIndex()

	// 1. on an R5 clause's own line.
	a := x.attribute([]gobraError{{File: "memstore.go", Line: 100, Message: "Postcondition might not hold."}})
	if len(a.onClause) != 1 || len(a.inMember) != 0 || len(a.offR5) != 0 || len(a.unlocated) != 0 {
		t.Fatalf("on-clause: %s", counts(a))
	}

	// 2. a non-R5 clause on a member that carries R5 clauses. Gobra reports
	// one failing postcondition per member, so those clauses are not
	// established either.
	a = x.attribute([]gobraError{{File: "memstore.go", Line: 101, Message: "Permission might not suffice."}})
	if len(a.onClause) != 0 || len(a.inMember) != 1 {
		t.Fatalf("sibling clause on an R5 member: %s", counts(a))
	}

	// 3. a loop invariant in the member body -- on no clause at all. This is
	// the case that used to be unlocated.
	a = x.attribute([]gobraError{{File: "memstore.go", Line: 140, Message: "Loop invariant might not be established."}})
	if len(a.inMember) != 1 || len(a.unlocated) != 0 {
		t.Fatalf("loop invariant inside an R5 member: %s", counts(a))
	}

	// 4. inside a member carrying no R5 clause: R4's kill, not R5's.
	a = x.attribute([]gobraError{{File: "memstore.go", Line: 205, Message: "Assert might fail."}})
	if len(a.offR5) != 1 || len(a.inMember) != 0 {
		t.Fatalf("member with no R5 clause: %s", counts(a))
	}

	// 5. in no member of any contract file: the one honestly undecidable case.
	a = x.attribute([]gobraError{{File: "memstore.go", Line: 9000, Message: "?"}})
	if len(a.unlocated) != 1 {
		t.Fatalf("unplaceable error: %s", counts(a))
	}
}

// R5 must not be credited with an R4 kill that never touched a refinement
// obligation, or the two rows are the same row.
func TestR5DoesNotInheritR4Kills(t *testing.T) {
	x := testIndex()
	a := x.attribute([]gobraError{{File: "memstore.go", Line: 205, Message: "Assert might fail."}})
	if len(a.onClause)+len(a.inMember) > 0 {
		t.Fatalf("an error in a member with no R5 clause was credited to R5: %s", counts(a))
	}
}

func TestR5RungVerdictLines(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{r5RungVerdict(true, 1, 0, 0, 1, 0),
			"R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (1 on the clause itself, 0 elsewhere in its member)   [0s]"},
		{r5RungVerdict(true, 0, 1, 0, 1, 0),
			"R5 FAILED: 1 of 1 failing obligation(s) hit a refinement clause (0 on the clause itself, 1 elsewhere in its member)   [0s]"},
		{r5RungVerdict(false, 0, 0, 2, 2, 0),
			"R5 PASSED: 2 failing obligation(s), none in a member carrying a refinement clause   [0s]"},
		{r5RungVerdict(false, 0, 0, 0, 0, 0),
			"R5 PASSED: Gobra has found 0 error(s); no refinement clause failed   [0s]"},
	} {
		if c.got != c.want {
			t.Errorf("got  %q\nwant %q", c.got, c.want)
		}
	}
}

// testIndex builds a two-member file: HomeTimeline carries an R5 clause at
// line 100, a framing clause at 101, and a body running to 150; PutTweet
// carries no R5 clause and runs 200-210.
func testIndex() *r5Index {
	const file = "internal/store/memstore.go"
	site := clause{File: file, Member: "(*MemStore).HomeTimeline", Text: "len(out) <= limit", StartLine: 100, EndLine: 100}
	framing := clause{File: file, Member: "(*MemStore).HomeTimeline", Text: "acc(s.LockP())", StartLine: 101, EndLine: 101}
	elsewhere := clause{File: file, Member: "(*MemStore).PutTweet", Text: "s.AbsLogLen() == old(s.AbsLogLen()) + 1", StartLine: 200, EndLine: 201}
	return &r5Index{
		sites: map[string]r5Site{
			file + "\x00" + site.Member + "\x00" + site.Text: {clause: site, Member: site.Member, N: []int{7}},
		},
		all:     []clause{site, framing, elsewhere},
		members: map[string][]int{file + "\x00(*MemStore).HomeTimeline": {7}},
		spans: map[string]map[string][2]int{file: {
			"(*MemStore).HomeTimeline": {99, 150},
			"(*MemStore).PutTweet":     {199, 210},
		}},
	}
}

func counts(a attributions) string {
	return fmtCounts(len(a.onClause), len(a.inMember), len(a.offR5), len(a.unlocated))
}

func fmtCounts(on, in, off, un int) string {
	return "onClause=" + itoa(on) + " inMember=" + itoa(in) + " offR5=" + itoa(off) + " unlocated=" + itoa(un)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
