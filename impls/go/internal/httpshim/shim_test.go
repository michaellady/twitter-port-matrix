package httpshim

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
)

func newServer() *httptest.Server {
	return httptest.NewServer(New(service.New(clock.New())))
}

func do(t *testing.T, srv *httptest.Server, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	var rdr *bytes.Buffer
	if body != "" {
		rdr = bytes.NewBufferString(body)
	} else {
		rdr = bytes.NewBufferString("")
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
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

func bodyContainsKey(t *testing.T, raw []byte, key string) any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("body not JSON: %s err=%v", raw, err)
	}
	v, ok := m[key]
	if !ok {
		t.Fatalf("body missing key %q: %s", key, raw)
	}
	return v
}

func TestUsersRejectsNonPost(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	for _, m := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		resp, _ := do(t, srv, m, "/users", "")
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s /users got status=%d, want 405", m, resp.StatusCode)
		}
	}
}

func TestUsersInvalidJSON(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, body := do(t, srv, http.MethodPost, "/users", "not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
	if got := bodyContainsKey(t, body, "error"); got != "invalid_json" {
		t.Fatalf("error=%v, want invalid_json", got)
	}
}

func TestUsersEmptyHandle(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, body := do(t, srv, http.MethodPost, "/users", `{"handle":""}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := bodyContainsKey(t, body, "error"); got != "empty_handle" {
		t.Fatalf("error=%v, want empty_handle", got)
	}
}

func TestUsersDuplicate(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, _ := do(t, srv, http.MethodPost, "/users", `{"handle":"alice"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create status=%d", resp.StatusCode)
	}
	resp, body := do(t, srv, http.MethodPost, "/users", `{"handle":"alice"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second create status=%d, want 409", resp.StatusCode)
	}
	if got := bodyContainsKey(t, body, "error"); got != "duplicate_user" {
		t.Fatalf("error=%v", got)
	}
}

func TestFollowMethodNotAllowed(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, _ := do(t, srv, http.MethodGet, "/follow", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, want 405", resp.StatusCode)
	}
}

func TestFollowInvalidJSON(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, _ := do(t, srv, http.MethodPost, "/follow", "{")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestTweetsMethodNotAllowed(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, _ := do(t, srv, http.MethodGet, "/tweets", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestTweetsInvalidJSON(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, _ := do(t, srv, http.MethodPost, "/tweets", "{")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestTweetsEmptyText(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	_, _ = do(t, srv, http.MethodPost, "/users", `{"handle":"alice"}`)
	resp, body := do(t, srv, http.MethodPost, "/tweets", `{"author":"alice","text":""}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := bodyContainsKey(t, body, "error"); got != "empty_text" {
		t.Fatalf("error=%v", got)
	}
}

func TestTweetsUnknownAuthor(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, body := do(t, srv, http.MethodPost, "/tweets", `{"author":"ghost","text":"hi"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := bodyContainsKey(t, body, "error"); got != "unknown_user" {
		t.Fatalf("error=%v", got)
	}
}

func TestTimelineMethodNotAllowed(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, _ := do(t, srv, http.MethodPost, "/timeline?user=alice", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestTimelineMissingUser(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, body := do(t, srv, http.MethodGet, "/timeline", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := bodyContainsKey(t, body, "error"); got != "empty_user" {
		t.Fatalf("error=%v", got)
	}
}

func TestTimelineInvalidLimit(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	for _, q := range []string{"?user=alice&limit=-1", "?user=alice&limit=abc"} {
		resp, body := do(t, srv, http.MethodGet, "/timeline"+q, "")
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("limit=%q status=%d", q, resp.StatusCode)
		}
		if got := bodyContainsKey(t, body, "error"); got != "invalid_limit" {
			t.Fatalf("error=%v", got)
		}
	}
}

func TestTimelineEmptyForUnknownUser(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	resp, body := do(t, srv, http.MethodGet, "/timeline?user=ghost", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"tweets":[]`) {
		t.Fatalf("expected empty tweets array, got %s", body)
	}
}

func TestUnfollowOnUnknownReturns400(t *testing.T) {
	srv := newServer()
	defer srv.Close()
	_, _ = do(t, srv, http.MethodPost, "/users", `{"handle":"alice"}`)
	resp, _ := do(t, srv, http.MethodDelete, "/follow", `{"from":"alice","to":"ghost"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
