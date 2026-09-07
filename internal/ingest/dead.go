package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeadUpdate is one give-up row, enumerated by DeadUpdates — the shape
// scheduler.DeadTask already has (design D14). It carries no raw update
// payload: the diagnostic surface is this projection, not a replay
// mechanism.
type DeadUpdate struct {
	// UpdateID is the given-up update's own update_id.
	UpdateID int64
	// Kind is the update's derived Kind.
	Kind Kind
	// ChatID is the update's destination chat, when its kind's payload
	// declares one (design D4, D13). Nil otherwise.
	ChatID *int64
	// ConsecutiveFailures is the attempt count at give-up.
	ConsecutiveFailures int
	// LastError is the last recorded failure reason.
	LastError string
	// CreatedAt is when the give-up row was written.
	CreatedAt time.Time
}

// writeDeadUpdate inserts d as a new ingest_dead_update row inside tx —
// the loop's own transaction for the give-up outcome (design D5, D14).
func writeDeadUpdate(ctx context.Context, tx pgx.Tx, d DeadUpdate) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO ingest_dead_update (update_id, kind, chat_id, consecutive_failures, last_error)
		 VALUES ($1, $2, $3, $4, $5)`,
		d.UpdateID, string(d.Kind), d.ChatID, d.ConsecutiveFailures, d.LastError,
	); err != nil {
		return fmt.Errorf("ingest: write dead update: %w", err)
	}
	return nil
}

// DeadUpdates returns up to limit give-up rows over tx, a caller-owned
// transaction (AC37): it neither commits nor rolls back tx, mirroring
// scheduler.DeadTasks' division of ownership. Rows are ordered by
// created_at then id, both ascending, so the result is deterministic even
// when several rows share a created_at instant.
func DeadUpdates(ctx context.Context, tx pgx.Tx, limit int) ([]DeadUpdate, error) {
	rows, err := tx.Query(ctx,
		`SELECT update_id, kind, chat_id, consecutive_failures, last_error, created_at
		 FROM ingest_dead_update
		 ORDER BY created_at, id
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ingest: query dead updates: %w", err)
	}
	defer rows.Close()

	var out []DeadUpdate
	for rows.Next() {
		var (
			d        DeadUpdate
			kindText string
		)
		if err := rows.Scan(&d.UpdateID, &kindText, &d.ChatID, &d.ConsecutiveFailures, &d.LastError, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("ingest: scan dead update: %w", err)
		}
		d.Kind = Kind(kindText)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ingest: query dead updates: %w", err)
	}
	return out, nil
}
