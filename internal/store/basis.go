package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PostingBasis is the sealed sum type of documents a journal_entry may
// reference — exactly the store package's two implementations,
// *PlayerOperation and *ManualCorrection (D8). The unexported methods make
// it unrepresentable to satisfy from outside the package, and both methods
// are safe to call on a nil receiver: entrySQL returns ErrNoBasis instead
// of issuing SQL, which is what lets Post reject a typed-nil basis before
// any statement.
type PostingBasis interface {
	// entrySQL returns the constant INSERT statement for this basis type's
	// journal_entry FK column. It issues no SQL itself; on a nil receiver it
	// returns ErrNoBasis.
	entrySQL() (string, error)
	// insert writes this basis's document row inside tx and returns its id.
	insert(ctx context.Context, tx pgx.Tx) (int64, error)
}

// PlayerOperation is the basis for a client-initiated player action:
// source identifies the client (today only SourceTelegram) and operationID
// is that client's idempotency key — store treats it as opaque.
type PlayerOperation struct {
	Source      OperationSource
	OperationID string
}

func (p *PlayerOperation) entrySQL() (string, error) {
	if p == nil {
		return "", ErrNoBasis
	}
	return `INSERT INTO journal_entry (player_operation_id) VALUES ($1) RETURNING id`, nil
}

func (p *PlayerOperation) insert(ctx context.Context, tx pgx.Tx) (int64, error) {
	if p == nil {
		return 0, ErrNoBasis
	}
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO player_operation (source, operation_id) VALUES ($1, $2)
		 ON CONFLICT (source, operation_id) DO NOTHING
		 RETURNING id`,
		p.Source, p.OperationID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAlreadyPosted
	}
	if err != nil {
		return 0, fmt.Errorf("insert player_operation: %w", err)
	}
	return id, nil
}

// ManualCorrection is the basis for an operator-initiated compensating
// batch: actor identifies who made the correction and reason records why.
type ManualCorrection struct {
	Actor  string
	Reason string
}

func (m *ManualCorrection) entrySQL() (string, error) {
	if m == nil {
		return "", ErrNoBasis
	}
	return `INSERT INTO journal_entry (manual_correction_id) VALUES ($1) RETURNING id`, nil
}

func (m *ManualCorrection) insert(ctx context.Context, tx pgx.Tx) (int64, error) {
	if m == nil {
		return 0, ErrNoBasis
	}
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO manual_correction (actor, reason) VALUES ($1, $2) RETURNING id`,
		m.Actor, m.Reason,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert manual_correction: %w", err)
	}
	return id, nil
}
