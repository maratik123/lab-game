package main

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
)

func TestMigrateOnly_AppliesPendingMigrations(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)

	var stderr bytes.Buffer
	code := migrateOnly(context.Background(), assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("migrateOnly() = %d, stderr = %q", code, stderr.String())
	}

	pool := connectToBotDSN(t, env["LAB_GAME_DSN"])
	pending, err := store.HasPendingMigrations(context.Background(), pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("HasPendingMigrations: %v", err)
	}
	if pending {
		t.Error("HasPendingMigrations = true after migrateOnly, want false")
	}
}

func TestMigrateOnly_SecondInvocationIsANoOp(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)

	for i := range 2 {
		var stderr bytes.Buffer
		code := migrateOnly(context.Background(), assembleOptions{
			Lookup: mapLookup(env),
			Stderr: &stderr,
		})
		if code != 0 {
			t.Fatalf("migrateOnly() call %d = %d, stderr = %q", i+1, code, stderr.String())
		}
	}
}

func TestMigrateOnly_UnreachableDatabase(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)
	env["LAB_GAME_DSN"] = "postgres://user:pass@127.0.0.1:1/nosuchdb"

	var stderr bytes.Buffer
	code := migrateOnly(context.Background(), assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if code == 0 {
		t.Error("migrateOnly() = 0, want non-zero")
	}
}

// connectToBotDSN opens a pool against dsn, closed on cleanup — a
// second connection to the same schema migrateOnly already migrated,
// used only to read state back for assertions.
func connectToBotDSN(tb testing.TB, dsn string) *pgxpool.Pool {
	tb.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		tb.Fatalf("parse DSN: %v", err)
	}
	pool, err := store.NewPool(context.Background(), cfg)
	if err != nil {
		tb.Fatalf("new pool: %v", err)
	}
	tb.Cleanup(pool.Close)
	return pool
}
