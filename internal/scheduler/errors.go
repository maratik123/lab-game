package scheduler

import "errors"

// Sentinels this package returns. Compare with errors.Is.
var (
	// ErrUnknownType is returned by Schedule when Request.Type has no
	// registered Declaration, and by the execution path when a claimed
	// row's type has none (design D6, D9).
	ErrUnknownType = errors.New("scheduler: unknown task type")

	// ErrInvalidDelay is returned by Schedule when Request.Delay is
	// negative (design D3).
	ErrInvalidDelay = errors.New("scheduler: invalid delay")

	// ErrDuplicateTask is returned by Schedule when a live row already
	// occupies the (type, instance key) identity — the database's own
	// refusal, mapped so a duplicate timer edge cannot abort the caller's
	// transaction (design D9).
	ErrDuplicateTask = errors.New("scheduler: duplicate task identity")

	// ErrInvalidPayload is returned by Schedule when Request.Payload is
	// not valid JSON.
	ErrInvalidPayload = errors.New("scheduler: invalid payload")

	// ErrInvalidDeclaration is returned by NewRegistry when a Declaration
	// is malformed: an empty type, a nil Handler, a duplicate type, or a
	// Recurrence with a nil Cadence or an empty configuration key (design
	// D9). Wrapped together with the offending type name.
	ErrInvalidDeclaration = errors.New("scheduler: invalid declaration")
)
