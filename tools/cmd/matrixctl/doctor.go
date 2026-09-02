package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	tlaJarPath       = "docker/tlc/tla2tools.jar"
	tlaJarSHA256     = "936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88"
	defaultVerusPath = "/Users/mikelady/.verus/verus-arm64-macos/verus"
	sobsImport       = "twitter-port-matrix/spec/s_obs"

	// The Gobra jar, pinned in docker/pins.json under tools.gobra. Kept here
	// as a constant for the same reason tla2tools.jar is: a check that reads
	// its own expected digest from a file the check is supposed to police is
	// not a check.
	defaultGobraJar = "/opt/gobra/gobra.jar"
	gobraJarSHA256  = "33d2dce591af60c48e3b11af1bf7f41a31a70fb7578ecefe1728748b58f30321"
)

type check struct {
	name   string
	detail string
	ok     bool
	fatal  bool
}

func doctor() error {
	fmt.Println("matrixctl doctor")
	fmt.Println(strings.Repeat("=", 72))

	var cs []check
	cs = append(cs, checkCmd("go", true, "go", "version"))
	cs = append(cs, checkCmd("java", true, "java", "-version"))
	cs = append(cs, checkCmd("rustc", false, "rustc", "--version"))
	cs = append(cs, checkCmd("kotlinc", false, "kotlinc", "-version"))
	cs = append(cs, checkCmd("z3", false, "z3", "--version"))
	cs = append(cs, checkCmd("jbmc", false, "jbmc", "--version"))
	cs = append(cs, checkCmd("docker daemon", false, "docker", "info", "--format", "{{.ServerVersion}}"))
	cs = append(cs, checkVerus())
	cs = append(cs, checkGobra())
	cs = append(cs, checkJar())
	cs = append(cs, checkVendoredSpec())
	cs = append(cs, checkIsolation())

	fmt.Println()
	failed := 0
	for _, c := range cs {
		mark := "ok  "
		if !c.ok {
			if c.fatal {
				mark = "FAIL"
				failed++
			} else {
				mark = "warn"
			}
		}
		fmt.Printf("  %s  %-24s %s\n", mark, c.name, c.detail)
	}
	fmt.Println(strings.Repeat("=", 72))
	reportRungs(cs)
	fmt.Println(strings.Repeat("=", 72))
	if failed > 0 {
		return fmt.Errorf("%d fatal check(s) failed", failed)
	}
	fmt.Println("doctor: all fatal checks passed")
	return nil
}

// reportRungs answers the question the inventory above only implies: which
// rungs of ASSURANCE.md can this box actually reach.
//
// It exists because "verus: ok" and "R4 is available" are different claims and
// the first was repeatedly read as the second. Every row here names the tool
// whose OWN OUTPUT was read (standing rule 1) and, when a rung is out of
// reach, what is missing rather than only that something is.
//
// The rows deliberately do not claim a rung PASSES -- only that the machinery
// to run it is present and runnable. Passing is what `matrixctl impls check`,
// `diffrun`, `proptest` and the verifiers report, and none of them is cheap
// enough to run from `doctor`.
func reportRungs(cs []check) {
	ok := map[string]bool{}
	for _, c := range cs {
		ok[c.name] = c.ok
	}
	fmt.Println("rung reachability (tooling only -- these say runnable, not green)")
	fmt.Println()

	type rung struct{ name, needs, detail string }
	rows := []rung{
		{"R0 corpus", "go", "replay + canaries, all four corners"},
		{"R1 diff-fuzz", "go", "tracegen + diffrun"},
		{"R2 property", "go", "proptest"},
		{"R3 model check", "java", "TLC over twitter.tla + the S_obs link check"},
		{"R4 proof / Rust", "verus", "cargo-verus over the five verify-enabled crates"},
		{"R4 proof / Go", "gobra jar", "java -Xss128m -jar gobra.jar"},
		// Kotlin only. The Java corner has the same checker available and the
		// same F014 wall, but no obligation set is written for it, so there is
		// nothing for the rung to run -- saying "Kotlin and Java" here would
		// report a rung that does not exist as runnable.
		{"R4 bounded / Kotlin", "jbmc", "jbmc verify: 7 decidable obligations, 8 blocked by F014"},
		{"R5 refinement", "gobra jar", "31 of 42 discharged clauses are Gobra-backed"},
	}
	for _, r := range rows {
		mark, why := "yes ", ""
		if !ok[r.needs] {
			mark, why = "NO  ", "  <- needs "+r.needs
		}
		fmt.Printf("  %s  %-18s %s%s\n", mark, r.name, r.detail, why)
	}
}

// gobraJar resolves the Gobra fat jar. GOBRA_JAR overrides the location the
// cloud setup script writes it to.
func gobraJar() string {
	if p := os.Getenv("GOBRA_JAR"); p != "" {
		return p
	}
	return defaultGobraJar
}

// checkGobra verifies the Gobra jar the way the rest of this file verifies
// tla2tools.jar, and then RUNS IT.
//
// Presence is not the check. A cloud session was observed with `java`, `z3`
// and a `/opt/gobra/` directory all in place and no jar inside it, because
// ghcr.io's blob host is not on the network allowlist and the fetch produced
// a zero-byte file (see CLOUD.md). `doctor` said nothing at all, because it
// had no Gobra check; the whole Go deductive rung and 31 of the 42 discharged
// R5 clauses were quietly unavailable.
func checkGobra() check {
	jar := gobraJar()
	b, err := os.ReadFile(jar)
	if err != nil {
		return check{name: "gobra jar", detail: "absent at " + jar + " -> R4/Go and R5 unavailable"}
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != gobraJarSHA256 {
		return check{name: "gobra jar", detail: fmt.Sprintf(
			"DIGEST MISMATCH at %s: pinned %s..., got %s... -- refusing it; an unpinned Gobra reports different numbers against the same findings",
			jar, gobraJarSHA256[:16], got[:16])}
	}
	// Run it. `--help` is the cheapest invocation that makes the jar load and
	// print its own version banner; the exit status is ignored on purpose.
	out, _ := exec.Command("java", "-Xss128m", "-jar", jar, "--help").CombinedOutput()
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.Contains(ln, "Gobra") {
			return check{name: "gobra jar", detail: strings.TrimSpace(ln) + "  (digest matches pin)", ok: true}
		}
	}
	return check{name: "gobra jar", detail: "digest matches the pin but the jar did not run: " +
		strings.TrimSpace(firstLine(string(out)))}
}

func firstLine(s string) string {
	return versionLine(s)
}

func checkCmd(name string, fatal bool, argv ...string) check {
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return check{name: name, detail: "not available: " + err.Error(), fatal: fatal}
	}
	return check{name: name, detail: versionLine(string(out)), ok: true, fatal: fatal}
}

// versionLine picks the first line of a tool's output that is actually about
// the tool. Every JVM here prints a "Picked up JAVA_TOOL_OPTIONS: ..." banner
// first when the environment sets it -- several hundred characters of proxy
// configuration that pushed the real version line off the report and made
// `java` and `kotlinc` indistinguishable from each other.
func versionLine(out string) string {
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Picked up JAVA_TOOL_OPTIONS") ||
			strings.HasPrefix(ln, "Picked up _JAVA_OPTIONS") {
			continue
		}
		return ln
	}
	return "(no output)"
}

// verusBinary resolves the Verus binary. Hardcoding an absolute macOS path
// made this repository unrunnable anywhere else; VERUS_PATH overrides it so a
// Linux container can point at its own build.
func verusBinary() string {
	if p := os.Getenv("VERUS_PATH"); p != "" {
		return p
	}
	if p, err := exec.LookPath("verus"); err == nil {
		return p
	}
	return defaultVerusPath
}

func checkVerus() check {
	verusPath := verusBinary()
	if _, err := os.Stat(verusPath); err != nil {
		return check{name: "verus", detail: "absent at " + verusPath + " (needed from Phase 1)", fatal: false}
	}
	out, err := exec.Command(verusPath, "--version").CombinedOutput()
	if err != nil {
		return check{name: "verus", detail: "present but not runnable: " + err.Error(), fatal: false}
	}
	v := ""
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.Contains(ln, "Version:") {
			v = strings.TrimSpace(ln)
		}
	}
	return check{name: "verus", detail: v + "  (not on PATH; invoke by absolute path)", ok: true}
}

func checkJar() check {
	b, err := os.ReadFile(tlaJarPath)
	if err != nil {
		return check{name: "tla2tools.jar", detail: "missing: " + err.Error(), fatal: true}
	}
	sum := sha256.Sum256(b)
	got := hex.EncodeToString(sum[:])
	if got != tlaJarSHA256 {
		return check{
			name:   "tla2tools.jar",
			detail: fmt.Sprintf("DIGEST MISMATCH\n        pinned %s\n        actual %s", tlaJarSHA256, got),
			fatal:  true,
		}
	}
	return check{name: "tla2tools.jar", detail: "digest matches pin (" + got[:16] + "...)", ok: true, fatal: true}
}

// checkVendoredSpec confirms the vendored twitter.tla still matches the digest
// recorded when it was extracted at the pinned SHA. The vendored copy is
// read-only input; a change here silently invalidates every R3 result.
func checkVendoredSpec() check {
	sums, err := os.ReadFile("spec/tla/SHA256SUMS")
	if err != nil {
		return check{name: "vendored spec", detail: "SHA256SUMS missing", fatal: true}
	}
	var bad []string
	n := 0
	for _, ln := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		f := strings.Fields(ln)
		if len(f) != 2 {
			continue
		}
		n++
		b, err := os.ReadFile(filepath.Join("spec/tla", f[1]))
		if err != nil {
			bad = append(bad, f[1]+" missing")
			continue
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != f[0] {
			bad = append(bad, f[1]+" MODIFIED")
		}
	}
	if len(bad) > 0 {
		return check{name: "vendored spec", detail: strings.Join(bad, ", "), fatal: true}
	}
	return check{name: "vendored spec", detail: fmt.Sprintf("%d file(s) match the pinned SHA", n), ok: true, fatal: true}
}

// checkIsolation enforces the correlated-failure rule: no implementation may
// import the reference machine. S_obs is written in Go and Go is one of the
// four targets, so without this the Go corner could pass every differential
// rung by construction rather than by agreement.
func checkIsolation() check {
	var offenders []string
	err := filepath.Walk("impls", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(b), sobsImport) {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		return check{name: "impl isolation", detail: err.Error(), fatal: true}
	}
	if len(offenders) > 0 {
		return check{
			name:   "impl isolation",
			detail: "implementations import S_obs: " + strings.Join(offenders, ", "),
			fatal:  true,
		}
	}
	return check{name: "impl isolation", detail: "no implementation imports S_obs", ok: true, fatal: true}
}
