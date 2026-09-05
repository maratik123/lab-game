package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setTimeoutsSQL sets the two transaction-local timeouts through
// set_config(name, $1, true) rather than SET LOCAL name = $1, which is
// not a parameterisable position (design D2 step 1).
const setTimeoutsSQL = `SELECT set_config('statement_timeout', $1, true), set_config('idle_in_transaction_session_timeout', $1, true)`

// handlerResult is what the handler goroutine reports back to executeOne.
type handlerResult struct {
	outcome                Outcome
	handlerErr, releaseErr error
}

// executeOne runs id's whole per-task transaction (design D2's steps
// 1-8, including the per-task execution deadline, design D11). batchSize
// is the discovery cardinality of the cycle id came from (AC12's
// Observation.BatchSize). An Observation is emitted after COMMIT
// succeeds — never before, since an observation emitted before the
// commit is a claim about work that may still roll back (design D2) —
// except when the row was skipped at re-claim, which produces no
// observation at all because nothing executed.
//
// The connection is acquired explicitly, rather than through
// (*pgxpool.Pool).Begin, because a deadline breach must hijack it
// (design D11) — an operation only pgxpool.Conn exposes.
func (w *Worker) executeOne(ctx context.Context, id TaskID, batchSize int) error {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: acquire connection for task %d: %w", id, err)
	}
	handled := false
	defer func() {
		if !handled {
			conn.Release()
		}
	}()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("scheduler: begin task %d: %w", id, err)
	}
	defer func() {
		if !handled {
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
		err := tx.Commit(ctx)
		handled = true
		conn.Release()
		return err
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
		handled = true
		conn.Release()
		obs.Outcome = OutcomeFailed
		obs.Failure = FailureUnregistered
		obs.ConsecutiveFailures = task.ConsecutiveFailures
		w.observeTask(obs)
		return nil
	}

	// Layer 1 of D11: the handler runs on its own goroutine with a
	// deadline-bearing context, handed the worker's own tx. The worker
	// selects on either its result or the deadline (layer 3).
	deadlineCtx, cancel := context.WithTimeout(ctx, w.cfg.TaskTimeout)
	defer cancel()
	resultCh := make(chan handlerResult, 1)
	go func() {
		outcome, handlerErr, releaseErr := runHandlerWithSavepoint(deadlineCtx, tx, decl.Handler, task)
		resultCh <- handlerResult{outcome, handlerErr, releaseErr}
	}()

	select {
	case r := <-resultCh:
		handled = true // settleAndAfter owns commit/rollback and conn.Release from here
		return w.settleAndAfter(ctx, tx, conn, task, decl, r, obs)
	case <-deadlineCtx.Done():
		// The deadline was breached: this task's transaction is
		// abandoned, not committed or rolled back — the row is still
		// locked until the server terminates the backend (layer 2) or the
		// watchdog below closes the hijacked connection once the orphaned
		// handler goroutine returns. Design D7's deferred settlement
		// closes the "attempt never counted" hole this would otherwise
		// leave.
		pconn := conn.Hijack()
		handled = true
		w.enqueuePending(pendingSettlement{
			id: id, runAt: task.RunAt, consecutiveFailures: task.ConsecutiveFailures,
			recurrence: decl.Recurrence, reason: "deadline exceeded",
		})
		go func() { //nolint:gosec,contextcheck // G118/contextcheck: context.Background() is deliberate here — ctx (and deadlineCtx) may already be done by the time this fires, and closing the connection must still happen, since that is what finally releases the row's lock for a handler that ignores its own ctx (design D11)
			<-resultCh // wait for the orphaned handler goroutine to return
			_ = pconn.Close(context.Background())
		}()
		obs.Outcome = OutcomeFailed
		obs.Failure = FailureDeadline
		obs.ConsecutiveFailures = task.ConsecutiveFailures + 1
		w.observeTask(obs)
		return nil
	}
}

// settleAndAfter applies r's settlement and commits tx, then releases
// conn. On a commit failure it defers the same settlement instead
// (design D2 step 8, D7): the commit that would have written it never
// happened, so the row is left exactly as due as it was, and only a
// later drain can count the attempt. The caller has already marked its
// own handled flag true; this function owns tx/conn from here on.
func (w *Worker) settleAndAfter(
	ctx context.Context, tx pgx.Tx, conn *pgxpool.Conn,
	task Task, decl Declaration, r handlerResult, obs Observation,
) error {
	defer conn.Release()

	outcome := r.outcome
	handlerErr := r.handlerErr
	failure := FailureNone
	switch {
	case r.releaseErr != nil:
		outcome = OutcomeFailed
		failure = FailureRolledBack
		handlerErr = r.releaseErr
	case handlerErr != nil:
		outcome = OutcomeFailed
		failure = FailureHandler
	}

	if err := settleOutcome(ctx, tx, task, decl, outcome, handlerErr, w.cfg); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("scheduler: settle task %d: %w", task.ID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		w.enqueuePending(pendingSettlement{
			id: task.ID, runAt: task.RunAt, consecutiveFailures: task.ConsecutiveFailures,
			recurrence: decl.Recurrence, reason: fmt.Sprintf("commit failed: %s", err),
		})
		obs.Outcome = OutcomeFailed
		obs.Failure = FailureRolledBack
		obs.ConsecutiveFailures = task.ConsecutiveFailures + 1
		w.observeTask(obs)
		return nil
	}

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
