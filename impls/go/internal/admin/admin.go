// Package admin is the EXPLICITLY TRUSTED HTTP boundary for Stream 2's
// snapshot contract. It exposes two endpoints:
//
//   POST /_admin/snapshot      — capture full service state as JSON
//   POST /_admin/load-snapshot — replace full service state from JSON
//
// Both require an X-Admin-Token header that constant-time-compares
// (crypto/subtle) against the ADMIN_TOKEN env var. Bound at Mount-time
// so rotation requires a restart. If ADMIN_TOKEN is empty (the special
// "admin disabled" sentinel set by cmd/server/main.go when the env var
// is missing), all admin requests get 503 admin_disabled.
//
// Schema: snapshot_version=1. Per K9/K15, the consumer accepts versions
// N-1, N, N+1 (currently 1 and 2; ignore unknown fields when loading 2).
// Anything else returns HTTP 422 snapshot_version_unsupported.
//
// This package is in TCB.md (Stream 2 Phase 0). Stdlib only — no
// third-party deps; project still has zero go.sum.
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/store"
)

// CurrentSnapshotVersion is what this impl emits when called as producer.
const CurrentSnapshotVersion = 1

// supportedVersions are the versions the consumer (load-snapshot) will
// accept per the K9/K15 rolling-compatibility window: N-1, N, N+1. With
// N=1, "N-1" (0) is excluded as nonsensical; we accept {1, 2}.
var supportedVersions = []int{1, 2}

// osReadFile is os.ReadFile with a tiny indirection so unit tests can
// stub it. Mirrors httpshim.osReadFile.
var osReadFile = os.ReadFile

// Mount registers the admin endpoints on the given mux. Reads
// ADMIN_TOKEN at mount time and binds it into the handlers — rotation
// requires a server restart (intentional; matches DEPLOY.md rotation
// drill).
func Mount(mux *http.ServeMux, svc *service.Service) {
	token := os.Getenv("ADMIN_TOKEN")
	h := &handlers{svc: svc, token: token}
	mux.HandleFunc("/_admin/snapshot", h.snapshot)
	mux.HandleFunc("/_admin/load-snapshot", h.loadSnapshot)
}

type handlers struct {
	svc   *service.Service
	token string // empty == admin disabled
}

// JSON wire types. These names are part of the cross-impl contract;
// changing any of them is a snapshot_version bump.

type userWire struct {
	ID     int64  `json:"id"`
	Handle string `json:"handle"`
}

type followWire struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type tweetWire struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

type stateWire struct {
	ClockNow        int64        `json:"clock_now"`
	IDCounterUsers  int64        `json:"id_counter_users"`
	IDCounterTweets int64        `json:"id_counter_tweets"`
	Users           []userWire   `json:"users"`
	Follows         []followWire `json:"follows"`
	Tweets          []tweetWire  `json:"tweets"`
}

type snapshotEnvelope struct {
	SnapshotVersion     int       `json:"snapshot_version"`
	CapturedAtUnixNanos int64     `json:"captured_at_unix_nanos"`
	GitSHA              string    `json:"git_sha"`
	ImageDigest         string    `json:"image_digest"`
	State               stateWire `json:"state"`
}

// loadEnvelope is the consumer-side decoding shape. Identical to
// snapshotEnvelope except we use json.RawMessage for State so we can
// route on snapshot_version BEFORE attempting to decode unknown fields.
type loadEnvelope struct {
	SnapshotVersion int             `json:"snapshot_version"`
	State           json.RawMessage `json:"state"`
}

// loadStateV1 is the consumer-side state decoder. Per K15, json.Decoder
// without DisallowUnknownFields means we silently ignore fields a future
// v(N+1) producer added — this is the "tolerant forward read" property.
type loadStateV1 struct {
	ClockNow        int64        `json:"clock_now"`
	IDCounterUsers  int64        `json:"id_counter_users"`
	IDCounterTweets int64        `json:"id_counter_tweets"`
	Users           []userWire   `json:"users"`
	Follows         []followWire `json:"follows"`
	Tweets          []tweetWire  `json:"tweets"`
}

func (h *handlers) snapshot(w http.ResponseWriter, r *http.Request) {
	if !h.checkMethod(w, r, http.MethodPost) {
		return
	}
	if !h.checkAuth(w, r) {
		return
	}
	snap := h.svc.Snapshot()
	gitSHA, imageDigest := readVersion()
	env := snapshotEnvelope{
		SnapshotVersion:     CurrentSnapshotVersion,
		CapturedAtUnixNanos: time.Now().UnixNano(),
		GitSHA:              gitSHA,
		ImageDigest:         imageDigest,
		State: stateWire{
			ClockNow:        snap.ClockNow,
			IDCounterUsers:  snap.IDCounterUsers,
			IDCounterTweets: snap.IDCounterTweets,
			Users:           usersToWire(snap.Store.Users),
			Follows:         followsToWire(snap.Store.Follows),
			Tweets:          tweetsToWire(snap.Store.Tweets),
		},
	}
	writeJSON(w, http.StatusOK, env)
}

func (h *handlers) loadSnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.checkMethod(w, r, http.MethodPost) {
		return
	}
	if !h.checkAuth(w, r) {
		return
	}
	var env loadEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if !versionSupported(env.SnapshotVersion) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":     "snapshot_version_unsupported",
			"supported": supportedVersions,
			"received":  env.SnapshotVersion,
		})
		return
	}
	var st loadStateV1
	if len(env.State) == 0 {
		writeErr(w, http.StatusBadRequest, "missing_state")
		return
	}
	if err := json.Unmarshal(env.State, &st); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_state")
		return
	}
	h.svc.LoadSnapshot(service.ServiceSnapshot{
		Store: store.StoreSnapshot{
			Users:   wireToUsers(st.Users),
			Follows: wireToFollows(st.Follows),
			Tweets:  wireToTweets(st.Tweets),
		},
		IDCounterUsers:  st.IDCounterUsers,
		IDCounterTweets: st.IDCounterTweets,
		ClockNow:        st.ClockNow,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) checkMethod(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method != want {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return false
	}
	return true
}

// checkAuth implements the X-Admin-Token gate. Empty server token =
// admin disabled (returns 503). Otherwise constant-time compare the
// header against the bound token.
func (h *handlers) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if h.token == "" {
		writeErr(w, http.StatusServiceUnavailable, "admin_disabled")
		return false
	}
	got := r.Header.Get("X-Admin-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) != 1 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func versionSupported(v int) bool {
	for _, ok := range supportedVersions {
		if v == ok {
			return true
		}
	}
	return false
}

// readVersion mirrors httpshim.version's read of /etc/version.json
// baked into the image at build time. On dev/local it returns the
// "dev" sentinels.
func readVersion() (string, string) {
	gitSHA := "dev"
	imageDigest := "sha256:dev"
	if data, err := osReadFile("/etc/version.json"); err == nil {
		var v struct {
			GitSHA      string `json:"git_sha"`
			ImageDigest string `json:"image_digest"`
		}
		if json.Unmarshal(data, &v) == nil {
			if v.GitSHA != "" {
				gitSHA = v.GitSHA
			}
			if v.ImageDigest != "" {
				imageDigest = v.ImageDigest
			}
		}
	}
	return gitSHA, imageDigest
}

func usersToWire(in []dom.User) []userWire {
	out := make([]userWire, 0, len(in))
	for _, u := range in {
		out = append(out, userWire{ID: u.ID, Handle: u.Handle})
	}
	return out
}

func followsToWire(in []dom.Follow) []followWire {
	out := make([]followWire, 0, len(in))
	for _, f := range in {
		out = append(out, followWire{From: f.From, To: f.To})
	}
	return out
}

func tweetsToWire(in []dom.Tweet) []tweetWire {
	out := make([]tweetWire, 0, len(in))
	for _, t := range in {
		out = append(out, tweetWire{
			ID: t.ID, Author: t.Author, Text: t.Text, CreatedAt: t.CreatedAt,
		})
	}
	return out
}

func wireToUsers(in []userWire) []dom.User {
	out := make([]dom.User, 0, len(in))
	for _, u := range in {
		out = append(out, dom.User{ID: u.ID, Handle: u.Handle})
	}
	return out
}

func wireToFollows(in []followWire) []dom.Follow {
	out := make([]dom.Follow, 0, len(in))
	for _, f := range in {
		out = append(out, dom.Follow{From: f.From, To: f.To})
	}
	return out
}

func wireToTweets(in []tweetWire) []dom.Tweet {
	out := make([]dom.Tweet, 0, len(in))
	for _, t := range in {
		out = append(out, dom.Tweet{
			ID: t.ID, Author: t.Author, Text: t.Text, CreatedAt: t.CreatedAt,
		})
	}
	return out
}

type errBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, errBody{Error: code})
}
