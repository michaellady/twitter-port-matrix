//go:build race

// Race-condition test. The Go race detector is the oracle: this test
// hammers Follow/Unfollow/PostTweet/HomeTimeline from many goroutines
// against a shared service. With `go test -race -tags=race ./...` the
// detector flags any unsynchronized access.
//
// F5 (data-race freedom, partial-correctness scope) is what this
// discharges at the implementation level. Liveness (deadlock,
// starvation) is explicitly out of scope.
package httpshim

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
)

const (
	raceWorkers = 8
	raceOps     = 200
)

func TestRace_HammerStore(t *testing.T) {
	srv := httptest.NewServer(New(service.New(clock.New())))
	defer srv.Close()

	// Seed: create raceWorkers users in advance so every worker has a
	// real handle to operate on.
	handles := make([]string, raceWorkers)
	for i := 0; i < raceWorkers; i++ {
		h := "u" + strconv.Itoa(i)
		handles[i] = h
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/users",
			bytes.NewBufferString(`{"handle":"`+h+`"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("seed %s: %v", h, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("seed %s: status=%d", h, resp.StatusCode)
		}
	}

	var wg sync.WaitGroup
	wg.Add(raceWorkers)

	// Each worker performs a deterministic mix of ops. The op selector
	// is op := j % 4 so the workload covers all four mutators evenly.
	for i := 0; i < raceWorkers; i++ {
		i := i
		go func() {
			defer wg.Done()
			from := handles[i]
			to := handles[(i+1)%raceWorkers]
			client := srv.Client()
			for j := 0; j < raceOps; j++ {
				switch j % 4 {
				case 0:
					body := `{"from":"` + from + `","to":"` + to + `"}`
					req, _ := http.NewRequest(http.MethodPost, srv.URL+"/follow", bytes.NewBufferString(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err != nil {
						t.Errorf("follow err: %v", err)
						return
					}
					resp.Body.Close()
					if resp.StatusCode != http.StatusNoContent {
						t.Errorf("follow status=%d", resp.StatusCode)
						return
					}
				case 1:
					body := `{"from":"` + from + `","to":"` + to + `"}`
					req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/follow", bytes.NewBufferString(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err != nil {
						t.Errorf("unfollow err: %v", err)
						return
					}
					resp.Body.Close()
					if resp.StatusCode != http.StatusNoContent {
						t.Errorf("unfollow status=%d", resp.StatusCode)
						return
					}
				case 2:
					body := `{"author":"` + from + `","text":"hi-` + strconv.Itoa(j) + `"}`
					req, _ := http.NewRequest(http.MethodPost, srv.URL+"/tweets", bytes.NewBufferString(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err != nil {
						t.Errorf("tweet err: %v", err)
						return
					}
					resp.Body.Close()
					if resp.StatusCode != http.StatusCreated {
						t.Errorf("tweet status=%d", resp.StatusCode)
						return
					}
				case 3:
					req, _ := http.NewRequest(http.MethodGet, srv.URL+"/timeline?user="+from, nil)
					resp, err := client.Do(req)
					if err != nil {
						t.Errorf("timeline err: %v", err)
						return
					}
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Errorf("timeline status=%d", resp.StatusCode)
						return
					}
				}
			}
		}()
	}
	wg.Wait()

	// Sanity: timeline still callable post-hammer.
	resp, err := srv.Client().Get(srv.URL + "/timeline?user=" + handles[0])
	if err != nil {
		t.Fatalf("post-hammer timeline err: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post-hammer timeline status=%d", resp.StatusCode)
	}
}
