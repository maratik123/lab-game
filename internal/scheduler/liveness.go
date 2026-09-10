package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LivenessOptions configures a Liveness.
type LivenessOptions struct {
	// Pool is the connection pool the persisted liveness row is read
	// from and written to. Must not be nil.
	Pool *pgxpool.Pool
	// Interval is how often Run refreshes the persisted liveness
	// instant. Must be strictly positive.
	Interval time.Duration
	// DowntimeThreshold is the gap, measured against the persisted
	// liveness instant, above which AbsorbDowntime shifts every overdue
	// pending row's run_at forward by the measured gap. Must be
	// strictly positive.
	DowntimeThreshold time.Duration
}

// Liveness owns the persisted process_liveness singleton: the
// restart-hygiene downtime shift that reads and clears it at start-up
// (AbsorbDowntime), and the running heartbeat that keeps it fresh
// (Run/Refresh). Every instant this type reasons about — seen_at, the
// measured gap, "now" — is read from the database; this file calls
// none of time.Now, time.Since or time.Until, and a guard in this
// package's test suite holds that structurally.
type Liveness struct {
	pool      *pgxpool.Pool
	interval  time.Duration
	threshold time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
}

// Downtime reports what one AbsorbDowntime call measured and did.
type Downtime struct {
	// Gap is the measured interval between the persisted liveness
	// instant and the database's own current instant — zero when Seeded
	// is true, since there was no prior instant to measure against.
	Gap time.Duration
	// Shifted is the number of scheduled_task rows whose run_at this
	// call moved forward by Gap.
	Shifted int
	// Seeded is true when no liveness instant had ever been recorded —
	// a database this process has never run against. Nothing moves and
	// the instant is written for the first time.
	Seeded bool
}

// NewLiveness builds a Liveness from opts, refusing a nil Pool or a
// non-positive Interval/DowntimeThreshold with an *OptionError naming
// the field.
func NewLiveness(opts LivenessOptions) (*Liveness, error) {
	if opts.Pool == nil {
		return nil, &OptionError{Field: "Pool", Reason: reasonMustNotBeNil}
	}
	if opts.Interval <= 0 {
		return nil, &OptionError{Field: "Interval", Reason: reasonMustBePositive}
	}
	if opts.DowntimeThreshold <= 0 {
		return nil, &OptionError{Field: "DowntimeThreshold", Reason: reasonMustBePositive}
	}
	return &Liveness{
		pool:      opts.Pool,
		interval:  opts.Interval,
		threshold: opts.DowntimeThreshold,
		stopCh:    make(chan struct{}),
	}, nil
}

// absorbLockAndGapSQL locks the singleton row for the rest of the
// transaction and reports its stored instant together with the gap to
// the database's own current instant — computed in SQL, so the decision
// never depends on this process's own clock. Both are NULL when the row
// has never been written: "gap > threshold" is then unknown, which is
// exactly "seeded, and nothing moves" without a separate branch.
//
// The lock is FOR NO KEY UPDATE, not the stronger FOR UPDATE: it is
// exactly the mode the transaction's own later UPDATE already takes,
// since seen_at is not a key column, and PostgreSQL's conflicting-locks
// table has FOR NO KEY UPDATE conflict with itself, which is all the
// mutual exclusion this transaction needs.
const absorbLockAndGapSQL = `
	SELECT seen_at, now() - seen_at
	FROM process_liveness
	WHERE id = 1
	FOR NO KEY UPDATE
`

// absorbShiftSQL moves every overdue pending row forward by the exact
// gap AbsorbDowntime already measured under the row lock above — the
// same value, never recomputed — so a second reader that raced in
// between cannot see a different "now".
const absorbShiftSQL = `
	UPDATE scheduled_task
	SET run_at = run_at + $1
	WHERE state = 'pending' AND run_at <= now()
`

// absorbSeenSQL records the current instant as the new liveness
// baseline, whether or not a shift ran.
const absorbSeenSQL = `UPDATE process_liveness SET seen_at = now() WHERE id = 1`

// AbsorbDowntime is the start-up restart-hygiene step: it locks the
// persisted liveness row, measures the gap to the database's current
// instant, and — when that gap exceeds DowntimeThreshold — moves every
// overdue pending scheduled_task row's run_at forward by the measured
// gap, so a long outage does not fire every due TTL and timer edge in
// one salvo. It writes to scheduled_task only: no basis-document table,
// no posting, no journal entry. The whole operation is one transaction,
// serialised against a concurrent caller by the row lock rather than by
// any advisory lock — the same guarantee the migration step's own lock
// gives the apply.
func (l *Liveness) AbsorbDowntime(ctx context.Context) (Downtime, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return Downtime{}, fmt.Errorf("scheduler: absorb downtime: begin: %w", err)
	}

	var seenAt *time.Time
	var gap pgtype.Interval
	if err := tx.QueryRow(ctx, absorbLockAndGapSQL).Scan(&seenAt, &gap); err != nil {
		_ = tx.Rollback(ctx)
		return Downtime{}, fmt.Errorf("scheduler: absorb downtime: lock liveness row: %w", err)
	}

	result := Downtime{Seeded: seenAt == nil}
	if seenAt != nil && gap.Valid {
		result.Gap = intervalDuration(gap)
		if result.Gap > l.threshold {
			tag, err := tx.Exec(ctx, absorbShiftSQL, gap)
			if err != nil {
				_ = tx.Rollback(ctx)
				return Downtime{}, fmt.Errorf("scheduler: absorb downtime: shift overdue rows: %w", err)
			}
			result.Shifted = int(tag.RowsAffected())
		}
	}

	if _, err := tx.Exec(ctx, absorbSeenSQL); err != nil {
		_ = tx.Rollback(ctx)
		return Downtime{}, fmt.Errorf("scheduler: absorb downtime: record seen_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return Downtime{}, fmt.Errorf("scheduler: absorb downtime: commit: %w", err)
	}
	return result, nil
}

// intervalDuration converts a Postgres interval into a time.Duration.
// now() - seen_at, the only interval this file ever scans, is a
// timestamptz difference: Postgres never populates such an interval's
// Months field, so only Days and Microseconds are read. An invalid
// (NULL) interval converts to zero.
func intervalDuration(iv pgtype.Interval) time.Duration {
	if !iv.Valid {
		return 0
	}
	return time.Duration(iv.Days)*24*time.Hour + time.Duration(iv.Microseconds)*time.Microsecond
}

// Refresh writes the current database instant as process_liveness's
// seen_at — one heartbeat write.
func (l *Liveness) Refresh(ctx context.Context) error {
	if _, err := l.pool.Exec(ctx, absorbSeenSQL); err != nil {
		return fmt.Errorf("scheduler: liveness refresh: %w", err)
	}
	return nil
}

// failureTolerance returns the number of consecutive failed Refresh
// calls Run tolerates before giving up — the point at which the stored
// instant has aged past threshold and a restart would begin shifting
// rows, rounded up from interval so a heartbeat that fails for less
// than threshold never trips it.
func failureTolerance(threshold, interval time.Duration) int {
	n := int64(threshold) / int64(interval)
	if int64(threshold)%int64(interval) != 0 {
		n++
	}
	if n < 1 {
		n = 1
	}
	return int(n)
}

// Run refreshes the persisted liveness instant at Interval until Stop is
// called, ctx is done, or the failure tolerance is spent. It returns nil
// when stopped — the drain's "completed" reading — and ctx.Err() when
// cancelled. A heartbeat that keeps failing is not tolerated forever:
// once failureTolerance(DowntimeThreshold, Interval) consecutive
// Refresh calls have failed, Run returns the last error, because a
// database this process cannot write to for that long could not have
// claimed a task or read an offset either — stopping is honest, and the
// next start's own AbsorbDowntime then measures the real gap. One
// successful Refresh resets the count.
func (l *Liveness) Run(ctx context.Context) error {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	tolerance := failureTolerance(l.threshold, l.interval)
	var consecutiveFailures int

	for {
		select {
		case <-l.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		select {
		case <-l.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		if err := l.Refresh(ctx); err != nil {
			consecutiveFailures++
			if consecutiveFailures >= tolerance {
				return err
			}
			continue
		}
		consecutiveFailures = 0
	}
}

// Stop makes Run return nil once the heartbeat in flight (if any)
// finishes, rather than waiting for ctx to be done — the drain's lever.
// Safe to call more than once and from any goroutine.
func (l *Liveness) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}
