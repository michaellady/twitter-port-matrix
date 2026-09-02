package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
)

// errR4Failed is the exit-code half of a FAILED verdict. The verdict itself is
// the "R4 FAILED" line on stdout; `calibrate` reads that line and then requires
// the exit code to agree with it, so both have to be produced together.
var errR4Failed = errors.New("R4 FAILED")

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	impl := fs.String("impl", "impls/go", "the Go implementation directory; with -registry, a registry entry name instead")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry (go, or go@<id> from `mutate apply`) and the directory is read from it")
	statsDir := fs.String("stats", "", "also write Gobra's stats.json here")
	repeat := fs.Int("repeat", 1, "run this many times (the member count is not deterministic)")
	budget := fs.Duration("budget", 20*time.Minute, "time budget per run; a run that exceeds it is UNDECIDED, never a pass")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}
	ws, err := newWorkspace(implDir)
	if err != nil {
		return err
	}
	defer ws.close()

	// The verdict is aggregated over the repeats and printed once, as the
	// LAST line, because calibrate counts verdict lines and treats two of
	// them as ambiguous.
	total := 0
	pkgs := 0
	var elapsed time.Duration
	for n := 1; n <= *repeat; n++ {
		sd, err := os.MkdirTemp("", "gobra-stats-")
		if err != nil {
			return err
		}
		res, err := runOnce(ws, sd, n, *repeat, *statsDir, *budget)
		os.RemoveAll(sd)
		if errors.Is(err, errTimeout) {
			// Not a pass and not a failure. Gobra's own transcript says a
			// package "got terminated" and then reports 0 error(s); reading
			// the count would score a proof that never ran as green.
			fmt.Printf("R4 UNDECIDED: Gobra exceeded its %s budget (run %d of %d); nothing was decided about this tree\n",
				*budget, n, *repeat)
			return err
		}
		if err != nil {
			return err
		}
		total += res.Total
		pkgs = len(res.Packages)
		elapsed += res.Elapsed
	}
	fmt.Println(r4Verdict(total, pkgs, elapsed))
	if total > 0 {
		return errR4Failed
	}
	return nil
}

// r4Verdict is the sentence calibrate reads. It carries Gobra's own count
// verbatim so the record says what the verifier said, not a paraphrase.
func r4Verdict(errorCount, packages int, elapsed time.Duration) string {
	word := "PASSED"
	if errorCount > 0 {
		word = "FAILED"
	}
	return fmt.Sprintf("R4 %s: Gobra has found %d error(s) over %d package(s)   [%s]",
		word, errorCount, packages, elapsed.Round(1e8))
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

// resolveImplDir turns -impl into a directory. With -registry it goes through
// the same registry the other rungs use, so the tree Gobra reads is the tree
// calibrate's guard hashed -- resolved by name, exactly as replay and diffrun
// resolve theirs.
func resolveImplDir(impl, registry string) (string, error) {
	if registry == "" {
		return implDirFromArgs([]string{"-impl", impl})
	}
	reg, err := implrun.LoadRegistry(registry)
	if err != nil {
		return "", fmt.Errorf("reading registry %s: %w", registry, err)
	}
	spec, err := reg.Get(impl)
	if err != nil {
		return "", err
	}
	return implDirFromArgs([]string{"-impl", spec.Dir})
}

// runOnce is one Gobra invocation over the whole matrix. The member count is
// re-derived every time because it is not stable run to run -- see
// evidence/findings/F019.
func runOnce(ws *workspace, sd string, n, repeat int, keep string, budget time.Duration) (*result, error) {
	res, err := runGobra(ws, verifiedPackages, sd, budget)
	if err != nil {
		if errors.Is(err, errTimeout) && res != nil {
			fmt.Println("Gobra's own report of the termination:")
			fmt.Println(tailLines(res.Raw, 6))
		}
		return res, err
	}
	if res.Total < 0 {
		return res, fmt.Errorf("gobra printed per-package lines but no `Gobra has found N error(s)` total; no verdict:\n%s", tailLines(res.Raw, 12))
	}
	if repeat > 1 {
		fmt.Printf("--- run %d of %d ---\n", n, repeat)
	}
	fmt.Println("Gobra's own verdict lines:")
	names := make([]string, 0, len(res.Packages))
	for k := range res.Packages {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Printf("  %-10s %s\n", k, res.Packages[k])
	}
	fmt.Printf("  %-10s Gobra has found %d error(s)   [%s]\n", "TOTAL", res.Total, res.Elapsed.Round(1e8))
	for _, e := range res.Errors {
		fmt.Printf("    %s:%d:%d %s\n", e.File, e.Line, e.Col, e.Message)
	}
	c, err := countMembers(res.StatsFile)
	if err != nil {
		return res, fmt.Errorf("reading Gobra's report: %w", err)
	}
	if keep != "" {
		dst := keep
		if repeat > 1 {
			dst = fmt.Sprintf("%s.%d", keep, n)
		}
		if b, err := os.ReadFile(res.StatsFile); err == nil {
			_ = os.MkdirAll(filepath.Dir(dst), 0o755)
			_ = os.WriteFile(dst, b, 0o644)
		}
	}
	fmt.Printf("\nViper members (from Gobra's own stats.json):\n")
	fmt.Printf("  %d distinct, %d with a body and verified\n", c.Distinct, c.BodyVerified)
	fmt.Printf("  (%d Gobra members, %d task rows before de-duplicating imports)\n", c.GobraMembers, c.Rows)
	pkgs := make([]string, 0, len(c.PerPackage))
	for k := range c.PerPackage {
		pkgs = append(pkgs, k)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		fmt.Printf("    %-10s %3d total  %3d verified\n", p, c.PerPackage[p][0], c.PerPackage[p][1])
	}
	fmt.Println()
	return res, nil
}
