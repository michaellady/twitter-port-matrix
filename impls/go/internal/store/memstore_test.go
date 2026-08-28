package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
)

func mkUsers(t *testing.T, s *MemStore, handles ...string) {
	t.Helper()
	for i, h := range handles {
		if err := s.PutUser(dom.User{ID: int64(i + 1), Handle: h}); err != nil {
			t.Fatalf("PutUser(%s): %v", h, err)
		}
	}
}

func TestPutUserAndHasUser(t *testing.T) {
	s := New()
	if s.HasUser("alice") {
		t.Fatal("HasUser(alice) before PutUser: want false")
	}
	if err := s.PutUser(dom.User{ID: 1, Handle: "alice"}); err != nil {
		t.Fatalf("PutUser: %v", err)
	}
	if !s.HasUser("alice") {
		t.Fatal("HasUser(alice) after PutUser: want true")
	}
}

func TestPutUserDuplicate(t *testing.T) {
	s := New()
	if err := s.PutUser(dom.User{ID: 1, Handle: "alice"}); err != nil {
		t.Fatal(err)
	}
	err := s.PutUser(dom.User{ID: 2, Handle: "alice"})
	if !errors.Is(err, ErrDuplicateUser) {
		t.Fatalf("PutUser duplicate: err=%v, want ErrDuplicateUser", err)
	}
}

func TestPutFollowRequiresKnownUsers(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice")
	f, _ := dom.NewFollow("alice", "bob")
	if err := s.PutFollow(f); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("PutFollow unknown to: err=%v, want ErrUnknownUser", err)
	}
	g, _ := dom.NewFollow("eve", "alice")
	if err := s.PutFollow(g); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("PutFollow unknown from: err=%v, want ErrUnknownUser", err)
	}
}

func TestPutFollowIdempotent(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice", "bob")
	f, _ := dom.NewFollow("alice", "bob")
	if err := s.PutFollow(f); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFollow(f); err != nil {
		t.Fatalf("PutFollow twice: %v", err)
	}
	set := s.FollowSet("alice")
	if len(set) != 1 || !set["bob"] {
		t.Fatalf("FollowSet=%v, want {bob}", set)
	}
}

func TestDeleteFollowIdempotent(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice", "bob")
	s.DeleteFollow("alice", "bob") // never followed — no panic
	f, _ := dom.NewFollow("alice", "bob")
	if err := s.PutFollow(f); err != nil {
		t.Fatal(err)
	}
	s.DeleteFollow("alice", "bob")
	s.DeleteFollow("alice", "bob") // again, no-op
	if set := s.FollowSet("alice"); len(set) != 0 {
		t.Fatalf("FollowSet after delete=%v, want {}", set)
	}
}

func TestPutTweetRequiresKnownAuthor(t *testing.T) {
	s := New()
	err := s.PutTweet(dom.Tweet{ID: 1, Author: "ghost", Text: "hi", CreatedAt: 0})
	if !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("PutTweet unknown author: err=%v, want ErrUnknownUser", err)
	}
}

func TestHomeTimelineSeesOwnAndFollowed(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice", "bob", "carol")
	if err := s.PutTweet(dom.Tweet{ID: 1, Author: "bob", Text: "b1", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTweet(dom.Tweet{ID: 2, Author: "alice", Text: "a1", CreatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTweet(dom.Tweet{ID: 3, Author: "carol", Text: "c1", CreatedAt: 3}); err != nil {
		t.Fatal(err)
	}
	f, _ := dom.NewFollow("alice", "bob")
	if err := s.PutFollow(f); err != nil {
		t.Fatal(err)
	}

	timeline := s.HomeTimeline("alice", 0)
	if len(timeline) != 2 {
		t.Fatalf("timeline length=%d, want 2 (alice's own + bob's)", len(timeline))
	}
	for _, tw := range timeline {
		if tw.Author == "carol" {
			t.Fatalf("carol's tweet leaked into alice's timeline")
		}
	}
}

func TestHomeTimelineOrderingDescByCreatedAtThenID(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice", "bob")
	f, _ := dom.NewFollow("alice", "bob")
	if err := s.PutFollow(f); err != nil {
		t.Fatal(err)
	}
	// Same timestamp, ascending IDs → expect descending ID order in result.
	for i := 1; i <= 4; i++ {
		if err := s.PutTweet(dom.Tweet{ID: int64(i), Author: "bob", Text: "x", CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	timeline := s.HomeTimeline("alice", 0)
	wantIDs := []int64{4, 3, 2, 1}
	for i, want := range wantIDs {
		if timeline[i].ID != want {
			t.Fatalf("position %d: got id=%d, want %d", i, timeline[i].ID, want)
		}
	}

	// Mixed timestamps + ties: created_at takes precedence.
	s2 := New()
	mkUsers(t, s2, "alice")
	for _, tw := range []dom.Tweet{
		{ID: 1, Author: "alice", CreatedAt: 1},
		{ID: 2, Author: "alice", CreatedAt: 2},
		{ID: 3, Author: "alice", CreatedAt: 2}, // tie with id=2 by ts
	} {
		if err := s2.PutTweet(tw); err != nil {
			t.Fatal(err)
		}
	}
	got := s2.HomeTimeline("alice", 0)
	wantIDs2 := []int64{3, 2, 1}
	for i, want := range wantIDs2 {
		if got[i].ID != want {
			t.Fatalf("mixed: pos %d id=%d, want %d", i, got[i].ID, want)
		}
	}
}

func TestHomeTimelineLimit(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice")
	for i := 1; i <= 10; i++ {
		if err := s.PutTweet(dom.Tweet{ID: int64(i), Author: "alice", CreatedAt: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.HomeTimeline("alice", 3); len(got) != 3 {
		t.Fatalf("limit=3 got len=%d", len(got))
	}
	if got := s.HomeTimeline("alice", 0); len(got) != 10 {
		t.Fatalf("limit=0 got len=%d, want all 10", len(got))
	}
	if got := s.HomeTimeline("alice", 100); len(got) != 10 {
		t.Fatalf("limit > available got len=%d, want 10", len(got))
	}
}

func TestHomeTimelineUnknownUserReturnsEmpty(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice")
	if err := s.PutTweet(dom.Tweet{ID: 1, Author: "alice", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if got := s.HomeTimeline("ghost", 0); len(got) != 0 {
		t.Fatalf("unknown user timeline got %d entries, want 0", len(got))
	}
}

func TestPostBeforeFollowAndUnfollowAfterVisible(t *testing.T) {
	// Codex K2: F1 must hold under arbitrary post/follow ordering.
	s := New()
	mkUsers(t, s, "alice", "bob")
	if err := s.PutTweet(dom.Tweet{ID: 1, Author: "bob", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	// alice follows AFTER bob posted — old tweet must be visible.
	f, _ := dom.NewFollow("alice", "bob")
	if err := s.PutFollow(f); err != nil {
		t.Fatal(err)
	}
	if got := s.HomeTimeline("alice", 0); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("post-before-follow: got %v, want [bob's tweet id=1]", got)
	}
	// Now unfollow — bob's tweet must vanish from alice's timeline.
	s.DeleteFollow("alice", "bob")
	if got := s.HomeTimeline("alice", 0); len(got) != 0 {
		t.Fatalf("after unfollow: got %v, want []", got)
	}
}

func TestSnapshotEmpty(t *testing.T) {
	s := New()
	snap := s.Snapshot()
	if len(snap.Users) != 0 || len(snap.Follows) != 0 || len(snap.Tweets) != 0 {
		t.Fatalf("empty store snapshot non-empty: %+v", snap)
	}
}

func TestSnapshotIsSorted(t *testing.T) {
	s := New()
	mkUsers(t, s, "carol", "alice", "bob")
	if err := s.PutFollow(dom.Follow{From: "carol", To: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFollow(dom.Follow{From: "alice", To: "bob"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTweet(dom.Tweet{ID: 7, Author: "carol", Text: "z", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTweet(dom.Tweet{ID: 3, Author: "alice", Text: "y", CreatedAt: 0}); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	// Users sorted by ID ascending: carol(1), alice(2), bob(3)
	if snap.Users[0].Handle != "carol" || snap.Users[2].Handle != "bob" {
		t.Fatalf("users not sorted by id: %+v", snap.Users)
	}
	// Follows sorted (From,To) ascending
	if snap.Follows[0].From != "alice" || snap.Follows[1].From != "carol" {
		t.Fatalf("follows not sorted: %+v", snap.Follows)
	}
	// Tweets sorted by ID ascending
	if snap.Tweets[0].ID != 3 || snap.Tweets[1].ID != 7 {
		t.Fatalf("tweets not sorted by id: %+v", snap.Tweets)
	}
}

func TestReplaceRoundTrip(t *testing.T) {
	src := New()
	mkUsers(t, src, "alice", "bob")
	if err := src.PutFollow(dom.Follow{From: "alice", To: "bob"}); err != nil {
		t.Fatal(err)
	}
	if err := src.PutTweet(dom.Tweet{ID: 1, Author: "bob", Text: "hi", CreatedAt: 5}); err != nil {
		t.Fatal(err)
	}
	snap := src.Snapshot()

	dst := New()
	dst.Replace(snap)
	if !dst.HasUser("alice") || !dst.HasUser("bob") {
		t.Fatal("users missing after Replace")
	}
	tw := dst.HomeTimeline("alice", 0)
	if len(tw) != 1 || tw[0].Author != "bob" {
		t.Fatalf("alice timeline after Replace: %+v", tw)
	}
}

func TestReplaceClearsExistingState(t *testing.T) {
	s := New()
	mkUsers(t, s, "old1", "old2")
	if err := s.PutTweet(dom.Tweet{ID: 1, Author: "old1", Text: "stale"}); err != nil {
		t.Fatal(err)
	}
	s.Replace(StoreSnapshot{
		Users:   []dom.User{{ID: 1, Handle: "fresh"}},
		Follows: nil,
		Tweets:  nil,
	})
	if s.HasUser("old1") || s.HasUser("old2") {
		t.Fatal("Replace did not clear pre-existing users")
	}
	if !s.HasUser("fresh") {
		t.Fatal("Replace did not install fresh user")
	}
	if got := s.HomeTimeline("fresh", 0); len(got) != 0 {
		t.Fatalf("Replace did not clear pre-existing tweets: %+v", got)
	}
}

// TestSnapshotReplaceFullCoverage exercises every branch of Snapshot
// and Replace — multiple users, multiple follow edges per user, and
// tweets with same created_at to hit the sort tie-break. Required for
// the Tier-3 100% line-coverage gate on internal/store.
func TestSnapshotReplaceFullCoverage(t *testing.T) {
	s := New()
	mkUsers(t, s, "alice", "bob", "carol", "dave")
	if err := s.PutFollow(dom.Follow{From: "alice", To: "bob"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFollow(dom.Follow{From: "alice", To: "carol"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutFollow(dom.Follow{From: "bob", To: "carol"}); err != nil {
		t.Fatal(err)
	}
	for i, t0 := range []dom.Tweet{
		{ID: 10, Author: "alice", Text: "a1", CreatedAt: 1},
		{ID: 11, Author: "alice", Text: "a2", CreatedAt: 1}, // tie-break
		{ID: 12, Author: "bob", Text: "b1", CreatedAt: 3},
		{ID: 13, Author: "carol", Text: "c1", CreatedAt: 4},
	} {
		if err := s.PutTweet(t0); err != nil {
			t.Fatalf("PutTweet[%d]: %v", i, err)
		}
	}
	snap := s.Snapshot()
	if got := len(snap.Users); got != 4 {
		t.Fatalf("snapshot users len=%d want 4", got)
	}
	if got := len(snap.Follows); got != 3 {
		t.Fatalf("snapshot follows len=%d want 3", got)
	}
	if got := len(snap.Tweets); got != 4 {
		t.Fatalf("snapshot tweets len=%d want 4", got)
	}
	s2 := New()
	s2.Replace(snap)
	snap2 := s2.Snapshot()
	if !reflect.DeepEqual(snap, snap2) {
		t.Fatalf("snapshot not stable across Replace: %+v vs %+v", snap, snap2)
	}
}
