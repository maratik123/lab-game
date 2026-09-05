package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/config"
)

// setTimeoutsSQL sets the two transaction-local timeouts through
// set_config(name, $1, true) rather than SET LOCAL name = $1, which is
// not a parameterisable position (design D2 step 1).
const setTimeoutsSQL = `SELECT set_config('statement_timeout', $1, true), set_config('idle_in_transaction_session_timeout', $1, true)`

// executeOne runs id's whole per-task transaction (design D2's steps
// 1-8, minus the per-task execution deadline, which design D11/subtask 9
// adds around the handler call below). batchSize is the discovery
// cardinality of the cycle id came from (AC12's Observation.BatchSize).
// An Observation is emitted after COMMIT succeeds — never before, since
// an observation emitted before the commit is a claim about work that
// may still roll back (design D2) — except when the row was skipped at
// re-claim, which produces no observation at all because nothing
// executed.
func (w *Worker) executeOne(ctx context.Context, id TaskID, batchSize int) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: begin task %d: %w", id, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	timeout := fmt.Sprintf("%dms", w.cfg.TaskTimeout.Milliseconds())
	if _, err := tx.Exec(ctx, setTimeoutsSQL, timeout); err != nil {
		return fmt.Errorf("scheduler: set timeouts for task %d: %w", id, err)
	}

	task, ok, err := reclaim(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("scheduler: reclaim task %d: %w", id, err)
	}
	if !ok {
		// Another worker took it, it was already settled, or its run_at
		// moved on. Silently skipped: no handler call, no observation.
		return tx.Commit(ctx)
	}

	// The execution instant t (design D3): read once, before the handler
	// runs. Its one job is AC12's lag; every future run_at is computed
	// from the settlement instant instead (D7).
	t, err := readSettlementInstant(ctx, tx)
	if err != nil {
		return fmt.Errorf("scheduler: read execution instant for task %d: %w", id, err)
	}
	obs := Observation{Type: task.Type, Lag: t.Sub(task.RunAt), BatchSize: batchSize}

	decl, declared := w.registry.declaration(task.Type)
	if !declared {
		if err := settleUnregistered(ctx, tx, id, w.cfg.RetryMaxDelay); err != nil {
			return fmt.Errorf("scheduler: settle unregistered task %d: %w", id, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("scheduler: commit task %d: %w", id, err)
		}
		committed = true
		obs.Outcome = OutcomeFailed
		obs.Failure = FailureUnregistered
		obs.ConsecutiveFailures = task.ConsecutiveFailures
		w.observeTask(obs)
		return nil
	}

	outcome, handlerErr, releaseErr := runHandlerWithSavepoint(ctx, tx, decl.Handler, task)

	failure := FailureNone
	switch {
	case releaseErr != nil:
		outcome = OutcomeFailed
		failure = FailureRolledBack
		handlerErr = releaseErr
	case handlerErr != nil:
		outcome = OutcomeFailed
		failure = FailureHandler
	}

	if err := settleOutcome(ctx, tx, task, decl, outcome, handlerErr, w.cfg); err != nil {
		return fmt.Errorf("scheduler: settle task %d: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("scheduler: commit task %d: %w", id, err)
	}
	committed = true

	obs.Outcome = outcome
	obs.Failure = failure
	if outcome == OutcomeFailed {
		obs.ConsecutiveFailures = task.ConsecutiveFailures + 1
	}
	w.observeTask(obs)
	return nil
}

// runHandlerWithSavepoint runs handler on task inside tx under a raw-SQL
// savepoint — never pgx.Tx.Begin's pseudo-nested transaction, which
// cannot recover a failed release (design § Approach, D2 step 5). On
// OutcomeDone it releases the savepoint; a release failure (a swallowed
// database error inside the handler) is reported as releaseErr and the
// subtransaction is then rolled back so the settlement statement can
// still apply. On OutcomeNoop or OutcomeFailed, or a non-nil handler
// error, it rolls back to the savepoint.
func runHandlerWithSavepoint(ctx context.Context, tx pgx.Tx, handler Handler, task Task) (outcome Outcome, handlerErr, releaseErr error) {
	if _, err := tx.Exec(ctx, "SAVEPOINT handler"); err != nil {
		return OutcomeFailed, fmt.Errorf("scheduler: savepoint: %w", err), nil
	}

	outcome, handlerErr = handler.Execute(ctx, tx, task)

	if handlerErr == nil && outcome == OutcomeDone {
		if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT handler"); err != nil {
			// The release failed — almost always because the handler hit a
			// database error and swallowed it. Roll back to the savepoint
			// (still accepted after a failed release) so the settlement
			// statement below can apply.
			if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT handler"); rbErr != nil {
				return OutcomeFailed, handlerErr, fmt.Errorf("scheduler: rollback after failed release: %w", errors.Join(err, rbErr))
			}
			return OutcomeFailed, handlerErr, fmt.Errorf("release failed: %w", err)
		}
		return OutcomeDone, nil, nil
	}

	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT handler"); err != nil {
		return outcome, handlerErr, fmt.Errorf("scheduler: rollback to savepoint: %w", err)
	}
	return outcome, handlerErr, nil
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
// (design D7's last row): the row is never executed, so consecutive_failures
// and state are untouched — it can never reach dead — and run_at is pushed
// out by ceiling so the refusal recurs at a bounded rate.
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
		runAt := s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))
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
