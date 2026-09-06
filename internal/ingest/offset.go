package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// readOffset reads the singleton ingest_offset row's next_update_id — the
// offset to TRANSMIT on the next getUpdates call, not the last update_id
// seen (design D14). The migration seeds exactly one row, so a missing
// row is an infrastructure error rather than a legitimate "no offset yet"
// state.
func readOffset(ctx context.Context, tx pgx.Tx) (int64, error) {
	var next int64
	if err := tx.QueryRow(ctx, `SELECT next_update_id FROM ingest_offset WHERE id = 1`).Scan(&next); err != nil {
		return 0, fmt.Errorf("ingest: read offset: %w", err)
	}
	return next, nil
}

// advanceOffset writes next as ingest_offset's next_update_id, guarded and
// monotone: the UPDATE applies only when next exceeds the currently stored
// value, so a re-run — a retried settlement, or two settlements racing —
// can never move the offset backwards (design D5, D14). Callers pass the
// settled update's update_id + 1, computed once at the call site so this
// function's own contract stays "write this exact value, if it is
// forward".
func advanceOffset(ctx context.Context, tx pgx.Tx, next int64) error {
	if _, err := tx.Exec(ctx,
		`UPDATE ingest_offset SET next_update_id = $1 WHERE id = 1 AND next_update_id < $1`,
		next,
	); err != nil {
		return fmt.Errorf("ingest: advance offset: %w", err)
	}
	return nil
}
