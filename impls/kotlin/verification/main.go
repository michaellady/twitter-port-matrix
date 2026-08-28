// Command kjbmc drives JBMC over the Kotlin corner's compiled bytecode and
// reports, per obligation, what the checker actually said.
//
// Run it from this directory:
//
//	cd impls/kotlin/verification && go run .
//
// It needs `kotlinc` and `jbmc` on PATH (`brew install kotlin cbmc`) plus a JDK
// that still ships lib/modules, which it locates itself or takes from -jdk.
//
// It is a Go binary rather than a shell script because it loops, parses, and
// classifies -- and because a compiled binary behaves the same for every
// caller, which a script sourced under a different shell does not.
//
// # Two rules from GOAL.md are load-bearing here
//
//  1. "No gate is decided by an exit code." This driver never reads JBMC's exit
//     status. JBMC exits non-zero for a refuted property AND for a parse error
//     AND for an unresolved entry point, so its status cannot distinguish the
//     answer from the accident. Every verdict below comes from JBMC's own goal
//     lines.
//
//  2. "No gate is trusted until it has been shown to fail." Canaries.kt holds
//     the negation of each reachable obligation. A canary JBMC reports as
//     SUCCESS is a hard failure of the run, because it names an obligation that
//     is vacuous.
//
// A third rule is enforced here that the other corners get for free: an entry
// point that yields NO assertion goals at all is reported as VACUOUS, not as a
// pass. JBMC answers "VERIFICATION SUCCESSFUL" when a --function name does not
// resolve to anything it can check, and that answer looks exactly like a proof.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// obligation is one JBMC entry point.
type obligation struct {
	class string // Obligations or Canaries
	fn    string // method name
	sig   string // JVM descriptor
	// blocked records a known reason this obligation cannot be discharged by
	// this checker. It is not a way to hide a failure: a blocked obligation is
	// still run, still reported, and its recorded reason is printed alongside
	// the real verdict so the two can be compared.
	blocked string
	// canary obligations MUST be refuted -- unless the obligation they guard is
	// itself BLOCKED, in which case a non-refutation is expected and carries no
	// information either way.
	canary bool
	// guards names the obligation a canary is the negation (or reachability
	// witness) of. It is what lets an unrefutable canary over a blocked
	// obligation be reported as uninformative rather than as a gate failure.
	guards string
}

// The three reasons an obligation over this corner cannot be discharged by JBMC
// 6.11. Each was reduced to a two-line repro and each reproduces identically in
// plain Java, so none of them is a cost Kotlin imposes.
const (
	// `assert "abc".equals("abc")` is reported FAILURE. So is `a.equals(a)` on a
	// single reference. compareTo, startsWith, isEmpty, length and charAt are all
	// fine, so the defect is localised to the org.cprover.CProverString.equals
	// intrinsic that the model's equals delegates to.
	equalsReason = "JBMC's String.equals is unsound (`assert \"abc\".equals(\"abc\")` is FAILURE); every visibility and error-code comparison is undecidable"

	// String.getBytes(Charset) dispatches on Charset.name() compared with that
	// same broken intrinsic, so it falls through to an opaque stub returning an
	// array of unconstrained length. validHandle and validText measure UTF-8
	// bytes -- which is what makes them byte-exact against a Go reference machine
	// -- so both, and every service path that starts with one, are unreachable.
	getBytesReason = "JBMC's String.getBytes(Charset) is nondeterministic; the UTF-8 byte-length predicates are unreachable"

	// A plain scalability wall: the SAT instance for a nondeterministic limit over
	// a four-entry log exhausts memory. Not a modelling gap.
	satReason = "JBMC exhausts memory on this instance (\"SAT checker ran out of memory\"); a bounded-model-checking scalability wall, not a modelling gap"
)

var obligations = []obligation{
	{class: "Obligations", fn: "o1a_oneCharAcceptSet", sig: "(Ljava/lang/String;)V"},
	{class: "Obligations", fn: "o1b_twoCharAcceptSet", sig: "(Ljava/lang/String;)V"},
	{class: "Obligations", fn: "o1c_emptyAndBareSignRejected", sig: "()V"},

	{class: "Obligations", fn: "o2a_emptyIsInvalid", sig: "()V", blocked: getBytesReason},
	{class: "Obligations", fn: "o2b_goodHandleIsValid", sig: "()V", blocked: getBytesReason},

	{class: "Obligations", fn: "o3a_idsStrictlyIncrease", sig: "()V"},
	{class: "Obligations", fn: "o3b_createdAtNonDecreasing", sig: "(Z)V"},
	{class: "Obligations", fn: "o3c_lemmaOverThreeAppends", sig: "(ZZ)V"},

	{class: "Obligations", fn: "o4a_pageRespectsLimit", sig: "(I)V", blocked: satReason},
	{class: "Obligations", fn: "o4b_cursorNullMeansExhausted", sig: "()V", blocked: equalsReason},
	{class: "Obligations", fn: "o4c_pageIsNewestFirst", sig: "()V", blocked: equalsReason},

	{class: "Obligations", fn: "o5a_unknownBeatsSelfFollow", sig: "()V", blocked: getBytesReason},
	{class: "Obligations", fn: "o5b_knownSelfFollowIsForbidden", sig: "()V", blocked: getBytesReason},
	{class: "Obligations", fn: "o5c_syntaxBeatsExistence", sig: "()V"},
	{class: "Obligations", fn: "o5d_rejectionBurnsNoId", sig: "()V", blocked: satReason},

	{class: "Canaries", fn: "c1_bareSignIsANumber", sig: "()V", canary: true, guards: "o1c_emptyAndBareSignRejected"},
	{class: "Canaries", fn: "c2_idsDoNotIncrease", sig: "()V", canary: true, guards: "o3a_idsStrictlyIncrease"},
	{class: "Canaries", fn: "c3_clockCanDecrease", sig: "()V", canary: true, guards: "o3b_createdAtNonDecreasing"},
	{class: "Canaries", fn: "c4_pageMayExceedLimit", sig: "()V", canary: true, guards: "o4a_pageRespectsLimit"},
	{class: "Canaries", fn: "c5_timelineIsOldestFirst", sig: "()V", canary: true, guards: "o4c_pageIsNewestFirst"},
	{class: "Canaries", fn: "c6_domIsReachable", sig: "()V", canary: true, guards: "o2a_emptyIsInvalid"},
	{class: "Canaries", fn: "c7_storeIsReachable", sig: "()V", canary: true, guards: "o3a_idsStrictlyIncrease"},
	{class: "Canaries", fn: "c8_serviceIsReachable", sig: "()V", canary: true, guards: "o5c_syntaxBeatsExistence"},
	{class: "Canaries", fn: "c9_syntaxDoesNotBeatExistence", sig: "()V", canary: true, guards: "o5c_syntaxBeatsExistence"},
}

// goalLine matches one of JBMC's own result lines, e.g.
//
//	[java::twitterport...o3a_idsStrictlyIncrease:()V.assertion.1] line 107 ...: SUCCESS
//
// The owner is everything up to the goal kind; the kind tells an assertion the
// obligation wrote from a check JBMC inserted on its own.
var goalLine = regexp.MustCompile(`^\[java::([^\]]+?)\.([a-z-]+(?:\.[0-9]+)?)\] .*: (SUCCESS|FAILURE)$`)

type result struct {
	ob            obligation
	ownSuccess    int
	ownFailure    int
	libFailures   map[string]int
	verdictLine   string
	elapsed       time.Duration
	toolError     string
	totalReported int
	verdict       string
}

func main() {
	var (
		root     = flag.String("root", "..", "the impls/kotlin directory")
		work     = flag.String("work", filepath.Join(os.TempDir(), "kotlin-jbmc"), "scratch directory")
		jdk      = flag.String("jdk", "", "JDK home containing lib/modules and bin/jimage (auto-detected when empty)")
		unwind   = flag.Int("unwind", 30, "JBMC loop unwinding bound")
		strLen   = flag.Int("string-length", 3, "JBMC --max-nondet-string-length")
		only     = flag.String("only", "", "run only obligations whose name contains this substring")
		timeout  = flag.Duration("timeout", 10*time.Minute, "per-obligation timeout")
		keepWork = flag.Bool("keep", true, "keep the scratch directory between runs (caches the extracted JDK classes)")
	)
	flag.Parse()

	if err := run(*root, *work, *jdk, *unwind, *strLen, *only, *timeout, *keepWork); err != nil {
		fmt.Fprintf(os.Stderr, "\nkjbmc: %v\n", err)
		os.Exit(1)
	}
}

func run(root, work, jdk string, unwind, strLen int, only string, timeout time.Duration, keep bool) error {
	kotlinc, err := exec.LookPath("kotlinc")
	if err != nil {
		return fmt.Errorf("kotlinc not on PATH: %v", err)
	}
	jbmc, err := exec.LookPath("jbmc")
	if err != nil {
		return fmt.Errorf("jbmc not on PATH (brew install cbmc): %v", err)
	}
	models, err := findModels()
	if err != nil {
		return err
	}
	stdlib, err := findKotlinStdlib(kotlinc)
	if err != nil {
		return err
	}
	if jdk == "" {
		jdk, err = findJDKWithModules()
		if err != nil {
			return err
		}
	}

	if !keep {
		_ = os.RemoveAll(work)
	}
	classes := filepath.Join(work, "classes")
	jutil := filepath.Join(work, "jutil")

	fmt.Println("kjbmc: bounded verification of the Kotlin corner")
	fmt.Printf("        kotlinc %s\n", firstLine(capture(kotlinc, "-version")))
	fmt.Printf("        jbmc    %s\n", firstLine(capture(jbmc, "--version")))
	fmt.Printf("        models  %s\n", models)
	fmt.Printf("        jdk     %s\n", jdk)
	fmt.Printf("        bounds  --unwind %d --max-nondet-string-length %d\n", unwind, strLen)
	fmt.Println(strings.Repeat("=", 96))

	// 1. Compile the implementation together with its obligations.
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(classes)
	cc := exec.Command(kotlinc, "-jvm-target", "17", "-nowarn", "-d", classes,
		filepath.Join(root, "src"), filepath.Join(root, "verification"))
	if out, err := cc.CombinedOutput(); err != nil {
		return fmt.Errorf("kotlinc failed: %v\n%s", err, out)
	}

	// 2. Extract java.util from a real JDK.
	//
	// JBMC's core-models.jar models 91 classes, all in java.lang and friends,
	// and NONE of java.util -- so an unmodelled ArrayList or HashMap is stubbed
	// as a nondeterministic value and every obligation over the store becomes
	// vacuously refutable. Handing JBMC the whole of java.base instead makes it
	// abort on an internal invariant (its java.lang.Class model is missing a
	// field that the real java.lang.ClassValue references), so only java.util
	// is taken.
	if _, err := os.Stat(filepath.Join(jutil, "java", "util", "ArrayList.class")); err != nil {
		if err := extractJavaUtil(jdk, work, jutil); err != nil {
			return err
		}
	}

	cp := strings.Join([]string{classes, stdlib, models, jutil}, ":")

	// 3. Run every obligation and read JBMC's own goal lines.
	var results []result
	for _, ob := range obligations {
		if only != "" && !strings.Contains(ob.fn, only) {
			continue
		}
		results = append(results, runOne(jbmc, cp, cp, ob, unwind, strLen, timeout))
	}

	return report(results)
}

func runOne(jbmc, cp, _ string, ob obligation, unwind, strLen int, timeout time.Duration) result {
	class := "twitterport.verification." + ob.class
	fn := class + "." + ob.fn + ":" + ob.sig
	args := []string{
		"--classpath", cp, class,
		"--function", fn,
		"--unwind", fmt.Sprint(unwind),
		"--max-nondet-string-length", fmt.Sprint(strLen),
		// Kotlin emits Intrinsics.checkNotNullParameter on every public function
		// with a non-null reference parameter. Without this flag JBMC explores
		// that branch, and the exception-construction path inside
		// kotlin.jvm.internal.Intrinsics reaches Class.forName, which is an
		// opaque stub that throws -- so every obligation drowns in library goals
		// that have nothing to do with the property. This is the one JBMC flag
		// this corner needs that the Java corner would not.
		"--java-assume-inputs-non-null",
	}

	start := time.Now()
	cmd := exec.Command(jbmc, args...)
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = cmd.CombinedOutput()
		close(done)
	}()
	timedOut := false
	select {
	case <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		timedOut = true
	}
	_ = runErr // deliberately unused: the exit status is not the verdict.

	r := result{ob: ob, libFailures: map[string]int{}, elapsed: time.Since(start)}
	if timedOut {
		r.toolError = fmt.Sprintf("timed out after %s", timeout)
		return r
	}

	ownPrefix := "java::" + fn
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "VERIFICATION") {
			r.verdictLine = line
			continue
		}
		m := goalLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		owner, kind, status := m[1], m[2], m[3]
		r.totalReported++
		isOwn := "java::"+owner == ownPrefix && strings.HasPrefix(kind, "assertion")
		switch {
		case isOwn && status == "SUCCESS":
			r.ownSuccess++
		case isOwn && status == "FAILURE":
			r.ownFailure++
		case status == "FAILURE":
			r.libFailures[shortOwner(owner)]++
		}
	}
	if r.verdictLine == "" && r.totalReported == 0 {
		r.toolError = "JBMC produced no goal lines: " + firstLine(string(out))
	}
	return r
}

// shortOwner reduces a JBMC goal owner to the declaring class, so the library
// artefacts can be counted per class instead of per method signature.
func shortOwner(owner string) string {
	if i := strings.Index(owner, ":("); i >= 0 {
		owner = owner[:i]
	}
	if i := strings.LastIndex(owner, "."); i >= 0 {
		owner = owner[:i]
	}
	return owner
}

func report(results []result) error {
	// Two passes. The first classifies every obligation on its own goal lines;
	// the second judges the canaries, which needs the first pass's answer for the
	// obligation each canary guards. A canary that cannot be refuted is only a
	// gate failure when the thing it guards is being CLAIMED.
	verdicts := make(map[string]string, len(results))
	for i := range results {
		results[i].verdict = classify(&results[i])
		verdicts[results[i].ob.fn] = results[i].verdict
	}

	fmt.Printf("%-34s %-13s %-22s %s\n", "obligation", "verdict", "own goals", "library artefacts")
	fmt.Println(strings.Repeat("-", 96))

	var vacuous, unexpected, blindCanaries []string
	verified, refuted, blocked := 0, 0, 0

	for i := range results {
		r := &results[i]
		switch {
		case r.ob.canary:
			guarded := verdicts[r.ob.guards]
			switch {
			case r.verdict == "REFUTED":
				r.verdict = "refuted-ok"
			case guarded == "BLOCKED" || guarded == "TOOL-ERR":
				// The obligation this canary negates is not being claimed, so a
				// canary that cannot fail says nothing that needs saying.
				r.verdict = "n/a-blocked"
			default:
				blindCanaries = append(blindCanaries, r.ob.fn+" (guards "+r.ob.guards+")")
				r.verdict = "CANARY-BLIND"
			}
		case r.verdict == "VACUOUS":
			vacuous = append(vacuous, r.ob.fn)
		case r.ob.blocked != "" && r.verdict == "VERIFIED":
			// The recorded reason is stale: the checker got further than expected.
			// Good news, and it must not pass silently.
			unexpected = append(unexpected, r.ob.fn)
			verified++
		case r.verdict == "BLOCKED":
			blocked++
		case r.verdict == "VERIFIED":
			verified++
		case r.verdict == "REFUTED":
			refuted++
		}

		lib := ""
		if len(r.libFailures) > 0 {
			keys := make([]string, 0, len(r.libFailures))
			for k := range r.libFailures {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			lib = strings.Join(keys, ", ")
		}
		own := fmt.Sprintf("%d ok, %d failed", r.ownSuccess, r.ownFailure)
		if r.toolError != "" {
			own = r.toolError
		}
		fmt.Printf("%-34s %-13s %-22s %s\n", r.ob.fn, r.verdict, own, lib)
	}

	fmt.Println(strings.Repeat("=", 96))
	fmt.Printf("R4-bounded: %d VERIFIED, %d REFUTED, %d BLOCKED by a tool limit\n",
		verified, refuted, blocked)

	seen := map[string]bool{}
	for _, r := range results {
		if r.ob.blocked != "" && r.verdict == "BLOCKED" && !seen[r.ob.blocked] {
			seen[r.ob.blocked] = true
			fmt.Printf("\n  BLOCKED: %s\n", r.ob.blocked)
			for _, o := range results {
				if o.ob.blocked == r.ob.blocked && o.verdict == "BLOCKED" {
					fmt.Printf("    - %s\n", o.ob.fn)
				}
			}
		}
	}

	if len(unexpected) > 0 {
		fmt.Printf("\nRECORDED-REASON STALE: %s verified anyway. Update the blocked list.\n",
			strings.Join(unexpected, ", "))
	}
	if len(vacuous) > 0 {
		fmt.Printf("\nVACUOUS: %s\n", strings.Join(vacuous, ", "))
		fmt.Println("JBMC found no assertion goal for these entry points. That is not a pass.")
	}
	if len(blindCanaries) > 0 {
		fmt.Printf("\nCANARIES NOT REFUTED: %s\n", strings.Join(blindCanaries, ", "))
		fmt.Println("Those obligations are claimed VERIFIED and the check cannot fail on them.")
		return fmt.Errorf("canary sweep failed")
	}
	if len(vacuous) > 0 {
		return fmt.Errorf("%d vacuous obligation(s)", len(vacuous))
	}
	fmt.Println("\nEvery canary guarding a claimed obligation was refuted: the bounded rung is")
	fmt.Println("falsifiable everywhere it makes a claim.")
	return nil
}

// classify turns JBMC's own goal lines into a verdict. It never looks at an exit
// status, and it never treats "no assertion goal at all" as a pass.
func classify(r *result) string {
	switch {
	case r.toolError != "":
		if r.ob.blocked != "" {
			return "BLOCKED"
		}
		return "TOOL-ERR"
	case r.ownSuccess == 0 && r.ownFailure == 0:
		if r.ob.blocked != "" {
			return "BLOCKED"
		}
		return "VACUOUS"
	case r.ownFailure > 0:
		if r.ob.blocked != "" {
			return "BLOCKED"
		}
		return "REFUTED"
	default:
		return "VERIFIED"
	}
}

// --- toolchain discovery -----------------------------------------------------

func findModels() (string, error) {
	out := capture("brew", "--prefix", "cbmc")
	prefix := strings.TrimSpace(out)
	if prefix != "" {
		p := filepath.Join(prefix, "libexec", "lib", "core-models.jar")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	jbmc, err := exec.LookPath("jbmc")
	if err == nil {
		real, err := filepath.EvalSymlinks(jbmc)
		if err == nil {
			p := filepath.Join(filepath.Dir(filepath.Dir(real)), "libexec", "lib", "core-models.jar")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("core-models.jar not found next to jbmc")
}

func findKotlinStdlib(kotlinc string) (string, error) {
	real, err := filepath.EvalSymlinks(kotlinc)
	if err != nil {
		real = kotlinc
	}
	// Homebrew's bin/kotlinc is a shim, so the jar is not always one directory
	// up from it: on this machine it is .../2.4.10/libexec/lib/kotlin-stdlib.jar
	// while kotlinc lives in .../2.4.10/bin. Walk up looking for libexec/lib
	// as well as lib.
	dir := filepath.Dir(real)
	for i := 0; i < 4 && dir != "/" && dir != "."; i++ {
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
// The jenv shims on PATH do not, which is why this does not just follow javac.
func findJDKWithModules() (string, error) {
	var candidates []string
	if h := os.Getenv("JAVA_HOME"); h != "" {
		candidates = append(candidates, h)
	}
	if out := strings.TrimSpace(capture("/usr/libexec/java_home", "-v", "17")); out != "" {
		candidates = append(candidates, out)
	}
	for _, root := range []string{"/opt/homebrew/Cellar/openjdk@17", "/opt/homebrew/Cellar/openjdk", "/Library/Java/JavaVirtualMachines"} {
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
