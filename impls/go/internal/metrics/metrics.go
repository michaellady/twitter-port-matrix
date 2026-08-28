// Package metrics is the Tier-4 Phase 4 observability layer.
// Prometheus exposition + JSON request log, both stdlib-only (no
// third-party deps; project still has no go.sum).
//
// Three metric families:
//   - http_requests_total{path,method,status}
//   - http_request_duration_seconds{path,method} — histogram
//   - f_property_violations_total{f} — incremented when an API
//     endpoint returns the JSON error code for an F-property violation.
//     Honest framing: the verified core ENFORCES F-properties by
//     REJECTING violating attempts; this counter measures the attempt
//     rate, not violations of the verified core itself.
package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// Counters and histograms (atomic-safe, lock-free hot path)
// =============================================================================

type counterKey struct{ path, method, status string }

type histoKey struct{ path, method string }

var (
	requestsMu   sync.RWMutex
	requests     = map[counterKey]*atomic.Uint64{}
	durationsMu  sync.RWMutex
	durations    = map[histoKey]*histogram{}
	violationsMu sync.RWMutex
	violations   = map[string]*atomic.Uint64{}
)

// histogram buckets in seconds (matches the Rust impl).
var bucketBounds = [...]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

const numBuckets = len(bucketBounds)

type histogram struct {
	buckets [numBuckets]atomic.Uint64
	infBkt  atomic.Uint64
	sum     atomic.Uint64 // microseconds (so we can do atomic add of integers)
	count   atomic.Uint64
}

func (h *histogram) observe(d time.Duration) {
	usec := uint64(d.Microseconds())
	h.sum.Add(usec)
	h.count.Add(1)
	secs := d.Seconds()
	for i, b := range bucketBounds {
		if secs <= b {
			h.buckets[i].Add(1)
			return
		}
	}
	h.infBkt.Add(1)
}

func incRequest(k counterKey) {
	requestsMu.RLock()
	c, ok := requests[k]
	requestsMu.RUnlock()
	if ok {
		c.Add(1)
		return
	}
	requestsMu.Lock()
	c, ok = requests[k]
	if !ok {
		c = &atomic.Uint64{}
		requests[k] = c
	}
	requestsMu.Unlock()
	c.Add(1)
}

func observeDuration(k histoKey, d time.Duration) {
	durationsMu.RLock()
	h, ok := durations[k]
	durationsMu.RUnlock()
	if ok {
		h.observe(d)
		return
	}
	durationsMu.Lock()
	h, ok = durations[k]
	if !ok {
		h = &histogram{}
		durations[k] = h
	}
	durationsMu.Unlock()
	h.observe(d)
}

// NoteViolation increments f_property_violations_total{f=<label>}. Call
// from API handlers when an F-property error is being returned.
func NoteViolation(f string) {
	violationsMu.RLock()
	c, ok := violations[f]
	violationsMu.RUnlock()
	if ok {
		c.Add(1)
		return
	}
	violationsMu.Lock()
	c, ok = violations[f]
	if !ok {
		c = &atomic.Uint64{}
		violations[f] = c
	}
	violationsMu.Unlock()
	c.Add(1)
}

// FromError maps a JSON error code + path to the F-property label.
// Hand-maintained: if a new ServiceError variant is added, extend here.
// Returns "" if the (code, path) pair is not an F-property attempt.
func FromError(code, path string) string {
	switch {
	case code == "self_follow_forbidden":
		return "F4"
	case code == "unknown_user" && strings.HasPrefix(path, "/tweets"):
		return "F6"
	case code == "unknown_user" && strings.HasPrefix(path, "/follow"):
		return "F9"
	case code == "handle_taken":
		return "F-uniqueness"
	}
	return ""
}

// =============================================================================
// Middleware: wraps every request, records counter + histogram + JSON log.
// =============================================================================

type recordingWriter struct {
	http.ResponseWriter
	status int
}

func (rw *recordingWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Track wraps an http.Handler. Adds Prometheus counter/histogram updates
// and a single-line JSON log per request.
func Track(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &recordingWriter{ResponseWriter: w, status: 200}
		h.ServeHTTP(rw, r)
		dur := time.Since(start)
		path := r.URL.Path
		method := r.Method
		incRequest(counterKey{path: path, method: method, status: strconv.Itoa(rw.status)})
		observeDuration(histoKey{path: path, method: method}, dur)
		// JSON log line to stdout (Fly captures it).
		_ = json.NewEncoder(loggingWriter{}).Encode(map[string]any{
			"ts":     start.Unix(),
			"level":  "info",
			"msg":    "http",
			"method": method,
			"path":   path,
			"status": rw.status,
			"dur_ms": dur.Milliseconds(),
		})
	})
}

type loggingWriter struct{}

func (loggingWriter) Write(p []byte) (int, error) {
	return fmt.Fprint(stdout(), string(p))
}

// stdout is indirected so tests can swap it.
var stdoutWriter = func() *fileLike {
	return &fileLike{}
}

func stdout() *fileLike { return stdoutWriter() }

// fileLike avoids importing os twice; just defers to fmt.Fprintf(os.Stderr).
// We use stderr so structured logs interleave with log.Printf output.
type fileLike struct{}

func (fileLike) Write(p []byte) (int, error) { return writeStderr(p) }

// writeStderr is a thin indirection so we can test without touching os.
var writeStderr = func(p []byte) (int, error) {
	return fmt.Fprint(stderrFile(), string(p))
}

// =============================================================================
// /metrics handler — Prometheus text exposition format
// =============================================================================

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		var b strings.Builder
		writeRequestsCounter(&b)
		writeDurationHistograms(&b)
		writeViolationsCounter(&b)
		_, _ = w.Write([]byte(b.String()))
	})
}

func writeRequestsCounter(b *strings.Builder) {
	fmt.Fprintln(b, "# HELP http_requests_total Total number of HTTP requests")
	fmt.Fprintln(b, "# TYPE http_requests_total counter")
	requestsMu.RLock()
	keys := make([]counterKey, 0, len(requests))
	for k := range requests {
		keys = append(keys, k)
	}
	requestsMu.RUnlock()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	for _, k := range keys {
		requestsMu.RLock()
		v := requests[k].Load()
		requestsMu.RUnlock()
		fmt.Fprintf(b, "http_requests_total{method=%q,path=%q,status=%q} %d\n",
			k.method, k.path, k.status, v)
	}
}

func writeDurationHistograms(b *strings.Builder) {
	fmt.Fprintln(b, "# HELP http_request_duration_seconds HTTP request duration")
	fmt.Fprintln(b, "# TYPE http_request_duration_seconds histogram")
	durationsMu.RLock()
	keys := make([]histoKey, 0, len(durations))
	for k := range durations {
		keys = append(keys, k)
	}
	durationsMu.RUnlock()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].method < keys[j].method
	})
	for _, k := range keys {
		durationsMu.RLock()
		h := durations[k]
		durationsMu.RUnlock()
		var cumulative uint64
		for i, bound := range bucketBounds {
			cumulative += h.buckets[i].Load()
			fmt.Fprintf(b, "http_request_duration_seconds_bucket{method=%q,path=%q,le=\"%g\"} %d\n",
				k.method, k.path, bound, cumulative)
		}
		cumulative += h.infBkt.Load()
		fmt.Fprintf(b, "http_request_duration_seconds_bucket{method=%q,path=%q,le=\"+Inf\"} %d\n",
			k.method, k.path, cumulative)
		fmt.Fprintf(b, "http_request_duration_seconds_sum{method=%q,path=%q} %f\n",
			k.method, k.path, float64(h.sum.Load())/1e6)
		fmt.Fprintf(b, "http_request_duration_seconds_count{method=%q,path=%q} %d\n",
			k.method, k.path, h.count.Load())
	}
}

func writeViolationsCounter(b *strings.Builder) {
	fmt.Fprintln(b, "# HELP f_property_violations_total Times a client attempted an action the verified core REJECTED. The verified core enforces F-properties by rejecting violating attempts; this counter measures the attempt rate, not violations of the verified core itself.")
	fmt.Fprintln(b, "# TYPE f_property_violations_total counter")
	violationsMu.RLock()
	labels := make([]string, 0, len(violations))
	for k := range violations {
		labels = append(labels, k)
	}
	violationsMu.RUnlock()
	sort.Strings(labels)
	for _, l := range labels {
		violationsMu.RLock()
		v := violations[l].Load()
		violationsMu.RUnlock()
		fmt.Fprintf(b, "f_property_violations_total{f=%q} %d\n", l, v)
	}
}
