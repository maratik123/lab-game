package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// discoveryLimitSQL is D2's discovery statement: a single FOR NO KEY
// UPDATE ... SKIP LOCKED claim, run outside any explicit transaction so
// its row locks live only for the statement. It answers "which ids are
// due and not currently being executed" — the batch's cardinality is
// AC12's BatchSize.
const discoverySQL = `
	SELECT id FROM scheduled_task
	WHERE state = 'pending' AND run_at <= now()
	ORDER BY run_at, id
	FOR NO KEY UPDATE SKIP LOCKED
	LIMIT $1
`

// discoverDue runs the discovery statement on pool and returns the
// claimed ids, in order.
func discoverDue(ctx context.Context, pool *pgxpool.Pool, limit int) ([]TaskID, error) {
	rows, err := pool.Query(ctx, discoverySQL, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduler: discover due tasks: %w", err)
	}
	defer rows.Close()

	var ids []TaskID
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scheduler: scan discovered id: %w", err)
		}
		ids = append(ids, TaskID(id))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scheduler: discovery rows: %w", err)
	}
	return ids, nil
}

// reclaimSQL is D2 step 2's per-id re-claim: the same lock mode the
// discovery statement takes (AC35), re-checking the state/run_at
// predicate. Zero rows means another worker took it, it was already
// settled, or a recurrence's run_at was already advanced — silently
// skipped by the caller, not a failure.
const reclaimSQL = `
	SELECT id, type, instance_key, payload, run_at, consecutive_failures
	FROM scheduled_task
	WHERE id = $1 AND state = 'pending' AND run_at <= now()
	FOR NO KEY UPDATE SKIP LOCKED
`

// reclaim re-claims id inside tx, returning the claimed Task and true, or
// a zero Task and false when the row was not available (design D2 step
// 2).
func reclaim(ctx context.Context, tx pgx.Tx, id TaskID) (Task, bool, error) {
	var (
		task        Task
		typ         string
		instanceKey *string
		payload     json.RawMessage
		scannedID   int64
	)
	err := tx.QueryRow(ctx, reclaimSQL, int64(id)).Scan(
		&scannedID, &typ, &instanceKey, &payload, &task.RunAt, &task.ConsecutiveFailures)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, fmt.Errorf("scheduler: reclaim task %d: %w", id, err)
	}
	task.ID = TaskID(scannedID)
	task.Type = Type(typ)
	if instanceKey != nil {
		task.InstanceKey = *instanceKey
	}
	task.Payload = payload
	return task, true, nil
}
