package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real --jobs 1 transcript from a cold tree, trimmed. The vstd line is the
// one that must NOT be counted: it appears only when the build cache is cold,
// so folding it in would make the headline number depend on the cache.
const coldTranscript = `    Checking verus_builtin v0.0.0-2026-04-12-0118
verification results:: 1556 verified, 0 errors
    Checking domain v0.1.0 (/tmp/tree/crates/domain)
verification results:: 9 verified, 0 errors
    Checking store v0.1.0 (/tmp/tree/crates/store)
verification results:: 7 verified, 0 errors
    Checking clock v0.1.0 (/tmp/tree/crates/clock)
verification results:: 2 verified, 0 errors
    Checking ids v0.1.0 (/tmp/tree/crates/ids)
verification results:: 0 verified, 0 errors
    Checking service v0.1.0 (/tmp/tree/crates/service)
verification results:: 5 verified, 0 errors
    Checking server v0.1.0 (/tmp/tree/crates/server)
    Finished ` + "`dev`" + ` profile [unoptimized + debuginfo] target(s) in 1m 48s
`

var five = []string{"clock", "domain", "ids", "service", "store"}

func TestParseAttributesResultsToCrates(t *testing.T) {
	res := parseVerus(coldTranscript, five)
	if len(res.Reported) != 5 {
		t.Fatalf("reported %d crates, want 5: %+v", len(res.Reported), res.Reported)
	}
	if res.Verified != 23 || res.Errors != 0 {
		t.Fatalf("got %d verified, %d errors; want 23 and 0 (vstd's 1556 must not be counted)", res.Verified, res.Errors)
	}
	byName := map[string]crateResult{}
	for _, c := range res.Reported {
		byName[c.Crate] = c
	}
	// F016's decomposition depends on these being attributed correctly: it is
	// `ids` that contributes nothing, because its one postcondition is
	// external_body and therefore assumed rather than proved.
	if byName["ids"].Verified != 0 {
		t.Errorf("ids attributed %d verified, want 0", byName["ids"].Verified)
	}
	if byName["domain"].Verified != 9 {
		t.Errorf("domain attributed %d verified, want 9", byName["domain"].Verified)
	}
}

func TestPassVerdictLine(t *testing.T) {
	res := parseVerus(coldTranscript, five)
	res.Elapsed = 4100 * time.Millisecond
	line, killed, err := res.verdict()
	if err != nil {
		t.Fatal(err)
	}
	if killed {
		t.Error("a clean run was scored as a kill")
	}
	want := "R4 PASSED: verification results:: 23 verified, 0 errors over 5 of 5 verify-enabled crate(s)   [4.1s]"
	if line != want {
		t.Errorf("verdict\n got %q\nwant %q", line, want)
	}
}

// A crate that fails verification fails to COMPILE, so the crates downstream
// of it are never checked. The FAILED verdict is partial by construction and
// has to say so rather than quietly reporting a smaller "verified" count.
const failedTranscript = `    Checking domain v0.1.0 (/tmp/tree/crates/domain)
error: postcondition not satisfied
   --> crates/domain/src/lib.rs:136:17
verification results:: 8 verified, 1 errors
error: could not compile ` + "`domain`" + ` (lib) due to 2 previous errors
`

func TestFailVerdictLineIsPartialAndSaysSo(t *testing.T) {
	res := parseVerus(failedTranscript, five)
	res.Elapsed = 3500 * time.Millisecond
	line, killed, err := res.verdict()
	if err != nil {
		t.Fatal(err)
	}
	if !killed {
		t.Fatal("1 error was not scored as a kill")
	}
	want := "R4 FAILED: verification results:: 8 verified, 1 errors over 1 of 5 verify-enabled crate(s)   [3.5s]"
	if line != want {
		t.Errorf("verdict\n got %q\nwant %q", line, want)
	}
}

// THE CACHE TRAP, and the reason this test exists at all.
//
// GOAL.md: "Verus caches; touch the crate source or a run prints nothing and
// looks like a pass." A cached run prints `Finished` and exits 0 with no
// result line whatsoever. Zero errors out of zero lines must never be read as
// a pass -- that is a green over a tree nothing looked at.
func TestCachedRunIsNotAPass(t *testing.T) {
	cached := "    Finished `dev` profile [unoptimized + debuginfo] target(s) in 0.30s\n"
	res := parseVerus(cached, five)
	line, killed, err := res.verdict()
	if err == nil {
		t.Fatalf("a cached run produced a verdict %q; it must produce none", line)
	}
	if !errors.Is(err, errNoVerdict) {
		t.Errorf("wrong error kind: %v", err)
	}
	if killed || line != "" {
		t.Errorf("a cached run yielded line=%q killed=%v", line, killed)
	}
	if !strings.Contains(err.Error(), "clock") || !strings.Contains(err.Error(), "store") {
		t.Errorf("the error should name the crates that said nothing: %v", err)
	}
}

// The same rule with the run half-done: some crates reported, none errored.
// A rustc error partway through the workspace looks exactly like this, and it
// is a missing measurement, not a survival.
func TestPartialCleanRunIsNotAPass(t *testing.T) {
	partial := `    Checking domain v0.1.0 (/tmp/tree/crates/domain)
verification results:: 9 verified, 0 errors
    Checking store v0.1.0 (/tmp/tree/crates/store)
error[E0308]: mismatched types
`
	res := parseVerus(partial, five)
	if _, _, err := res.verdict(); !errors.Is(err, errNoVerdict) {
		t.Fatalf("a half-finished clean run must produce no verdict; got %v", err)
	}
}

// The budget path. A run that exceeds it returns errTimeout and NO result, so
// cmdVerify prints "R4 UNDECIDED" and no verdict sentence; calibrate then
// finds no verdict line and records an error cell rather than a survival.
func TestBudgetExhaustedIsUndecidedNotAPass(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "cargo-verus-slow")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CARGO_VERUS", fake)

	tree := filepath.Join(dir, "tree")
	src := filepath.Join(tree, "crates", "domain", "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "lib.rs"), []byte("// x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := runVerus(tree, []crate{{Name: "domain", Rel: "crates/domain"}}, 900*time.Millisecond)
	if !errors.Is(err, errTimeout) {
		t.Fatalf("want errTimeout, got %v", err)
	}
	if res == nil {
		t.Fatal("a timed-out run should still return what it captured")
	}
	if len(res.Reported) != 0 {
		t.Errorf("a timed-out run reported %d crate results; it decided nothing", len(res.Reported))
	}
	if _, _, err := res.verdict(); err == nil {
		t.Error("a timed-out run must not be able to produce a verdict")
	}
}

// The cache defence itself: every .rs file under a verify-enabled crate gets a
// fresh mtime, which is what makes cargo re-run rust_verify over it.
func TestTouchSourcesBumpsEveryRustFile(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "crates", "store", "src", "inner")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	rs := filepath.Join(deep, "mod.rs")
	other := filepath.Join(dir, "crates", "store", "src", "notes.txt")
	for _, p := range []string{rs, other} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, p := range []string{rs, other} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := touchSources(dir, []crate{{Name: "store", Rel: "crates/store"}}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(rs)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().After(old) {
		t.Error("a .rs file was not touched; the next run would be served from cache and read as a pass")
	}
	if fi, err := os.Stat(other); err == nil && fi.ModTime().After(old) {
		t.Error("a non-Rust file was touched; only sources need re-checking")
	}
}
