// Package tracegen produces randomized request sequences over the S_obs
// alphabet.
//
// The generator is deterministic given a seed, so any failure is replayable
// from the seed alone rather than from a saved artefact.
//
// Two design points matter more than the randomness itself:
//
//   - It generates from the ALPHABET, not from a model of correct usage.
//     Malformed bodies, unknown handles, out-of-range limits and unrouted
//     paths are first-class outputs, because totality means those inputs have
//     defined answers and an implementation can disagree there just as easily
//     as on the happy path. Ten of the 54 R0 baseline steps were exactly that
//     class, and both implementations failed all ten.
//
//   - It tracks which handles it has created so most requests land on real
//     users. A generator that only ever emits random handles spends its whole
//     budget re-testing unknown_user and never reaches a timeline with
//     content in it.
package tracegen

import (
	"fmt"
	"math/rand"
)

// Request mirrors the observable wire shape.
type Request struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// Config tunes the mix.
type Config struct {
	Seed  int64
	Steps int
	// MalformedRate is the share of requests drawn from the hostile pool.
	MalformedRate float64
}

// DefaultConfig is a sensible mix: mostly well-formed traffic against real
// users, with a steady trickle of hostile input.
func DefaultConfig(seed int64, steps int) Config {
	return Config{Seed: seed, Steps: steps, MalformedRate: 0.25}
}

type gen struct {
	r       *rand.Rand
	cfg     Config
	handles []string
	next    int
}

// Generate produces a deterministic trace.
func Generate(cfg Config) []Request {
	g := &gen{r: rand.New(rand.NewSource(cfg.Seed)), cfg: cfg}
	out := make([]Request, 0, cfg.Steps)
	// Seed the world with a couple of users so the trace reaches interesting
	// states quickly instead of spending its budget on unknown_user.
	for i := 0; i < 2; i++ {
		out = append(out, g.createUser())
	}
	for len(out) < cfg.Steps {
		out = append(out, g.step())
	}
	return out[:cfg.Steps]
}

func (g *gen) step() Request {
	if g.r.Float64() < g.cfg.MalformedRate {
		return g.hostile()
	}
	switch g.r.Intn(100) {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9:
		return g.createUser()
	case 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24:
		return g.follow()
	case 25, 26, 27, 28, 29, 30, 31, 32:
		return g.unfollow()
	case 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50:
		return g.tweet()
	case 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65:
		return Request{Method: "POST", Path: "/tick"}
	default:
		return g.timeline()
	}
}

func (g *gen) createUser() Request {
	h := fmt.Sprintf("u%d", g.next)
	g.next++
	g.handles = append(g.handles, h)
	return Request{Method: "POST", Path: "/users", Body: `{"handle":"` + h + `"}`}
}

// handle returns a known handle most of the time, an unknown one occasionally.
func (g *gen) handle() string {
	if len(g.handles) == 0 || g.r.Intn(10) == 0 {
		return fmt.Sprintf("ghost%d", g.r.Intn(1000))
	}
	return g.handles[g.r.Intn(len(g.handles))]
}

func (g *gen) follow() Request {
	return Request{Method: "POST", Path: "/follow",
		Body: `{"from":"` + g.handle() + `","to":"` + g.handle() + `"}`}
}

func (g *gen) unfollow() Request {
	return Request{Method: "DELETE", Path: "/follow",
		Body: `{"from":"` + g.handle() + `","to":"` + g.handle() + `"}`}
}

var texts = []string{"a", "hello world", "second", "  spaced  ", "unicode ok", "0"}

func (g *gen) tweet() Request {
	t := texts[g.r.Intn(len(texts))]
	return Request{Method: "POST", Path: "/tweets",
		Body: `{"author":"` + g.handle() + `","text":"` + t + `"}`}
}

func (g *gen) timeline() Request {
	p := "/timeline?user=" + g.handle()
	switch g.r.Intn(4) {
	case 1:
		p += fmt.Sprintf("&limit=%d", 1+g.r.Intn(5))
	case 2:
		p += fmt.Sprintf("&cursor=%d", 1+g.r.Intn(8))
	case 3:
		p += fmt.Sprintf("&limit=%d&cursor=%d", 1+g.r.Intn(4), 1+g.r.Intn(8))
	}
	return Request{Method: "GET", Path: p}
}

// hostile draws from the totality surface: inputs whose answers are defined
// but where a lenient implementation will quietly differ.
func (g *gen) hostile() Request {
	h := g.handle()
	pool := []Request{
		{Method: "POST", Path: "/users", Body: `{}`},
		{Method: "POST", Path: "/users", Body: `{`},
		{Method: "POST", Path: "/users", Body: ``},
		{Method: "POST", Path: "/users", Body: `{"handle":"` + h + `","x":1}`},
		{Method: "POST", Path: "/users", Body: `{"handle":"UPPER"}`},
		{Method: "POST", Path: "/users", Body: `{"handle":""}`},
		{Method: "POST", Path: "/users", Body: `{"handle":null}`},
		{Method: "POST", Path: "/users", Body: `{"handle":7}`},
		{Method: "POST", Path: "/users", Body: `{"handle":"a"} {}`},
		{Method: "POST", Path: "/follow", Body: `{"from":"` + h + `"}`},
		{Method: "POST", Path: "/follow", Body: `{"from":"` + h + `","to":"` + h + `"}`},
		{Method: "POST", Path: "/tweets", Body: `{"author":"` + h + `","text":""}`},
		{Method: "POST", Path: "/tweets", Body: `{"author":"` + h + `","text":"` + longText() + `"}`},
		{Method: "POST", Path: "/tick", Body: `{"n":1}`},
		{Method: "GET", Path: "/timeline"},
		{Method: "GET", Path: "/timeline?user=" + h + "&bogus=1"},
		{Method: "GET", Path: "/timeline?user=" + h + "&user=" + h},
		{Method: "GET", Path: "/timeline?user=" + h + "&limit=0"},
		{Method: "GET", Path: "/timeline?user=" + h + "&limit=101"},
		{Method: "GET", Path: "/timeline?user=" + h + "&limit=abc"},
		{Method: "GET", Path: "/timeline?user=" + h + "&cursor=0"},
		{Method: "GET", Path: "/timeline?user=" + h + "&cursor=-3"},
		{Method: "GET", Path: "/nope"},
		{Method: "PATCH", Path: "/users", Body: `{"handle":"x"}`},
		{Method: "DELETE", Path: "/tweets", Body: `{}`},
	}
	return pool[g.r.Intn(len(pool))]
}

func longText() string {
	b := make([]byte, 281)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
