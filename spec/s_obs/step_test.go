package sobs

import (
	"reflect"
	"testing"
)

func run(t *testing.T, s State, reqs ...Request) (State, []Response) {
	t.Helper()
	var out []Response
	for _, r := range reqs {
		var resp Response
		resp, s = Step(s, r)
		out = append(out, resp)
	}
	return s, out
}

func post(path, body string) Request { return Request{Method: "POST", Path: path, Body: body} }
func del(path, body string) Request  { return Request{Method: "DELETE", Path: path, Body: body} }
func get(path string) Request        { return Request{Method: "GET", Path: path} }
func mkUser(h string) Request        { return post("/users", `{"handle":"`+h+`"}`) }
func mkFollow(a, b string) Request   { return post("/follow", `{"from":"`+a+`","to":"`+b+`"}`) }
func mkUnfollow(a, b string) Request { return del("/follow", `{"from":"`+a+`","to":"`+b+`"}`) }
func mkTweet(a, txt string) Request  { return post("/tweets", `{"author":"`+a+`","text":"`+txt+`"}`) }
func tick() Request                  { return post("/tick", "") }

// TestDeterminism: Step is a pure function of (state, request).
func TestDeterminism(t *testing.T) {
	s, _ := run(t, Init(), mkUser("alice"), mkUser("bob"), mkFollow("alice", "bob"), tick(), mkTweet("bob", "hi"))
	req := get("/timeline?user=alice")
	r1, s1 := Step(s, req)
	r2, s2 := Step(s, req)
	if r1 != r2 {
		t.Fatalf("non-deterministic response: %+v vs %+v", r1, r2)
	}
	if !reflect.DeepEqual(s1.Handles(), s2.Handles()) || s1.Clock() != s2.Clock() {
		t.Fatal("non-deterministic state")
	}
}

// TestPurity: Step never mutates the state it was given.
func TestPurity(t *testing.T) {
	s, _ := run(t, Init(), mkUser("alice"), mkUser("bob"))
	before := len(s.Handles())
	beforeTweets := s.TweetCount()
	beforeClock := s.Clock()
	_, _ = Step(s, mkUser("carol"))
	_, _ = Step(s, mkTweet("alice", "x"))
	_, _ = Step(s, tick())
	_, _ = Step(s, mkFollow("alice", "bob"))
	if len(s.Handles()) != before || s.TweetCount() != beforeTweets || s.Clock() != beforeClock {
		t.Fatal("Step mutated its argument state")
	}
	if len(s.FollowEdges()) != 0 {
		t.Fatal("Step mutated the follow set of its argument state")
	}
}

// TestTotality: every request in a hostile grab-bag gets a defined response,
// and rejections never change state.
func TestTotality(t *testing.T) {
	base, _ := run(t, Init(), mkUser("alice"), mkUser("bob"))
	hostile := []Request{
		{Method: "POST", Path: "/users", Body: ""},
		{Method: "POST", Path: "/users", Body: "{"},
		{Method: "POST", Path: "/users", Body: `{"handle":"alice","extra":1}`},
		{Method: "POST", Path: "/users", Body: `{"handle":"Alice"}`},
		{Method: "POST", Path: "/users", Body: `{"handle":""}`},
		{Method: "POST", Path: "/users", Body: `{"handle":null}`},
		{Method: "POST", Path: "/users", Body: `{"handle":"a"}{"handle":"b"}`},
		{Method: "POST", Path: "/users", Body: `{"handle":123}`},
		{Method: "PATCH", Path: "/users", Body: `{"handle":"x"}`},
		{Method: "GET", Path: "/nope"},
		{Method: "GET", Path: "/timeline"},
		{Method: "GET", Path: "/timeline?user=alice&bogus=1"},
		{Method: "GET", Path: "/timeline?user=alice&user=bob"},
		{Method: "GET", Path: "/timeline?user=eve"},
		{Method: "GET", Path: "/timeline?user=alice&limit=0"},
		{Method: "GET", Path: "/timeline?user=alice&limit=101"},
		{Method: "GET", Path: "/timeline?user=alice&limit=abc"},
		{Method: "GET", Path: "/timeline?user=alice&cursor=0"},
		{Method: "GET", Path: "/timeline?user=alice&cursor=-1"},
		{Method: "POST", Path: "/follow", Body: `{"from":"alice"}`},
		{Method: "POST", Path: "/follow", Body: `{"from":"eve","to":"eve"}`},
		{Method: "POST", Path: "/tweets", Body: `{"author":"alice","text":""}`},
		{Method: "POST", Path: "/tick", Body: `{"n":1}`},
	}
	for _, r := range hostile {
		resp, after := Step(base, r)
		if resp.Status < 200 || resp.Status > 599 {
			t.Fatalf("undefined status %d for %+v", resp.Status, r)
		}
		if resp.Status >= 400 {
			if len(after.Handles()) != len(base.Handles()) ||
				after.TweetCount() != base.TweetCount() ||
				after.Clock() != base.Clock() ||
				len(after.FollowEdges()) != len(base.FollowEdges()) {
				t.Fatalf("rejected request changed state: %+v -> %+v", r, resp)
			}
		}
	}
}

// TestMonotonicityLemma is the proof obligation the sort-free timeline rests
// on: in the append-ordered log, ids strictly increase and created_at never
// decreases. Reverse iteration is therefore (created_at desc, id desc).
func TestMonotonicityLemma(t *testing.T) {
	s := Init()
	s, _ = run(t, s, mkUser("alice"), mkUser("bob"), mkUser("carol"))
	s, _ = run(t, s, mkFollow("alice", "bob"), mkFollow("alice", "carol"))
	seq := []Request{
		mkTweet("bob", "b1"), mkTweet("carol", "c1"), tick(),
		mkTweet("bob", "b2"), tick(), tick(),
		mkTweet("carol", "c2"), mkTweet("bob", "b3"),
	}
	s, _ = run(t, s, seq...)

	log := s.Tweets()
	for i := 1; i < len(log); i++ {
		if log[i-1].ID >= log[i].ID {
			t.Fatalf("ids not strictly increasing at %d: %d >= %d", i, log[i-1].ID, log[i].ID)
		}
		if log[i-1].CreatedAt > log[i].CreatedAt {
			t.Fatalf("created_at decreased at %d: %d > %d", i, log[i-1].CreatedAt, log[i].CreatedAt)
		}
	}

	// The timeline must equal descending lexicographic (created_at, id).
	resp, _ := Step(s, get("/timeline?user=alice"))
	if resp.Status != 200 {
		t.Fatalf("timeline status %d", resp.Status)
	}
	want := `{"tweets":[` +
		`{"id":5,"author":"bob","text":"b3","created_at":3},` +
		`{"id":4,"author":"carol","text":"c2","created_at":3},` +
		`{"id":3,"author":"bob","text":"b2","created_at":1},` +
		`{"id":2,"author":"carol","text":"c1","created_at":0},` +
		`{"id":1,"author":"bob","text":"b1","created_at":0}` +
		`],"next_cursor":null}`
	if resp.Body != want {
		t.Fatalf("timeline order wrong.\n got: %s\nwant: %s", resp.Body, want)
	}
}

// TestIdempotence covers F3 for both follow and unfollow.
func TestIdempotence(t *testing.T) {
	s, _ := run(t, Init(), mkUser("alice"), mkUser("bob"))
	s1, r1 := run(t, s, mkFollow("alice", "bob"))
	s2, r2 := run(t, s1, mkFollow("alice", "bob"))
	if r1[0] != r2[0] || len(s1.FollowEdges()) != len(s2.FollowEdges()) {
		t.Fatal("follow not idempotent")
	}
	s3, r3 := run(t, s2, mkUnfollow("alice", "bob"))
	s4, r4 := run(t, s3, mkUnfollow("alice", "bob"))
	if r3[0] != r4[0] || len(s3.FollowEdges()) != len(s4.FollowEdges()) {
		t.Fatal("unfollow not idempotent")
	}
}

// TestPinnedAmbiguities locks the decisions twitter.tla leaves open.
func TestPinnedAmbiguities(t *testing.T) {
	s, _ := run(t, Init(), mkUser("alice"))

	// D4: unknown_user wins over self_follow_forbidden for an unknown self-edge.
	resp, _ := Step(s, mkFollow("eve", "eve"))
	if resp.Body != errorBody(ErrUnknownUser) {
		t.Fatalf("D4: want unknown_user, got %s", resp.Body)
	}
	// Known self-follow is still self_follow_forbidden.
	resp, _ = Step(s, mkFollow("alice", "alice"))
	if resp.Body != errorBody(ErrSelfFollow) {
		t.Fatalf("D4: want self_follow_forbidden, got %s", resp.Body)
	}
	// D5: self-unfollow of a known user is a legal no-op.
	resp, after := Step(s, mkUnfollow("alice", "alice"))
	if resp.Status != 204 || len(after.FollowEdges()) != 0 {
		t.Fatalf("D5: want 204 no-op, got %+v", resp)
	}
	// Syntax before existence.
	resp, _ = Step(s, mkFollow("EVE", "eve"))
	if resp.Body != errorBody(ErrInvalidHandle) {
		t.Fatalf("syntax-before-existence: got %s", resp.Body)
	}
}

// TestPagination: pages partition the visible set with no fabrication and no
// loss, and next_cursor is null exactly when nothing remains.
func TestPagination(t *testing.T) {
	s, _ := run(t, Init(), mkUser("alice"), mkUser("bob"), mkFollow("alice", "bob"))
	for i := 0; i < 7; i++ {
		s, _ = run(t, s, mkTweet("bob", "t"), tick())
	}
	seen := map[int64]bool{}
	path := "/timeline?user=alice&limit=3"
	pages := 0
	for {
		resp, _ := Step(s, get(path))
		if resp.Status != 200 {
			t.Fatalf("status %d", resp.Status)
		}
		pages++
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
		var body struct {
			Tweets []struct {
				ID        int64  `json:"id"`
				Author    string `json:"author"`
				Text      string `json:"text"`
				CreatedAt int64  `json:"created_at"`
			} `json:"tweets"`
			NextCursor *int64 `json:"next_cursor"`
		}
		if !decodeStrict(resp.Body, &body) {
			t.Fatalf("undecodable page: %s", resp.Body)
		}
		for _, tw := range body.Tweets {
			if seen[tw.ID] {
				t.Fatalf("tweet %d fabricated across pages", tw.ID)
			}
			seen[tw.ID] = true
		}
		if body.NextCursor == nil {
			break
		}
		path = "/timeline?user=alice&limit=3&cursor=" + itoa(*body.NextCursor)
	}
	if len(seen) != 7 {
		t.Fatalf("paginated set has %d tweets, want 7", len(seen))
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
