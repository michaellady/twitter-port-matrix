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
