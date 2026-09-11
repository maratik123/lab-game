package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestReadiness_NeitherLatchSet(t *testing.T) {
	t.Parallel()
	pool := newBotPool(t)
	r := newReadiness(pool)

	if err := r.Ready(context.Background()); !errors.Is(err, errNotReadyMigrating) {
		t.Errorf("Ready() = %v, want errNotReadyMigrating", err)
	}
}

func TestReadiness_MigratedNotDrainingPoolAnswers(t *testing.T) {
	t.Parallel()
	pool := newBotPool(t)
	r := newReadiness(pool)
	r.setMigrated()

	if err := r.Ready(context.Background()); err != nil {
		t.Errorf("Ready() = %v, want nil", err)
	}
}

func TestReadiness_Draining_EvenAfterMigrationAndPoolAnswers(t *testing.T) {
	t.Parallel()
	pool := newBotPool(t)
	r := newReadiness(pool)
	r.setMigrated()
	r.setDraining()

	if err := r.Ready(context.Background()); !errors.Is(err, errNotReadyDraining) {
		t.Errorf("Ready() = %v, want errNotReadyDraining", err)
	}
}

func TestReadiness_DrainingLatchIsOneWay(t *testing.T) {
	t.Parallel()
	pool := newBotPool(t)
	r := newReadiness(pool)
	r.setMigrated()
	r.setDraining()
	// setDraining a second time, and re-checking migrated, must not
	// un-latch draining.
	r.setMigrated()
	r.setDraining()

	if err := r.Ready(context.Background()); !errors.Is(err, errNotReadyDraining) {
		t.Errorf("Ready() = %v, want errNotReadyDraining (one-way latch)", err)
	}
}

func TestReadiness_MigratedPoolUnreachable(t *testing.T) {
	t.Parallel()
	pool := newBotPool(t)
	r := newReadiness(pool)
	r.setMigrated()
	pool.Close()

	if err := r.Ready(context.Background()); !errors.Is(err, errNotReadyDatabase) {
		t.Errorf("Ready() = %v, want errNotReadyDatabase", err)
	}
}

// newBotPool builds a fresh, migrated schema-scoped pool for tb, closed
// on cleanup.
func newBotPool(tb testing.TB) *pgxpool.Pool {
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
