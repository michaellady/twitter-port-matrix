// Command proptest is rung R2: property and metamorphic checks.
//
// R2 exists because R0 and R1 both compare against S_obs, so they share a
// single point of failure: if S_obs is wrong, both go green anyway. The
// properties here assert RELATIONS that must hold whatever the correct
// answers are -- "following twice equals following once", "the pages of a
// timeline partition it exactly" -- and they never consult S_obs. A defect in
// the reference machine cannot make them pass.
//
// That independence is the reason to run them at all. Adding a third check
// that shares R1's oracle would add cost without adding coverage.
package main

import (
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

	"github.com/michaellady/twitter-port-matrix/tools/internal/tracegen"
)

type client struct {
	h    *harness
	http *http.Client
}

type resp struct {
	status int
	body   string
}

func (c *client) do(method, path, body string) resp {
	var r io.Reader
	if body != "" {
		r = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, c.h.base+path, r)
	if err != nil {
		return resp{status: -1, body: err.Error()}
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	hr, err := c.http.Do(req)
	if err != nil {
		return resp{status: -1, body: err.Error()}
	}
	raw, _ := io.ReadAll(hr.Body)
	hr.Body.Close()
	return resp{status: hr.StatusCode, body: string(raw)}
}

type tweet struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

type page struct {
	Tweets     []tweet `json:"tweets"`
	NextCursor *int64  `json:"next_cursor"`
}

func (c *client) timeline(user, extra string) (page, resp) {
	r := c.do("GET", "/timeline?user="+user+extra, "")
	var p page
	_ = json.Unmarshal([]byte(r.body), &p)
	return p, r
}

// property is one relation, checked against a randomly-reached state.
type property struct {
	name  string
	about string
	check func(c *client, seed int64, users []string) error
}

func main() {
	impls := flag.String("impls", "go,rust", "comma-separated implementations")
	rounds := flag.Int("rounds", 40, "random states per property")
	seed0 := flag.Int64("seed", 1, "base seed")
	setup := flag.Int("setup", 40, "requests used to reach each random state")
	regPath := flag.String("registry", "impls/registry.json", "implementation registry")
	flag.Parse()

	reg, err := loadRegistry(*regPath)
	if err != nil {
		die("loading registry: %v", err)
	}

	fmt.Printf("proptest: R2 properties and metamorphic relations\n")
	fmt.Printf("        impls=%s properties=%d rounds=%d\n", *impls, len(properties), *rounds)
	fmt.Println(strings.Repeat("=", 72))

	exit := 0
	for _, name := range strings.Split(*impls, ",") {
		spec, ok := reg.Impls[name]
		if !ok {
			die("unknown implementation %q", name)
		}
		fmt.Printf("\n[%s]\n%s\n", name, strings.Repeat("-", 72))
		for _, p := range properties {
			failures := 0
			var first error
			var firstSeed int64
			start := time.Now()
			for i := 0; i < *rounds; i++ {
				seed := *seed0 + int64(i)
				if err := runRound(name, spec, p, seed, *setup); err != nil {
					failures++
					if first == nil {
						first, firstSeed = err, seed
					}
				}
			}
			if failures == 0 {
				fmt.Printf("  ok    %-26s %-38s %.1fs\n", p.name, p.about, time.Since(start).Seconds())
			} else {
				exit = 1
				fmt.Printf("  FAIL  %-26s %d/%d rounds\n", p.name, failures, *rounds)
				fmt.Printf("        %s\n        first failure at seed=%d: %v\n", p.about, firstSeed, first)
			}
		}
	}
	fmt.Println("\n" + strings.Repeat("=", 72))
	if exit != 0 {
		fmt.Println("R2 FAILED")
	} else {
		fmt.Println("R2 PASSED: every relation holds on every generated state")
	}
	os.Exit(exit)
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "proptest: "+f+"\n", a...)
	os.Exit(1)
}

// runRound drives the implementation to a random state, then checks one
// property against it.
func runRound(name string, spec implSpec, p property, seed int64, setup int) error {
	h, err := start(name, spec)
	if err != nil {
		die("starting %s: %v", name, err)
	}
	defer h.stop()
	c := &client{h: h, http: &http.Client{Timeout: 10 * time.Second}}

	users := map[string]bool{}
	for _, req := range tracegen.Generate(tracegen.DefaultConfig(seed, setup)) {
		r := c.do(req.Method, req.Path, req.Body)
		if req.Path == "/users" && r.status == 201 {
			var u struct {
				Handle string `json:"handle"`
			}
			if json.Unmarshal([]byte(r.body), &u) == nil {
				users[u.Handle] = true
			}
		}
	}
	var known []string
	for u := range users {
		known = append(known, u)
	}
	sort.Strings(known)
	if len(known) < 2 {
		return nil // not enough state to say anything; not a failure
	}
	return p.check(c, seed, known)
}
