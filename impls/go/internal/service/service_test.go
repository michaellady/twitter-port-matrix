package service

import (
	"errors"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/clock"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/store"
)

func newSvc() *Service { return New(clock.New()) }

func TestCreateUserEmptyHandle(t *testing.T) {
	s := newSvc()
	_, err := s.CreateUser("")
	if !errors.Is(err, ErrEmptyHandle) {
		t.Fatalf("err=%v, want ErrEmptyHandle", err)
	}
}

func TestCreateUserHappy(t *testing.T) {
	s := newSvc()
	u, err := s.CreateUser("alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 1 || u.Handle != "alice" {
		t.Fatalf("got %+v, want {1, alice}", u)
	}
	if !s.HasUser("alice") {
		t.Fatal("HasUser(alice) false after CreateUser")
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	s := newSvc()
	if _, err := s.CreateUser("alice"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateUser("alice")
	if !errors.Is(err, store.ErrDuplicateUser) {
		t.Fatalf("err=%v, want ErrDuplicateUser", err)
	}
}

func TestFollowSelfRejected(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	if err := s.Follow("alice", "alice"); !errors.Is(err, dom.ErrSelfFollow) {
		t.Fatalf("err=%v, want ErrSelfFollow", err)
	}
}

func TestFollowUnknownRejected(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	if err := s.Follow("alice", "bob"); !errors.Is(err, store.ErrUnknownUser) {
		t.Fatalf("err=%v, want ErrUnknownUser", err)
	}
}

func TestFollowIdempotent(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	_, _ = s.CreateUser("bob")
	if err := s.Follow("alice", "bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.Follow("alice", "bob"); err != nil {
		t.Fatalf("Follow twice: %v", err)
	}
}

func TestUnfollowIdempotentAndUnknownGuard(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	_, _ = s.CreateUser("bob")
	// Unfollow without prior follow — succeeds (idempotent).
	if err := s.Unfollow("alice", "bob"); err != nil {
		t.Fatalf("idempotent unfollow err=%v", err)
	}
	// Unknown user is rejected.
	if err := s.Unfollow("alice", "ghost"); !errors.Is(err, store.ErrUnknownUser) {
		t.Fatalf("err=%v, want ErrUnknownUser", err)
	}
	if err := s.Unfollow("ghost", "alice"); !errors.Is(err, store.ErrUnknownUser) {
		t.Fatalf("err=%v, want ErrUnknownUser", err)
	}
}

func TestPostTweetEmptyText(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	if _, err := s.PostTweet("alice", ""); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("err=%v, want ErrEmptyText", err)
	}
}

func TestPostTweetUnknownAuthor(t *testing.T) {
	s := newSvc()
	if _, err := s.PostTweet("ghost", "hi"); !errors.Is(err, store.ErrUnknownUser) {
		t.Fatalf("err=%v, want ErrUnknownUser", err)
	}
}

func TestPostTweetMonotonicID(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	t1, _ := s.PostTweet("alice", "first")
	t2, _ := s.PostTweet("alice", "second")
	if t1.ID >= t2.ID {
		t.Fatalf("IDs not monotonic: t1.ID=%d t2.ID=%d", t1.ID, t2.ID)
	}
}

func TestPostTweetUsesInjectedClock(t *testing.T) {
	c := clock.NewAt(100)
	s := New(c)
	_, _ = s.CreateUser("alice")
	tw, _ := s.PostTweet("alice", "hi")
	if tw.CreatedAt != 100 {
		t.Fatalf("created_at=%d, want 100", tw.CreatedAt)
	}
	c.Tick()
	tw2, _ := s.PostTweet("alice", "hi2")
	if tw2.CreatedAt != 101 {
		t.Fatalf("created_at after tick=%d, want 101", tw2.CreatedAt)
	}
}

func TestHomeTimelineDelegates(t *testing.T) {
	s := newSvc()
	_, _ = s.CreateUser("alice")
	_, _ = s.CreateUser("bob")
	_ = s.Follow("alice", "bob")
	_, _ = s.PostTweet("bob", "hi")
	got := s.HomeTimeline("alice", 0)
	if len(got) != 1 || got[0].Author != "bob" {
		t.Fatalf("got %v, want [bob's tweet]", got)
	}
}

func TestTickWithLogicalClock(t *testing.T) {
	c := clock.New()
	s := New(c)
	before := c.Now()
	s.Tick()
	if c.Now() != before+1 {
		t.Fatalf("Tick did not advance clock: %d -> %d", before, c.Now())
	}
}

type fakeClock struct{ v int64 }

func (f *fakeClock) Now() int64 { return f.v }
func (f *fakeClock) Tick()      {} // no-op

func TestPostTweetUsesNonLogicalClock(t *testing.T) {
	// Exercises nowLogical's fallback path (clk is not *clock.Logical).
	c := &fakeClock{v: 42}
	s := New(c)
	_, _ = s.CreateUser("alice")
	tw, err := s.PostTweet("alice", "hi")
	if err != nil {
		t.Fatalf("PostTweet err=%v", err)
	}
	if tw.CreatedAt != 42 {
		t.Fatalf("CreatedAt=%d, want 42 (from fakeClock)", tw.CreatedAt)
	}
}

func TestTickIsNoopWithNonLogicalClock(t *testing.T) {
	// Service.Tick only advances clock.Logical; non-Logical clocks are unchanged.
	c := &fakeClock{v: 5}
	s := New(c)
	s.Tick()
	if c.v != 5 {
		t.Fatalf("non-Logical clock advanced: %d", c.v)
	}
}

func TestSnapshotCapturesGenerators(t *testing.T) {
	s := New(clock.New())
	if _, err := s.CreateUser("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob"); err != nil {
		t.Fatal(err)
	}
	s.Tick()
	if _, err := s.PostTweet("alice", "hi"); err != nil {
		t.Fatal(err)
	}

	snap := s.Snapshot()
	if snap.IDCounterUsers != 2 {
		t.Fatalf("IDCounterUsers: want 2, got %d", snap.IDCounterUsers)
	}
	if snap.IDCounterTweets != 1 {
		t.Fatalf("IDCounterTweets: want 1, got %d", snap.IDCounterTweets)
	}
	if snap.ClockNow != 1 {
		t.Fatalf("ClockNow: want 1, got %d", snap.ClockNow)
	}
	if len(snap.Store.Users) != 2 || len(snap.Store.Tweets) != 1 {
		t.Fatalf("store snapshot wrong shape: %+v", snap.Store)
	}
}

func TestLoadSnapshotRestoresGenerators(t *testing.T) {
	src := New(clock.New())
	if _, err := src.CreateUser("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := src.CreateUser("bob"); err != nil {
		t.Fatal(err)
	}
	src.Tick()
	src.Tick()
	src.Tick()
	if _, err := src.PostTweet("bob", "hi"); err != nil {
		t.Fatal(err)
	}
	snap := src.Snapshot()

	dst := New(clock.New())
	dst.LoadSnapshot(snap)
	// Next tweet ID must be 2 (one was consumed for "hi"); next user ID must be 3.
	tw, err := dst.PostTweet("alice", "after-load")
	if err != nil {
		t.Fatalf("PostTweet on dst: %v", err)
	}
	if tw.ID != 2 {
		t.Fatalf("post-load tweet ID: want 2, got %d", tw.ID)
	}
	if tw.CreatedAt != 3 {
		t.Fatalf("post-load created_at: want 3, got %d", tw.CreatedAt)
	}
	u, err := dst.CreateUser("carol")
	if err != nil {
		t.Fatalf("CreateUser carol: %v", err)
	}
	if u.ID != 3 {
		t.Fatalf("post-load user ID: want 3, got %d", u.ID)
	}
}
