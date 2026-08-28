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
//
// The `// @ trusted` marker this method used to carry is gone: with ErrorMem()
// defined as `true` and IsDuplicableMem() a pure constant, Gobra discharges
// both clauses directly against the body.
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

// invalidHandleError and invalidTextError are sentinel-error value types in
// the same shape as selfFollowError above. Gobra's `error` interface is wider
// than Go's -- it also demands ErrorMem(), IsDuplicableMem() and Duplicate() --
// so assigning a bare struct to an `error` var is a type error under Gobra even
// though `go build` accepts it. These were added in S-03 with only the Go
// half of the interface, which is what made the whole dom package fail to
// type-check. Both carry no state, so every permission obligation collapses to
// a trivial truth and none of the three methods needs a trusted marker.
type invalidHandleError struct{}

// @ preserves e.ErrorMem()
// @ ensures   e.IsDuplicableMem() == old(e.IsDuplicableMem())
// @ decreases e.ErrorMem()
func (e invalidHandleError) Error() string { return "invalid_handle" }

// @ pred (e invalidHandleError) ErrorMem() { true }
//
// @ ghost
// @ requires  acc(e.ErrorMem(), _)
// @ decreases
// @ pure
// @ func (e invalidHandleError) IsDuplicableMem() bool { return true }
//
// @ ghost
// @ preserves e.ErrorMem()
// @ ensures   e.IsDuplicableMem() == old(e.IsDuplicableMem())
// @ ensures   e.IsDuplicableMem() ==> e.ErrorMem()
// @ decreases e.ErrorMem()
// @ func (e invalidHandleError) Duplicate() { fold e.ErrorMem() }
//
// @ invalidHandleError implements error

type invalidTextError struct{}

// @ preserves e.ErrorMem()
// @ ensures   e.IsDuplicableMem() == old(e.IsDuplicableMem())
// @ decreases e.ErrorMem()
func (e invalidTextError) Error() string { return "invalid_text" }

// @ pred (e invalidTextError) ErrorMem() { true }
//
// @ ghost
// @ requires  acc(e.ErrorMem(), _)
// @ decreases
// @ pure
// @ func (e invalidTextError) IsDuplicableMem() bool { return true }
//
// @ ghost
// @ preserves e.ErrorMem()
// @ ensures   e.IsDuplicableMem() == old(e.IsDuplicableMem())
// @ ensures   e.IsDuplicableMem() ==> e.ErrorMem()
// @ decreases e.ErrorMem()
// @ func (e invalidTextError) Duplicate() { fold e.ErrorMem() }
//
// @ invalidTextError implements error

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
// GOBRA CAPABILITY GAP -- indexing a string.
//
// This Gobra build (1.1-SNAPSHOT, image sha256:2ef080cc) rejects `h[i]` with
// "Indexing a string is currently not supported", and `range` over a string
// with "got string but expected rangeable type". Neither byte-wise form of
// this loop is expressible on a `string` directly.
//
// The way through is a `[]byte(h)` conversion, which Gobra DOES model: the
// resulting slice is indexable, and `len([]byte(h)) == len(h)` is provable.
// So the scan runs over `b` instead of `h`. This is not a weakening -- in Go
// the two loops are byte-for-byte identical, and the alphabet property is
// still fully proved, by the third loop invariant below.
//
// WHAT IS AND IS NOT IN THE POSTCONDITION. The alphabet property is proved
// INSIDE the function (invariant 3 carries it to the `return true`), but it
// cannot be RE-EXPORTED in the postcondition, because stating it would need a
// pure ghost function that indexes `h` -- the exact operation Gobra rejects.
// The postcondition therefore carries only the length half. Callers get
// "result implies the length bounds"; they do not get "result implies the
// alphabet" even though it was proved locally.
//
// The prior postcondition was `result == (len(h) > 0 && len(h) <= MaxHandleLen)`,
// which is FALSE: ValidHandle("ABC") returns false with the length in range.
// It survived only because the package never type-checked far enough for
// Gobra to attempt it. It is an implication now, which is what is true.
//
// @ requires true
// @ ensures result ==> (len(h) > 0 && len(h) <= MaxHandleLen)
// @ decreases
func ValidHandle(h string) (result bool) {
	b := []byte(h)
	if len(b) == 0 || len(b) > MaxHandleLen {
		return false
	}
	// @ invariant acc(b)
	// @ invariant 0 <= i && i <= len(b)
	// @ invariant forall k int :: 0 <= k && k < i ==> ((b[k] >= 'a' && b[k] <= 'z') || (b[k] >= '0' && b[k] <= '9') || b[k] == '_')
	// @ decreases len(b) - i
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// ValidText accepts MinTextLen..MaxTextLen bytes with no control characters.
//
// Same `[]byte` conversion and the same export limit as ValidHandle above:
// the no-control-character property is proved by invariant 3 but cannot be
// restated over `t` in the postcondition, since that needs string indexing.
//
// @ requires true
// @ ensures result ==> (len(t) >= MinTextLen && len(t) <= MaxTextLen)
// @ decreases
func ValidText(t string) (result bool) {
	b := []byte(t)
	if len(b) < MinTextLen || len(b) > MaxTextLen {
		return false
	}
	// @ invariant acc(b)
	// @ invariant 0 <= i && i <= len(b)
	// @ invariant forall k int :: 0 <= k && k < i ==> b[k] >= 0x20
	// @ decreases len(b) - i
	for i := 0; i < len(b); i++ {
		if b[i] < 0x20 {
			return false
		}
	}
	return true
}
