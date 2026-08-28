// Command server starts the twitter-clone API on :8080. Trusted code.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/admin"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/httpshim"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/metrics"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/ui"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	svc := service.New(clock.New())

	if os.Getenv("SEED_DEMO") == "true" {
		seedDemo(svc)
	}

	// ADMIN_TOKEN gate (Stream 2 Phase 0). When unset, admin.Mount still
	// registers handlers but they reject every request with 503
	// admin_disabled. We log a warning so operators notice in non-prod.
	// Per the gate: NO per-process random fallback — that would mean the
	// diffsplitter could never reach the snapshot endpoints anyway.
	if os.Getenv("ADMIN_TOKEN") == "" {
		log.Println("warning: ADMIN_TOKEN not set; /_admin/* will return 503 admin_disabled")
	}

	mux := http.NewServeMux()
	ui.Mount(mux, svc)                        // Phase 2: UI ("/", "/u/", forms)
	httpshim.Register(mux, svc)               // Phase 1: JSON API + /healthz + /version
	admin.Mount(mux, svc)                     // Stream 2 Phase 0: /_admin/snapshot, /_admin/load-snapshot
	mux.Handle("/metrics", metrics.Handler()) // Phase 4: Prometheus exposition

	srv := &http.Server{Addr: addr, Handler: metrics.Track(mux)}
	log.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// seedDemo: Tier-4 Phase 1b mirror of the Rust primary. Pre-populates
// the in-memory state so first-time visitors to twitter-formal-go.fly.dev
// see something interesting immediately. Trusted (hard-coded literals,
// no user input). See TCB.md.
func seedDemo(svc *service.Service) {
	log.Println("seedDemo: pre-populating alice, bob, carol + sample tweets")
	for _, h := range []string{"alice", "bob", "carol"} {
		if _, err := svc.CreateUser(h); err != nil {
			log.Printf("seedDemo: CreateUser(%s) failed: %v", h, err)
		}
	}
	tweets := []struct {
		author, text string
	}{
		{"alice", "verified backend (Go side), deployed to fly.io"},
		{"alice", "every method is gobra-verified or honestly trusted"},
		{"bob", "TLC says 141M states, 0 invariant violations"},
		{"carol", "no follow yet — see /timeline?user=alice — bob's tweet shows up, mine doesn't"},
	}
	for _, t := range tweets {
		if _, err := svc.PostTweet(t.author, t.text); err != nil {
			log.Printf("seedDemo: PostTweet(%s) failed: %v", t.author, err)
		}
	}
	if err := svc.Follow("alice", "bob"); err != nil {
		log.Printf("seedDemo: Follow(alice,bob) failed: %v", err)
	}
	log.Println("seedDemo: done")
}
