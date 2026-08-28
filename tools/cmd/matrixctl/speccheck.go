package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// specCheck is the Phase 0 gate. Four sub-gates, each read from the tool's own
// output:
//
//  1. corpus determinism -- regenerating from S_obs reproduces the committed
//     bytes exactly. Guards against the corpus and the spec drifting apart,
//     which is the defect recorded in evidence/findings/F001.
//  2. model check -- TLC finds no violation of F1-F9 on twitter.tla.
//  3. link -- every S_obs transition is a legal twitter.tla step.
//  4. canary -- a deliberately corrupted trace is REJECTED by the link check.
//
// Sub-gate 4 is not optional. Without it, sub-gate 3 could be passing because
// the check is incapable of failing.
func specCheck() error {
	fmt.Println("matrixctl spec check")
	fmt.Println(strings.Repeat("=", 72))

	if err := gateCorpusDeterminism(); err != nil {
		return err
	}
	if err := gateModelCheck(); err != nil {
		return err
	}
	if err := gateLink(); err != nil {
		return err
	}
	if err := gateCanary(); err != nil {
		return err
	}

	fmt.Println(strings.Repeat("=", 72))
	fmt.Println("spec check: ALL GATES PASSED")
	return nil
}

func banner(n int, name string) {
	fmt.Printf("\n[%d/4] %s\n%s\n", n, name, strings.Repeat("-", 72))
}

func gateCorpusDeterminism() error {
	banner(1, "corpus determinism")
	const committed = "generated/conformance.jsonl"

	want, err := os.ReadFile(committed)
	if err != nil {
		return fmt.Errorf("reading committed corpus: %w", err)
	}

	tmp, err := os.CreateTemp("", "corpus-*.jsonl")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	out, err := exec.Command("go", "run", "./tools/cmd/corpusgen", "-mode=emit", "-out="+tmp.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("corpusgen failed: %v\n%s", err, out)
	}
	fmt.Printf("  %s", out)

	got, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("REGENERATED CORPUS DIFFERS FROM THE COMMITTED ONE\n"+
			"  committed %d bytes, regenerated %d bytes\n"+
			"  the corpus is stale: run corpusgen -mode=emit -out=%s and review the diff",
			len(want), len(got), committed)
	}
	fmt.Printf("  ok: regeneration reproduces %s byte-for-byte (%d bytes, %d steps)\n",
		committed, len(want), bytes.Count(want, []byte("\n")))
	return nil
}

func gateModelCheck() error {
	banner(2, "model check (TLC on twitter.tla)")

	work, err := os.MkdirTemp("", "tlc-model-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	for _, f := range []string{"twitter.tla", "twitter.cfg"} {
		b, err := os.ReadFile(filepath.Join("spec/tla", f))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, f), b, 0o644); err != nil {
			return err
		}
	}
	cfg, _ := os.ReadFile(filepath.Join("spec/tla", "twitter.cfg"))
	fmt.Printf("  bound: %s\n", strings.Join(strings.Fields(string(cfg)), " "))

	jar, err := filepath.Abs(tlaJarPath)
	if err != nil {
		return err
	}
	cmd := exec.Command("java", "-XX:+UseParallelGC", "-cp", jar, "tlc2.TLC",
		"-workers", "auto", "-cleanup", "twitter")
	cmd.Dir = work
	out, _ := cmd.CombinedOutput()
	text := string(out)

	for _, ln := range strings.Split(text, "\n") {
		t := strings.TrimSpace(ln)
		if strings.Contains(t, "distinct states found") ||
			strings.Contains(t, "Model checking completed") ||
			strings.Contains(t, "depth of the complete state graph") ||
			strings.HasPrefix(t, "Error:") {
			fmt.Printf("    %s\n", t)
		}
	}
	if !strings.Contains(text, "Model checking completed. No error has been found") {
		return fmt.Errorf("TLC did not report a clean model check")
	}
	fmt.Println("  ok: F1-F9 hold on the abstract model at this bound")
	return nil
}

func gateLink() error {
	banner(3, "S_obs -> twitter.tla link")
	out, err := exec.Command("go", "run", "./tools/cmd/tlclink").CombinedOutput()
	fmt.Printf("%s", indent(string(out)))
	if err != nil {
		return fmt.Errorf("link check failed")
	}
	if !strings.Contains(string(out), "LINK OK") {
		return fmt.Errorf("link check did not report LINK OK")
	}
	return nil
}

func gateCanary() error {
	banner(4, "canary -- the link check must be able to fail")
	out, err := exec.Command("go", "run", "./tools/cmd/tlclink", "-canary").CombinedOutput()
	fmt.Printf("%s", indent(string(out)))
	if err != nil {
		return fmt.Errorf("canary run errored: %v", err)
	}
	if !strings.Contains(string(out), "CANARY correctly rejected") {
		return fmt.Errorf("THE CANARY WAS NOT REJECTED -- gate 3 proves nothing, because the link check cannot fail")
	}
	return nil
}

func indent(s string) string {
	var b strings.Builder
	for _, ln := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("  " + ln + "\n")
	}
	return b.String()
}
