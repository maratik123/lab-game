package store

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

// newStore builds a fresh, migrated schema-scoped pool for tb and closes it
// on cleanup.
func newStore(tb testing.TB) *pgxpool.Pool {
	tb.Helper()

	ctx := context.Background()
	cfg := testdb.Schema(tb)

	pool, err := NewPool(ctx, cfg)
	if err != nil {
		tb.Fatalf("new pool: %v", err)
	}
	tb.Cleanup(pool.Close)

	if err := Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		tb.Fatalf("migrate: %v", err)
	}

	return pool
}

// newStoreWithRecorder builds a fresh, migrated schema-scoped pool for tb
// with a queryRecorder attached to every connection (Test Design's
// "Recorder rule": it also records Migrate, CreateOwner and funding, so
// every tracer-based assertion calls rec.Reset() immediately before the
// Post under test).
func newStoreWithRecorder(tb testing.TB) (*pgxpool.Pool, *queryRecorder) {
	tb.Helper()

	ctx := context.Background()
	cfg := testdb.Schema(tb)
	rec := &queryRecorder{}
	cfg.ConnConfig.Tracer = rec

	pool, err := NewPool(ctx, cfg)
	if err != nil {
		tb.Fatalf("new pool: %v", err)
	}
	tb.Cleanup(pool.Close)

	if err := Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		tb.Fatalf("migrate: %v", err)
	}

	return pool, rec
}
