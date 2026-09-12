package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/panicguard"
)

// setTimeoutsSQL sets the two transaction-local timeouts through
// set_config(name, $1, true) rather than SET LOCAL name = $1, which is
// not a parameterisable position.
//
// $1 is bound twice, deliberately: w.cfg.TaskTimeout drives both server-
// side timeouts through the one knob, and context.WithTimeout below
// (executeOne's deadlineCtx) is a third consumer of the same value, so
// TaskTimeout drives three deadlines in total. The two set here differ
// in what they measure: statement_timeout bounds one statement's own
// duration, while idle_in_transaction_session_timeout bounds the Go-side
// gaps BETWEEN statements inside this open transaction — whichever
// goroutine issues them — not any single query.
//
// That distinction matters when choosing a value: the deadline branch of
// executeOne's select hijacks the connection without rolling back. The
// row's lock is normally released promptly by the synchronous terminate
// that branch issues from another pooled connection; if that terminate
// itself fails or finds the pid already gone, the row falls back to
// whichever of the pre-existing layers reaches it first —
// idle_in_transaction_session_timeout expiring server-side, or the
// watchdog goroutine closing the hijacked connection once the orphaned
// handler returns. The timeout is load-bearing for that fallback
// lock-release timing, not only for classifying the failure as
// FailureDeadline.
const setTimeoutsSQL = `SELECT set_config('statement_timeout', $1, true), set_config('idle_in_transaction_session_timeout', $1, true)`

// detachedCloseTimeout bounds the watchdog goroutine's close of a
// hijacked connection once its orphaned handler returns. A named
// constant, not a configuration key: the close is a local cleanup, not
// a tunable balance value, and a socket that never drains its own
// Close within this long is one the goroutine must stop waiting on
// rather than block forever.
const detachedCloseTimeout = 5 * time.Second

// terminateTimeout bounds the deadline branch's own synchronous
// pg_terminate_backend call, issued on a context derived via
// context.WithoutCancel so the terminate still runs once the caller's
// ctx (and deadlineCtx) are already done. A named constant, not a
// configuration key, for the same reason detachedCloseTimeout is one:
// the call is a local cleanup, not a tunable balance value. The
// statement itself carries no server-side wait — it is the signalling,
// one-argument form of pg_terminate_backend — so this bound is the only
// one that applies to it.
const terminateTimeout = 5 * time.Second

// terminateBackendSQL is the signalling (one-argument) form of
// pg_terminate_backend: it returns once the signal has been sent,
// reporting true when the pid was a live backend and false when it was
// not — never whether the backend has actually died, which the caller
// must not wait to confirm (measured: the waiting two-argument form
// buys no earlier row release, only a hundred-millisecond floor on this
// call's own return).
const terminateBackendSQL = `SELECT pg_terminate_backend($1)`

// handlerResult is what the handler goroutine reports back to executeOne.
type handlerResult struct {
	outcome                Outcome
	handlerErr, releaseErr error
	// panicked is true when handlerErr wraps a recovered handler panic
	// rather than an error the Handler returned.
	panicked bool
}

// executeOne runs id's whole per-task transaction, including the
// per-task execution deadline. batchSize
// is the discovery cardinality of the cycle id came from (via
// Observation.BatchSize). An Observation is emitted after COMMIT
// succeeds — never before, since an observation emitted before the
// commit is a claim about work that may still roll back —
// except when the row was skipped at re-claim, which produces no
// observation at all because nothing executed.
//
// The connection is acquired explicitly, rather than through
// (*pgxpool.Pool).Begin, because a deadline breach must hijack it
// — an operation only pgxpool.Conn exposes.
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

	// The execution instant t: read once, before the handler
	// runs. Its one job is the observed lag; every future run_at is
	// computed from the settlement instant instead.
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

	// Layer 1 of a three-layer deadline defence: the handler runs on its
	// own goroutine with a deadline-bearing context, handed the worker's
	// own tx. The worker
	// selects on either its result or the deadline (layer 3).
	deadlineCtx, cancel := context.WithTimeout(ctx, w.cfg.TaskTimeout)
	defer cancel()

	// The backend pid this attempt's connection holds, read here — after
	// the deadline context exists but before the handler goroutine
	// launches — because this is the last point at which conn is
	// provably idle: pgx documents PgConn's escape hatch as safe only on
	// an idle connection, and once the handler goroutine starts it may
	// be mid-statement. A connection's backend pid cannot change while
	// the connection stays open, so the value read here is still the
	// breach branch's own victim if the deadline is later breached.
	pid := conn.Conn().PgConn().PID()

	resultCh := make(chan handlerResult, 1)
	go func() {
		outcome, handlerErr, releaseErr, panicked := runHandlerWithSavepoint(deadlineCtx, tx, decl.Handler, task, w.logger)
		resultCh <- handlerResult{outcome, handlerErr, releaseErr, panicked}
	}()

	select {
	case r := <-resultCh:
		handled = true // settleAndAfter owns commit/rollback and conn.Release from here
		return w.settleAndAfter(ctx, tx, conn, task, decl, r, obs)
	case <-deadlineCtx.Done():
		// The deadline was breached: this task's transaction is
		// abandoned, not committed or rolled back. The terminate issued
		// synchronously below is layer 2 of the deadline defence: it
		// reclaims the row's lock directly, without waiting for the
		// orphaned handler goroutine to notice or return. The pre-existing
		// layers — idle_in_transaction_session_timeout expiring
		// server-side, and the watchdog below closing the hijacked
		// connection once the orphaned handler returns — remain as the
		// fallback for a terminate that itself fails or finds the pid
		// already gone. The deferred settlement closes the "attempt never
		// counted" hole this would otherwise leave.
		pconn := conn.Hijack()
		handled = true
		w.enqueuePending(pendingSettlement{
			id: id, runAt: task.RunAt, consecutiveFailures: task.ConsecutiveFailures,
			recurrence: decl.Recurrence, reason: "deadline exceeded",
		})

		// Layer 2: terminate this attempt's own breached backend from
		// another pooled connection, synchronously — the signalling
		// (one-argument) form, which returns once the signal is sent
		// rather than waiting to confirm the death that waiting buys no
		// earlier row release for. Two non-success shapes are told apart
		// and logged at different levels: a Go error is a fault (a
		// permission failure, or a pool that cannot hand out a
		// connection) and is unexpected in ordinary operation; a false
		// return with a nil error means the pid was not a live backend —
		// benign by construction, since whatever ended that backend
		// already released its locks with it, and reachable in ordinary
		// operation because this branch's own
		// idle_in_transaction_session_timeout is armed against the same
		// TaskTimeout and can reach the backend first.
		// context.WithoutCancel is deliberate: the terminate must still
		// run once ctx (and deadlineCtx) are already done, since that is
		// exactly the case this branch handles.
		termCtx, termCancel := context.WithTimeout(context.WithoutCancel(ctx), terminateTimeout)
		var terminated bool
		if err := w.pool.QueryRow(termCtx, terminateBackendSQL, int64(pid)).Scan(&terminated); err != nil {
			w.logger.LogAttrs(ctx, slog.LevelError, "scheduler: terminate breached backend",
				slog.Int64("task_id", int64(id)), slog.Uint64("pid", uint64(pid)), slog.String("error", err.Error()))
		} else if !terminated {
			w.logger.LogAttrs(ctx, slog.LevelDebug, "scheduler: terminate found no live backend for breached task",
				slog.Int64("task_id", int64(id)), slog.Uint64("pid", uint64(pid)))
		}
		termCancel()

		go func() { //nolint:gosec,contextcheck // G118/contextcheck: context.Background() is deliberate here — ctx (and deadlineCtx) may already be done by the time this fires, and closing the connection must still happen: the row's own lock is normally already released by the terminate above, but the connection itself still needs closing to free pooled resources and to unblock a handler still writing on it
			<-resultCh                                                                          // wait for the orphaned handler goroutine to return
			closeCtx, cancel := context.WithTimeout(context.Background(), detachedCloseTimeout) //nolint:forbidigo // this is the watchdog's detached close: no shutdown path joins this goroutine, so it owns its own bounded root
			defer cancel()
			_ = pconn.Close(closeCtx)
		}()
		obs.Outcome = OutcomeFailed
		obs.Failure = FailureDeadline
		obs.ConsecutiveFailures = task.ConsecutiveFailures + 1
		w.observeTask(obs)
		return nil
	}
}

// settleAndAfter applies r's settlement and commits tx, then releases
// conn. On a commit failure it defers the same settlement instead: the
// commit that would have written it never happened, so the row is left
// exactly as due as it was, and only a
// later drain can count the attempt. A panic that also left the savepoint
// rollback unable to apply is routed the same way, before any settlement
// statement is attempted: the connection is unusable at that point,
// so no statement on tx — not a rollback, not the settlement UPDATE, not
// COMMIT — can be relied on to apply. The caller has already marked its
// own handled flag true; this function owns tx/conn from here on.
func (w *Worker) settleAndAfter(
	ctx context.Context, tx pgx.Tx, conn *pgxpool.Conn,
	task Task, decl Declaration, r handlerResult, obs Observation,
) error {
	defer conn.Release()

	if r.panicked && r.releaseErr != nil {
		// The recovered panic left rows open, which leaves the connection
		// busy: every later statement on it fails, including the
		// savepoint's own rollback (already failed, in r.releaseErr) and
		// any settlement statement this function would otherwise issue.
		// Releasing conn is what frees the row — pgxpool destroys a busy
		// connection on release rather than returning it to the pool —
		// so the deferred settlement above is the only route left, using
		// the rendered panic (not the deadline's fixed string) as its
		// reason so the stack still reaches last_error on the later drain.
		w.enqueuePending(pendingSettlement{
			id: task.ID, runAt: task.RunAt, consecutiveFailures: task.ConsecutiveFailures,
			recurrence: decl.Recurrence, reason: r.handlerErr.Error(),
		})
		obs.Outcome = OutcomeFailed
		obs.Failure = FailurePanic
		obs.ConsecutiveFailures = task.ConsecutiveFailures + 1
		w.observeTask(obs)
		return nil
	}

	outcome := r.outcome
	handlerErr := r.handlerErr
	failure := FailureNone
	switch {
	case r.panicked:
		// Tested first: a panic outranks a rollback failure in the
		// classification. By this point r.releaseErr is nil — the
		// branch above already routed the alternative.
		outcome = OutcomeFailed
		failure = FailurePanic
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
// cannot recover a failed release. On
// OutcomeDone it releases the savepoint; a release failure (a swallowed
// database error inside the handler) is reported as releaseErr and the
// subtransaction is then rolled back so the settlement statement can
// still apply. On OutcomeNoop or OutcomeFailed, or a non-nil handler
// error, it rolls back to the savepoint. A panic from handler.Execute is
// recovered at that call — and only at that call, never around the
// savepoint statements themselves — per this module's goroutine-
// ownership rule; logger receives the recovered panic's log record,
// emitted at this recovery point since not every attempt leaves a
// settled row for its stack to ride.
func runHandlerWithSavepoint(ctx context.Context, tx pgx.Tx, handler Handler, task Task, logger *slog.Logger) (outcome Outcome, handlerErr, releaseErr error, panicked bool) {
	if _, err := tx.Exec(ctx, "SAVEPOINT handler"); err != nil {
		return OutcomeFailed, fmt.Errorf("scheduler: savepoint: %w", err), nil, false
	}

	outcome, handlerErr, panicked = callHandlerRecovered(ctx, tx, handler, task, logger)

	if handlerErr == nil && outcome == OutcomeDone {
		if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT handler"); err != nil {
			// The release failed — almost always because the handler hit a
			// database error and swallowed it. Roll back to the savepoint
			// (still accepted after a failed release) so the settlement
			// statement below can apply.
			if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT handler"); rbErr != nil {
				return OutcomeFailed, handlerErr, fmt.Errorf("scheduler: rollback after failed release: %w", errors.Join(err, rbErr)), false
			}
			return OutcomeFailed, handlerErr, fmt.Errorf("release failed: %w", err), false
		}
		return OutcomeDone, nil, nil, false
	}

	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT handler"); err != nil {
		return outcome, handlerErr, fmt.Errorf("scheduler: rollback to savepoint: %w", err), panicked
	}
	return outcome, handlerErr, nil, panicked
}

// callHandlerRecovered calls handler.Execute on task inside tx, recovering
// a panic at that boundary: the ownership rule that a goroutine running a
// handler supplied through an interface recovers a panic there, records
// its stack, and routes it into the same failure path a returned error
// takes. A recovered panic is reported as OutcomeFailed with handlerErr
// wrapping the recovered value, panicked set true, and a log record at
// error level carrying the task identity and the stack as its own
// attribute.
func callHandlerRecovered(ctx context.Context, tx pgx.Tx, handler Handler, task Task, logger *slog.Logger) (outcome Outcome, handlerErr error, panicked bool) {
	defer func() {
		rec := panicguard.New(recover())
		if rec == nil {
			return
		}
		panicked = true
		outcome = OutcomeFailed
		handlerErr = fmt.Errorf("scheduler: handler panic: %w", rec)
		logger.LogAttrs(ctx, slog.LevelError, "scheduler: recovered handler panic",
			slog.Int64("task_id", int64(task.ID)),
			slog.String("task_type", string(task.Type)),
			slog.String("stack", string(rec.Stack)),
		)
	}()
	outcome, handlerErr = handler.Execute(ctx, tx, task)
	return outcome, handlerErr, false
}
