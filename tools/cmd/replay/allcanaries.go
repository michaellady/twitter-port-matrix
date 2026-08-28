package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runAllCanaries runs every mutation in turn and requires each to break R0.
//
// This is what makes R0's falsifiability a gate rather than a habit. Each
// mutation targets a different class of defect, so a mutation that fails to
// break R0 names a blind spot precisely rather than reporting a vague
// weakness.
func runAllCanaries(impl, corpus, registry string) int {
	fmt.Printf("replay: canary sweep -- impl=%s, %d mutations\n", impl, len(mutations))
	fmt.Println(strings.Repeat("=", 72))

	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	var blind []string
	for _, name := range mutationNames() {
		cmd := exec.Command(self,
			"-impl="+impl, "-corpus="+corpus, "-registry="+registry,
			"-canary="+name, "-max-diffs=0")
		out, _ := cmd.CombinedOutput()
		text := string(out)
		detected := strings.Contains(text, "correctly rejected")
		mark := "ok  "
		if !detected {
			mark = "BLIND"
			blind = append(blind, name)
		}
		var n string
		for _, ln := range strings.Split(text, "\n") {
			if strings.HasPrefix(ln, "R0 result:") {
				n = strings.TrimPrefix(ln, "R0 result: ")
			}
		}
		fmt.Printf("  %-5s %-8s %-46s %s\n", mark, name, mutations[name].desc, n)
	}
	fmt.Println(strings.Repeat("=", 72))
	if len(blind) > 0 {
		fmt.Printf("R0 IS BLIND TO: %s\n", strings.Join(blind, ", "))
		fmt.Println("A green R0 run says nothing about those defect classes.")
		return 1
	}
	fmt.Printf("all %d canaries rejected: R0 detects every mutation class tested\n", len(mutations))
	return 0
}
