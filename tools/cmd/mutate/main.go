// Command mutate injects semantic defects into a copy of an implementation.
//
// It is the input side of the deliverable. The kill table asks, for each
// verification rung, what fraction of injected defects it catches and what that
// costs; this tool produces the defects and never judges them. `calibrate`
// (S-15) runs the rungs.
//
// Four things this tool is deliberately built around:
//
//   - The catalogue is DATA (mutants.json), not Go control flow. Adding the
//     Java and Kotlin corners must be a manifest edit, not a code change.
//
//   - Nothing is ever written inside impls/. A mutant is a copy. The rig judges
//     implementations, and an injector able to modify the thing it measures is
//     one bad path join away from reproducing the exact defect this repository
//     exists to study.
//
//   - Every mutant is content-addressed, so a kill recorded against a hash is
//     reproducible and source drift is visible instead of silent.
//
//   - A mutant that does not change observable behaviour is worse than useless:
//     no rung can kill it, so it drags down every rung's measured kill rate for
//     a reason that has nothing to do with the rung. `probe` detects that case
//     and reports it as a failure.
//
// Subcommands:
//
//	list     print the catalogue
//	apply    materialise one mutant into a scratch directory
//	verify   every anchor still matches exactly one site, and every mutant still compiles
//	probe    the mutant actually changes observable behaviour
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
)

const (
	defaultManifest = "tools/cmd/mutate/mutants.json"
	defaultRegistry = "impls/registry.json"
	defaultCorpus   = "generated/conformance.jsonl"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "list":
		os.Exit(cmdList(os.Args[2:]))
	case "apply":
		os.Exit(cmdApply(os.Args[2:]))
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "probe":
		os.Exit(cmdProbe(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "mutate: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mutate -- semantic mutant injection for the kill table

  mutate list   [-impl X] [-family F] [-hashes] [-v]
        print the catalogue

  mutate apply  -impl X -id Y -out DIR [-force]
        write a mutated copy of X into DIR/tree, with a registry.json the
        other tools can be pointed at

  mutate verify [-impl X] [-id Y] [-build=false]
        every anchor still matches exactly one site in the current source, and
        every mutant still compiles. Anchor drift is the quiet failure: a
        mutant that no longer applies looks exactly like a defect every rung
        caught

  mutate probe  -impl X [-id Y] [-traces N] [-steps M]
        run the mutant and the unmutated implementation side by side and find a
        request where they answer differently. A mutant with no such request is
        equivalent and must not be counted

`)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "mutate: "+format+"\n", a...)
	os.Exit(1)
}

func rule() string { return strings.Repeat("=", 72) }

// loadAll reads the two inputs every subcommand needs.
func loadAll(manifestPath, registryPath string) (*mutants.Manifest, implrun.Registry) {
	man, err := mutants.Load(manifestPath)
	if err != nil {
		die("loading manifest: %v", err)
	}
	reg, err := implrun.LoadRegistry(registryPath)
	if err != nil {
		die("loading registry: %v", err)
	}
	return man, reg
}

// corpusRequests reads the R0 corpus as a plain request sequence.
//
// Only the requests are used: probe compares a mutant against the unmutated
// implementation, so the corpus's expected responses are not the oracle here.
// It is used because it is the one trace guaranteed to exercise every decision
// in DECISIONS.md at least once.
func corpusRequests(path string) ([]request, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []request
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var step struct {
			Request request `json:"request"`
		}
		if err := json.Unmarshal(sc.Bytes(), &step); err != nil {
			return nil, err
		}
		out = append(out, step.Request)
	}
	return out, sc.Err()
}

type request struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}
