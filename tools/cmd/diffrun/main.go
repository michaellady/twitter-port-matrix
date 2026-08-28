// Command diffrun is rung R1: randomized differential testing against S_obs.
//
// The oracle is S_obs, NOT another implementation. That distinction is the
// whole point of this rung. Before retargeting, Go and Rust diverged from
// S_obs on exactly the same 39 corpus steps and agreed with each other on 30
// of them (finding F006) -- so implementation-vs-implementation agreement was
// blind to 77% of the gap. Comparing against an oracle neither implementation
// produced has no such ceiling.
//
// Each trace runs against a FRESH server process, so no state leaks between
// traces and a failure is reproducible from its seed alone.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	sobs "github.com/michaellady/twitter-port-matrix/spec/s_obs"
	"github.com/michaellady/twitter-port-matrix/tools/internal/tracegen"
)

type mismatch struct {
	impl       string
	seed       int64
	step       int
	req        tracegen.Request
	wantStatus int
	wantBody   string
	gotStatus  int
	gotBody    string
}

func main() {
	impls := flag.String("impls", "go,rust", "comma-separated implementations")
	traces := flag.Int("traces", 50, "independent traces (fresh server each)")
	steps := flag.Int("steps", 200, "requests per trace")
	seed0 := flag.Int64("seed", 1, "base seed; trace i uses seed+i")
	regPath := flag.String("registry", "impls/registry.json", "implementation registry")
	stopFirst := flag.Bool("stop-on-first", true, "stop an implementation at its first mismatch")
	flag.Parse()

	reg, err := loadRegistry(*regPath)
	if err != nil {
		die("loading registry: %v", err)
	}
	names := strings.Split(*impls, ",")

	fmt.Printf("diffrun: R1 differential vs S_obs\n")
	fmt.Printf("        impls=%s traces=%d steps=%d seed=%d..%d (%d requests each)\n",
		*impls, *traces, *steps, *seed0, *seed0+int64(*traces)-1, *traces**steps)
	fmt.Println(strings.Repeat("=", 72))

	exit := 0
	for _, name := range names {
		spec, ok := reg.Impls[name]
		if !ok {
			die("unknown implementation %q", name)
		}
		found := run(name, spec, *traces, *steps, *seed0, *stopFirst)
		if found != nil {
			exit = 1
			reportMismatch(*found)
			shrink(name, spec, *found)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 72))
	if exit != 0 {
		fmt.Println("R1 FAILED: at least one implementation diverges from S_obs")
	} else {
		fmt.Printf("R1 PASSED: %d requests per implementation, zero unexplained mismatches\n", *traces**steps)
	}
	os.Exit(exit)
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "diffrun: "+f+"\n", a...)
	os.Exit(1)
}

func run(name string, spec implSpec, traces, steps int, seed0 int64, stopFirst bool) *mismatch {
	start := time.Now()
	done := 0
	for i := 0; i < traces; i++ {
		seed := seed0 + int64(i)
		trace := tracegen.Generate(tracegen.DefaultConfig(seed, steps))
		m := runTrace(name, spec, seed, trace, -1)
		done += steps
		if m != nil {
			fmt.Printf("  %-6s %d/%d requests, MISMATCH at trace seed=%d step=%d\n",
				name, done, traces*steps, seed, m.step)
			if stopFirst {
				return m
			}
		}
	}
	fmt.Printf("  %-6s %d requests, no mismatch  (%.1fs)\n", name, done, time.Since(start).Seconds())
	return nil
}

// runTrace replays one trace against a fresh server and against S_obs,
// comparing byte-for-byte. upTo limits the trace length (used by shrinking);
// -1 means the whole trace.
func runTrace(name string, spec implSpec, seed int64, trace []tracegen.Request, upTo int) *mismatch {
	if upTo >= 0 && upTo < len(trace) {
		trace = trace[:upTo]
	}
	h, err := start(name, spec)
	if err != nil {
		die("starting %s: %v", name, err)
	}
	defer h.stop()

	client := &http.Client{Timeout: 10 * time.Second}
	st := sobs.Init()
	for i, req := range trace {
		want, next := sobs.Step(st, sobs.Request{Method: req.Method, Path: req.Path, Body: req.Body})
		st = next

		var body io.Reader
		if req.Body != "" {
			body = bytes.NewReader([]byte(req.Body))
		}
		hr, err := http.NewRequest(req.Method, h.base+req.Path, body)
		if err != nil {
			continue
		}
		if req.Body != "" {
			hr.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(hr)
		if err != nil {
			return &mismatch{impl: name, seed: seed, step: i, req: req,
				wantStatus: want.Status, wantBody: want.Body, gotStatus: 0, gotBody: "transport: " + err.Error()}
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		got := string(raw)
		if resp.StatusCode != want.Status || got != want.Body {
			return &mismatch{impl: name, seed: seed, step: i, req: req,
				wantStatus: want.Status, wantBody: want.Body,
				gotStatus: resp.StatusCode, gotBody: got}
		}
	}
	return nil
}

func reportMismatch(m mismatch) {
	fmt.Printf("\n  MISMATCH  %s  seed=%d  step=%d\n", m.impl, m.seed, m.step)
	fmt.Printf("    request   %s %s %s\n", m.req.Method, m.req.Path, m.req.Body)
	fmt.Printf("    S_obs     %d %s\n", m.wantStatus, m.wantBody)
	fmt.Printf("    %-9s %d %s\n", m.impl, m.gotStatus, m.gotBody)
}

// shrink reduces the trace to a minimal reproducing subsequence.
//
// Two passes, because prefix shrinking alone is not enough. Binary search over
// prefixes finds the earliest point at which the failure exists, but the
// failing request is usually near the end and every preceding step is kept --
// a 64-step reproduction for a one-step defect. The second pass removes
// interior steps.
//
// This matters for calibration, not just ergonomics: a mutant whose
// reproduction is 64 steps long is expensive to triage, and the kill table is
// only useful if the kills are diagnosable.
func shrink(name string, spec implSpec, m mismatch) {
	full := tracegen.Generate(tracegen.DefaultConfig(m.seed, m.step+1))
	fails := func(t []tracegen.Request) bool {
		return runTrace(name, spec, m.seed, t, -1) != nil
	}

	// Pass 1 -- shortest failing prefix.
	lo, hi := 1, len(full)
	for lo < hi {
		mid := (lo + hi) / 2
		if fails(full[:mid]) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	cur := append([]tracegen.Request(nil), full[:lo]...)
	fmt.Printf("\n  shrinking %d steps -> prefix %d", len(full), len(cur))

	// Pass 2 -- ddmin over interior steps. The last step is the failing
	// request itself and is never removed.
	attempts := 0
	for chunk := len(cur) / 2; chunk >= 1; chunk /= 2 {
		progress := true
		for progress {
			progress = false
			for i := 0; i+chunk < len(cur); i += chunk {
				cand := make([]tracegen.Request, 0, len(cur)-chunk)
				cand = append(cand, cur[:i]...)
				cand = append(cand, cur[i+chunk:]...)
				attempts++
				if len(cand) > 0 && fails(cand) {
					cur = cand
					progress = true
					break
				}
			}
		}
	}
	fmt.Printf(" -> minimal %d (%d attempts)\n\n", len(cur), attempts)
	fmt.Printf("  MINIMAL REPRODUCTION (%d step(s)):\n", len(cur))
	for i, r := range cur {
		fmt.Printf("    %2d  %-6s %-42s %s\n", i, r.Method, r.Path, r.Body)
	}
	if f := runTrace(name, spec, m.seed, cur, -1); f != nil {
		fmt.Printf("\n    S_obs     %d %s\n", f.wantStatus, f.wantBody)
		fmt.Printf("    %-9s %d %s\n", name, f.gotStatus, f.gotBody)
	}
}
