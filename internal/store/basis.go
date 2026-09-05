package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PostingBasis is the sealed sum type of documents a journal_entry may
// reference — exactly the store package's four implementations,
// *PlayerOperation, *ManualCorrection, *DeferredTask and *RecurrentTask.
// The unexported methods make it unrepresentable to satisfy from outside
// the package, and every method is safe to call on a nil receiver:
// entrySQL returns ErrNoBasis instead of issuing SQL, which is what lets
// Post reject a typed-nil basis before any statement.
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

// DeferredTask is the basis for a mechanic's effects posted by a one-shot
// scheduled task's handler (design D14, internal/scheduler). TaskID is the
// scheduled_task row's id, carried by value with deliberately NO foreign
// key back to scheduled_task (AC27): delete-on-done removes the task row
// while its basis document and postings live on. TaskID is the Go zero
// value when the handler has no task id to name (stored as NULL via
// NULLIF, since scheduled_task.id is GENERATED ALWAYS AS IDENTITY and so
// is never zero); InstanceKey is likewise stored NULL when empty. TaskID
// exists because two concurrent keyless one-shots of the same type and
// instant would otherwise produce byte-identical, permanently
// indistinguishable basis documents once delete-on-done has removed the
// only other evidence.
type DeferredTask struct {
	TaskID      int64
	TaskType    string
	InstanceKey string
	RunAt       time.Time
}

func (d *DeferredTask) entrySQL() (string, error) {
	if d == nil {
		return "", ErrNoBasis
	}
	return `INSERT INTO journal_entry (deferred_task_id) VALUES ($1) RETURNING id`, nil
}

func (d *DeferredTask) insert(ctx context.Context, tx pgx.Tx) (int64, error) {
	if d == nil {
		return 0, ErrNoBasis
	}
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO deferred_task (task_id, task_type, instance_key, run_at)
		 VALUES (NULLIF($1, 0), $2, NULLIF($3, ''), $4)
		 RETURNING id`,
		d.TaskID, d.TaskType, d.InstanceKey, d.RunAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert deferred_task: %w", err)
	}
	return id, nil
}

// RecurrentTask is the basis for a mechanic's effects posted by a
// recurring scheduled task's handler (design D14, internal/scheduler).
// Same shape as DeferredTask, and for the same reasons: TaskID by value
// with deliberately no foreign key back to scheduled_task (AC27), zero
// stored as NULL via NULLIF, and InstanceKey's empty string likewise.
type RecurrentTask struct {
	TaskID      int64
	TaskType    string
	InstanceKey string
	RunAt       time.Time
}

func (r *RecurrentTask) entrySQL() (string, error) {
	if r == nil {
		return "", ErrNoBasis
	}
	return `INSERT INTO journal_entry (recurrent_task_id) VALUES ($1) RETURNING id`, nil
}

func (r *RecurrentTask) insert(ctx context.Context, tx pgx.Tx) (int64, error) {
	if r == nil {
		return 0, ErrNoBasis
	}
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO recurrent_task (task_id, task_type, instance_key, run_at)
		 VALUES (NULLIF($1, 0), $2, NULLIF($3, ''), $4)
		 RETURNING id`,
		r.TaskID, r.TaskType, r.InstanceKey, r.RunAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert recurrent_task: %w", err)
	}
	return id, nil
}
