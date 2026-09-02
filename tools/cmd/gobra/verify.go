package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	impl := fs.String("impl", "impls/go", "the Go implementation directory")
	statsDir := fs.String("stats", "", "keep Gobra's report directory here")
	repeat := fs.Int("repeat", 1, "run this many times (the member count is not deterministic)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := implDirFromArgs([]string{"-impl", *impl})
	if err != nil {
		return err
	}
	ws, err := newWorkspace(implDir)
	if err != nil {
		return err
	}
	defer ws.close()

	for n := 1; n <= *repeat; n++ {
		sd := *statsDir
		if sd == "" {
			sd, err = os.MkdirTemp("", "gobra-stats-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(sd)
		}
		res, err := runGobra(ws, verifiedPackages, sd)
		if err != nil {
			return err
		}
		if *repeat > 1 {
			fmt.Printf("--- run %d of %d ---\n", n, *repeat)
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
			return fmt.Errorf("reading Gobra's report: %w", err)
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
	}
	return nil
}
