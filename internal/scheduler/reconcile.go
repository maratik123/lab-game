package scheduler

import (
	"context"
	"fmt"
	"time"
)

// reconcileSeedSQL is the seed statement: one statement per declared
// recurrence, idempotent by the constraint rather than by a lock, so
// several workers starting at once is safe by construction. $2, the
// instance key, is the recurrence's type name — derived, not declared —
// which is what puts a freshly seeded row inside
// scheduled_task_identity_key's predicate.
const reconcileSeedSQL = `
	INSERT INTO scheduled_task (type, instance_key, payload, run_at)
	VALUES ($1, $2, '{}'::jsonb, $3)
	ON CONFLICT (type, instance_key) WHERE instance_key IS NOT NULL AND state = 'pending' DO NOTHING
`

// reconcileCorrectSQL is the correction statement: it leaves an in-flight
// or imminent occurrence alone (the same FOR NO KEY UPDATE ... SKIP
// LOCKED mode the claim takes), and fires only when the declaration's
// cadence now disagrees with a later run_at than it computes — i.e. the
// cadence was shortened. A lengthened cadence leaves run_at <= $3, no
// row matches, and the change is absorbed by one early occurrence.
const reconcileCorrectSQL = `
	WITH candidate AS (
		SELECT id FROM scheduled_task
		WHERE type = $1 AND instance_key = $2 AND state = 'pending'
		  AND run_at > $3
		  AND run_at > now() + $4
		FOR NO KEY UPDATE SKIP LOCKED
	)
	UPDATE scheduled_task s SET run_at = $3 FROM candidate c WHERE s.id = c.id
`

// Reconcile runs the seed and the correction for every declared
// recurrence, and no third move: there is no revive branch
// (a declared recurrence has no dead state to revive from) and no
// collector for a type no longer declared (retiring a recurrence deletes
// its row in the same change that drops its declaration). It is
// idempotent — safe to call more than once, and safe for concurrent
// callers — so Run calls it once before its first cycle and a
// composition root may legitimately call it again before any worker
// starts.
func (w *Worker) Reconcile(ctx context.Context) error {
	var now time.Time
	if err := w.pool.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return fmt.Errorf("scheduler: reconcile: read now: %w", err)
	}

	for _, decl := range w.registry.decls {
		if decl.Recurrence == nil {
			continue
		}
		instanceKey := string(decl.Type)
		next := decl.Recurrence.Cadence(now, now)

		if _, err := w.pool.Exec(ctx, reconcileSeedSQL, string(decl.Type), instanceKey, next); err != nil {
			return fmt.Errorf("scheduler: reconcile: seed %q: %w", decl.Type, err)
		}
		if _, err := w.pool.Exec(ctx, reconcileCorrectSQL, string(decl.Type), instanceKey, next, w.cfg.PollInterval); err != nil {
			return fmt.Errorf("scheduler: reconcile: correct %q: %w", decl.Type, err)
		}
	}
	return nil
}
