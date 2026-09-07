package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/store"
)

// runAttempts drives u's bounded retry (design D6, D23, D24): one
// attempt per transaction, a strictly positive and non-shrinking delay
// between failed attempts via backoff.Exponential, up to
// Config.RetryMaxAttempts. It returns nil once the update is settled —
// handled, duplicate or given-up all count as settled — and a non-nil
// ctx.Err() only when ctx is cancelled mid-retry (design D25), leaving
// the update unsettled with the offset behind it.
func (l *Loop) runAttempts(ctx context.Context, h Handler, u Update) error {
	var lastErr error
	for attempt := range l.cfg.RetryMaxAttempts {
		settled, err := l.attemptOnce(ctx, h, u, attempt)
		if settled {
			return nil
		}
		lastErr = err

		if attempt == l.cfg.RetryMaxAttempts-1 {
			break
		}

		delay := backoff.Exponential(attempt, l.cfg.RetryBaseDelay, l.cfg.RetryMaxDelay, l.cfg.RetryFactor)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return l.settleGivenUp(ctx, u, l.cfg.RetryMaxAttempts, lastErr)
}

// attemptOnce runs h.Handle inside its own transaction for u (design
// D6): a panic is recovered (design D7), and the outcome is classified
// by design D8's sentinel test. It returns settled=true once the update
// no longer needs another attempt — OutcomeHandled or OutcomeDuplicate,
// both of which advance the offset before returning — and settled=false
// with the handler's error otherwise, having already rolled the attempt
// back and reported its Observation.
func (l *Loop) attemptOnce(ctx context.Context, h Handler, u Update, attempt int) (settled bool, retryErr error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("ingest: begin attempt: %w", err)
	}

	// duration measures h.Handle itself, per Observation.Duration's
	// contract (design D12: "the call's duration") — not the surrounding
	// transaction plumbing (advanceOffset, Commit, Rollback), so a slow
	// database has no bearing on this number and a fast handler always
	// reports as fast, whatever its transaction later does.
	handlerStart := time.Now()
	handlerErr, panicked := safeHandle(ctx, h, tx, u)
	duration := time.Since(handlerStart)

	switch {
	case handlerErr == nil:
		if err := advanceOffset(ctx, tx, int64(u.Raw.UpdateID)+1); err != nil {
			_ = tx.Rollback(ctx)
			l.reportAttempt(u, attempt, OutcomeFailed, duration)
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			err = fmt.Errorf("ingest: commit handled attempt: %w", err)
			l.reportAttempt(u, attempt, OutcomeFailed, duration)
			return false, err
		}
		l.reportAttempt(u, attempt, OutcomeHandled, duration)
		return true, nil

	case errors.Is(handlerErr, store.ErrAlreadyPosted):
		// The effects already exist from an earlier delivery: roll this
		// attempt back first (design D5) — any other write the handler
		// made before Post is a replayed effect, not a wanted one — then
		// advance the offset in a transaction of its own.
		_ = tx.Rollback(ctx)
		if err := l.advanceOffsetFresh(ctx, u); err != nil {
			l.reportAttempt(u, attempt, OutcomeFailed, duration)
			return false, err
		}
		l.reportAttempt(u, attempt, OutcomeDuplicate, duration)
		return true, nil

	default:
		_ = tx.Rollback(ctx)
		outcome := OutcomeFailed
		if panicked {
			outcome = OutcomePanic
		}
		l.reportAttempt(u, attempt, outcome, duration)
		return false, handlerErr
	}
}

// safeHandle calls h.Handle under recover (design D7): a panic never
// terminates the process, and is reported to the caller as an error
// distinct from a returned one via the panicked return.
func safeHandle(ctx context.Context, h Handler, tx pgx.Tx, u Update) (err error, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("ingest: handler panic: %v", r)
		}
	}()
	err = h.Handle(ctx, tx, u)
	return err, false
}

// reportAttempt reports one per-attempt Observation, filling Lag from
// design D13's rule.
func (l *Loop) reportAttempt(u Update, attempt int, outcome Outcome, duration time.Duration) {
	lag, lagKnown := lagFor(u)
	observeUpdate(l.observer, Observation{
		Kind:     u.Kind,
		Attempt:  attempt,
		Outcome:  outcome,
		Duration: duration,
		Lag:      lag,
		LagKnown: lagKnown,
	})
}
