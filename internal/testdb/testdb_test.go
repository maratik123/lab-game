package testdb_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

func TestSchema_isolated_and_cleaned_up(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cfg1 := testdb.Schema(t)
	pool1, err := pgxpool.NewWithConfig(ctx, cfg1)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool1.Close()

	var name1 string
	if err := pool1.QueryRow(ctx, "SELECT current_schema()").Scan(&name1); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if name1 == "" || name1 == "public" {
		t.Fatalf("expected a dedicated schema, got %q", name1)
	}

	cfg2 := testdb.Schema(t)
	pool2, err := pgxpool.NewWithConfig(ctx, cfg2)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool2.Close()

	var name2 string
	if err := pool2.QueryRow(ctx, "SELECT current_schema()").Scan(&name2); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if name2 == name1 {
		t.Fatalf("expected distinct schema names, got %q twice", name1)
	}
}

func TestSchema_dropped_after_cleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	adminCfg := testdb.Schema(t)
	pool, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	var childName string
	t.Run("child", func(t *testing.T) {
		cfg := testdb.Schema(t) //nolint:contextcheck // testdb.Schema takes no context parameter by design; the closure's later use of the outer ctx is unrelated
		childName = cfg.ConnConfig.RuntimeParams["search_path"]

		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", childName,
		).Scan(&exists); err != nil {
			t.Fatalf("check schema exists: %v", err)
		}
		if !exists {
			t.Fatalf("schema %s should exist while the subtest is running", childName)
		}
	})
	// The child subtest's t.Cleanup has already run by this point.

	var exists bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", childName,
	).Scan(&exists); err != nil {
		t.Fatalf("check schema dropped: %v", err)
	}
	if exists {
		t.Fatalf("schema %s should have been dropped by cleanup", childName)
	}
}
