package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	sobs "github.com/michaellady/twitter-port-matrix/spec/s_obs"
	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
	"github.com/michaellady/twitter-port-matrix/tools/internal/mutants"
	"github.com/michaellady/twitter-port-matrix/tools/internal/tracegen"
)

// cmdProbe answers the one question compiling cannot: does this mutant change
// what the implementation DOES?
//
// An equivalent mutant -- one that compiles, applies cleanly, and behaves
// identically -- is worse than no mutant at all. No rung can kill it, so it
// counts as a survivor everywhere and drags every rung's measured kill rate
// down for a reason that has nothing to do with the rung. A calibration run
// containing equivalent mutants reports a number that looks like evidence and
// is not.
//
// The comparison is against the UNMUTATED implementation, not against S_obs.
// "Differs from the spec" and "differs from the original" are not the same
// question, and only the second one establishes that the mutation did
// something. S_obs is stepped alongside anyway, because knowing whether the
// original also disagreed at that request is exactly the context needed to
// read the result.
func cmdProbe(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	manifestPath := fs.String("manifest", defaultManifest, "mutant manifest")
	registryPath := fs.String("registry", defaultRegistry, "implementation registry")
	corpusPath := fs.String("corpus", defaultCorpus, "R0 corpus, used as the first probe trace")
	implName := fs.String("impl", "", "restrict to one corner")
	id := fs.String("id", "", "restrict to one mutant id")
	family := fs.String("family", "", "restrict to one family")
	traces := fs.Int("traces", 4, "randomized traces to try after the corpus")
	steps := fs.Int("steps", 120, "requests per randomized trace")
	seed0 := fs.Int64("seed", 1, "base seed; trace i uses seed+i")
	keep := fs.Bool("keep", false, "keep the mutant trees")
	first := fs.Bool("first", false, "stop at the first trace that finds a difference (faster; loses the reachability report)")
	_ = fs.Parse(args)

	man, reg := loadAll(*manifestPath, *registryPath)
	sel := man.Select(*implName, *id, *family)
	if len(sel) == 0 {
		die("no mutants match impl=%q id=%q family=%q", *implName, *id, *family)
	}

	probes := buildProbes(*corpusPath, *traces, *steps, *seed0)
	total := 0
	for _, p := range probes {
		total += len(p.reqs)
	}
	fmt.Printf("mutate probe: %d mutant(s) against %d probe traces (%d requests each pass)\n",
		len(sel), len(probes), total)
	fmt.Println(rule())

	baselines := map[string]*implrun.Build{}
	defer func() {
		for _, b := range baselines {
			b.Close()
		}
	}()

	live, equivalent := 0, 0
	var suspects []string

	for _, m := range sel {
		spec, err := reg.Get(m.Impl)
		if err != nil {
			die("%v", err)
		}
		if _, ok := baselines[m.Impl]; !ok {
			b, err := implrun.Compile(spec)
			if err != nil {
				die("the unmutated %s corner does not build, so nothing can be compared against it: %v\n%s",
					m.Impl, err, b.Output)
			}
			baselines[m.Impl] = b
		}

		fmt.Printf("\n%s  (%s)\n", m.Key(), m.Family)
		reachedBy, checked, ok := probeOne(m, spec, baselines[m.Impl], probes, *first, *keep)
		if !ok {
			suspects = append(suspects, m.Key()+" (does not compile)")
			continue
		}
		if len(reachedBy) > 0 {
			live++
			fmt.Printf("  verdict   LIVE -- changes observable behaviour (%s)\n", strings.Join(reachedBy, ", "))
			if len(reachedBy) == 1 && reachedBy[0] == "witness" && !*first {
				fmt.Printf("            REACHED ONLY BY ITS OWN WITNESS. Neither the R0 corpus nor\n")
				fmt.Printf("            %d tracegen traces produce a request that tells it apart, so\n", *traces)
				fmt.Printf("            no rung driven by those inputs can kill it. That is a fact\n")
				fmt.Printf("            about the inputs, not about the rungs.\n")
			}
			continue
		}
		equivalent++
		suspects = append(suspects, m.Key())
		fmt.Printf("  verdict   NO OBSERVABLE CHANGE in %d requests\n", checked)
		if len(m.Witness) == 0 {
			fmt.Printf("            No witness is declared for this mutant, so this is a failed\n")
			fmt.Printf("            search, not a proof of equivalence. Add one, or drop it.\n")
		}
		fmt.Printf("            Treat as equivalent until shown otherwise. Counting it would\n")
		fmt.Printf("            record a survivor at every rung and understate all of them.\n")
	}

	fmt.Println("\n" + rule())
	fmt.Printf("live: %d/%d   no observable change: %d\n", live, len(sel), equivalent)
	if len(suspects) > 0 {
		fmt.Printf("\nprobe FAILED: %v\n", suspects)
		return 1
	}
	fmt.Println("\nprobe PASSED: every mutant answers some request differently from the original")
	return 0
}

// probeOne materialises, compiles and probes a single mutant, releasing its
// scratch tree before returning. It reports which traces reached the defect
// and how many requests were compared.
//
// Every trace is run rather than stopping at the first difference. WHICH
// inputs reach a defect is the interesting half: a mutant only its own witness
// distinguishes is live, but no rung driven by the corpus or by tracegen can
// ever kill it, and a kill table that does not say so reads as if those rungs
// were weak rather than blind.
func probeOne(m mutants.Mutant, spec implrun.Spec, base *implrun.Build, probes []probeTrace, first, keep bool) ([]string, int, bool) {
	start := time.Now()
	out, err := os.MkdirTemp("", "mutate-probe-")
	if err != nil {
		die("scratch dir: %v", err)
	}
	if !keep {
		defer os.RemoveAll(out)
	}
	res, err := mutants.Materialize(spec.Dir, m, out)
	if err != nil {
		die("%v", err)
	}
	mspec := spec
	mspec.Dir = res.TreeDir
	mb, err := implrun.Compile(mspec)
	if mb != nil {
		defer mb.Close()
	}
	if err != nil {
		fmt.Printf("  FAIL      mutant does not compile: %v\n", err)
		for _, line := range tailLines(mb.Output, 10) {
			fmt.Printf("            | %s\n", line)
		}
		return nil, 0, false
	}
	fmt.Printf("  tree      %s  (build %.1fs, copy %s)\n",
		short(res.TreeHash), time.Since(start).Seconds(), res.CopyMode)

	var reachedBy []string
	checked := 0
	for _, p := range append(witnessProbe(m), probes...) {
		d := comparePair(base, mb, p)
		checked += d.checked
		if d.diff == nil {
			fmt.Printf("  %-9s no difference in %d requests\n", p.name, d.checked)
			continue
		}
		if len(reachedBy) == 0 {
			reportDifference(p.name, *d.diff)
		} else {
			fmt.Printf("  %-9s also differs, at request %d\n", p.name, d.diff.index)
		}
		reachedBy = append(reachedBy, p.name)
		if first {
			break
		}
	}
	return reachedBy, checked, true
}

type probeTrace struct {
	name string
	reqs []request
}

// witnessProbe turns a mutant's declared witness into a probe trace. It runs
// before the shared traces because it is short, targeted, and -- unlike random
// search -- is a claim someone wrote down and can be held to.
func witnessProbe(m mutants.Mutant) []probeTrace {
	if len(m.Witness) == 0 {
		return nil
	}
	reqs := make([]request, 0, len(m.Witness))
	for _, w := range m.Witness {
		reqs = append(reqs, request{Method: w.Method, Path: w.Path, Body: w.Body})
	}
	return []probeTrace{{name: "witness", reqs: reqs}}
}

func buildProbes(corpusPath string, traces, steps int, seed0 int64) []probeTrace {
	var out []probeTrace
	// The corpus goes first: it is the one trace guaranteed to touch every
	// decision in DECISIONS.md, and a difference there is the cheapest and
	// most readable evidence a mutant is live.
	if reqs, err := corpusRequests(corpusPath); err == nil && len(reqs) > 0 {
		out = append(out, probeTrace{name: "corpus", reqs: reqs})
	} else if err != nil {
		fmt.Printf("note: corpus unavailable (%v); probing with random traces only\n", err)
	}
	for i := 0; i < traces; i++ {
		seed := seed0 + int64(i)
		gen := tracegen.Generate(tracegen.DefaultConfig(seed, steps))
		reqs := make([]request, 0, len(gen))
		for _, g := range gen {
			reqs = append(reqs, request{Method: g.Method, Path: g.Path, Body: g.Body})
		}
		out = append(out, probeTrace{name: fmt.Sprintf("seed=%d", seed), reqs: reqs})
	}
	return out
}

type difference struct {
	index      int
	req        request
	baseStatus int
	baseBody   string
	mutStatus  int
	mutBody    string
	specStatus int
	specBody   string
}

type compareResult struct {
	checked int
	diff    *difference
}

// comparePair replays one trace against a fresh baseline process and a fresh
// mutant process, stepping S_obs alongside, and stops at the first request
// where the two processes answer differently.
//
// Fresh processes per trace: state must not leak between traces, or a
// difference cannot be attributed to the trace that found it.
func comparePair(base, mut *implrun.Build, p probeTrace) compareResult {
	bh, err := base.Start()
	if err != nil {
		die("starting the unmutated implementation: %v", err)
	}
	defer bh.Stop()
	mh, err := mut.Start()
	if err != nil {
		// A mutant that cannot start is a behaviour change of the loudest
		// kind, but it is also indistinguishable from a rig fault, so it is
		// reported rather than silently counted as a kill.
		fmt.Printf("  %-9s mutant process failed to start: %v\n", p.name, err)
		return compareResult{}
	}
	defer mh.Stop()

	st := sobs.Init()
	for i, r := range p.reqs {
		want, next := sobs.Step(st, sobs.Request{Method: r.Method, Path: r.Path, Body: r.Body})
		st = next

		bs, bb, berr := bh.Do(r.Method, r.Path, r.Body)
		if berr != nil {
			die("the unmutated implementation failed on %s %s: %v", r.Method, r.Path, berr)
		}
		ms, mb, merr := mh.Do(r.Method, r.Path, r.Body)
		if merr != nil {
			return compareResult{checked: i + 1, diff: &difference{
				index: i, req: r,
				baseStatus: bs, baseBody: bb,
				mutStatus: 0, mutBody: "transport: " + merr.Error(),
				specStatus: want.Status, specBody: want.Body,
			}}
		}
		if bs != ms || bb != mb {
			return compareResult{checked: i + 1, diff: &difference{
				index: i, req: r,
				baseStatus: bs, baseBody: bb,
				mutStatus: ms, mutBody: mb,
				specStatus: want.Status, specBody: want.Body,
			}}
		}
	}
	return compareResult{checked: len(p.reqs)}
}

func reportDifference(traceName string, d difference) {
	fmt.Printf("  %-9s DIFFERS at request %d\n", traceName, d.index)
	fmt.Printf("            request   %s %s %s\n", d.req.Method, d.req.Path, d.req.Body)
	fmt.Printf("            original  %d %s\n", d.baseStatus, trunc(d.baseBody))
	fmt.Printf("            mutant    %d %s\n", d.mutStatus, trunc(d.mutBody))

	baseAgrees := d.baseStatus == d.specStatus && d.baseBody == d.specBody
	mutAgrees := d.mutStatus == d.specStatus && d.mutBody == d.specBody
	switch {
	case baseAgrees && !mutAgrees:
		fmt.Printf("            S_obs     agrees with the original; the mutant diverges from the spec\n")
	case !baseAgrees && !mutAgrees:
		fmt.Printf("            S_obs     %d %s\n", d.specStatus, trunc(d.specBody))
		fmt.Printf("                      NOTE: the UNMUTATED corner also disagrees with S_obs here.\n")
		fmt.Printf("                      That is a finding about the implementation, not the mutant.\n")
	case !baseAgrees && mutAgrees:
		fmt.Printf("            S_obs     agrees with the MUTANT, not the original. Read this one\n")
		fmt.Printf("                      before using it: the corner has drifted from the spec.\n")
	default:
		fmt.Printf("            S_obs     agrees with both, which cannot happen if they differ\n")
	}
}

func trunc(s string) string {
	const n = 160
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("... (%d bytes)", len(s))
}
