package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// nullJSON is what a nil Request.Payload is stored as: JSON null. An
// absent payload and a payload the handler must actively decode as null
// are different things, and this makes the choice explicit rather than
// leaving it to whatever an empty byte slice happens to encode as.
var nullJSON = json.RawMessage("null")

// Schedule inserts req as a new row, returning its TaskID. tx is
// caller-owned: Schedule neither commits nor rolls back, so a mechanic
// can schedule a task's timer edges inside its own transition's
// transaction (design D9).
//
// Schedule refuses req.Type with no Declaration (ErrUnknownType),
// req.Delay < 0 (ErrInvalidDelay), a malformed req.Payload
// (ErrInvalidPayload), and a live duplicate identity (ErrDuplicateTask) —
// the last of these by declining the INSERT rather than raising a raw
// SQLSTATE 23505, exactly as store.Post's PlayerOperation basis does, so
// a duplicate timer edge cannot abort the caller's transaction.
func (r *Registry) Schedule(ctx context.Context, tx pgx.Tx, req Request) (TaskID, error) {
	if _, ok := r.declaration(req.Type); !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownType, req.Type)
	}
	if req.Delay < 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidDelay, req.Delay)
	}
	payload := req.Payload
	if payload == nil {
		payload = nullJSON
	} else if !json.Valid(payload) {
		return 0, fmt.Errorf("%w: not valid JSON", ErrInvalidPayload)
	}

	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at)
		 VALUES ($1, NULLIF($2, ''), $3, clock_timestamp() + $4)
		 ON CONFLICT (type, instance_key) WHERE instance_key IS NOT NULL AND state = 'pending' DO NOTHING
		 RETURNING id`,
		string(req.Type), req.InstanceKey, payload, req.Delay,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: type %q, instance key %q", ErrDuplicateTask, req.Type, req.InstanceKey)
	}
	if err != nil {
		return 0, fmt.Errorf("scheduler: insert scheduled_task: %w", err)
	}
	return TaskID(id), nil
}

// DeadTasks returns every give-up row (state = 'dead'), ordered by
// run_at then id, up to limit. tx is caller-owned — the same shape
// Schedule and store.Post take (design D1's NOTE 3(a)).
func DeadTasks(ctx context.Context, tx pgx.Tx, limit int) ([]DeadTask, error) {
	rows, err := tx.Query(ctx,
		`SELECT type, coalesce(instance_key, ''), run_at, consecutive_failures, coalesce(last_error, '')
		 FROM scheduled_task
		 WHERE state = 'dead'
		 ORDER BY run_at, id
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduler: select dead tasks: %w", err)
	}
	defer rows.Close()

	var out []DeadTask
	for rows.Next() {
		var d DeadTask
		var typ string
		if err := rows.Scan(&typ, &d.InstanceKey, &d.RunAt, &d.ConsecutiveFailures, &d.LastError); err != nil {
			return nil, fmt.Errorf("scheduler: scan dead task: %w", err)
		}
		d.Type = Type(typ)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scheduler: dead tasks rows: %w", err)
	}
	return out, nil
}
