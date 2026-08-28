package main

import (
	"fmt"
	"sort"
	"strings"
)

// report prints the R0 result and returns the process exit code.
//
// The per-kind breakdown is the useful part. "31 of 54 steps differ" says
// almost nothing; "22 of them are one absent route" says what to do next.
func report(rs []result, maxShow int) int {
	var match, trim, diff int
	byKind := map[string]int{}
	byCovers := map[string][2]int{} // covers tag -> [passing, total]

	for _, r := range rs {
		switch r.verdict {
		case vMatch:
			match++
		case vTrim:
			trim++
		default:
			diff++
			byKind[r.kind]++
		}
		for _, c := range r.step.Covers {
			v := byCovers[c]
			v[1]++
			if r.verdict != vDiff {
				v[0]++
			}
			byCovers[c] = v
		}
	}

	shown := 0
	for _, r := range rs {
		if r.verdict != vDiff {
			continue
		}
		if shown >= maxShow {
			fmt.Printf("\n  ... %d further differing steps not shown\n", diff-shown)
			break
		}
		shown++
		fmt.Printf("\n  DIFF %2d %s   [%s]\n", r.step.Step, r.step.Name, r.kind)
		fmt.Printf("       request  %s %s %s\n", r.step.Request.Method, r.step.Request.Path, r.step.Request.Body)
		if r.transport != nil {
			fmt.Printf("       error    %v\n", r.transport)
			continue
		}
		fmt.Printf("       expected %d %s\n", r.step.Expected.Status, r.step.Expected.Body)
		fmt.Printf("       got      %d %s\n", r.gotStatus, r.gotBody)
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 72))
	fmt.Printf("R0 result: %d/%d exact, %d whitespace-only, %d differ\n",
		match, len(rs), trim, diff)

	if len(byKind) > 0 {
		fmt.Println("\n  gap by kind:")
		type kv struct {
			k string
			n int
		}
		var ks []kv
		for k, n := range byKind {
			ks = append(ks, kv{k, n})
		}
		sort.Slice(ks, func(i, j int) bool {
			if ks[i].n != ks[j].n {
				return ks[i].n > ks[j].n
			}
			return ks[i].k < ks[j].k
		})
		for _, e := range ks {
			fmt.Printf("    %3d  %s\n", e.n, e.k)
		}
	}

	if len(byCovers) > 0 {
		fmt.Println("\n  by property / decision:")
		var tags []string
		for t := range byCovers {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		for _, t := range tags {
			v := byCovers[t]
			mark := " "
			if v[0] < v[1] {
				mark = "*"
			}
			fmt.Printf("    %s %-4s %d/%d\n", mark, t, v[0], v[1])
		}
	}

	fmt.Println(strings.Repeat("=", 72))
	if diff > 0 {
		fmt.Printf("R0 FAILED: %d step(s) disagree with S_obs\n", diff)
		return 1
	}
	if trim > 0 {
		fmt.Printf("R0 FAILED: %d step(s) match only after trimming; D8 requires byte equality\n", trim)
		return 1
	}
	fmt.Println("R0 PASSED: every step matches S_obs byte-for-byte")
	return 0
}
