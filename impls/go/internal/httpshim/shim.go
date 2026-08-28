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
	"net/http"
	"os"
	"strconv"
	"strings"
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
	NextCursor *string     `json:"next_cursor"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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

func (h *handlers) users(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	var req struct {
		Handle string `json:"handle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	u, err := h.svc.CreateUser(req.Handle)
	if err != nil {
		switch err {
		case service.ErrEmptyHandle:
			writeErr(w, http.StatusBadRequest, "empty_handle")
		case store.ErrDuplicateUser:
			writeErrFor(w, r, http.StatusConflict, "duplicate_user")
		default:
			writeErr(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, userBody{Handle: u.Handle, ID: u.ID})
}

func (h *handlers) follow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodDelete:
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if r.Method == http.MethodPost {
		if err := h.svc.Follow(req.From, req.To); err != nil {
			switch err {
			case dom.ErrSelfFollow:
				writeErrFor(w, r, http.StatusBadRequest, "self_follow_forbidden")
			case store.ErrUnknownUser:
				writeErrFor(w, r, http.StatusBadRequest, "unknown_user")
			default:
				writeErr(w, http.StatusInternalServerError, "internal_error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// DELETE
	if err := h.svc.Unfollow(req.From, req.To); err != nil {
		switch err {
		case store.ErrUnknownUser:
			writeErrFor(w, r, http.StatusBadRequest, "unknown_user")
		default:
			writeErr(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) tweets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	var req struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	t, err := h.svc.PostTweet(req.Author, req.Text)
	if err != nil {
		switch err {
		case service.ErrEmptyText:
			writeErr(w, http.StatusBadRequest, "empty_text")
		case store.ErrUnknownUser:
			writeErrFor(w, r, http.StatusBadRequest, "unknown_user")
		default:
			writeErr(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, tweetBody{
		ID: t.ID, Author: t.Author, Text: t.Text, CreatedAt: t.CreatedAt,
	})
}

func (h *handlers) timeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	user := r.URL.Query().Get("user")
	if user == "" {
		writeErr(w, http.StatusBadRequest, "empty_user")
		return
	}
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v < 0 {
			writeErr(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = v
	}
	tw := h.svc.HomeTimeline(user, limit)
	out := timelineBody{Tweets: make([]tweetBody, 0, len(tw))}
	for _, t := range tw {
		out.Tweets = append(out.Tweets, tweetBody{
			ID: t.ID, Author: t.Author, Text: t.Text, CreatedAt: t.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
