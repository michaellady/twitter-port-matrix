// Concurrency regression test for F018, user-id axis.
//
// The tweet axis of F018 is covered over real HTTP in
// internal/httpshim/concurrency_test.go, where the defect is loud: the losing
// append is rejected and the client gets HTTP 500. This one is quiet -- the
// losing registration gets 409 `handle_taken`, which is the RIGHT answer --
// so it is tested here, at the service boundary, where the window between
// `HasUser` and `PutUser` is not swamped by HTTP round-trip jitter.
//
// Neither test consults `S_obs`. It is a sequential state machine with no
// vocabulary for interleaving, which is why R0, R1 and R2 -- all derived from
// it, all single-threaded -- slept through this defect on a corner passing
// 56/56. The oracle used instead is a counting one: N successful allocations
// must consume exactly N ids.
package service

import (
	"fmt"
	"sync"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/store"
)

const (
	// Workers contending for each handle, and handles contested in sequence.
	burnWorkers = 64
	burnHandles = 200
	// Trials. Each is an independent Service; the race is probabilistic, so
	// the test's reliability comes from repetition, not from one attempt.
	//
	// Measured against the unfixed code on a 4-vCPU box: 28 of 40 single-trial
	// runs detected the burn, so one trial catches it 70% of the time and ten
	// miss it with probability 0.3^10, about 6 in a million.
	// The rate is recorded here because it is the only thing that makes this
	// test's reliability checkable rather than asserted -- re-measure it
	// before trusting the number on different hardware.
	burnTrials = 10
)

// TestF018_ConcurrentCreateUserBurnsNoID.
//
// Before the fix, `CreateUser` ran three separately-atomic steps: `HasUser`
// under the store's read lock, `ids.Next()` under the generator's lock, and
// `PutUser` under the store's write lock. Two goroutines registering the same
// handle could both pass `HasUser`, both take an id, and only one land the
// write. The loser's answer is correct -- `handle_taken` is what `S_obs` says
// -- but the id it took is gone.
//
// That gap is observable: `S_obs` allocates a user id only on a successful
// registration, so N registrations must yield exactly the ids 1..N. A gap
// means a rejected request consumed one, which `S_obs` cannot do. The
// assertion is on the id set, not on the error, because the error was never
// wrong.
func TestF018_ConcurrentCreateUserBurnsNoID(t *testing.T) {
	for trial := 0; trial < burnTrials; trial++ {
		svc := New(clock.New())
		ids, taken := registerConcurrently(t, svc)

		if len(ids) != burnHandles {
			t.Fatalf("trial %d: %d handles registered, want %d", trial, len(ids), burnHandles)
		}
		if want := burnWorkers*burnHandles - burnHandles; taken != want {
			t.Fatalf("trial %d: %d duplicate registrations rejected, want %d",
				trial, taken, want)
		}
		// The whole assertion: the ids handed out are exactly 1..N.
		for id := int64(1); id <= int64(burnHandles); id++ {
			if !ids[id] {
				t.Fatalf("trial %d: F018: %d handles registered but id %d was never "+
					"handed out; a rejected registration burned it, which S_obs "+
					"(which allocates only on success) cannot do. highest id issued: %d",
					trial, burnHandles, id, maxKey(ids))
			}
		}
	}
}

// registerConcurrently has burnWorkers goroutines race for each of
// burnHandles handles, one handle at a time, with every worker held at a
// barrier until all of them are ready to attempt that handle.
//
// The barrier is load-bearing. Without it the goroutines start staggered, the
// first one through registers every handle before the others reach it, and
// the contended window never opens: the same workload without a barrier
// burned an id in 1 trial out of 5 rather than 3 out of 4.
func registerConcurrently(t *testing.T, svc *Service) (ids map[int64]bool, taken int) {
	t.Helper()

	gates := make([]chan struct{}, burnHandles)
	ready := make([]sync.WaitGroup, burnHandles)
	for j := range gates {
		gates[j] = make(chan struct{})
		ready[j].Add(burnWorkers)
	}

	ids = map[int64]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(burnWorkers)
	for w := 0; w < burnWorkers; w++ {
		go func() {
			defer wg.Done()
			for j := 0; j < burnHandles; j++ {
				ready[j].Done()
				<-gates[j]
				u, err := svc.CreateUser(fmt.Sprintf("h%03d", j))
				mu.Lock()
				switch {
				case err == nil:
					if ids[u.ID] {
						t.Errorf("F018: user id %d handed to more than one request", u.ID)
					}
					ids[u.ID] = true
				case err == store.ErrHandleTaken:
					taken++
				default:
					t.Errorf("F018: CreateUser answered %v; only nil and handle_taken "+
						"are reachable for a valid handle", err)
				}
				mu.Unlock()
			}
		}()
	}
	for j := 0; j < burnHandles; j++ {
		ready[j].Wait()
		close(gates[j])
	}
	wg.Wait()
	return ids, taken
}

func maxKey(m map[int64]bool) int64 {
	var max int64
	for k := range m {
		if k > max {
			max = k
		}
	}
	return max
}
