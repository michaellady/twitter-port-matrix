// Package dom defines the immutable types of the system.
// F4 (no self-follow): NewFollow refuses a == b.
//
// Naming note: this package was originally `internal/domain`, but
// `domain` is a reserved keyword in Gobra/Viper (used to declare
// axiomatized data domains and as the map-domain operator in specs).
// Renamed to `dom` so the Gobra parser accepts the package declaration.
package dom

// selfFollowError is the concrete sentinel-error type for ErrSelfFollow.
//
// Gobra's `errors.New` stub returns a heap-allocated `*errorString`
// whose memory the caller is required to track via permissions. That
// pattern fights the "package-level sentinel" idiom we want here, so we
// implement Gobra's full `error` interface (ErrorMem, IsDuplicableMem,
// Duplicate, Error) directly on a unit value type. The permission
// obligations collapse to trivial truths because the type carries no
// state.
type selfFollowError struct{}

// Error satisfies the error interface.
//
// @ preserves e.ErrorMem()
// @ ensures   e.IsDuplicableMem() == old(e.IsDuplicableMem())
// @ decreases e.ErrorMem()
// @ trusted
func (e selfFollowError) Error() string { return "self_follow_forbidden" }

// @ pred (e selfFollowError) ErrorMem() { true }
//
// @ ghost
// @ requires  acc(e.ErrorMem(), _)
// @ decreases
// @ pure
// @ func (e selfFollowError) IsDuplicableMem() bool { return true }
//
// @ ghost
// @ preserves e.ErrorMem()
// @ ensures   e.IsDuplicableMem() == old(e.IsDuplicableMem())
// @ ensures   e.IsDuplicableMem() ==> e.ErrorMem()
// @ decreases e.ErrorMem()
// @ func (e selfFollowError) Duplicate() { fold e.ErrorMem() }
//
// @ selfFollowError implements error

// ErrSelfFollow is returned by NewFollow when both ends are the same user.
// Consumers compare against it via `errors.Is(err, dom.ErrSelfFollow)`.
var ErrSelfFollow error = selfFollowError{}

// User is a registered handle. Handle is the canonical identifier; ID is
// the deterministic numeric id for clients.
type User struct {
	ID     int64
	Handle string
}

// Tweet is an immutable post. F8 ensures ID is unique and per-author monotonic.
type Tweet struct {
	ID        int64
	Author    string
	Text      string
	CreatedAt int64
}

// Follow is an edge in the follow graph. F4 forbids a == b at construction.
type Follow struct {
	From string
	To   string
}

// NewFollow constructs a Follow, rejecting self-follow per F4.
//
// @ requires isComparable(ErrSelfFollow)
// @ ensures from == to ==> (err == ErrSelfFollow && f == Follow{})
// @ ensures from != to ==> (err == nil && f.From == from && f.To == to)
// @ decreases
func NewFollow(from, to string) (f Follow, err error) {
	if from == to {
		return Follow{}, ErrSelfFollow
	}
	return Follow{From: from, To: to}, nil
}

// --- Observable validation (S_obs decisions D1, D6) -------------------------
//
// These bounds are part of the observable contract, so they live in the
// verified core rather than the HTTP shim. Gobra verifies [clock ids dom store
// service] and does not verify httpshim; validation in the shim would mean the
// proofs never see the rules that decide what the service accepts.

const (
	// MaxHandleLen bounds a handle at 32 bytes.
	MaxHandleLen = 32
	// MinTextLen and MaxTextLen bound tweet text.
	MinTextLen = 1
	MaxTextLen = 280
)

type invalidHandleError struct{}

func (e invalidHandleError) Error() string { return "invalid_handle" }

type invalidTextError struct{}

func (e invalidTextError) Error() string { return "invalid_text" }

// ErrInvalidHandle is returned for a handle outside [a-z0-9_]{1,32}.
var ErrInvalidHandle error = invalidHandleError{}

// ErrInvalidText is returned for text outside 1..280 bytes, or containing a
// control character.
var ErrInvalidText error = invalidTextError{}

// ValidHandle accepts 1..MaxHandleLen bytes drawn from [a-z0-9_].
//
// The alphabet is deliberately narrow. A narrow alphabet is a narrow surface
// on which two implementations can disagree about what they accept.
//
// @ requires true
// @ ensures result == (len(h) > 0 && len(h) <= MaxHandleLen)
// @ decreases
func ValidHandle(h string) (result bool) {
	if len(h) == 0 || len(h) > MaxHandleLen {
		return false
	}
	// @ invariant 0 <= i && i <= len(h)
	for i := 0; i < len(h); i++ {
		c := h[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// ValidText accepts MinTextLen..MaxTextLen bytes with no control characters.
//
// @ requires true
// @ decreases
func ValidText(t string) (result bool) {
	if len(t) < MinTextLen || len(t) > MaxTextLen {
		return false
	}
	// @ invariant 0 <= i && i <= len(t)
	for i := 0; i < len(t); i++ {
		if t[i] < 0x20 {
			return false
		}
	}
	return true
}
