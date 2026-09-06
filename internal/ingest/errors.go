package ingest

import "errors"

// Sentinels for this package's own failure classes. Every concrete
// failure wraps one of these via fmt.Errorf's %w; compare with
// errors.Is.
var (
	// ErrUnknownKind is returned by NewRouter when a Route names a Kind
	// with no kindTable row — a route can never be registered into a
	// hole (design D4).
	ErrUnknownKind = errors.New("ingest: unknown kind")

	// ErrDuplicateRoute is returned by NewRouter when two Routes name the
	// same Kind.
	ErrDuplicateRoute = errors.New("ingest: duplicate route")

	// ErrEmptyIDSpace is returned by the operation_id builder when its
	// IDSpace is empty (design D9).
	ErrEmptyIDSpace = errors.New("ingest: empty IDSpace")

	// ErrEmptyID is returned by the operation_id builder when its raw
	// identifier is empty (design D9).
	ErrEmptyID = errors.New("ingest: empty id")
)
