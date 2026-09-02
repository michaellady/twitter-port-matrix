// Concurrency regression tests for F018.
//
// # WHY THESE EXIST, AND WHY THEY ARE NOT A RUNG
//
// Every rung in ASSURANCE.md is derived from `S_obs`, and `S_obs` is a
// deterministic sequential state machine with no vocabulary for interleaving.
// R0 replays a corpus, R1 replays traces, R2 drives properties -- all
// single-threaded against an oracle that cannot express two requests being
// in flight at once. So no rung can score a concurrency defect, and F018
// survived all of them on a corner passing 56/56.
//
// The oracle these tests use instead is a COUNTING one, and it is available
// precisely because it does not consult `S_obs`:
//
//	N concurrent accepted writes must produce N distinct records, and no
//	write may be reported as a server error.
//
// That is checkable without a reference machine, which is the whole reason
// it can see what every S_obs-derived rung is blind to.
//
// These are ordinary `go test` tests with no build tag, unlike race_test.go,
// so they run in the default suite. The race detector is NOT the oracle here:
// the F018 defect is a lost update through correctly-locked state, which the
// detector does not flag and did not flag. Only counting the results finds it.
package httpshim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
)

// The F018 audit saw the monotonicity guard fire 49 times in 1,280 concurrent
// POST /tweets, and the handle-taken branch once. 1,280 is kept as the width
// so a regression reproduces at the rate that was measured, not at a rate
// this file chose to be comfortable.
const (
	concWorkers  = 64
	concPerWork  = 20
	concRequests = concWorkers * concPerWork // 1280
)

func postJSON(t *testing.T, c *http.Client, url, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, buf.Bytes()
}

// result carries one worker request's outcome back to the counting oracle.
type result struct {
	status int
	body   string
	id     int64
}

func runConcurrent(t *testing.T, srv *httptest.Server, bodyFor func(w, j int) string, path string) []result {
	t.Helper()
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("needs GOMAXPROCS >= 2 to interleave")
	}
	out := make([]result, concRequests)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(concWorkers)
	for w := 0; w < concWorkers; w++ {
		w := w
		go func() {
			defer wg.Done()
			client := srv.Client()
			<-start // release all workers at once, to maximise overlap
			for j := 0; j < concPerWork; j++ {
				status, body := postJSON(t, client, srv.URL+path, bodyFor(w, j))
				var parsed struct {
					ID int64 `json:"id"`
				}
				_ = json.Unmarshal(body, &parsed)
				out[w*concPerWork+j] = result{status: status, body: string(body), id: parsed.ID}
			}
		}()
	}
	close(start)
	wg.Wait()
	return out
}

// summarise reports, per distinct (status, body) pair, how many requests
// produced it -- so a failure names the wire error the client actually saw
// rather than only a count.
func summarise(rs []result, want int) (bad int, detail string) {
	byResp := map[string]int{}
	for _, r := range rs {
		if r.status != want {
			byResp[fmt.Sprintf("%d %s", r.status, r.body)]++
			bad++
		}
	}
	for k, n := range byResp {
		detail += fmt.Sprintf("\n    %4d x %s", n, k)
	}
	return bad, detail
}

// TestF018_ConcurrentPostTweetLosesNoTweet is the regression test for F018.
//
// Before the fix, `Service.PostTweet` took an id from the generator and read
// the clock OUTSIDE the store lock, then appended. Two goroutines holding ids
// 5 and 6 race to append; the one holding 5 loses, `MemStore.PutTweet`'s
// monotonicity guard rejects it for being out of order relative to a tweet
// that only exists because it lost the race, and `ErrNonMonotonic` -- absent
// from `writeErrFromDomain`'s table -- falls through to `default` and becomes
// HTTP 500 `internal_error`. The tweet is discarded with no other trace.
//
// The assertion is deliberately not "the guard never fires": that would pass
// against a fix that merely mapped ErrNonMonotonic to a nicer status code
// while still dropping the tweet. It is that every accepted request produced
// a durable, distinctly-identified tweet.
func TestF018_ConcurrentPostTweetLosesNoTweet(t *testing.T) {
	srv := httptest.NewServer(New(service.New(clock.New())))
	defer srv.Close()

	for w := 0; w < concWorkers; w++ {
		h := fmt.Sprintf("u%03d", w)
		if status, body := postJSON(t, srv.Client(), srv.URL+"/users",
			fmt.Sprintf(`{"handle":%q}`, h)); status != http.StatusCreated {
			t.Fatalf("seed user %s: status=%d body=%s", h, status, body)
		}
	}

	rs := runConcurrent(t, srv, func(w, j int) string {
		return fmt.Sprintf(`{"author":"u%03d","text":"t-%d-%d"}`, w, w, j)
	}, "/tweets")

	if bad, detail := summarise(rs, http.StatusCreated); bad > 0 {
		t.Errorf("F018: %d of %d concurrent POST /tweets were not 201 Created:%s",
			bad, concRequests, detail)
	}

	// Independently of the status codes: count what actually landed. A fix
	// that returns 201 while dropping the write would pass the check above
	// and must not pass this one.
	ids := map[int64]int{}
	for _, r := range rs {
		if r.status == http.StatusCreated {
			ids[r.id]++
		}
	}
	for id, n := range ids {
		if n > 1 {
			t.Errorf("F018: tweet id %d handed to %d requests; ids must be unique", id, n)
		}
	}
	readable := 0
	for w := 0; w < concWorkers; w++ {
		readable += countTimeline(t, srv, fmt.Sprintf("u%03d", w))
	}
	if readable != len(ids) {
		t.Errorf("F018: %d tweets accepted but %d are readable back; %d lost",
			len(ids), readable, len(ids)-readable)
	}
	if len(ids) != concRequests {
		t.Errorf("F018: %d of %d tweets survived", len(ids), concRequests)
	}
}

// TestF018_ConcurrentCreateUserBurnsNoID covers the sibling instance named in
// the finding: `Service.CreateUser` checks `HasUser`, then allocates a user
// id, then calls `PutUser` -- three steps, no single lock. The F018 audit saw
// `PutUser`'s handle-taken branch reached once in 1,280 concurrent
// registrations.
//
// Losing that race is not itself a wrong answer -- `handle_taken` is what
// `S_obs` says the second registration gets. The observable divergence is the
// id: `Service.CreateUser`'s own comment records that it "rejects a duplicate
// BEFORE consuming an id ... S_obs allocates only on success", and the losing
// goroutine has already consumed one by the time it finds out. So the winner
// of a later handle is handed an id that skips the burned ones, and the wire
// shows a gap `S_obs` cannot produce.
//
// Every worker races for the same small pool of handles, in the same order,
// so each handle is contested by all `concWorkers` goroutines at once.
func TestF018_ConcurrentCreateUserBurnsNoID(t *testing.T) {
	srv := httptest.NewServer(New(service.New(clock.New())))
	defer srv.Close()

	rs := runConcurrent(t, srv, func(w, j int) string {
		return fmt.Sprintf(`{"handle":"h%03d"}`, j)
	}, "/users")

	created, taken := map[int64]bool{}, 0
	var unexpected []result
	for _, r := range rs {
		switch {
		case r.status == http.StatusCreated:
			if created[r.id] {
				t.Errorf("F018: user id %d handed to more than one request", r.id)
			}
			created[r.id] = true
		case r.status == http.StatusConflict:
			taken++
		default:
			unexpected = append(unexpected, r)
		}
	}
	if len(unexpected) > 0 {
		bad, detail := summarise(unexpected, -1)
		t.Errorf("F018: %d concurrent POST /users answered neither 201 nor 409:%s", bad, detail)
	}
	if len(created) != concPerWork {
		t.Errorf("F018: %d handles registered, want %d (each handle must succeed exactly once)",
			len(created), concPerWork)
	}
	if want := concRequests - concPerWork; taken != want {
		t.Errorf("F018: %d requests got 409 handle_taken, want %d", taken, want)
	}

	// The id assertion. S_obs allocates a user id only on a successful
	// registration, so N successful registrations must consume exactly the
	// ids 1..N with no gap. A gap is an id burned by a rejected request --
	// the observable trace of the check-then-allocate-then-put race.
	for id := int64(1); id <= int64(len(created)); id++ {
		if !created[id] {
			var got []int64
			for k := range created {
				got = append(got, k)
			}
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			t.Errorf("F018: user id %d was burned by a rejected registration; "+
				"%d handles registered but the assigned ids are %v", id, len(created), got)
			break
		}
	}
}

// countTimeline pages the whole timeline of `user` and returns how many
// tweets are readable. Every seeded user follows nobody, so this counts that
// user's own tweets only; the caller sums it over all authors to get the
// number of tweets that actually reached the log.
func countTimeline(t *testing.T, srv *httptest.Server, user string) int {
	t.Helper()
	total, cursor := 0, int64(0)
	for {
		url := fmt.Sprintf("%s/timeline?user=%s&limit=50", srv.URL, user)
		if cursor > 0 {
			url = fmt.Sprintf("%s&cursor=%d", url, cursor)
		}
		resp, err := srv.Client().Get(url)
		if err != nil {
			t.Fatalf("timeline: %v", err)
		}
		var page struct {
			Tweets []struct {
				ID int64 `json:"id"`
			} `json:"tweets"`
			NextCursor *int64 `json:"next_cursor"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			t.Fatalf("timeline decode: %v", err)
		}
		resp.Body.Close()
		total += len(page.Tweets)
		if page.NextCursor == nil {
			return total
		}
		cursor = *page.NextCursor
	}
}
