package store

import "errors"

// Post's sentinels, and one which AppendEvent also raises. Compare with
// errors.Is; see Post's doc comment for which phase raises each and what
// state the transaction is left in.
var (
	// ErrNoBasis is returned when basis is nil, or a typed-nil pointer of one
	// of the PostingBasis implementations. The transaction is untouched.
	ErrNoBasis = errors.New("store: posting basis is nil")

	// ErrEmptyBatch is returned when postings is empty. The transaction is
	// untouched.
	ErrEmptyBatch = errors.New("store: posting batch is empty")

	// ErrInvalidAmount is returned when a posting's amount is zero, not
	// representable at scale 5, or has more than 25 integer digits. The
	// transaction is untouched.
	ErrInvalidAmount = errors.New("store: posting amount is invalid")

	// ErrUnknownAccount is returned when a posting references an account_id
	// with no row in account. The transaction is usable; nothing was
	// written.
	ErrUnknownAccount = errors.New("store: unknown account")

	// ErrUnbalanced is returned when the batch's amounts do not sum to zero
	// within some ledger kind. The transaction is usable; nothing was
	// written.
	ErrUnbalanced = errors.New("store: posting batch is unbalanced")

	// ErrAlreadyPosted is returned when the batch's PlayerOperation basis
	// replays a (source, operation_id) pair already posted. The transaction
	// is usable; nothing was written.
	ErrAlreadyPosted = errors.New("store: operation already posted")

	// ErrOverdraft is returned when a controlled account's balance would go
	// negative — the database's CHECK (balance >= 0) violation surfaced as a
	// domain rejection. The transaction is aborted; the caller must roll
	// back.
	ErrOverdraft = errors.New("store: overdraft on a controlled account")

	// ErrBalanceRowMissing is an infrastructure error: a controlled account
	// has no account_balance row, which should be impossible after
	// CreateOwner. It is distinct from ErrOverdraft and must never be
	// rendered to a player as "cannot afford" — it signals a broken
	// invariant. The document and entry of phase d were already written, so
	// the caller must still roll back.
	ErrBalanceRowMissing = errors.New("store: controlled account has no balance row")

	// ErrUnknownEventType is returned when an Event's Type names no row of
	// event_type_definition — the database's foreign-key refusal (SQLSTATE
	// 23503 on event_type_fkey), surfaced through Post (phase d, via
	// Event.insert) and through AppendEvent. The transaction is aborted; the
	// caller must roll back.
	ErrUnknownEventType = errors.New("store: unknown event type")
)

// ErrInvalidOwner is CreateOwner's sentinel: kind is not a known OwnerKind,
// kind is OwnerWorld (seeded by migration, never created here), or kind is
// OwnerPlayer/OwnerChat with a nil telegramID. No statement is issued.
var ErrInvalidOwner = errors.New("store: invalid owner")
