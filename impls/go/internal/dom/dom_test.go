package dom

import (
	"errors"
	"testing"
)

func TestNewFollowRejectsSelf(t *testing.T) {
	_, err := NewFollow("alice", "alice")
	if !errors.Is(err, ErrSelfFollow) {
		t.Fatalf("NewFollow(alice,alice) err=%v, want ErrSelfFollow", err)
	}
}

func TestNewFollowAcceptsDistinct(t *testing.T) {
	f, err := NewFollow("alice", "bob")
	if err != nil {
		t.Fatalf("NewFollow err=%v, want nil", err)
	}
	if f.From != "alice" || f.To != "bob" {
		t.Fatalf("NewFollow returned %+v", f)
	}
}

func TestNewFollowEmptyStringsAreDistinct(t *testing.T) {
	// Empty handles are still distinct from non-empty; the service layer
	// rejects empty handles. Domain only enforces F4.
	f, err := NewFollow("", "bob")
	if err != nil {
		t.Fatalf("NewFollow(\"\",\"bob\") err=%v, want nil", err)
	}
	if f.From != "" || f.To != "bob" {
		t.Fatalf("got %+v", f)
	}
}

func TestErrSelfFollowMessage(t *testing.T) {
	// Exercise the Error() method on the sentinel type so the verified-
	// coverage gate sees it as covered (Gobra's full error interface
	// pattern uses a value receiver that callers seldom invoke directly).
	if got := ErrSelfFollow.Error(); got != "self_follow_forbidden" {
		t.Fatalf("ErrSelfFollow.Error() = %q, want %q", got, "self_follow_forbidden")
	}
}
