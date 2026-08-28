package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	manifestPath := fs.String("manifest", defaultManifest, "mutant manifest")
	registryPath := fs.String("registry", defaultRegistry, "implementation registry")
	implName := fs.String("impl", "", "restrict to one corner")
	family := fs.String("family", "", "restrict to one family")
	verbose := fs.Bool("v", false, "print the edit sites too")
	hashes := fs.Bool("hashes", false, "compute each mutant's content address (reads the implementation sources)")
	_ = fs.Parse(args)

	man, reg := loadAll(*manifestPath, *registryPath)
	sel := man.Select(*implName, "", *family)
	if len(sel) == 0 {
		die("no mutants match impl=%q family=%q", *implName, *family)
	}

	// Group by defect id so the catalogue reads as "one defect, N corners".
	// That is the shape the kill table needs: a defect present in only one
	// corner cannot be compared across a port.
	type defect struct {
		id      string
		family  string
		desc    string
		corners []mutants.Mutant
	}
	byFamily := map[string][]*defect{}
	index := map[string]*defect{}
	for _, m := range sel {
		d, ok := index[m.Family+"/"+m.ID]
		if !ok {
			d = &defect{id: m.ID, family: m.Family, desc: m.Description}
			index[m.Family+"/"+m.ID] = d
			byFamily[m.Family] = append(byFamily[m.Family], d)
		}
		d.corners = append(d.corners, m)
	}

	corners := man.Impls()
	fmt.Printf("mutate list: %s\n", *manifestPath)
	fmt.Printf("             %d mutants -- %d defects across %d families, corners: %s\n",
		len(sel), len(index), len(byFamily), strings.Join(corners, ", "))
	fmt.Println(rule())

	for _, f := range man.FamilyNames() {
		ds := byFamily[f]
		if len(ds) == 0 {
			continue
		}
		fmt.Printf("\n%s -- %s\n", f, man.Families[f])
		sort.Slice(ds, func(i, j int) bool { return ds[i].id < ds[j].id })
		for _, d := range ds {
			var names []string
			for _, c := range d.corners {
				names = append(names, c.Impl)
			}
			sort.Strings(names)
			fmt.Printf("  %-32s [%s]\n", d.id, strings.Join(names, " "))
			for _, line := range wrap(d.desc, 68) {
				fmt.Printf("      %s\n", line)
			}
			if *verbose {
				for _, c := range d.corners {
					for _, e := range c.Edits {
						fmt.Printf("      %-6s %s  (%d anchor lines)\n",
							c.Impl, e.File, 1+strings.Count(e.Anchor.String(), "\n"))
					}
				}
			}
			if *hashes {
				for _, c := range d.corners {
					printHash(reg, c)
				}
			}
		}
	}

	fmt.Println("\n" + rule())
	fmt.Println("by family:")
	for _, f := range man.FamilyNames() {
		n := len(man.Select(*implName, "", f))
		if n > 0 {
			fmt.Printf("  %3d  %s\n", n, f)
		}
	}
	fmt.Println("by corner:")
	for _, c := range corners {
		if n := len(man.Select(c, "", *family)); n > 0 {
			fmt.Printf("  %3d  %s\n", n, c)
		}
	}
	return 0
}

// printHash computes what a mutant tree WOULD hash to, without writing
// anything. Two runs of `mutate apply` must agree with this and with each
// other; that is the reproducibility claim.
func printHash(reg implrun.Registry, m mutants.Mutant) {
	spec, err := reg.Get(m.Impl)
	if err != nil {
		fmt.Printf("      %-6s hash unavailable: %v\n", m.Impl, err)
		return
	}
	base, mut, err := mutants.PlanHash(spec.Dir, m)
	if err != nil {
		fmt.Printf("      %-6s hash unavailable: %v\n", m.Impl, err)
		return
	}
	fmt.Printf("      %-6s base %s\n", m.Impl, short(base))
	fmt.Printf("      %-6s mut  %s\n", "", short(mut))
}

func short(h string) string {
	if len(h) > 7+16 {
		return h[:7+16]
	}
	return h
}

// wrap breaks a description onto lines of at most n columns. Descriptions
// carry the reason a mutant exists, and a reason nobody can read is a reason
// nobody checks.
func wrap(s string, n int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var out []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > n {
			out = append(out, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	return append(out, cur)
}
