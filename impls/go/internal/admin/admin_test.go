package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
)

const testToken = "test-admin-token-please-rotate"

// newServer wires up a Service + admin handlers with a known token.
// Sets ADMIN_TOKEN for the lifetime of Mount, then restores it.
func newServer(t *testing.T, token string) (*service.Service, *http.ServeMux) {
	t.Helper()
	prev := os.Getenv("ADMIN_TOKEN")
	t.Cleanup(func() { _ = os.Setenv("ADMIN_TOKEN", prev) })
	_ = os.Setenv("ADMIN_TOKEN", token)
	svc := service.New(clock.New())
	mux := http.NewServeMux()
	Mount(mux, svc)
	return svc, mux
}

func do(mux http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// seedSvc populates the service with alice/bob/carol + sample tweets +
// alice→bob follow. Mirrors cmd/server/main.go's seedDemo so the tests
// match what the smoke test exercises.
func seedSvc(t *testing.T, svc *service.Service) {
	t.Helper()
	for _, h := range []string{"alice", "bob", "carol"} {
		if _, err := svc.CreateUser(h); err != nil {
			t.Fatalf("CreateUser(%s): %v", h, err)
		}
	}
	if _, err := svc.PostTweet("alice", "hello from alice"); err != nil {
		t.Fatalf("PostTweet alice: %v", err)
	}
	if _, err := svc.PostTweet("bob", "hello from bob"); err != nil {
		t.Fatalf("PostTweet bob: %v", err)
	}
	if err := svc.Follow("alice", "bob"); err != nil {
		t.Fatalf("Follow alice→bob: %v", err)
	}
}

func TestSnapshotRequiresAuth(t *testing.T) {
	_, mux := newServer(t, testToken)
	w := do(mux, http.MethodPost, "/_admin/snapshot", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", w.Code)
	}
	w = do(mux, http.MethodPost, "/_admin/snapshot", nil, map[string]string{"X-Admin-Token": "wrong"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: want 401, got %d", w.Code)
	}
}

func TestLoadSnapshotRequiresAuth(t *testing.T) {
	_, mux := newServer(t, testToken)
	w := do(mux, http.MethodPost, "/_admin/load-snapshot", []byte(`{"snapshot_version":1,"state":{}}`), nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAdminDisabledWhenTokenEmpty(t *testing.T) {
	_, mux := newServer(t, "")
	w := do(mux, http.MethodPost, "/_admin/snapshot", nil, map[string]string{"X-Admin-Token": "anything"})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin_disabled") {
		t.Fatalf("want admin_disabled body, got %q", w.Body.String())
	}
}

func TestSnapshotMethodNotAllowed(t *testing.T) {
	_, mux := newServer(t, testToken)
	w := do(mux, http.MethodGet, "/_admin/snapshot", nil, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

func TestSnapshotShape(t *testing.T) {
	svc, mux := newServer(t, testToken)
	seedSvc(t, svc)

	w := do(mux, http.MethodPost, "/_admin/snapshot", nil, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}

	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, _ := env["snapshot_version"].(float64); int(v) != 1 {
		t.Fatalf("snapshot_version: want 1, got %v", env["snapshot_version"])
	}
	for _, k := range []string{"snapshot_version", "captured_at_unix_nanos", "git_sha", "image_digest", "state"} {
		if _, ok := env[k]; !ok {
			t.Fatalf("missing top-level field %q", k)
		}
	}
	state, ok := env["state"].(map[string]any)
	if !ok {
		t.Fatalf("state not an object")
	}
	for _, k := range []string{"clock_now", "id_counter_users", "id_counter_tweets", "users", "follows", "tweets"} {
		if _, ok := state[k]; !ok {
			t.Fatalf("missing state field %q", k)
		}
	}

	users, _ := state["users"].([]any)
	if len(users) != 3 {
		t.Fatalf("want 3 users, got %d", len(users))
	}
	// determinism: users sorted by id ascending → alice, bob, carol
	wantHandles := []string{"alice", "bob", "carol"}
	for i, u := range users {
		um := u.(map[string]any)
		if um["handle"].(string) != wantHandles[i] {
			t.Fatalf("user[%d]: want %s got %s", i, wantHandles[i], um["handle"])
		}
	}

	follows, _ := state["follows"].([]any)
	if len(follows) != 1 {
		t.Fatalf("want 1 follow, got %d", len(follows))
	}
	tweets, _ := state["tweets"].([]any)
	if len(tweets) != 2 {
		t.Fatalf("want 2 tweets, got %d", len(tweets))
	}
	if v, _ := state["id_counter_users"].(float64); int64(v) != 3 {
		t.Fatalf("id_counter_users: want 3, got %v", state["id_counter_users"])
	}
	if v, _ := state["id_counter_tweets"].(float64); int64(v) != 2 {
		t.Fatalf("id_counter_tweets: want 2, got %v", state["id_counter_tweets"])
	}
}

func TestLoadSnapshotMalformedJSON(t *testing.T) {
	_, mux := newServer(t, testToken)
	w := do(mux, http.MethodPost, "/_admin/load-snapshot", []byte(`{not json`), map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_json") {
		t.Fatalf("want invalid_json, got %q", w.Body.String())
	}
}

func TestLoadSnapshotUnsupportedVersion(t *testing.T) {
	_, mux := newServer(t, testToken)
	body := []byte(`{"snapshot_version":99,"state":{}}`)
	w := do(mux, http.MethodPost, "/_admin/load-snapshot", body, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "snapshot_version_unsupported" {
		t.Fatalf("error: want snapshot_version_unsupported, got %v", resp["error"])
	}
	if v, _ := resp["received"].(float64); int(v) != 99 {
		t.Fatalf("received: want 99, got %v", resp["received"])
	}
	supported, _ := resp["supported"].([]any)
	if len(supported) != 2 {
		t.Fatalf("supported: want len=2, got %v", resp["supported"])
	}
}

func TestLoadSnapshotAcceptsV2WithUnknownFields(t *testing.T) {
	_, mux := newServer(t, testToken)
	// Per K15 tolerant forward read: a v(N+1) snapshot with extra fields
	// must be accepted by the v(N) consumer (this impl).
	body := []byte(`{
		"snapshot_version": 2,
		"captured_at_unix_nanos": 1714600000000000000,
		"git_sha": "future",
		"image_digest": "sha256:future",
		"new_top_level_field": "ignored",
		"state": {
			"clock_now": 0,
			"id_counter_users": 0,
			"id_counter_tweets": 0,
			"users": [],
			"follows": [],
			"tweets": [],
			"new_state_field": {"reason": "still ignored"}
		}
	}`)
	w := do(mux, http.MethodPost, "/_admin/load-snapshot", body, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestSnapshotRoundTripAddsUser(t *testing.T) {
	// Capture seeded snapshot, hand-edit to add `dave`, load into a fresh
	// service, verify dave appears + alice's followee remains bob.
	src, srcMux := newServer(t, testToken)
	seedSvc(t, src)
	w := do(srcMux, http.MethodPost, "/_admin/snapshot", nil, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot: %d", w.Code)
	}
	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	state := env["state"].(map[string]any)
	users := state["users"].([]any)
	state["users"] = append(users, map[string]any{"id": float64(4), "handle": "dave"})
	state["id_counter_users"] = float64(4)

	dst, dstMux := newServer(t, testToken)
	body, _ := json.Marshal(env)
	w = do(dstMux, http.MethodPost, "/_admin/load-snapshot", body, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusNoContent {
		t.Fatalf("load: want 204, got %d body=%s", w.Code, w.Body.String())
	}
	if !dst.HasUser("dave") {
		t.Fatalf("dave missing after load")
	}
	if !dst.HasUser("alice") || !dst.HasUser("bob") {
		t.Fatalf("alice/bob missing after load")
	}
	// alice's home timeline must include bob's tweet (follow restored).
	tw, _ := dst.HomeTimeline("alice", 1000, 0)
	if len(tw) != 2 {
		t.Fatalf("alice timeline: want 2 tweets, got %d", len(tw))
	}
}

func TestLoadSnapshotMissingState(t *testing.T) {
	_, mux := newServer(t, testToken)
	body := []byte(`{"snapshot_version":1}`)
	w := do(mux, http.MethodPost, "/_admin/load-snapshot", body, map[string]string{"X-Admin-Token": testToken})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
