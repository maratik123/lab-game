package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
)

// settle.go owns design D7 in full: the settlement statement for every
// outcome, the backoff and next-cadence instants computed from the
// settlement instant s, and the pending-settlement set with its guarded,
// non-blocking drain (design D2 step 0, D11). execute.go owns the cycle
// around it — re-claim, savepoint, handler call, deadline — and calls
// into this file rather than issuing settlement statements of its own.

// pendingSettlement is one task whose transaction died before it could
// settle it: a deadline breach or a failed COMMIT. It carries everything
// a later drain needs to recompute the same Failed row D7 would have
// written inline: the run_at read at claim time (the drain's guard), the
// failure count before this attempt, and the recurrence (nil for a
// one-shot).
type pendingSettlement struct {
	id                  TaskID
	runAt               time.Time
	consecutiveFailures int
	recurrence          *Recurrence
	reason              string
}

// enqueuePending records p in w's pending-settlement set, guarded by a
// mutex because Run and a caller's RunOnce may both touch it.
func (w *Worker) enqueuePending(p pendingSettlement) {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()
	if w.pending == nil {
		w.pending = make(map[TaskID]pendingSettlement)
	}
	w.pending[p.id] = p
}

// drainPending issues the guarded, non-blocking deferred settlement for
// every task in w's pending set, at the top of a cycle before discovery
// (design D2 step 0). One settlement instant is read per drain and
// shared by every id in it, so every deferred backoff or next-cadence
// instant is measured from the moment the row is actually deferred
// (design D7).
func (w *Worker) drainPending(ctx context.Context) error {
	w.pendingMu.Lock()
	items := make([]pendingSettlement, 0, len(w.pending))
	for _, p := range w.pending {
		items = append(items, p)
	}
	w.pendingMu.Unlock()
	if len(items) == 0 {
		return nil
	}

	var s time.Time
	if err := w.pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&s); err != nil {
		return fmt.Errorf("scheduler: read drain instant: %w", err)
	}

	for _, p := range items {
		settledOrGone, err := drainOne(ctx, w.pool, p, s, w.cfg)
		if err != nil {
			return fmt.Errorf("scheduler: drain task %d: %w", p.id, err)
		}
		if settledOrGone {
			w.pendingMu.Lock()
			delete(w.pending, p.id)
			w.pendingMu.Unlock()
		}
	}
	return nil
}

// drainOneSQL is D7's guarded deferred-settlement statement: the
// candidate CTE re-checks state, run_at (the guard) and takes the same
// lock mode the claim does, so a row another worker has since finished
// matches nothing and the UPDATE is a no-op rather than an attempt
// counted against work that succeeded. setClause is one of D7's three
// Failed rows, built by the caller.
const drainOneCandidateSQL = `
	WITH candidate AS (
		SELECT id FROM scheduled_task
		WHERE id = $1 AND state = 'pending' AND run_at = $2
		FOR NO KEY UPDATE SKIP LOCKED
	)
`

// drainOne issues one deferred settlement for p, non-blocking. It
// returns true when the caller should drop p from the pending set:
// either the settlement applied, or the probe below found the row gone
// (moved on without us). It returns false when the row is still locked
// by the abandoned transaction, so p must be retried on a later drain.
func drainOne(ctx context.Context, pool *pgxpool.Pool, p pendingSettlement, s time.Time, cfg config.Scheduler) (bool, error) {
	sql, args := deferredFailedStatement(p, s, cfg)

	tag, err := pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}

	// UPDATE 0 is ambiguous: locked, gone, or already moved on. One
	// unlocked probe tells them apart without blocking.
	var exists int
	err = pool.QueryRow(ctx,
		`SELECT 1 FROM scheduled_task WHERE id = $1 AND state = 'pending' AND run_at = $2`,
		int64(p.id), p.runAt,
	).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil // moved on without us — drop it
	}
	if err != nil {
		return false, err
	}
	return false, nil // still locked — retry next drain
}

// deferredFailedStatement builds the SQL and args for p's deferred
// settlement — the same Failed row D7's table selects for an inline
// attempt, wrapped in drainOneCandidateSQL's guard.
func deferredFailedStatement(p pendingSettlement, s time.Time, cfg config.Scheduler) (string, []any) {
	k := p.consecutiveFailures + 1

	if p.recurrence == nil {
		if k >= cfg.RetryMaxAttempts {
			return drainOneCandidateSQL + `
				UPDATE scheduled_task t SET state = 'dead', consecutive_failures = $3, last_error = $4
				FROM candidate c WHERE t.id = c.id
			`, []any{int64(p.id), p.runAt, k, p.reason}
		}
		runAt := s.Add(backoff.Exponential(k-1, cfg.RetryBaseDelay, cfg.RetryMaxDelay, cfg.RetryFactor))
		return drainOneCandidateSQL + `
			UPDATE scheduled_task t SET consecutive_failures = $3, last_error = $4, run_at = $5
			FROM candidate c WHERE t.id = c.id
		`, []any{int64(p.id), p.runAt, k, p.reason, runAt}
	}

	next := p.recurrence.Cadence(p.runAt, s)
	return drainOneCandidateSQL + `
		UPDATE scheduled_task t SET consecutive_failures = $3, last_error = $4, run_at = $5
		FROM candidate c WHERE t.id = c.id
	`, []any{int64(p.id), p.runAt, k, p.reason, next}
}

// readSettlementInstant reads the settlement instant s (design D3, D7):
// clock_timestamp() on tx, immediately before a settlement statement that
// writes a run_at. Read after the handler has returned, so a backoff or
// next-cadence instant is measured from now rather than from before the
// handler ran.
func readSettlementInstant(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var s time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&s); err != nil {
		return time.Time{}, fmt.Errorf("scheduler: read settlement instant: %w", err)
	}
	return s, nil
}

// settleUnregistered settles a claimed row whose type has no Declaration
// (design D7's last row): the row is never executed, so
// consecutive_failures and state are untouched — it can never reach dead
// — and run_at is pushed out by ceiling so the refusal recurs at a
// bounded rate.
func settleUnregistered(ctx context.Context, tx pgx.Tx, id TaskID, ceiling time.Duration) error {
	s, err := readSettlementInstant(ctx, tx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE scheduled_task SET run_at = $2 WHERE id = $1`, int64(id), s.Add(ceiling))
	if err != nil {
		return fmt.Errorf("scheduler: settle unregistered: %w", err)
	}
	return nil
}

// settleOutcome writes task's settlement statement for outcome (design
// D7's table), computing every future run_at from the settlement instant
// s — never from the execution instant — per D3/D7.
func settleOutcome(ctx context.Context, tx pgx.Tx, task Task, decl Declaration, outcome Outcome, handlerErr error, cfg config.Scheduler) error {
	switch outcome {
	case OutcomeDone, OutcomeNoop:
		return settleDoneOrNoop(ctx, tx, task, decl)
	case OutcomeFailed:
		return settleFailed(ctx, tx, task, decl, handlerErr, cfg)
	default:
		return fmt.Errorf("scheduler: unknown outcome %d", outcome)
	}
}

func settleDoneOrNoop(ctx context.Context, tx pgx.Tx, task Task, decl Declaration) error {
	if decl.Recurrence == nil {
		if _, err := tx.Exec(ctx, `DELETE FROM scheduled_task WHERE id = $1`, int64(task.ID)); err != nil {
			return fmt.Errorf("scheduler: delete done task: %w", err)
		}
		return nil
	}
	s, err := readSettlementInstant(ctx, tx)
	if err != nil {
		return err
	}
	next := decl.Recurrence.Cadence(task.RunAt, s)
	if _, err := tx.Exec(ctx,
		`UPDATE scheduled_task SET run_at = $2, consecutive_failures = 0, last_error = NULL WHERE id = $1`,
		int64(task.ID), next,
	); err != nil {
		return fmt.Errorf("scheduler: reschedule recurrent task: %w", err)
	}
	return nil
}

func settleFailed(ctx context.Context, tx pgx.Tx, task Task, decl Declaration, handlerErr error, cfg config.Scheduler) error {
	lastError := ""
	if handlerErr != nil {
		lastError = handlerErr.Error()
	}
	k := task.ConsecutiveFailures + 1

	if decl.Recurrence == nil {
		if k >= cfg.RetryMaxAttempts {
			if _, err := tx.Exec(ctx,
				`UPDATE scheduled_task SET state = 'dead', consecutive_failures = $2, last_error = $3 WHERE id = $1`,
				int64(task.ID), k, lastError,
			); err != nil {
				return fmt.Errorf("scheduler: give up on task: %w", err)
			}
			return nil
		}
		s, err := readSettlementInstant(ctx, tx)
		if err != nil {
			return err
		}
		runAt := s.Add(backoff.Exponential(k-1, cfg.RetryBaseDelay, cfg.RetryMaxDelay, cfg.RetryFactor))
		if _, err := tx.Exec(ctx,
			`UPDATE scheduled_task SET consecutive_failures = $2, last_error = $3, run_at = $4 WHERE id = $1`,
			int64(task.ID), k, lastError, runAt,
		); err != nil {
			return fmt.Errorf("scheduler: settle failed one-shot: %w", err)
		}
		return nil
	}

	s, err := readSettlementInstant(ctx, tx)
	if err != nil {
		return err
	}
	next := decl.Recurrence.Cadence(task.RunAt, s)
	if _, err := tx.Exec(ctx,
		`UPDATE scheduled_task SET consecutive_failures = $2, last_error = $3, run_at = $4 WHERE id = $1`,
		int64(task.ID), k, lastError, next,
	); err != nil {
		return fmt.Errorf("scheduler: settle failed recurrent: %w", err)
	}
	return nil
}
