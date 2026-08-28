// Command tlclink checks that S_obs is a refinement of twitter.tla.
//
// This is rung R3's missing half. The existing artefact set had an abstract
// model checked by TLC, concrete code checked by per-language verifiers, and
// a hand-written corpus bridging them that was never checked against either.
// tlclink closes the top half of that gap mechanically.
//
// Method. Replay the generated corpus through S_obs, project each state onto
// the model's five variables, and drop steps that leave the projection
// unchanged (rejected requests and reads are not TLA+ transitions). Then emit
// a TLA+ module that INSTANCEs the real twitter.tla and forces the behaviour
// to walk exactly the recorded states:
//
//	Next == idx < TraceLen /\ T!Next /\ AtStatePrimed(idx+1) /\ idx' = idx+1
//
// Every step must satisfy the model's own Next AND land on the next recorded
// state. The model is never reimplemented here -- TLC evaluates twitter.tla's
// actual transition relation.
//
// Reading the result. The check asks TLC to prove the final trace index is
// UNREACHABLE. If TLC reports that invariant violated, every transition was a
// legal model step and the link PASSES. If TLC finds no violation, some step
// was not a legal model step and the link FAILS.
//
// That inversion is easy to get wrong, so -canary runs a deliberately
// corrupted trace which MUST fail. A run that cannot fail proves nothing.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sobs "github.com/michaellady/twitter-port-matrix/spec/s_obs"
)

type corpusLine struct {
	Step    int    `json:"step"`
	Name    string `json:"name"`
	Request struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   string `json:"body"`
	} `json:"request"`
}

func main() {
	corpus := flag.String("corpus", "generated/conformance.jsonl", "corpus to replay")
	specDir := flag.String("spec", "spec/tla", "directory holding twitter.tla")
	jar := flag.String("jar", "docker/tlc/tla2tools.jar", "path to tla2tools.jar")
	canary := flag.Bool("canary", false, "corrupt the trace; the link check MUST then fail")
	keep := flag.String("keep", "", "keep generated TLA+ files in this directory")
	flag.Parse()

	states, err := traceFromCorpus(*corpus)
	if err != nil {
		fail("building trace: %v", err)
	}
	if *canary {
		if len(states) < 3 {
			fail("trace too short to corrupt")
		}
		// Advance the clock backwards mid-trace. twitter.tla has no action
		// that decrements the clock, so no legal step can produce this.
		states[len(states)-1].Clock = 0
		fmt.Println("CANARY: clock forced backwards in the final state")
	}

	fmt.Printf("tlclink: %d state-changing steps projected from %s\n", len(states)-1, *corpus)

	ok, detail, err := runCheck(states, *specDir, *jar, *keep)
	if err != nil {
		fail("running TLC: %v", err)
	}

	fmt.Println(detail)
	if *canary {
		if ok {
			fail("CANARY DID NOT FAIL -- the link check cannot detect a bad trace, so a passing run proves nothing")
		}
		fmt.Println("tlclink: CANARY correctly rejected. The check can fail.")
		return
	}
	if !ok {
		fail("LINK FAILED -- some S_obs transition is not a legal twitter.tla step")
	}
	fmt.Println("tlclink: LINK OK -- every S_obs transition is a legal twitter.tla step")
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "tlclink: "+format+"\n", a...)
	os.Exit(1)
}

// traceFromCorpus replays the corpus and returns the projected states, with
// consecutive duplicates collapsed.
func traceFromCorpus(path string) ([]tlaState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st := sobs.Init()
	states := []tlaState{project(st)}
	last := states[0].key()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var cl corpusLine
		if err := json.Unmarshal(sc.Bytes(), &cl); err != nil {
			return nil, err
		}
		_, st = sobs.Step(st, sobs.Request{
			Method: cl.Request.Method, Path: cl.Request.Path, Body: cl.Request.Body,
		})
		p := project(st)
		if k := p.key(); k != last {
			states = append(states, p)
			last = k
		}
	}
	return states, sc.Err()
}

func runCheck(states []tlaState, specDir, jar, keep string) (bool, string, error) {
	work := keep
	if work == "" {
		d, err := os.MkdirTemp("", "tlclink-")
		if err != nil {
			return false, "", err
		}
		defer os.RemoveAll(d)
		work = d
	} else if err := os.MkdirAll(work, 0o755); err != nil {
		return false, "", err
	}

	src, err := os.ReadFile(filepath.Join(specDir, "twitter.tla"))
	if err != nil {
		return false, "", err
	}
	if err := os.WriteFile(filepath.Join(work, "twitter.tla"), src, 0o644); err != nil {
		return false, "", err
	}

	mod, cfg := renderModule(states)
	if err := os.WriteFile(filepath.Join(work, "trace_check.tla"), []byte(mod), 0o644); err != nil {
		return false, "", err
	}
	if err := os.WriteFile(filepath.Join(work, "trace_check.cfg"), []byte(cfg), 0o644); err != nil {
		return false, "", err
	}

	absJar, err := filepath.Abs(jar)
	if err != nil {
		return false, "", err
	}
	cmd := exec.Command("java", "-cp", absJar, "tlc2.TLC", "-cleanup", "-workers", "auto", "trace_check")
	cmd.Dir = work
	out, _ := cmd.CombinedOutput()
	text := string(out)

	// The exit code is deliberately not consulted. TLC exits nonzero both for
	// the violation we WANT and for parse errors we do not, so the decision is
	// made by reading TLC's own words.
	violated := strings.Contains(text, "Invariant NotReached is violated")
	noError := strings.Contains(text, "Model checking completed. No error has been found")
	noInitial := strings.Contains(text, "There is no state satisfying the initial state predicate")
	deadlock := strings.Contains(text, "Deadlock reached")
	parseErr := strings.Contains(text, "was not successfully parsed") ||
		strings.Contains(text, "Semantic errors") ||
		strings.Contains(text, "Unknown operator")

	var verdict string
	switch {
	case parseErr:
		return false, summarize(text, "TLC could not parse the generated module"), fmt.Errorf("parse error")
	case noInitial:
		verdict = "TLC found no initial state: the first projected state is not twitter.tla's Init"
	case violated:
		verdict = "TLC: 'Invariant NotReached is violated' -- the full trace is walkable under twitter.tla's Next"
	case deadlock:
		verdict = "TLC: 'Deadlock reached' -- the trace stalls before its end, so some recorded step is not a legal model transition"
	case noError:
		verdict = "TLC: no violation -- the trace never reached its final index, so some step is not a legal model transition"
	default:
		verdict = "TLC produced an unrecognized result"
	}
	return violated, summarize(text, verdict), nil
}

func summarize(tlcOutput, verdict string) string {
	var keep []string
	for _, ln := range strings.Split(tlcOutput, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "TLC2 Version") ||
			strings.Contains(t, "states generated") ||
			strings.Contains(t, "Invariant NotReached") ||
			strings.Contains(t, "Model checking completed") ||
			strings.Contains(t, "no state satisfying") ||
			strings.Contains(t, "Error:") {
			keep = append(keep, "    "+t)
		}
	}
	return "  TLC output:\n" + strings.Join(keep, "\n") + "\n  verdict: " + verdict
}
