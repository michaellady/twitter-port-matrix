package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
)

// errR4Failed is the exit-code half of a FAILED verdict. The verdict itself is
// the "R4 FAILED" line on stdout; `calibrate` reads that line and then requires
// the exit code to agree with it, so both have to be produced together. Copied
// deliberately from tools/cmd/gobra/verify.go: the two corners' R4 cells are
// only comparable if the contract calibrate reads is identical.
var errR4Failed = errors.New("R4 FAILED")

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	impl := fs.String("impl", "impls/rust", "the Rust implementation directory; with -registry, a registry entry name instead")
	registry := fs.String("registry", "", "implementation registry; when set, -impl names an entry (rust, or rust@<id> from `mutate apply`) and the directory is read from it")
	budget := fs.Duration("budget", 20*time.Minute, "time budget per run; a run that exceeds it is UNDECIDED, never a pass")
	if err := fs.Parse(args); err != nil {
		return err
	}
	implDir, err := resolveImplDir(*impl, *registry)
	if err != nil {
		return err
	}
	crates, err := verifyEnabledCrates(implDir)
	if err != nil {
		return err
	}

	fmt.Printf("Verus over %s\n", implDir)
	fmt.Printf("  verify-enabled crates: ")
	for i, c := range crates {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(c.Name)
	}
	fmt.Println()

	res, err := runVerus(implDir, crates, *budget)
	if errors.Is(err, errTimeout) {
		// Not a pass and not a failure. A budget that runs out leaves the
		// tree undecided, and an undecided tree recorded as a survival would
		// be the strongest possible false green: it would read as "the proof
		// looked and found nothing wrong".
		fmt.Printf("R4 UNDECIDED: Verus exceeded its %s budget; nothing was decided about this tree\n", *budget)
		if res != nil {
			fmt.Println(tailLines(res.Raw, 8))
		}
		return err
	}
	if err != nil {
		return err
	}

	fmt.Println("Verus's own result lines:")
	for _, c := range res.Reported {
		fmt.Printf("  %-10s verification results:: %d verified, %d errors\n", c.Crate, c.Verified, c.Errors)
	}
	if m := res.missing(); len(m) > 0 {
		for _, c := range m {
			fmt.Printf("  %-10s (no result line: not reached by this run)\n", c)
		}
	}
	fmt.Println()

	line, killed, err := res.verdict()
	if err != nil {
		// No verdict is printed at all in this case, on purpose. calibrate
		// counts verdict lines and turns "none" into an error cell.
		return err
	}
	fmt.Println(line)
	if killed {
		return errR4Failed
	}
	return nil
}

// resolveImplDir turns -impl into a directory. With -registry it goes through
// the same registry the other rungs use, so the tree Verus reads is the tree
// calibrate's guard hashed -- resolved by name, exactly as replay and diffrun
// resolve theirs, and exactly as tools/cmd/gobra does on the Go corner.
func resolveImplDir(impl, registry string) (string, error) {
	if registry == "" {
		return implDirFrom(impl)
	}
	reg, err := implrun.LoadRegistry(registry)
	if err != nil {
		return "", fmt.Errorf("reading registry %s: %w", registry, err)
	}
	spec, err := reg.Get(impl)
	if err != nil {
		return "", err
	}
	return implDirFrom(spec.Dir)
}

func implDirFrom(dir string) (string, error) {
	if dir == "" {
		dir = "impls/rust"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "Cargo.toml")); err != nil {
		return "", fmt.Errorf("%s does not look like the Rust implementation (no Cargo.toml)", abs)
	}
	return abs, nil
}
