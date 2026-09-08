package ingest

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

// newIngestPool builds a fresh, migrated schema-scoped pool for tb and
// closes it on cleanup — this package's fixture, shaped like the same
// helper in the task-scheduler and ledger packages' own test suites.
func newIngestPool(tb testing.TB) *pgxpool.Pool {
	tb.Helper()

	ctx := context.Background()
	cfg := testdb.Schema(tb)

	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		tb.Fatalf("new pool: %v", err)
	}
	tb.Cleanup(pool.Close)

	if err := store.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		tb.Fatalf("migrate: %v", err)
	}

	return pool
}
