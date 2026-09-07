package ingest

import (
	"context"
	"fmt"
	"time"
)

// advanceOffsetFresh advances the offset for u in a transaction of its
// own — the shape design D5 gives every settled outcome that does not
// commit alongside a handler's own effects: a duplicate (whose attempt
// transaction was already rolled back) and, via settleUnrouted and
// settleGivenUp, the attempt-less outcomes.
func (l *Loop) advanceOffsetFresh(ctx context.Context, u Update) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ingest: begin offset advance: %w", err)
	}
	if err := advanceOffset(ctx, tx, int64(u.Raw.UpdateID)+1); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ingest: commit offset advance: %w", err)
	}
	return nil
}

// settleUnrouted settles u whose derived Kind has no registered Handler
// (design D9, AC9): no attempt ran, and the offset still advances so an
// unrouted kind never blocks the loop. derivationErr carries NewUpdate's
// error when u could not be fully derived (a malformed raw payload) —
// nil for a genuinely-unrouted Kind — and is reported on the resulting
// Observation rather than silently dropped.
func (l *Loop) settleUnrouted(ctx context.Context, u Update, derivationErr error) error {
	start := time.Now()
	if err := l.advanceOffsetFresh(ctx, u); err != nil {
		return err
	}
	lag, lagKnown := lagFor(u)
	observeUpdate(l.observer, Observation{
		Kind:     u.Kind,
		Outcome:  OutcomeUnrouted,
		Duration: time.Since(start),
		Lag:      lag,
		LagKnown: lagKnown,
		Err:      derivationErr,
	})
	return nil
}

// settleGivenUp settles u after every configured attempt failed or
// panicked (design D6, D14): its identity is written to
// ingest_dead_update — never its raw payload — the offset still
// advances, and the loop continues polling.
func (l *Loop) settleGivenUp(ctx context.Context, u Update, attempts int, lastErr error) error {
	start := time.Now()

	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ingest: begin give-up: %w", err)
	}

	lastErrText := ""
	if lastErr != nil {
		lastErrText = lastErr.Error()
	}
	chatID, hasChatID := ChatID(&u.Raw)
	dead := DeadUpdate{
		UpdateID:            int64(u.Raw.UpdateID),
		Kind:                u.Kind,
		ConsecutiveFailures: attempts,
		LastError:           lastErrText,
	}
	if hasChatID {
		dead.ChatID = &chatID
	}
	if err := writeDeadUpdate(ctx, tx, dead); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := advanceOffset(ctx, tx, int64(u.Raw.UpdateID)+1); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ingest: commit give-up: %w", err)
	}

	lag, lagKnown := lagFor(u)
	observeUpdate(l.observer, Observation{
		Kind:     u.Kind,
		Attempt:  attempts - 1,
		Outcome:  OutcomeGivenUp,
		Duration: time.Since(start),
		Lag:      lag,
		LagKnown: lagKnown,
	})
	return nil
}
