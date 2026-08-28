// Package httpshim is the EXPLICITLY TRUSTED HTTP boundary.
//
// This file is part of the trusted computing base — it is not verified by
// Gobra. Gobra has no support for net/http; verification stops at the call
// into service.*. Correctness of this layer is established by the
// functional, integration, conformance, and race tests.
//
// Discipline: handlers do exactly three things — decode the request, call a
// verified service function, encode the response. No business logic here.
package httpshim

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/metrics"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/store"
)

var startedAt = time.Now()

// osReadFile is os.ReadFile with a tiny indirection so unit tests could
// stub it later if needed. Phase 1 trusts the filesystem.
var osReadFile = os.ReadFile

// New returns an http.Handler wired to svc with only the JSON-API routes.
// Used by tests and by main.go when no UI is mounted.
func New(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	Register(mux, svc)
	return mux
}

// Register attaches the JSON-API routes onto the given mux. Lets main.go
// own the mux so other layers (e.g. internal/ui) can mount their own
// routes on the same mux. Go's ServeMux uses longest-prefix-match, so
// the specific JSON paths (/users, /follow, /tweets, /timeline,
// /healthz, /version) take precedence over the UI's catch-all "/".
func Register(mux *http.ServeMux, svc *service.Service) {
	h := &handlers{svc: svc}
	mux.HandleFunc("/users", h.users)
	mux.HandleFunc("/follow", h.follow)
	mux.HandleFunc("/tweets", h.tweets)
	mux.HandleFunc("/timeline", h.timeline)
	mux.HandleFunc("/tick", h.tick)
	// Catch-all. Without it net/http's ServeMux answers an unrouted path with
	// its own plain-text "404 page not found", which is not the observable
	// contract: S_obs requires {"error":"not_found"}. Totality means every
	// request has a DEFINED response, so the default route is part of the
	// contract rather than a fallback.
	mux.HandleFunc("/", h.notFound)
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/version", h.version)
}

// healthz: liveness/readiness probe used by Fly's load balancer.
func (h *handlers) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// version: Tier-4 phase 1 image-digest provenance. Reads
// /etc/version.json baked into the image at build time. On rollback
// the previous image is re-pulled, so /version automatically reports
// the rolled-back release's digest (K21 in the converged Tier-4 plan).
func (h *handlers) version(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"git_sha":                gitSHA,
		"image_digest":           imageDigest,
		"process_uptime_seconds": int(time.Since(startedAt).Seconds()),
		"snapshot_version":       1,
	})
}

type handlers struct {
	svc *service.Service
}

type errBody struct {
	Error string `json:"error"`
}

type userBody struct {
	Handle string `json:"handle"`
	ID     int64  `json:"id"`
}

type tweetBody struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

type timelineBody struct {
	Tweets     []tweetBody `json:"tweets"`
	NextCursor *int64      `json:"next_cursor"`
}

// clockBody renders {"clock":<n>}.
type clockBody struct {
	Clock int64 `json:"clock"`
}

// Pagination bounds from S_obs decision D10.
const (
	defaultLimit = 50
	maxLimit     = 100
)

// writeJSON emits the canonical encoding required by S_obs decision D8.
//
// json.Encoder.Encode appends a trailing newline; Marshal does not. Under a
// byte-equality conformance rule that newline is a real observable
// difference, and it accounted for 8 of the 54 R0 baseline steps.
func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, errBody{Error: code})
}

// writeErrFor mirrors writeErr but also increments the F-property
// violations counter for the (code, path) pair if it maps to one. Use
// this in handlers that issue F-property error codes; pure write
// failures (json decode, method-not-allowed) keep using writeErr.
func writeErrFor(w http.ResponseWriter, r *http.Request, status int, code string) {
	if f := metrics.FromError(code, r.URL.Path); f != "" {
		metrics.NoteViolation(f)
	}
	writeJSON(w, status, errBody{Error: code})
}

// decodeStrict parses exactly one JSON object into dst, rejecting unknown
// fields and trailing content (S_obs decision D7).
//
// Lenient parsing is a classic source of cross-language divergence: it is
// precisely where two implementations can accept different inputs and both
// look correct. Ten of the 54 R0 baseline steps failed here.
//
// Known limitation, stated rather than hidden: duplicate JSON keys resolve
// last-wins, which is Go's decoder behaviour. No generated trace emits them.
func decodeStrict(r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		return false
	}
	return true
}

// writeErrFromDomain maps a core error onto its wire status.
//
// The verified core already speaks the observable vocabulary, so this is a
// status lookup rather than a renaming. A translation table here would be one
// more place for the vocabulary to drift out from under the proofs.
func writeErrFromDomain(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case store.ErrHandleTaken:
		writeErrFor(w, r, http.StatusConflict, "handle_taken")
	case store.ErrUnknownUser:
		writeErrFor(w, r, http.StatusBadRequest, "unknown_user")
	case dom.ErrSelfFollow:
		writeErrFor(w, r, http.StatusBadRequest, "self_follow_forbidden")
	case dom.ErrInvalidHandle:
		writeErr(w, http.StatusBadRequest, "invalid_handle")
	case dom.ErrInvalidText:
		writeErr(w, http.StatusBadRequest, "invalid_text")
	default:
		writeErr(w, http.StatusInternalServerError, "internal_error")
	}
}

func (h *handlers) users(w http.ResponseWriter, r *http.Request) {
	if !exactRoute(w, r, http.MethodPost, "/users", false) {
		return
	}
	var req struct {
		Handle *string `json:"handle"`
	}
	if !decodeStrict(r, &req) || req.Handle == nil {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	u, err := h.svc.CreateUser(*req.Handle)
	if err != nil {
		writeErrFromDomain(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, userBody{Handle: u.Handle, ID: u.ID})
}

func (h *handlers) follow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if !exactRoute(w, r, r.Method, "/follow", false) {
		return
	}
	var req struct {
		From *string `json:"from"`
		To   *string `json:"to"`
	}
	if !decodeStrict(r, &req) || req.From == nil || req.To == nil {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	var err error
	if r.Method == http.MethodPost {
		err = h.svc.Follow(*req.From, *req.To)
	} else {
		err = h.svc.Unfollow(*req.From, *req.To)
	}
	if err != nil {
		writeErrFromDomain(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) tweets(w http.ResponseWriter, r *http.Request) {
	if !exactRoute(w, r, http.MethodPost, "/tweets", false) {
		return
	}
	var req struct {
		Author *string `json:"author"`
		Text   *string `json:"text"`
	}
	if !decodeStrict(r, &req) || req.Author == nil || req.Text == nil {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	t, err := h.svc.PostTweet(*req.Author, *req.Text)
	if err != nil {
		writeErrFromDomain(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, tweetBody{
		ID: t.ID, Author: t.Author, Text: t.Text, CreatedAt: t.CreatedAt,
	})
}

// tick advances the logical clock (S_obs decision D3).
//
// The clock previously had no route at all. That is why the shared corpus
// asserted a created_at no sequence of its own requests could reach, and why
// both conformance harnesses resolved it by writing to the clock directly --
// see evidence/findings/F001. One request now maps 1:1 onto one TLA+ Tick
// step, so every timestamp in a trace is produced by the trace.
func (h *handlers) tick(w http.ResponseWriter, r *http.Request) {
	if !exactRoute(w, r, http.MethodPost, "/tick", false) {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	// No TrimSpace: S_obs accepts exactly "" or "{}" (D3), so " {} " is
	// malformed. Trimming quietly widened the accept set. (F008)
	if s := string(body); s != "" && s != "{}" {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	h.svc.Tick()
	writeJSON(w, http.StatusOK, clockBody{Clock: h.svc.Now()})
}

func (h *handlers) timeline(w http.ResponseWriter, r *http.Request) {
	if !exactRoute(w, r, http.MethodGet, "/timeline", true) {
		return
	}
	if r.URL.RawQuery == "" {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	// r.URL.Query() discards the error url.ParseQuery returns, so a malformed
	// percent-escape silently dropped the parameter and the request was served
	// with a default. Parse explicitly and reject. (F008)
	q, qerr := url.ParseQuery(r.URL.RawQuery)
	if qerr != nil {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	// Unknown or repeated query parameters are rejected (D7).
	for k, v := range q {
		if k != "user" && k != "limit" && k != "cursor" {
			writeErr(w, http.StatusBadRequest, "malformed_request")
			return
		}
		if len(v) != 1 {
			writeErr(w, http.StatusBadRequest, "malformed_request")
			return
		}
	}
	users, ok := q["user"]
	if !ok {
		writeErr(w, http.StatusBadRequest, "malformed_request")
		return
	}
	user := users[0]
	if !dom.ValidHandle(user) {
		writeErr(w, http.StatusBadRequest, "invalid_handle")
		return
	}
	if !h.svc.HasUser(user) {
		writeErrFor(w, r, http.StatusBadRequest, "unknown_user")
		return
	}

	limit := defaultLimit
	if raw, ok := q["limit"]; ok {
		v, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || v < 1 || v > maxLimit {
			writeErr(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = int(v)
	}
	var cursor int64
	if raw, ok := q["cursor"]; ok {
		v, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || v < 1 {
			writeErr(w, http.StatusBadRequest, "invalid_cursor")
			return
		}
		cursor = v
	}

	tw, more := h.svc.HomeTimeline(user, limit, cursor)
	out := timelineBody{Tweets: make([]tweetBody, 0, len(tw))}
	for _, t := range tw {
		out.Tweets = append(out.Tweets, tweetBody{
			ID: t.ID, Author: t.Author, Text: t.Text, CreatedAt: t.CreatedAt,
		})
	}
	// next_cursor is null exactly when nothing remains below the page (D10).
	if more && len(tw) > 0 {
		last := tw[len(tw)-1].ID
		out.NextCursor = &last
	}
	writeJSON(w, http.StatusOK, out)
}

// notFound is the total-by-construction default route (S_obs D7).
func (h *handlers) notFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "not_found")
}

// exactRoute enforces S_obs's routing, which net/http's ServeMux does not
// implement and cannot be configured into.
//
// Two gaps, both found by widening the trace alphabet (finding F008):
//
//   - ServeMux matches on r.URL.Path, which is PERCENT-DECODED, so
//     POST /%75sers reached the /users handler. S_obs compares the raw path,
//     so that is not a route.
//   - ServeMux ignores the query string entirely, so POST /users?x=1 reached
//     the handler. S_obs routes on (method, path) with no query for every
//     endpoint except /timeline.
//
// Returns false when the request is not this route, having already written
// the not_found response.
func exactRoute(w http.ResponseWriter, r *http.Request, method, path string, allowQuery bool) bool {
	if r.URL.EscapedPath() != path || r.Method != method {
		writeErr(w, http.StatusNotFound, "not_found")
		return false
	}
	if !allowQuery && r.URL.RawQuery != "" {
		writeErr(w, http.StatusNotFound, "not_found")
		return false
	}
	return true
}
