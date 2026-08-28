// Command replay is rung R0: drive an implementation through the generated
// conformance corpus over HTTP and compare responses byte-for-byte.
//
// Verdicts are three-way rather than pass/fail, because "differs by a trailing
// newline" and "returns the wrong error code" are not the same kind of news
// and collapsing them makes the report useless:
//
//	MATCH  status and body identical byte-for-byte
//	TRIM   identical after trailing-whitespace trim -- an encoding nit (D8)
//	DIFF   a real disagreement
//
// DIFFs are further classified so the summary says what KIND of gap exists,
// not just how many there are.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type corpusStep struct {
	Step    int      `json:"step"`
	Name    string   `json:"name"`
	Covers  []string `json:"covers"`
	Request struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   string `json:"body"`
	} `json:"request"`
	Expected struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	} `json:"expected"`
}

type verdict int

const (
	vMatch verdict = iota
	vTrim
	vDiff
)

type result struct {
	step      corpusStep
	verdict   verdict
	gotStatus int
	gotBody   string
	kind      string
	transport error
}

func main() {
	implName := flag.String("impl", "", "implementation name from impls/registry.json")
	corpusPath := flag.String("corpus", "generated/conformance.jsonl", "corpus to replay")
	regPath := flag.String("registry", "impls/registry.json", "implementation registry")
	verbose := flag.Bool("v", false, "print every step, not only the differences")
	maxShow := flag.Int("max-diffs", 12, "how many differing steps to print in full")
	canary := flag.String("canary", "", "corrupt responses with this mutation; R0 must then FAIL")
	flag.Parse()

	if *canary == "all" {
		os.Exit(runAllCanaries(*implName, *corpusPath, *regPath))
	}

	if *implName == "" {
		fmt.Fprintln(os.Stderr, "replay: -impl is required")
		os.Exit(2)
	}
	reg, err := loadRegistry(*regPath)
	if err != nil {
		die("loading registry: %v", err)
	}
	spec, ok := reg.Impls[*implName]
	if !ok {
		var names []string
		for k := range reg.Impls {
			names = append(names, k)
		}
		sort.Strings(names)
		die("unknown implementation %q; registry has: %s", *implName, strings.Join(names, ", "))
	}
	steps, err := loadCorpus(*corpusPath)
	if err != nil {
		die("loading corpus: %v", err)
	}

	fmt.Printf("replay: R0 conformance -- impl=%s (%s)\n", *implName, spec.Language)
	if spec.Status != "" {
		fmt.Printf("        impl status: %s\n", spec.Status)
	}
	fmt.Printf("        corpus=%s (%d steps)\n", *corpusPath, len(steps))

	h, err := start(*implName, spec)
	if err != nil {
		die("starting %s: %v", *implName, err)
	}
	// NOT `defer h.stop()`: every path out of this function goes through
	// os.Exit, which does not run deferred calls. The old code leaked a
	// server process on every single run -- five orphaned JVMs after one
	// canary sweep. Stop explicitly before each exit instead.
	exit := func(code int) {
		h.stop()
		os.Exit(code)
	}

	var mut *mutation
	if *canary != "" {
		m, ok := mutations[*canary]
		if !ok {
			die("unknown canary %q; available:\n%s", *canary, describeMutations())
		}
		mut = &m
		fmt.Printf("        CANARY: %s -- R0 must FAIL\n", m.desc)
	}
	fmt.Println(strings.Repeat("=", 72))

	results := runAll(h, steps, *verbose, mut)
	code := report(results, *maxShow)
	if mut != nil {
		// Inverted: a canary that does not break R0 means R0 cannot detect a
		// wrong implementation, which makes every green R0 run meaningless.
		if code == 0 {
			fmt.Printf("\nCANARY DID NOT FAIL: mutation %q left R0 green.\n"+
				"R0 cannot detect this class of defect, so a passing R0 run proves nothing about it.\n", mut.name)
			exit(1)
		}
		fmt.Printf("\ncanary %q correctly rejected: R0 can fail.\n", mut.name)
		exit(0)
	}
	exit(code)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "replay: "+format+"\n", a...)
	os.Exit(1)
}

func loadCorpus(path string) ([]corpusStep, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []corpusStep
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var s corpusStep
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func runAll(h *harness, steps []corpusStep, verbose bool, mut *mutation) []result {
	client := &http.Client{Timeout: 10 * time.Second}
	out := make([]result, 0, len(steps))
	for _, s := range steps {
		r := result{step: s}
		var body io.Reader
		if s.Request.Body != "" {
			body = bytes.NewReader([]byte(s.Request.Body))
		}
		req, err := http.NewRequest(s.Request.Method, h.base+s.Request.Path, body)
		if err != nil {
			r.transport, r.verdict = err, vDiff
			r.kind = "malformed request"
			out = append(out, r)
			continue
		}
		if s.Request.Body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			r.transport, r.verdict = err, vDiff
			r.kind = "transport error"
			out = append(out, r)
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		r.gotStatus, r.gotBody = resp.StatusCode, string(raw)
		if mut != nil {
			if st, body, changed := mut.apply(r.gotStatus, r.gotBody); changed {
				r.gotStatus, r.gotBody = st, body
			}
		}
		r.verdict, r.kind = classify(s, r.gotStatus, r.gotBody)
		out = append(out, r)
		if verbose {
			fmt.Printf("  %-5s %2d %s\n", verdictName(r.verdict), s.Step, s.Name)
		}
	}
	return out
}

func verdictName(v verdict) string {
	switch v {
	case vMatch:
		return "ok"
	case vTrim:
		return "trim"
	default:
		return "DIFF"
	}
}

// classify decides the verdict and, for a DIFF, what kind of gap it is.
func classify(s corpusStep, gotStatus int, gotBody string) (verdict, string) {
	wantStatus, wantBody := s.Expected.Status, s.Expected.Body

	if gotStatus == wantStatus && gotBody == wantBody {
		return vMatch, ""
	}
	if gotStatus == wantStatus && strings.TrimRight(gotBody, " \t\r\n") == strings.TrimRight(wantBody, " \t\r\n") {
		return vTrim, ""
	}

	switch {
	case gotStatus == 404 && wantStatus != 404:
		return vDiff, "route absent"
	case gotStatus == 405:
		return vDiff, "method not allowed"
	case gotStatus != wantStatus:
		return vDiff, fmt.Sprintf("status %d, want %d", gotStatus, wantStatus)
	}

	wantErr, wantIsErr := errCode(wantBody)
	gotErr, gotIsErr := errCode(gotBody)
	if wantIsErr && gotIsErr && wantErr != gotErr {
		return vDiff, fmt.Sprintf("error code %q, want %q", gotErr, wantErr)
	}
	if wantIsErr != gotIsErr {
		return vDiff, "error-shape mismatch"
	}
	return vDiff, "body differs"
}

func errCode(body string) (string, bool) {
	var m struct {
		Error *string `json:"error"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Error == nil {
		return "", false
	}
	return *m.Error, true
}
