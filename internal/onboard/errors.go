package onboard

import "errors"

// Sentinels for this package's start-payload codec and link builder.
// Compare with errors.Is.
var (
	// ErrEmptyPayload is returned when a /start payload is the empty
	// string.
	ErrEmptyPayload = errors.New("onboard: empty start payload")

	// ErrUnknownPayloadPrefix is returned when a /start payload does not
	// begin with a namespace this package recognises.
	ErrUnknownPayloadPrefix = errors.New("onboard: unknown start payload prefix")

	// ErrMalformedPayload is returned when a recognised payload's
	// remainder does not parse as a base-10 int64, or carries trailing
	// text after one.
	ErrMalformedPayload = errors.New("onboard: malformed start payload")

	// ErrEmptyUsername is returned when ChatStartLink is called with an
	// empty bot username.
	ErrEmptyUsername = errors.New("onboard: empty bot username")
)
