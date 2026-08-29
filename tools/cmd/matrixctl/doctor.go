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
	cs = append(cs, checkCmd("docker daemon", false, "docker", "info", "--format", "{{.ServerVersion}}"))
	cs = append(cs, checkVerus())
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
	if failed > 0 {
		return fmt.Errorf("%d fatal check(s) failed", failed)
	}
	fmt.Println("doctor: all fatal checks passed")
	return nil
}

func checkCmd(name string, fatal bool, argv ...string) check {
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
	if err != nil {
		return check{name: name, detail: "not available: " + err.Error(), fatal: fatal}
	}
	return check{name: name, detail: line, ok: true, fatal: fatal}
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
