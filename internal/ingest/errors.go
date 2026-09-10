package ingest

import "errors"

// Sentinels for this package's own failure classes. Every concrete
// failure wraps one of these via fmt.Errorf's %w; compare with
// errors.Is.
var (
	// ErrUnknownKind is returned by NewRouter when a Route names a Kind
	// with no kindTable row — a route can never be registered into a
	// hole.
	ErrUnknownKind = errors.New("ingest: unknown kind")

	// ErrDuplicateRoute is returned by NewRouter when two Routes name the
	// same Kind.
	ErrDuplicateRoute = errors.New("ingest: duplicate route")

	// ErrEmptyIDSpace is returned by the operation_id builder when its
	// IDSpace is empty.
	ErrEmptyIDSpace = errors.New("ingest: empty IDSpace")

	// ErrEmptyID is returned by the operation_id builder when its raw
	// identifier is empty.
	ErrEmptyID = errors.New("ingest: empty id")

	// ErrChatRefused is Gate.AllowCall's sentinel: the destination is
	// unverifiable (ChatUnknown), not an integer chat id, or neither
	// allowlisted nor a known player.
	ErrChatRefused = errors.New("ingest: chat refused")

	// ErrPollDiscarded is PollOnce's sentinel for a getUpdates call
	// Stop cancelled while the parent context was still live: the poll
	// returned nothing worth acting on, and no update in it was ever
	// processed, so it is a discarded cycle rather than a failed one —
	// the observation this call reports carries no Err.
	ErrPollDiscarded = errors.New("ingest: poll discarded by Stop")
)
