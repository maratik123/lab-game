package testdb_test

import (
	"context"
	"net/url"
	"os"
	"strconv"
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

// TestSchemaDSN_ParsesAndCarriesTheSchemaAndPoolCap asserts the returned
// string parses as a connection string, names a fresh (non-"public")
// schema in its search_path, and carries the same per-test pool cap
// Schema's own *pgxpool.Config gives — the ceiling arithmetic a
// consumer's own pool must land inside rather than beside.
func TestSchemaDSN_ParsesAndCarriesTheSchemaAndPoolCap(t *testing.T) {
	t.Parallel()

	dsn := testdb.SchemaDSN(t)

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", dsn, err)
	}
	schemaName := u.Query().Get("search_path")
	if schemaName == "" || schemaName == "public" {
		t.Fatalf("search_path = %q, want a dedicated schema name", schemaName)
	}

	wantMaxConns := testdb.Schema(t).MaxConns
	gotMaxConns, err := strconv.Atoi(u.Query().Get("pool_max_conns"))
	if err != nil {
		t.Fatalf("pool_max_conns %q does not parse as an int: %v", u.Query().Get("pool_max_conns"), err)
	}
	if int32(gotMaxConns) != wantMaxConns {
		t.Errorf("pool_max_conns = %d, want %d (Schema's own MaxConns)", gotMaxConns, wantMaxConns)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig(%q): %v", dsn, err)
	}
	if cfg.MaxConns != wantMaxConns {
		t.Errorf("parsed config MaxConns = %d, want %d", cfg.MaxConns, wantMaxConns)
	}
	if cfg.ConnConfig.RuntimeParams["search_path"] != schemaName {
		t.Errorf("parsed config search_path = %q, want %q", cfg.ConnConfig.RuntimeParams["search_path"], schemaName)
	}
}

// TestSchemaDSN_PoolLandsInTheSchemaAndCleanupDrops proves the returned
// DSN is directly usable: a pool opened from it lands in the fresh
// schema, and the same registered cleanup Schema uses drops it.
func TestSchemaDSN_PoolLandsInTheSchemaAndCleanupDrops(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	adminCfg := testdb.Schema(t)
	adminPool, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("connect (admin): %v", err)
	}
	t.Cleanup(adminPool.Close)

	var childName string
	t.Run("child", func(t *testing.T) {
		dsn := testdb.SchemaDSN(t) //nolint:contextcheck // testdb.SchemaDSN takes no context parameter by design; the closure's later use of the outer ctx is unrelated
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatalf("pgxpool.ParseConfig: %v", err)
		}
		childName = cfg.ConnConfig.RuntimeParams["search_path"]

		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatalf("connect from SchemaDSN: %v", err)
		}
		defer pool.Close()

		var current string
		if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&current); err != nil {
			t.Fatalf("current_schema: %v", err)
		}
		if current != childName {
			t.Fatalf("current_schema() = %q, want %q", current, childName)
		}
	})
	// The child subtest's t.Cleanup has already run by this point.

	var exists bool
	if err := adminPool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", childName,
	).Scan(&exists); err != nil {
		t.Fatalf("check schema dropped: %v", err)
	}
	if exists {
		t.Fatalf("schema %s should have been dropped by cleanup", childName)
	}
}
