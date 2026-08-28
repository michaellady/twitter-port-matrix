package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// implsCheck is the R0 gate across every registered implementation.
//
// Two halves, and the second is not optional: every implementation must pass
// R0, and R0 must be shown capable of failing against each one. A conformance
// suite that cannot go red is not evidence.
func implsCheck() error {
	fmt.Println("matrixctl impls check")
	fmt.Println(strings.Repeat("=", 72))

	b, err := os.ReadFile("impls/registry.json")
	if err != nil {
		return err
	}
	var reg struct {
		Impls map[string]struct {
			Language string `json:"language"`
		} `json:"impls"`
	}
	if err := json.Unmarshal(b, &reg); err != nil {
		return err
	}
	var names []string
	for k := range reg.Impls {
		names = append(names, k)
	}
	sort.Strings(names)

	var failed []string
	for _, n := range names {
		fmt.Printf("\n[%s / %s]\n%s\n", n, reg.Impls[n].Language, strings.Repeat("-", 72))

		out, _ := exec.Command("go", "run", "./tools/cmd/replay", "-impl="+n, "-max-diffs=4").CombinedOutput()
		line := grepLine(string(out), "R0 result:")
		if !strings.Contains(string(out), "R0 PASSED") {
			fmt.Printf("  R0   FAIL  %s\n", line)
			failed = append(failed, n+" (R0)")
			for _, l := range strings.Split(string(out), "\n") {
				if strings.Contains(l, "DIFF ") || strings.HasPrefix(strings.TrimSpace(l), "expected ") || strings.HasPrefix(strings.TrimSpace(l), "got ") {
					fmt.Printf("       %s\n", strings.TrimSpace(l))
				}
			}
		} else {
			fmt.Printf("  R0   ok    %s\n", line)
		}

		out, _ = exec.Command("go", "run", "./tools/cmd/replay", "-impl="+n, "-canary=all").CombinedOutput()
		if strings.Contains(string(out), "R0 IS BLIND TO") {
			fmt.Printf("  canary FAIL  %s\n", grepLine(string(out), "R0 IS BLIND TO"))
			failed = append(failed, n+" (canary)")
		} else {
			fmt.Printf("  canary ok    %s\n", grepLine(string(out), "canaries rejected"))
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 72))
	if len(failed) > 0 {
		return fmt.Errorf("failed: %s", strings.Join(failed, ", "))
	}
	fmt.Printf("impls check: %d implementation(s) pass R0, and R0 can fail against each\n", len(names))
	return nil
}

func grepLine(text, prefix string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, prefix) {
			return strings.TrimSpace(l)
		}
	}
	return ""
}
