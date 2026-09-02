package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A crate is one workspace member Verus is asked to verify.
type crate struct {
	Name string // [package] name
	Rel  string // path relative to the implementation root, e.g. "crates/domain"
}

// verifyEnabledCrates reads the Rust corner's verification matrix OUT OF THE
// TREE rather than hardcoding it.
//
// The matrix is `[package.metadata.verus] verify = true` in a member's
// Cargo.toml, and it is what makes `crates/server` -- the trusted transport
// shim, the Rust analogue of Go's internal/httpshim -- absent from every
// R4 measurement on this corner (F012, TCB.md). A hardcoded list would go
// quietly stale the day a crate is added or a key removed, and every later
// "N of 5 crates reported" check would be measuring the wrong denominator.
//
// The parse is line-based rather than a TOML library because this repository
// has no third-party dependencies. It reads exactly two things -- the
// [package] name and whether a `verify = true` appears under
// [package.metadata.verus] -- and it stops trusting the section the moment
// another [header] begins, so a `verify = true` under some other table is not
// mistaken for this one.
func verifyEnabledCrates(implDir string) ([]crate, error) {
	entries, err := os.ReadDir(filepath.Join(implDir, "crates"))
	if err != nil {
		return nil, fmt.Errorf("%s does not look like the Rust implementation (no crates/ directory): %w", implDir, err)
	}
	var out []crate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := filepath.Join("crates", e.Name())
		b, err := os.ReadFile(filepath.Join(implDir, rel, "Cargo.toml"))
		if err != nil {
			continue
		}
		name, verify := parseCargoToml(string(b))
		if !verify {
			continue
		}
		if name == "" {
			return nil, fmt.Errorf("%s/Cargo.toml marks verify = true but has no [package] name", rel)
		}
		out = append(out, crate{Name: name, Rel: rel})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no crate under %s/crates carries [package.metadata.verus] verify = true; there is nothing for Verus to verify", implDir)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func parseCargoToml(s string) (name string, verify bool) {
	section := ""
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		if i := strings.Index(t, "#"); i == 0 {
			continue
		}
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = t
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch {
		case section == "[package]" && k == "name":
			name = strings.Trim(v, `"'`)
		case section == "[package.metadata.verus]" && k == "verify" && v == "true":
			verify = true
		}
	}
	return name, verify
}

func cmdCrates(args []string) error {
	fs := flag.NewFlagSet("crates", flag.ContinueOnError)
	impl := fs.String("impl", "impls/rust", "the Rust implementation directory; with -registry, a registry entry name instead")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry (rust, or rust@<id> from `mutate apply`)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}
	cs, err := verifyEnabledCrates(dir)
	if err != nil {
		return err
	}
	fmt.Printf("Verus verification matrix for %s\n", dir)
	for _, c := range cs {
		fmt.Printf("  %-10s %s\n", c.Name, c.Rel)
	}
	fmt.Printf("  %d verify-enabled crate(s); every other member is trusted\n", len(cs))
	return nil
}
