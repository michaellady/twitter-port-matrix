package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

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
)

// runGobra invokes the jar and reads the verdict out of its own output. It
// never decides anything from the exit code -- standing rule 1.
func runGobra(w *workspace, pkgs []string, statsDir string) (*result, error) {
	args := []string{"-Xss128m", "-jar", gobraJar(), "-p"}
	args = append(args, pkgs...)
	args = append(args, "-I", "stubs", w.root, "--projectRoot", ".")
	if statsDir != "" {
		if err := os.MkdirAll(statsDir, 0o755); err != nil {
			return nil, err
		}
		args = append(args, "-g", statsDir)
	}
	cmd := exec.Command("java", args...)
	cmd.Dir = w.module
	start := time.Now()
	out, _ := cmd.CombinedOutput() // exit code deliberately ignored; see above
	elapsed := time.Since(start)

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
