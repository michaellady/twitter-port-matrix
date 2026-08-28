package mutants

import (
	"strings"
	"testing"
)

// The anchor assertions are the safety property of the whole rig, and a green
// `mutate verify` run does NOT exercise their failure paths: no shipped mutant
// currently has a missing or duplicated anchor, so the only way those branches
// get checked is here. They are checked because the failure they prevent is
// invisible -- a mutant that silently injects nothing reads in the kill table
// exactly like a defect every rung caught.

func testTree(files map[string]string) *Tree {
	t := &Tree{Root: "/testdata", Files: map[string][]byte{}, Modes: nil}
	for p, c := range files {
		t.Paths = append(t.Paths, p)
		t.Files[p] = []byte(c)
	}
	// Paths must be sorted for the hash to be stable; LoadTree does this.
	for i := 1; i < len(t.Paths); i++ {
		for j := i; j > 0 && t.Paths[j] < t.Paths[j-1]; j-- {
			t.Paths[j], t.Paths[j-1] = t.Paths[j-1], t.Paths[j]
		}
	}
	return t
}

func mutant(file, anchor, repl string) Mutant {
	return Mutant{
		ID: "m", Impl: "x", Family: "f", Description: "d",
		Edits: []Edit{{File: file, Anchor: Text{anchor}, Replacement: Text{repl}}},
	}
}

func TestApplyRejectsMissingAnchor(t *testing.T) {
	tree := testTree(map[string]string{"a.go": "package a\n\nfunc f() {}\n"})
	_, _, err := Apply(tree, mutant("a.go", "func g() {}", "func h() {}"))
	if err == nil {
		t.Fatal("a missing anchor must be an error; applying it would inject nothing")
	}
	if !strings.Contains(err.Error(), "anchor not found") {
		t.Fatalf("error must name the cause, got: %v", err)
	}
}

func TestApplyRejectsAmbiguousAnchor(t *testing.T) {
	tree := testTree(map[string]string{"a.go": "x := 1\ny := 2\nx := 1\n"})
	_, _, err := Apply(tree, mutant("a.go", "x := 1", "x := 2"))
	if err == nil {
		t.Fatal("an anchor matching two sites must be an error, not a silent first-match")
	}
	if !strings.Contains(err.Error(), "matches 2 sites") {
		t.Fatalf("error must report how many sites matched, got: %v", err)
	}
}

func TestApplyLeavesTheOriginalUntouched(t *testing.T) {
	const src = "limit := 50\n"
	tree := testTree(map[string]string{"a.go": src})
	out, edits, err := Apply(tree, mutant("a.go", "limit := 50", "limit := 51"))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := string(tree.Files["a.go"]); got != src {
		t.Fatalf("the source tree was mutated in place: %q", got)
	}
	if got := string(out.Files["a.go"]); got != "limit := 51\n" {
		t.Fatalf("mutation not applied: %q", got)
	}
	if len(edits) != 1 || edits[0].Line != 1 {
		t.Fatalf("edit site not reported correctly: %+v", edits)
	}
}

func TestApplyEditsSeeEarlierEdits(t *testing.T) {
	// Edits run in order against the running result. A later anchor is
	// matched against the text an earlier edit produced, so a mutant whose
	// second edit depends on the first is expressible -- and a second edit
	// whose anchor an earlier edit destroyed fails loudly instead of being
	// applied somewhere else.
	tree := testTree(map[string]string{"a.go": "one\ntwo\n"})
	m := Mutant{ID: "m", Impl: "x", Family: "f", Description: "d", Edits: []Edit{
		{File: "a.go", Anchor: Text{"one"}, Replacement: Text{"three"}},
		{File: "a.go", Anchor: Text{"three\ntwo"}, Replacement: Text{"four"}},
	}}
	out, _, err := Apply(tree, m)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := string(out.Files["a.go"]); got != "four\n" {
		t.Fatalf("sequential edits wrong: %q", got)
	}
}

func TestHashIsStableAndSensitive(t *testing.T) {
	a := testTree(map[string]string{"a.go": "x", "b.go": "y"})
	b := testTree(map[string]string{"b.go": "y", "a.go": "x"})
	if a.Hash() != b.Hash() {
		t.Fatal("the content address must not depend on file discovery order")
	}
	c := testTree(map[string]string{"a.go": "x", "b.go": "z"})
	if a.Hash() == c.Hash() {
		t.Fatal("differing content must produce a differing address")
	}
	// Path boundaries are hashed, so moving a byte from a name into a file
	// must not collide. Without length-prefixing these two would be equal.
	d := testTree(map[string]string{"ab": "c"})
	e := testTree(map[string]string{"a": "bc"})
	if d.Hash() == e.Hash() {
		t.Fatal("the address must separate path bytes from content bytes")
	}
}

func TestManifestRejectsANoOpMutant(t *testing.T) {
	m := &Manifest{
		Families: map[string]string{"f": "family"},
		Mutants: []Mutant{{
			ID: "m", Impl: "x", Family: "f", Description: "d",
			Edits: []Edit{{File: "a.go", Anchor: Text{"same"}, Replacement: Text{"same"}}},
		}},
	}
	if err := m.validate(); err == nil {
		t.Fatal("a replacement equal to its anchor is a no-op mutant and must be refused")
	}
}

func TestTextAcceptsBothManifestForms(t *testing.T) {
	var a, b Text
	if err := a.UnmarshalJSON([]byte(`"one\ntwo"`)); err != nil {
		t.Fatalf("string form: %v", err)
	}
	if err := b.UnmarshalJSON([]byte(`["one", "two"]`)); err != nil {
		t.Fatalf("array form: %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("the two manifest forms must denote the same text: %q vs %q", a.String(), b.String())
	}
}
