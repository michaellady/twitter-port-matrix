package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// errTimeout means JBMC did not finish inside its budget. It is its own
// outcome and is never folded into VERIFIED or REFUTED: a query the solver
// could not decide is not a claim about the code. Same contract as
// tools/cmd/gobra's errTimeout.
var errTimeout = errors.New("jbmc exceeded its time budget")

// toolchain is everything one verify run needs on disk.
type toolchain struct {
	JBMC     string
	Compile  string // kotlinc
	Models   string // JBMC's core-models.jar
	Stdlib   string // kotlin-stdlib.jar
	JDK      string
	JavaUtil string
}

// goalLine matches one of JBMC's own result lines, e.g.
//
//	[java::twitterport...o3a_idsStrictlyIncrease:()V.assertion.1] line 107 ...: SUCCESS
//
// The owner is everything up to the goal kind; the kind separates an assertion
// the obligation wrote from a check JBMC inserted on its own. Only the
// obligation's OWN assertions decide anything -- a library goal failing inside
// java.util says something about JBMC's model, not about the port.
var goalLine = regexp.MustCompile(`^\[java::([^\]]+?)\.([a-z-]+(?:\.[0-9]+)?)\] .*: (SUCCESS|FAILURE)$`)

// runResult is what one JBMC invocation reported about ONE obligation.
type runResult struct {
	Ob          obligation
	OwnSuccess  int
	OwnFailure  int
	LibFailures map[string]int
	TotalGoals  int
	VerdictLine string // JBMC's own "VERIFICATION SUCCESSFUL" / "FAILED"
	Elapsed     time.Duration
	ToolError   string
	TimedOut    bool
	Raw         string
}

// runOne invokes JBMC over one entry point and reads its own goal lines.
//
// The exit status is read and discarded on purpose. JBMC exits non-zero for a
// refuted property, for a malformed classpath and for an entry point that does
// not resolve, so it cannot distinguish the answer from the accident -- GOAL.md
// standing rule 1.
func runOne(tc toolchain, cp string, pkg string, ob obligation, unwind, strLen int, budget time.Duration) runResult {
	entry := ob.Entry(pkg)
	class := pkg + "." + ob.Class
	args := []string{
		"--classpath", cp, class,
		"--function", entry,
		"--unwind", fmt.Sprint(unwind),
		"--max-nondet-string-length", fmt.Sprint(strLen),
		// Kotlin emits Intrinsics.checkNotNullParameter on every public
		// function with a non-null reference parameter. Without this flag JBMC
		// explores that branch and the exception-construction path inside
		// kotlin.jvm.internal.Intrinsics reaches Class.forName, an opaque stub
		// that throws -- so every obligation drowns in library goals that have
		// nothing to do with the property.
		"--java-assume-inputs-non-null",
	}

	ctx := context.Background()
	cancel := func() {}
	if budget > 0 {
		ctx, cancel = context.WithTimeout(ctx, budget)
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, tc.JBMC, args...)
	// Own process group, killed as a group: a wedged SAT child must not
	// outlive the jbmc process that spawned it and go on eating a CPU that
	// three other corners' sweeps are sharing.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

	start := time.Now()
	out, _ := cmd.CombinedOutput() // exit status deliberately ignored; see above
	r := parseJBMC(string(out), ob, entry)
	r.Elapsed = time.Since(start)
	if ctx.Err() != nil {
		r.TimedOut = true
		r.ToolError = fmt.Sprintf("JBMC exceeded its %s budget", budget)
	}
	return r
}

// parseJBMC turns JBMC's own output into counts. It is separated from the
// process handling so the accounting can be unit-tested against recorded
// transcripts rather than against a live solver.
func parseJBMC(out string, ob obligation, entry string) runResult {
	r := runResult{Ob: ob, LibFailures: map[string]int{}, Raw: out}
	ownPrefix := "java::" + entry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "VERIFICATION") {
			r.VerdictLine = line
			continue
		}
		m := goalLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		owner, kind, status := m[1], m[2], m[3]
		r.TotalGoals++
		own := "java::"+owner == ownPrefix && strings.HasPrefix(kind, "assertion")
		switch {
		case own && status == "SUCCESS":
			r.OwnSuccess++
		case own && status == "FAILURE":
			r.OwnFailure++
		case status == "FAILURE":
			r.LibFailures[shortOwner(owner)]++
		}
	}
	if r.VerdictLine == "" && r.TotalGoals == 0 {
		r.ToolError = "JBMC produced no goal lines: " + firstLine(out)
	}
	return r
}

// shortOwner reduces a JBMC goal owner to the declaring class.
func shortOwner(owner string) string {
	if i := strings.Index(owner, ":("); i >= 0 {
		owner = owner[:i]
	}
	if i := strings.LastIndex(owner, "."); i >= 0 {
		owner = owner[:i]
	}
	return owner
}

// --- compilation --------------------------------------------------------

// compileCorner compiles the corner's implementation together with its
// obligations into one class directory.
//
// Both are compiled together on purpose: the obligations must link against the
// tree under test, not against a pristine copy. On a mutant tree that is the
// whole point -- the mutated Store.kt is what the obligation calls.
func compileCorner(tc toolchain, c corner, implDir, classes string, budget time.Duration) (time.Duration, error) {
	_ = os.RemoveAll(classes)
	args := []string{"-jvm-target", "17", "-nowarn", "-d", classes}
	for _, d := range c.SrcDirs {
		args = append(args, filepath.Join(implDir, d))
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	cmd := exec.CommandContext(ctx, tc.Compile, args...)
	start := time.Now()
	out, err := cmd.CombinedOutput()
	el := time.Since(start)
	if ctx.Err() != nil {
		return el, fmt.Errorf("%w: kotlinc did not finish inside %s", errTimeout, budget)
	}
	if err != nil {
		return el, fmt.Errorf("kotlinc failed: %v\n%s", err, out)
	}
	return el, nil
}

// --- toolchain discovery ------------------------------------------------

func findToolchain(jdkFlag, workDir string) (toolchain, error) {
	var tc toolchain
	var err error
	if tc.Compile, err = exec.LookPath("kotlinc"); err != nil {
		return tc, fmt.Errorf("kotlinc not on PATH: %v", err)
	}
	if tc.JBMC, err = exec.LookPath("jbmc"); err != nil {
		return tc, fmt.Errorf("jbmc not on PATH: %v", err)
	}
	if tc.Models, err = findModels(); err != nil {
		return tc, err
	}
	if tc.Stdlib, err = findKotlinStdlib(tc.Compile); err != nil {
		return tc, err
	}
	tc.JDK = jdkFlag
	if tc.JDK == "" {
		if tc.JDK, err = findJDKWithModules(); err != nil {
			return tc, err
		}
	}
	// JBMC's core-models.jar models 91 classes, all in java.lang and friends,
	// and NONE of java.util -- so an unmodelled ArrayList or HashMap is stubbed
	// as a nondeterministic value and every obligation over the store becomes
	// vacuously refutable. Handing JBMC the whole of java.base instead makes it
	// abort on an internal invariant, so only java.util is taken.
	tc.JavaUtil = filepath.Join(workDir, "jutil")
	if _, statErr := os.Stat(filepath.Join(tc.JavaUtil, "java", "util", "ArrayList.class")); statErr != nil {
		if err := extractJavaUtil(tc.JDK, workDir, tc.JavaUtil); err != nil {
			return tc, err
		}
	}
	return tc, nil
}

// findModels locates core-models.jar, JBMC's model of the JDK classes. Two
// packagings, two layouts: Homebrew puts it under libexec/lib, the Ubuntu .deb
// under <prefix>/lib next to bin.
func findModels() (string, error) {
	var cands []string
	if prefix := strings.TrimSpace(capture("brew", "--prefix", "cbmc")); prefix != "" {
		cands = append(cands, filepath.Join(prefix, "libexec", "lib", "core-models.jar"))
	}
	if jbmc, err := exec.LookPath("jbmc"); err == nil {
		real, err := filepath.EvalSymlinks(jbmc)
		if err != nil {
			real = jbmc
		}
		prefix := filepath.Dir(filepath.Dir(real))
		cands = append(cands,
			filepath.Join(prefix, "libexec", "lib", "core-models.jar"),
			filepath.Join(prefix, "lib", "core-models.jar"),
			filepath.Join(prefix, "share", "cbmc", "core-models.jar"))
	}
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("core-models.jar not found; looked in %v", cands)
}

func findKotlinStdlib(kotlinc string) (string, error) {
	real, err := filepath.EvalSymlinks(kotlinc)
	if err != nil {
		real = kotlinc
	}
	dir := filepath.Dir(real)
	for i := 0; i < 5 && dir != "/" && dir != "."; i++ {
		for _, rel := range []string{
			filepath.Join("lib", "kotlin-stdlib.jar"),
			filepath.Join("libexec", "lib", "kotlin-stdlib.jar"),
		} {
			p := filepath.Join(dir, rel)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("kotlin-stdlib.jar not found near %s", real)
}

// findJDKWithModules locates a JDK that still ships lib/modules and bin/jimage.
func findJDKWithModules() (string, error) {
	var candidates []string
	if h := os.Getenv("JAVA_HOME"); h != "" {
		candidates = append(candidates, h)
	}
	if out := strings.TrimSpace(capture("/usr/libexec/java_home", "-v", "17")); out != "" {
		candidates = append(candidates, out)
	}
	if javac, err := exec.LookPath("javac"); err == nil {
		if real, err := filepath.EvalSymlinks(javac); err == nil {
			candidates = append(candidates, filepath.Dir(filepath.Dir(real)))
		}
	}
	for _, root := range []string{
		"/usr/lib/jvm",
		"/opt/homebrew/Cellar/openjdk@17", "/opt/homebrew/Cellar/openjdk",
		"/Library/Java/JavaVirtualMachines",
	} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			base := filepath.Join(root, e.Name())
			candidates = append(candidates,
				filepath.Join(base, "libexec", "openjdk.jdk", "Contents", "Home"),
				filepath.Join(base, "Contents", "Home"),
				base)
		}
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "lib", "modules")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "bin", "jimage")); err != nil {
			continue
		}
		return c, nil
	}
	return "", fmt.Errorf("no JDK with lib/modules and bin/jimage found; pass -jdk")
}

func extractJavaUtil(jdk, work, dest string) error {
	staging := filepath.Join(work, "jdk-extract")
	_ = os.RemoveAll(staging)
	cmd := exec.Command(filepath.Join(jdk, "bin", "jimage"), "extract",
		"--dir="+staging, filepath.Join(jdk, "lib", "modules"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("jimage extract failed: %v\n%s", err, out)
	}
	src := filepath.Join(staging, "java.base", "java", "util")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("java.util not found in extracted JDK: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dest, "java"), 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("cp", "-R", src, filepath.Join(dest, "java", "util")).CombinedOutput(); err != nil {
		return fmt.Errorf("copying java.util: %v\n%s", err, out)
	}
	_ = os.RemoveAll(staging)
	return nil
}

func capture(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return string(out)
}

// firstLine is the first line that says something. The JVM prepends a "Picked
// up JAVA_TOOL_OPTIONS: ..." line under this environment's proxy settings, and
// reporting that as `kotlinc -version` would put four hundred characters of
// proxy configuration in the run header where the toolchain version belongs.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Picked up ") {
			continue
		}
		return ln
	}
	return ""
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
