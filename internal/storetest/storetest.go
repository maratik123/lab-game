// Package storetest provides a fresh, migrated, schema-scoped connection
// pool for a test in another package of this module. Only tests import it.
package storetest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/testdb"
)

// Pool builds a fresh, migrated schema-scoped pool for tb and closes it on
// tb's cleanup.
func Pool(tb testing.TB) *pgxpool.Pool {
	tb.Helper()

	ctx := context.Background() //nolint:forbidigo // this is Pool's own root: its signature is fixed by design with no context parameter, and it owns the pool and migration calls until it returns
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
