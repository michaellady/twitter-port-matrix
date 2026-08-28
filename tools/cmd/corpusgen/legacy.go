package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	sobs "github.com/michaellady/twitter-port-matrix/spec/s_obs"
)

// legacyStep is one line of the hand-written conformance.jsonl that shipped
// with twitter_formal_spec.
type legacyStep struct {
	Step    int    `json:"step"`
	Name    string `json:"name"`
	Request struct {
		Method string          `json:"method"`
		Path   string          `json:"path"`
		Body   json.RawMessage `json:"body"`
	} `json:"request"`
	Expected struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	} `json:"expected"`
}

// legacyDiff replays the original hand-written corpus through S_obs and
// reports every divergence. This is the evidence for the spec/corpus drift:
// the corpus asserts behaviour the TLA+ model does not describe, and its
// clock values are not reachable by any request it contains.
func legacyDiff(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var steps []legacyStep
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var ls legacyStep
		if err := json.Unmarshal(sc.Bytes(), &ls); err != nil {
			return fmt.Errorf("line %d: %w", len(steps)+1, err)
		}
		steps = append(steps, ls)
	}
	if err := sc.Err(); err != nil {
		return err
	}

	fmt.Printf("Replaying %d legacy steps through S_obs\n", len(steps))
	fmt.Printf("%s\n", divider)

	st := sobs.Init()
	diverged := 0
	for _, ls := range steps {
		req := sobs.Request{Method: ls.Request.Method, Path: ls.Request.Path}
		if len(ls.Request.Body) > 0 {
			req.Body = string(ls.Request.Body)
		}
		got, next := sobs.Step(st, req)
		st = next

		statusOK := got.Status == ls.Expected.Status
		bodyOK := semanticEqual(got.Body, ls.Expected.Body)

		if statusOK && bodyOK {
			fmt.Printf("  ok   %2d %-42s %d\n", ls.Step, ls.Name, got.Status)
			continue
		}
		diverged++
		fmt.Printf("  DIFF %2d %-42s\n", ls.Step, ls.Name)
		fmt.Printf("         request  %s %s %s\n", req.Method, req.Path, string(ls.Request.Body))
		fmt.Printf("         expected %d %s\n", ls.Expected.Status, string(ls.Expected.Body))
		fmt.Printf("         S_obs    %d %s\n", got.Status, got.Body)
	}

	fmt.Printf("%s\n", divider)
	fmt.Printf("steps=%d matched=%d diverged=%d\n", len(steps), len(steps)-diverged, diverged)
	return nil
}

const divider = "  ------------------------------------------------------------------------"

// semanticEqual compares a canonical S_obs body against a legacy expected body
// by decoded value, so key order and whitespace are not counted as drift.
func semanticEqual(got string, want json.RawMessage) bool {
	if got == "" && len(want) == 0 {
		return true
	}
	if got == "" || len(want) == 0 {
		return false
	}
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		return false
	}
	if err := json.Unmarshal(want, &w); err != nil {
		return false
	}
	return reflect.DeepEqual(g, w)
}
