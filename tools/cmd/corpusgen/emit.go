package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sobs "github.com/michaellady/twitter-port-matrix/spec/s_obs"
)

// scenario is one named request in the generated corpus, tagged with what it
// exercises so coverage can be reported per F-property and per decision.
type scenario struct {
	name   string
	req    sobs.Request
	covers []string
}

// corpusStep is one emitted JSONL line. Expected values are produced by
// running S_obs, never written by hand, so every assertion is reachable by
// the requests that precede it in this same file.
// Bodies are carried as raw strings, not embedded JSON objects.
//
// Two reasons, both load-bearing. Totality requires that a malformed body be
// representable -- `{` and `{"handle":"dave"} {}` are legitimate corpus inputs
// and cannot be embedded as JSON values. And carrying the expected response as
// exact bytes makes R0 a literal byte-equality check, which is what D8 says
// conformance means.
type corpusStep struct {
	Step     int            `json:"step"`
	Name     string         `json:"name"`
	Covers   []string       `json:"covers"`
	Request  corpusRequest  `json:"request"`
	Expected corpusExpected `json:"expected"`
}

type corpusRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

type corpusExpected struct {
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"`
}

func u(h string) string     { return `{"handle":"` + h + `"}` }
func e(a, b string) string  { return `{"from":"` + a + `","to":"` + b + `"}` }
func tw(a, t string) string { return `{"author":"` + a + `","text":"` + t + `"}` }

func post(p, b string) sobs.Request { return sobs.Request{Method: "POST", Path: p, Body: b} }
func del(p, b string) sobs.Request  { return sobs.Request{Method: "DELETE", Path: p, Body: b} }
func get(p string) sobs.Request     { return sobs.Request{Method: "GET", Path: p} }

// scenarios is the curated corpus. It strictly supersedes the legacy 18-step
// file: every legacy behaviour appears here, plus the cases the legacy corpus
// could not express (an observable clock), plus the decisions the model left
// open, plus the totality surface.
func scenarios() []scenario {
	var s []scenario
	add := func(name string, r sobs.Request, covers ...string) {
		s = append(s, scenario{name: name, req: r, covers: covers})
	}

	// -- registration ------------------------------------------------------
	add("create_alice", post("/users", u("alice")), "D2")
	add("create_bob", post("/users", u("bob")), "D2")
	add("create_carol", post("/users", u("carol")), "D2")
	add("reject_duplicate_handle", post("/users", u("alice")), "D2")
	add("reject_uppercase_handle", post("/users", u("Alice")), "D6")
	add("reject_empty_handle", post("/users", `{"handle":""}`), "D6")
	add("reject_missing_handle_field", post("/users", `{}`), "D7")
	add("reject_unknown_field", post("/users", `{"handle":"dave","x":1}`), "D7")
	add("reject_trailing_content", post("/users", `{"handle":"dave"} {}`), "D7")
	// The id after a run of rejections. Without this the corpus never
	// registers another user once it has rejected one, so an implementation
	// that burns an id on a rejected registration is INVISIBLE to R0 -- the
	// exact defect the R0 baseline exhibited as {"handle":"Alice","id":5}.
	// The mutation catalogue's id-burned-on-reject survived 54/54 until this
	// step existed. See evidence/findings/F009.
	add("id_not_burned_by_rejections", post("/users", u("dave")), "D2")

	// -- follow semantics --------------------------------------------------
	add("reject_self_follow_known", post("/follow", e("alice", "alice")), "F4", "D4")
	add("reject_self_follow_unknown_is_unknown_user", post("/follow", e("eve", "eve")), "D4")
	add("reject_follow_unknown_target", post("/follow", e("alice", "eve")), "F9")
	add("reject_follow_invalid_syntax_before_existence", post("/follow", e("EVE", "eve")), "D6")
	add("alice_follows_bob", post("/follow", e("alice", "bob")), "F9")
	add("alice_follows_bob_idempotent", post("/follow", e("alice", "bob")), "F3")

	// -- the clock is now observable (D3) ----------------------------------
	add("tick_to_one", post("/tick", ""), "F7", "D3")
	add("reject_tick_with_body", post("/tick", `{"n":1}`), "D7")

	// -- posting -----------------------------------------------------------
	add("bob_posts_first", post("/tweets", tw("bob", "hello world")), "F8", "D3")
	add("alice_timeline_sees_bob", get("/timeline?user=alice"), "F1")
	add("carol_timeline_empty", get("/timeline?user=carol"), "F1")
	add("carol_follows_bob_after_post", post("/follow", e("carol", "bob")), "F1")
	add("carol_timeline_sees_older_bob_tweet", get("/timeline?user=carol"), "F1")
	add("bob_posts_second_same_clock", post("/tweets", tw("bob", "second")), "F8", "F2")
	add("timeline_orders_by_id_when_clock_ties", get("/timeline?user=alice"), "F2", "D9")
	add("tick_to_two", post("/tick", ""), "F7", "D3")
	add("carol_posts_at_later_clock", post("/tweets", tw("carol", "carol here")), "F8")
	add("alice_follows_carol", post("/follow", e("alice", "carol")), "F9")
	add("timeline_orders_by_clock_across_authors", get("/timeline?user=alice"), "F2", "D9")
	add("author_sees_own_tweets_without_following", get("/timeline?user=bob"), "F1")
	add("reject_post_unknown_author", post("/tweets", tw("eve", "hi")), "F6")
	add("reject_post_empty_text", post("/tweets", tw("bob", "")), "D1")
	add("reject_post_overlong_text", post("/tweets", tw("bob", strings.Repeat("x", 281))), "D1")

	// -- unfollow ----------------------------------------------------------
	add("self_unfollow_is_a_legal_noop", del("/follow", e("alice", "alice")), "D5")
	add("alice_unfollows_bob", del("/follow", e("alice", "bob")), "F3")
	add("alice_unfollows_bob_idempotent", del("/follow", e("alice", "bob")), "F3")
	add("alice_timeline_after_unfollow", get("/timeline?user=alice"), "F1")

	// -- pagination (D10) --------------------------------------------------
	add("tick_to_three", post("/tick", ""), "F7")
	add("bob_posts_third", post("/tweets", tw("bob", "third")), "F8")
	add("tick_to_four", post("/tick", ""), "F7")
	add("bob_posts_fourth", post("/tweets", tw("bob", "fourth")), "F8")
	add("bob_timeline_limit_two_page_one", get("/timeline?user=bob&limit=2"), "D10")
	add("bob_timeline_limit_two_page_two", get("/timeline?user=bob&limit=2&cursor=3"), "D10")
	add("bob_timeline_limit_two_page_three", get("/timeline?user=bob&limit=2&cursor=1"), "D10")
	add("reject_limit_zero", get("/timeline?user=bob&limit=0"), "D10")
	add("reject_limit_over_max", get("/timeline?user=bob&limit=101"), "D10")
	add("reject_limit_nonnumeric", get("/timeline?user=bob&limit=abc"), "D10")
	add("reject_cursor_zero", get("/timeline?user=bob&cursor=0"), "D10")

	// -- totality surface (D7) ---------------------------------------------
	add("reject_timeline_without_query", get("/timeline"), "D7")
	add("reject_timeline_unknown_param", get("/timeline?user=bob&bogus=1"), "D7")
	add("reject_timeline_repeated_param", get("/timeline?user=bob&user=alice"), "D7")
	add("reject_timeline_unknown_user", get("/timeline?user=eve"), "F6")
	add("reject_unknown_path", get("/nope"), "D7")
	add("reject_unknown_method", sobs.Request{Method: "PATCH", Path: "/users", Body: u("x")}, "D7")
	add("reject_malformed_json", post("/users", "{"), "D7")

	return s
}

func emit(path string) error {
	st := sobs.Init()
	var b strings.Builder
	for i, sc := range scenarios() {
		resp, next := sobs.Step(st, sc.req)
		st = next

		step := corpusStep{
			Step:   i + 1,
			Name:   sc.name,
			Covers: sc.covers,
			Request: corpusRequest{
				Method: sc.req.Method,
				Path:   sc.req.Path,
			},
			Expected: corpusExpected{Status: resp.Status},
		}
		step.Request.Body = sc.req.Body
		step.Expected.Body = resp.Body

		line, err := json.Marshal(step)
		if err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, sc.name, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("corpusgen: emitted %d steps to %s\n", len(scenarios()), path)
	fmt.Printf("corpusgen: final state clock=%d users=%d tweets=%d follows=%d\n",
		st.Clock(), len(st.Handles()), st.TweetCount(), len(st.FollowEdges()))
	return nil
}
