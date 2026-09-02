package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// F040's reproduction, as a unit.
//
// A one-line method never produces a line equal to "}". The old memberSpans
// found a member's end by scanning forward for exactly that, so `Error` ran
// past its own body and claimed every line up to the next function's closing
// brace -- including the whole of ValidHandle. Gobra reported a failing loop
// invariant inside ValidHandle and the R5 rung named `Error`, a one-line method
// ninety lines away.
func TestMemberSpansEndsAOneLineMethodOnItsOwnLine(t *testing.T) {
	src := `package dom

type e struct{}

// Error satisfies the error interface.
//
// @ ensures true
func (e invalidHandleError) Error() string { return "invalid_handle" }

// ValidHandle accepts 1..32 bytes.
//
// @ ensures result ==> len(h) > 0
func ValidHandle(h string) (result bool) {
	b := []byte(h)
	// @ invariant 0 <= i && i <= len(b)
	for i := 0; i < len(b); i++ {
		_ = b[i]
	}
	return true
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "dom.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	spans, err := memberSpans(path)
	if err != nil {
		t.Fatal(err)
	}
	errSpan, ok := spans["(*invalidHandleError).Error"]
	if !ok {
		t.Fatalf("Error not found; got %v", keysOf(spans))
	}
	vhSpan, ok := spans["ValidHandle"]
	if !ok {
		t.Fatalf("ValidHandle not found; got %v", keysOf(spans))
	}
	// The invariant is at line 16 (1-based), inside ValidHandle.
	const invariantLine = 16
	if invariantLine >= errSpan[0] && invariantLine <= errSpan[1] {
		t.Errorf("line %d is claimed by Error, span [%d,%d]: a one-line method swallowed the next function (F040)",
			invariantLine, errSpan[0], errSpan[1])
	}
	if invariantLine < vhSpan[0] || invariantLine > vhSpan[1] {
		t.Errorf("line %d is NOT inside ValidHandle, span [%d,%d]", invariantLine, vhSpan[0], vhSpan[1])
	}
}

// The gate F040 said was needed and left unwritten because it landed red.
//
// No two members may claim the same line. An attribution engine whose spans
// overlap does not have a wrong answer so much as no answer: which member a
// line resolves to depends on which key the map yields first, and that is not a
// property an instrument deciding the R5 column may have.
func TestMemberSpansDoNotOverlap(t *testing.T) {
	root := repoRootForSpans(t)
	files := append([]string{}, contractFiles...)
	files = append(files, "internal/dom/dom.go")

	for _, f := range files {
		path := filepath.Join(root, "impls", "go", f)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s not present: %v", f, err)
		}
		spans, err := memberSpans(path)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		names := keysOf(spans)
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				a, b := spans[names[i]], spans[names[j]]
				if a[0] <= b[1] && b[0] <= a[1] {
					t.Errorf("%s: %s[%d,%d] and %s[%d,%d] both claim lines %d..%d",
						f, names[i], a[0], a[1], names[j], b[0], b[1],
						maxInt(a[0], b[0]), minInt(a[1], b[1]))
				}
			}
		}
	}
}

// Braces inside a comment or a string literal are not structure. A Gobra
// annotation carries braces of its own -- `// @ pred (e T) ErrorMem() { true }`
// -- and counting them would close a body inside its own doc block.
func TestMemberSpansIgnoresBracesInCommentsAndStrings(t *testing.T) {
	src := `package p

// @ pred (e T) ErrorMem() { true }
func f() string {
	s := "a { b"
	if s != "" {
		return "}"
	}
	return s
}

func g() int { return 1 }
`
	dir := t.TempDir()
	path := filepath.Join(dir, "p.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	spans, err := memberSpans(path)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := spans["f"]
	if !ok {
		t.Fatalf("f not found; got %v", keysOf(spans))
	}
	// f's body closes at line 10; the brace in the string at line 5 and the
	// "}" returned as a string at line 7 must not end it early.
	if f[1] != 10 {
		t.Errorf("f ends at %d, want 10: a brace in a comment or a string was counted as structure", f[1])
	}
	if g, ok := spans["g"]; !ok || g[1] != 12 {
		t.Errorf("g span = %v (found=%v), want it to end at 12 on its own line", g, ok)
	}
}

func keysOf(m map[string][2]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func repoRootForSpans(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("repository root not found")
	return ""
}
