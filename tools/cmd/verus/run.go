package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// errTimeout means Verus did not finish inside the budget. It is its own
// outcome, never folded into "verified" or "refuted": a query the solver could
// not decide is not a claim about the code. Same contract as
// tools/cmd/gobra/run.go's errTimeout.
var errTimeout = errors.New("verus exceeded its time budget")

// errNoVerdict means the run produced no answer that can be read as a pass or
// a failure. It is deliberately NOT a pass. The commonest cause is the cargo
// cache: see touchSources.
var errNoVerdict = errors.New("verus produced no readable verdict")

// cargoVerus resolves the cargo-verus driver.
//
// It is looked for beside `verus` as well as on PATH because cloud-setup.sh
// symlinks only `verus` into /usr/local/bin, and cargo-verus lives next to the
// real binary inside the unpacked release. Hardcoding one absolute path is the
// mistake matrixctl/doctor.go already had to correct.
func cargoVerus() string {
	if p := os.Getenv("CARGO_VERUS"); p != "" {
		return p
	}
	if p := os.Getenv("VERUS_PATH"); p != "" {
		if c := siblingCargoVerus(p); c != "" {
			return c
		}
	}
	if p, err := exec.LookPath("cargo-verus"); err == nil {
		return p
	}
	if p, err := exec.LookPath("verus"); err == nil {
		if c := siblingCargoVerus(p); c != "" {
			return c
		}
	}
	return "/opt/verus/verus-x86-linux/cargo-verus"
}

// siblingCargoVerus finds cargo-verus next to a resolved `verus` binary.
// EvalSymlinks matters: /usr/local/bin/verus is a symlink into /opt/verus, and
// the directory that holds cargo-verus is the target's, not the link's.
func siblingCargoVerus(verusPath string) string {
	real, err := filepath.EvalSymlinks(verusPath)
	if err != nil {
		real = verusPath
	}
	c := filepath.Join(filepath.Dir(real), "cargo-verus")
	if _, err := os.Stat(c); err == nil {
		return c
	}
	return ""
}

// A crateResult is one crate's own `verification results::` line.
type crateResult struct {
	Crate    string
	Verified int
	Errors   int
}

// result is what one cargo-verus invocation reported about itself.
type result struct {
	// Reported holds only the verify-enabled crates of the tree under test,
	// in the order Verus reported them. vstd's own 1556-obligation line --
	// which appears on a cold tree and not on a warm one -- is deliberately
	// excluded: it is a property of the Verus release, not of this tree, and
	// folding it in would make the headline count depend on whether the
	// build cache happened to be warm.
	Reported []crateResult
	Expected []string // verify-enabled crates found in the tree
	Verified int      // sum over Reported
	Errors   int      // sum over Reported
	Raw      string
	Elapsed  time.Duration
}

var (
	// "    Checking domain v0.1.0 (/path/to/crates/domain)" -- the path is
	// present only for path (workspace) crates, which is exactly the set we
	// care about, so it doubles as the filter that keeps registry crates out.
	reChecking = regexp.MustCompile(`^\s*Checking (\S+) v\S+ \((.+)\)\s*$`)
	// Verus's own words. Two colons is not a typo; that is what it prints.
	reResults = regexp.MustCompile(`^verification results:: (\d+) verified, (\d+) errors\s*$`)
)

// parseVerus reads cargo-verus output and attributes each results line to the
// crate whose `Checking` line most recently preceded it.
//
// The attribution is only sound because the run is made with --jobs 1. With
// cargo's default parallelism the two kinds of line interleave across crates,
// and a results line cannot be told from its neighbours which crate produced
// it -- observed directly: three `Checking` lines followed by three results
// lines, in an order matching neither.
func parseVerus(out string, expected []string) *result {
	want := map[string]bool{}
	for _, c := range expected {
		want[c] = true
	}
	res := &result{Expected: expected, Raw: out}
	current := ""
	for _, ln := range strings.Split(out, "\n") {
		if m := reChecking.FindStringSubmatch(ln); m != nil {
			current = m[1]
			continue
		}
		m := reResults.FindStringSubmatch(strings.TrimSpace(ln))
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		if !want[current] {
			// vstd, or a dependency that also carries obligations. Not this
			// tree's measurement.
			continue
		}
		res.Reported = append(res.Reported, crateResult{Crate: current, Verified: v, Errors: e})
		res.Verified += v
		res.Errors += e
	}
	return res
}

// touchSources is the cache defence, and it is not optional.
//
// GOAL.md records the trap in one line: "Verus caches; touch the crate source
// or a run prints nothing and looks like a pass." Measured here: a second
// `cargo-verus verus verify` over an unchanged tree finishes in 0.3s, prints
// `Finished dev profile` and NOT ONE `verification results::` line, and exits
// 0. Anything that reads only the error count scores that as 23 verified,
// 0 errors -- a pass over a tree nothing looked at. It is the F013 false green
// with cargo as the cause.
//
// Bumping the mtime of every .rs file in each verify-enabled crate makes cargo
// consider those crates dirty and re-run rust_verify over them, while the
// dependency build (axum, serde, vstd -- 1m40 cold) stays cached: measured
// 1m49 cold, 3-4s warm-with-touch.
//
// The touch is belt; the braces are in verdict(), which refuses to call a run
// PASSED unless every verify-enabled crate actually reported a line. If the
// touch ever stops working, the rung goes UNDECIDED rather than green.
func touchSources(implDir string, crates []crate) error {
	now := time.Now()
	for _, c := range crates {
		err := filepath.Walk(filepath.Join(implDir, c.Rel), func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() || !strings.HasSuffix(p, ".rs") {
				return nil
			}
			return os.Chtimes(p, now, now)
		})
		if err != nil {
			return fmt.Errorf("touching %s: %w", c.Rel, err)
		}
	}
	return nil
}

// runVerus invokes cargo-verus over one tree and returns what it said. The
// exit code is deliberately ignored -- standing rule 1 -- because cargo exits
// 101 both for "a postcondition was refuted" and for "rustc could not parse
// this file", and those are a kill and a missing measurement respectively.
func runVerus(implDir string, crates []crate, budget time.Duration) (*result, error) {
	if err := touchSources(implDir, crates); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(crates))
	for _, c := range crates {
		names = append(names, c.Name)
	}

	ctx := context.Background()
	cancel := func() {}
	if budget > 0 {
		ctx, cancel = context.WithTimeout(ctx, budget)
	}
	defer cancel()

	// --jobs 1 is what makes the crate attribution above sound. It costs
	// little: the verification itself is seconds once the dependency graph is
	// built, and the dependency build is cached across runs anyway.
	cmd := exec.CommandContext(ctx, cargoVerus(), "verus", "verify", "--jobs", "1")
	cmd.Dir = implDir
	cmd.Env = append(os.Environ(), "CARGO_TERM_COLOR=never")
	// Own process group, killed as a group: cargo spawns rustc, rust_verify
	// spawns z3, and a wedged solver must not outlive the run that budgeted
	// it. Same reasoning as tools/cmd/gobra/run.go.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	start := time.Now()
	out, _ := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if ctx.Err() != nil {
		return &result{Expected: names, Raw: string(out), Elapsed: elapsed}, errTimeout
	}
	res := parseVerus(string(out), names)
	res.Elapsed = elapsed
	return res, nil
}

// verdict turns the run into the one sentence calibrate reads, or into no
// sentence at all.
//
// The three cases, and why they are these three:
//
//   - any reported crate has errors > 0  -> FAILED. Verus refuted something.
//     The report is partial by construction: a crate that fails verification
//     fails to compile, so every crate downstream of it is never checked
//     (measured: injecting one false postcondition into `domain` leaves
//     store, service, clock and ids unreported). The sentence says how many
//     of the verify-enabled crates were reached, so the partiality is on the
//     record rather than hidden inside a smaller "verified" count.
//   - every verify-enabled crate reported, and no errors -> PASSED.
//   - anything else -> no verdict. Fewer crates than expected with zero
//     errors is the cargo-cache false green (see touchSources) or a rustc
//     error that stopped the run early; neither is a survival, and calibrate
//     records it as an error cell.
func (r *result) verdict() (line string, killed bool, err error) {
	if r.Errors > 0 {
		return fmt.Sprintf("R4 FAILED: verification results:: %d verified, %d errors over %d of %d verify-enabled crate(s)   [%s]",
			r.Verified, r.Errors, len(r.Reported), len(r.Expected), r.Elapsed.Round(1e8)), true, nil
	}
	if len(r.Reported) == len(r.Expected) && len(r.Expected) > 0 {
		return fmt.Sprintf("R4 PASSED: verification results:: %d verified, %d errors over %d of %d verify-enabled crate(s)   [%s]",
			r.Verified, r.Errors, len(r.Reported), len(r.Expected), r.Elapsed.Round(1e8)), false, nil
	}
	return "", false, fmt.Errorf("%w: %d of %d verify-enabled crate(s) reported a `verification results::` line and none reported an error; "+
		"a Verus run that says nothing is not a pass (cargo cache, or a compile error that stopped the run). Missing: %s\n%s",
		errNoVerdict, len(r.Reported), len(r.Expected), strings.Join(r.missing(), ", "), tailLines(r.Raw, 15))
}

func (r *result) missing() []string {
	got := map[string]bool{}
	for _, c := range r.Reported {
		got[c.Crate] = true
	}
	var out []string
	for _, c := range r.Expected {
		if !got[c] {
			out = append(out, c)
		}
	}
	return out
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = "    | " + l
	}
	return strings.Join(lines, "\n")
}
