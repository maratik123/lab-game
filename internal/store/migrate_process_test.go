package store

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/testdb"
)

// newUnmigratedStore builds a pool over a fresh, empty schema — no
// migration applied — for a test that is itself about the migration
// step (HasPendingMigrations before Migrate, or Migrate's own locking).
func newUnmigratedStore(tb testing.TB) *pgxpool.Pool {
	tb.Helper()
	ctx := context.Background()
	cfg := testdb.Schema(tb)
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		tb.Fatalf("new pool: %v", err)
	}
	tb.Cleanup(pool.Close)
	return pool
}

// TestMigrate_ProcessLivenessSeededWithNullInstant asserts the migration's
// own seed: the singleton row exists with a NULL instant — the
// never-run-before state AbsorbDowntime's first read must distinguish
// from an ordinary short gap.
func TestMigrate_ProcessLivenessSeededWithNullInstant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := newStore(t)

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM process_liveness`).Scan(&count); err != nil {
		t.Fatalf("count process_liveness: %v", err)
	}
	if count != 1 {
		t.Fatalf("process_liveness rows = %d, want exactly 1 (the seeded singleton)", count)
	}

	var seenAt *string
	if err := pool.QueryRow(ctx, `SELECT seen_at::text FROM process_liveness WHERE id = 1`).Scan(&seenAt); err != nil {
		t.Fatalf("read seeded seen_at: %v", err)
	}
	if seenAt != nil {
		t.Fatalf("seeded seen_at = %v, want NULL (never run before)", *seenAt)
	}
}

// TestSchema_ProcessLivenessSecondRowRefused pins the singleton CHECK,
// the same shape ingest_offset's own singleton test already pins.
func TestSchema_ProcessLivenessSecondRowRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := newStore(t)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	_, err = tx.Exec(ctx, `INSERT INTO process_liveness (id, seen_at) VALUES (2, now())`)
	sqlstate(t, err, "23514", "process_liveness_singleton")
}

// TestHasPendingMigrations_TrueThenFalse asserts HasPendingMigrations
// reports true against a fresh, unmigrated schema and false once Migrate
// has applied everything — the answer the auto-apply-off refusal and the
// "nothing pending, start normally" path both read.
func TestHasPendingMigrations_TrueThenFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := newUnmigratedStore(t)
	logger := slog.New(slog.DiscardHandler)

	pending, err := HasPendingMigrations(ctx, pool, logger)
	if err != nil {
		t.Fatalf("HasPendingMigrations (fresh schema): %v", err)
	}
	if !pending {
		t.Fatalf("HasPendingMigrations (fresh schema) = false, want true")
	}

	if err := Migrate(ctx, pool, logger); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	pending, err = HasPendingMigrations(ctx, pool, logger)
	if err != nil {
		t.Fatalf("HasPendingMigrations (migrated schema): %v", err)
	}
	if pending {
		t.Fatalf("HasPendingMigrations (migrated schema) = true, want false")
	}
}

// TestHasPendingMigrations_IgnoresConfiguredLock proves HasPendingMigrations
// never waits behind Migrate's own advisory lock: it is called while a
// held lock (taken by a live transaction on a second connection) would
// block any statement that tried to acquire the same session-level lock,
// and it still returns promptly.
func TestHasPendingMigrations_IgnoresConfiguredLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := testdb.Schema(t)

	holder, err := NewPool(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("new pool (holder): %v", err)
	}
	defer holder.Close()

	const testLockID = 918273645

	tx, err := holder.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_lock($1)`, testLockID); err != nil {
		t.Fatalf("pg_advisory_lock: %v", err)
	}

	checker, err := NewPool(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("new pool (checker): %v", err)
	}
	defer checker.Close()

	pending, err := HasPendingMigrations(ctx, checker, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("HasPendingMigrations while a lock is held: %v", err)
	}
	if !pending {
		t.Fatalf("HasPendingMigrations while a lock is held = false, want true (fresh schema)")
	}
}

// TestMigrate_ConcurrentApplyUnderSameLockIDAppliesOnce drives two
// concurrent Migrate calls under the same caller-supplied lock id
// against the same schema: both must return nil, and the migration set
// applies exactly once — the loser finds nothing pending and returns
// nil rather than an error. A per-test lock id is used, never
// ProcessLockID, whose whole point is that it is database-wide and must
// never be taken by an ordinary fixture.
func TestMigrate_ConcurrentApplyUnderSameLockIDAppliesOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := testdb.Schema(t)
	const testLockID = 728394651

	pool1, err := NewPool(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("new pool 1: %v", err)
	}
	defer pool1.Close()
	pool2, err := NewPool(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("new pool 2: %v", err)
	}
	defer pool2.Close()

	logger := slog.New(slog.DiscardHandler)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = Migrate(ctx, pool1, logger, WithAdvisoryLock(testLockID))
	}()
	go func() {
		defer wg.Done()
		errs[1] = Migrate(ctx, pool2, logger, WithAdvisoryLock(testLockID))
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Migrate call %d: %v", i, err)
		}
	}

	var count int
	if err := pool1.QueryRow(ctx, `SELECT count(*) FROM goose_db_version`).Scan(&count); err != nil {
		t.Fatalf("count goose_db_version: %v", err)
	}
	if count != 11 {
		t.Fatalf("goose_db_version rows = %d, want 11 (the migration set applied exactly once)", count)
	}
}
