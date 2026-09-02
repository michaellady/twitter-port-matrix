package main

import (
	"context"
	"encoding/json"
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

// errTimeout means Gobra did not finish inside the budget. It is its own
// outcome, never folded into "verified" or "refuted": a query the solver could
// not decide is not a claim about the code.
var errTimeout = errors.New("gobra exceeded its time budget")

// modulePath is the Go module the verified packages live in. Gobra resolves
// an import path by looking under the directories given to -I, GOPATH-style,
// so the workspace below is laid out as <root>/<modulePath> rather than being
// verified in place. Without that, every cross-package import inside the
// module fails with "No existing directory found for import path".
const modulePath = "github.com/michaellady/twitter-port-matrix-impl-go"

// verifiedPackages is Gobra's verification matrix, exactly as TCB.md records
// it. internal/httpshim is deliberately absent: it is trusted transport.
var verifiedPackages = []string{
	"internal/clock",
	"internal/ids",
	"internal/dom",
	"internal/store",
	"internal/service",
}

func gobraJar() string {
	if v := os.Getenv("GOBRA_JAR"); v != "" {
		return v
	}
	return "/opt/gobra/gobra.jar"
}

// workspace is a throwaway GOPATH-shaped copy of impls/go that Gobra can
// resolve imports in, and that a canary can edit without touching the repo.
type workspace struct {
	root   string // <tmp>
	module string // <tmp>/github.com/.../twitter-port-matrix-impl-go
}

func newWorkspace(implDir string) (*workspace, error) {
	root, err := os.MkdirTemp("", "gobra-ws-")
	if err != nil {
		return nil, err
	}
	mod := filepath.Join(root, filepath.FromSlash(modulePath))
	if err := os.MkdirAll(filepath.Dir(mod), 0o755); err != nil {
		os.RemoveAll(root)
		return nil, err
	}
	if out, err := exec.Command("cp", "-r", implDir, mod).CombinedOutput(); err != nil {
		os.RemoveAll(root)
		return nil, fmt.Errorf("copying %s: %v: %s", implDir, err, out)
	}
	return &workspace{root: root, module: mod}, nil
}

func (w *workspace) close() { os.RemoveAll(w.root) }

// result is what one Gobra invocation reported about itself.
type result struct {
	Packages  map[string]string // package name -> "no errors" | "N error(s)"
	Errors    []gobraError
	Total     int    // "Gobra has found N error(s)"
	Raw       string // full stdout+stderr, minus JVM noise
	Elapsed   time.Duration
	StatsFile string
}

type gobraError struct {
	File    string
	Line    int
	Col     int
	Message string
}

var (
	reVerifying = regexp.MustCompile(`Verifying package (\S+) - (\S+)`)
	reNoErrors  = regexp.MustCompile(`Gobra found no errors`)
	rePkgErrors = regexp.MustCompile(`Gobra has found (\d+) error\(s\) in package (\S+) - (\S+)`)
	reTotal     = regexp.MustCompile(`Gobra has found (\d+) error\(s\)\s*$`)
	reErrorAt   = regexp.MustCompile(`Error at: <([^:>]+):(\d+):(\d+)> (.*)$`)
	// Gobra's own words when --packageTimeout fires.
	reTerminated = regexp.MustCompile(`The verification of package .* got terminated after `)
)

// runGobra invokes the jar and reads the verdict out of its own output. It
// never decides anything from the exit code -- standing rule 1.
//
// budget bounds the run. Some negated quantifier clauses send Z3 somewhere it
// does not come back from: two canaries over (*MemStore).HomeTimeline ran 35
// and 43 minutes at 2% CPU before being killed, which is a hung solver rather
// than a slow one. Without a bound those two wedge every worker behind them,
// and an unbounded sweep that never finishes reports nothing at all.
func runGobra(w *workspace, pkgs []string, statsDir string, budget time.Duration) (*result, error) {
	args := []string{"-Xss128m", "-jar", gobraJar(), "-p"}
	args = append(args, pkgs...)
	args = append(args, "-I", "stubs", w.root, "--projectRoot", ".")
	if statsDir != "" {
		if err := os.MkdirAll(statsDir, 0o755); err != nil {
			return nil, err
		}
		args = append(args, "-g", statsDir)
	}
	ctx := context.Background()
	cancel := func() {}
	if budget > 0 {
		// Gobra's own --packageTimeout usually lands first and reports a
		// timeout in its output; the context is the backstop for when the JVM
		// stops responding to it at all.
		// Gobra parses this with Scala's Duration, which rejects Go's
		// "6m0s" with a NumberFormatException -- and the stack trace that
		// produces contains "packageTimeoutDuration", which is exactly long
		// enough to fool a naive substring check for "timeout".
		args = append(args, "--packageTimeout", fmt.Sprintf("%d seconds", int(budget.Seconds())))
		ctx, cancel = context.WithTimeout(ctx, budget+30*time.Second)
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, "java", args...)
	cmd.Dir = w.module
	// Put the JVM in its own process group and kill the group, so a wedged
	// Z3 child does not outlive the java process that spawned it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	start := time.Now()
	out, _ := cmd.CombinedOutput() // exit code deliberately ignored; see above
	elapsed := time.Since(start)
	if ctx.Err() != nil {
		return &result{Elapsed: elapsed, Total: -1, Raw: string(out)}, errTimeout
	}

	res := &result{Packages: map[string]string{}, Elapsed: elapsed, Total: -1}
	var kept []string
	// Gobra interleaves per-package lines; the last "Verifying package" seen
	// before a verdict line is the package that verdict belongs to.
	current := ""
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "Picked up ") || strings.TrimSpace(ln) == "" {
			continue
		}
		kept = append(kept, ln)
		if m := reVerifying.FindStringSubmatch(ln); m != nil {
			current = m[2]
			continue
		}
		if m := rePkgErrors.FindStringSubmatch(ln); m != nil {
			res.Packages[m[3]] = m[1] + " error(s)"
			continue
		}
		if reNoErrors.MatchString(ln) && current != "" {
			res.Packages[current] = "no errors"
			continue
		}
		if m := reErrorAt.FindStringSubmatch(ln); m != nil {
			line, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			res.Errors = append(res.Errors, gobraError{File: m[1], Line: line, Col: col, Message: m[4]})
			continue
		}
		if m := reTotal.FindStringSubmatch(ln); m != nil {
			n, _ := strconv.Atoi(m[1])
			res.Total = n
		}
	}
	res.Raw = strings.Join(kept, "\n")
	if statsDir != "" {
		res.StatsFile = filepath.Join(statsDir, "stats.json")
	}
	// A package that times out prints "got terminated after ..." and then
	// "Gobra has found 0 error(s)". Reading the count alone would score a
	// verification that never ran as a clean pass -- and in a negation sweep
	// that becomes "the negation verified", i.e. VACUOUS. The termination
	// line has to be checked first, and matched exactly: any looser test also
	// matches the stack trace above.
	if reTerminated.MatchString(res.Raw) {
		return res, errTimeout
	}
	if res.Total < 0 && len(res.Packages) == 0 {
		return res, fmt.Errorf("gobra produced no verdict line; output was:\n%s", res.Raw)
	}
	return res, nil
}

// --- Viper member accounting -------------------------------------------

type statMember struct {
	Pkg          string `json:"pkg"`
	Name         string `json:"name"`
	NodeType     string `json:"nodeType"`
	Trusted      bool   `json:"trusted"`
	Abstract     bool   `json:"abstract"`
	ViperMembers []struct {
		Name     string `json:"name"`
		TaskName string `json:"taskName"`
		HasBody  bool   `json:"hasBody"`
		Verified bool   `json:"verified"`
		Success  bool   `json:"success"`
	} `json:"viperMembers"`
}

type memberCounts struct {
	GobraMembers int
	Rows         int // one row per (viper member, verifying task)
	Distinct     int // distinct viper member names
	BodyVerified int // distinct names with a body, verified
	PerPackage   map[string][2]int
}

// countMembers reads Gobra's own stats.json.
//
// A Viper member that several packages import is reported once per verifying
// task, so the row count double-counts. Distinct names are the honest total:
// the name carries a package hash, so two entries sharing a name really are
// the same member.
func countMembers(path string) (*memberCounts, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var members []statMember
	if err := json.Unmarshal(b, &members); err != nil {
		return nil, err
	}
	c := &memberCounts{GobraMembers: len(members), PerPackage: map[string][2]int{}}
	seen := map[string]bool{}
	for _, m := range members {
		for _, vm := range m.ViperMembers {
			c.Rows++
			if seen[vm.Name] {
				continue
			}
			seen[vm.Name] = true
			c.Distinct++
			p := c.PerPackage[m.Pkg]
			p[0]++
			if vm.HasBody && vm.Verified {
				c.BodyVerified++
				p[1]++
			}
			c.PerPackage[m.Pkg] = p
		}
	}
	return c, nil
}
