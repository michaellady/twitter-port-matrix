//go:build integration

// Integration tests: full POST/follow/PostTweet/timeline flows wired through
// the in-process HTTP shim against the real (in-memory) service.
package httpshim

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
)

func intDo(t *testing.T, srv *httptest.Server, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp, buf.Bytes()
}

func TestIntegration_FullFlow(t *testing.T) {
	c := clock.New()
	srv := httptest.NewServer(New(service.New(c)))
	defer srv.Close()

	// Create three users.
	for _, h := range []string{"alice", "bob", "carol"} {
		resp, _ := intDo(t, srv, http.MethodPost, "/users", `{"handle":"`+h+`"}`)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: status=%d", h, resp.StatusCode)
		}
	}

	// alice follows bob.
	resp, _ := intDo(t, srv, http.MethodPost, "/follow", `{"from":"alice","to":"bob"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("follow status=%d", resp.StatusCode)
	}

	// bob posts; tick the clock between to make ordering deterministic.
	c.Tick()
	resp, body := intDo(t, srv, http.MethodPost, "/tweets", `{"author":"bob","text":"first"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post first status=%d", resp.StatusCode)
	}
	var first struct {
		ID        int64 `json:"id"`
		CreatedAt int64 `json:"created_at"`
	}
	if err := json.Unmarshal(body, &first); err != nil {
		t.Fatal(err)
	}
	if first.ID != 1 || first.CreatedAt != 1 {
		t.Fatalf("first tweet id=%d ts=%d, want 1,1", first.ID, first.CreatedAt)
	}

	c.Tick()
	resp, body = intDo(t, srv, http.MethodPost, "/tweets", `{"author":"bob","text":"second"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post second status=%d", resp.StatusCode)
	}
	var second struct {
		ID        int64 `json:"id"`
		CreatedAt int64 `json:"created_at"`
	}
	if err := json.Unmarshal(body, &second); err != nil {
		t.Fatal(err)
	}
	if second.ID != 2 || second.CreatedAt != 2 {
		t.Fatalf("second tweet id=%d ts=%d", second.ID, second.CreatedAt)
	}

	// alice's timeline: 2 tweets, descending.
	resp, body = intDo(t, srv, http.MethodGet, "/timeline?user=alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("timeline status=%d", resp.StatusCode)
	}
	var tl struct {
		Tweets []struct {
			ID        int64
			Author    string
			Text      string
			CreatedAt int64 `json:"created_at"`
		}
	}
	if err := json.Unmarshal(body, &tl); err != nil {
		t.Fatal(err)
	}
	if len(tl.Tweets) != 2 {
		t.Fatalf("timeline len=%d, want 2", len(tl.Tweets))
	}
	if tl.Tweets[0].ID != 2 || tl.Tweets[1].ID != 1 {
		t.Fatalf("ordering wrong: %v", tl.Tweets)
	}

	// carol timeline empty.
	resp, body = intDo(t, srv, http.MethodGet, "/timeline?user=carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !bytes.Contains(body, []byte(`"tweets":[]`)) {
		t.Fatalf("carol timeline not empty: %s", body)
	}

	// carol follows bob → backfills bob's existing tweets (F1 K2 fix).
	resp, _ = intDo(t, srv, http.MethodPost, "/follow", `{"from":"carol","to":"bob"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("carol follow status=%d", resp.StatusCode)
	}
	resp, body = intDo(t, srv, http.MethodGet, "/timeline?user=carol", "")
	if err := json.Unmarshal(body, &tl); err != nil {
		t.Fatal(err)
	}
	if len(tl.Tweets) != 2 {
		t.Fatalf("post-follow backfill: len=%d, want 2", len(tl.Tweets))
	}

	// alice unfollows bob → timeline empty.
	resp, _ = intDo(t, srv, http.MethodDelete, "/follow", `{"from":"alice","to":"bob"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unfollow status=%d", resp.StatusCode)
	}
	resp, body = intDo(t, srv, http.MethodGet, "/timeline?user=alice", "")
	if !bytes.Contains(body, []byte(`"tweets":[]`)) {
		t.Fatalf("alice timeline post-unfollow: %s", body)
	}
}
