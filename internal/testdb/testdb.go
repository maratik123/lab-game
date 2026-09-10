// Package testdb provisions a PostgreSQL instance for package tests that
// exercise real database behaviour — Postgres is tested against Postgres.
// It never skips: with neither the
// LAB_GAME_TEST_DSN environment variable nor a reachable container runtime,
// Main returns a non-zero exit code instead of letting tests report a false
// pass.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Image is the container image pulled when LAB_GAME_TEST_DSN is unset.
// Pinned to the major version only (never a patch tag) per the project's
// standing container-image policy.
const Image = "docker.io/library/postgres:18"

// DSNEnv is the environment variable that, when set, points the test suite
// at an existing PostgreSQL server instead of starting a container. It is a
// mechanism for CI, for a wrapper that provisions a shared server, or for a
// developer with a server already running — it must never be pointed at a
// local development instance in any committed file.
const DSNEnv = "LAB_GAME_TEST_DSN"

var (
	baseDSN     string
	schemaCount atomic.Uint64
)

// Main provisions a PostgreSQL server for the calling test binary and runs
// m.Run(), returning the exit code the caller's TestMain must pass to
// os.Exit. It never skips: if LAB_GAME_TEST_DSN is unset and no container
// runtime is reachable, it prints the error and returns 1 without running
// any test.
func Main(m *testing.M) int {
	ctx := context.Background()

	if dsn := os.Getenv(DSNEnv); dsn != "" {
		baseDSN = dsn
		return m.Run()
	}

	server, err := StartServer(ctx, ServerOptions{})
	if err != nil {
		// No prefix of its own: every error the provisioner returns already
		// names this package, and a second copy buries the cause the message
		// exists to surface.
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	baseDSN = server.DSN()

	code := m.Run()
	if err := server.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "testdb: terminating container: %v\n", err)
	}
	return code
}

// schemaMaxConns caps each per-test pool so 16 parallel subtests stay
// well under the server's connection limit (16 × 4 = 64 against
// max_connections = 100).
const schemaMaxConns = 4

// Schema creates a uniquely named schema on the provisioned server and
// returns a pgxpool.Config whose search_path targets it. It fails tb via
// tb.Fatalf when Main has not run (baseDSN unset) or the schema cannot be
// created, and registers a tb.Cleanup that drops the schema.
func Schema(tb testing.TB) *pgxpool.Config {
	tb.Helper()

	if baseDSN == "" {
		tb.Fatalf("testdb: Schema called before testdb.Main provisioned a database")
	}

	cfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		tb.Fatalf("testdb: parse base DSN: %v", err)
	}

	name := fmt.Sprintf("t_%d_%d", time.Now().UnixNano(), schemaCount.Add(1))

	quoted := pgx.Identifier{name}.Sanitize()

	// Schema's signature is fixed by design — Schema(tb) *pgxpool.Config,
	// no context parameter — so it necessarily starts its own background
	// context for the short-lived admin connection that creates the schema.
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		tb.Fatalf("testdb: connect to create schema: %v", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		tb.Fatalf("testdb: create schema %s: %v", name, err)
	}

	tb.Cleanup(func() {
		dctx := context.Background()
		dropper, err := pgxpool.NewWithConfig(dctx, cfg.Copy())
		if err != nil {
			tb.Errorf("testdb: connect to drop schema %s: %v", name, err)
			return
		}
		defer dropper.Close()
		if _, err := dropper.Exec(dctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			tb.Errorf("testdb: drop schema %s: %v", name, err)
		}
	})

	cfg.ConnConfig.RuntimeParams["search_path"] = name
	cfg.MaxConns = schemaMaxConns

	return cfg
}

// SchemaDSN creates a fresh schema exactly as Schema does — the same
// drop-on-cleanup registration — and returns it as a connection string
// rather than a *pgxpool.Config, for a caller outside this package (the
// composition root's own pool construction) that takes a DSN. The
// string carries both the search_path that isolates the schema and the
// pool_max_conns cap Schema already gives every schema-scoped pool, so
// a pool the caller builds from it lands inside the same
// connection-count arithmetic Schema's own callers do, rather than
// beside it.
func SchemaDSN(tb testing.TB) string {
	tb.Helper()

	cfg := Schema(tb)
	name := cfg.ConnConfig.RuntimeParams["search_path"]

	u, err := url.Parse(baseDSN)
	if err != nil {
		tb.Fatalf("testdb: parse base DSN: %v", err)
	}
	q := u.Query()
	q.Set("search_path", name)
	q.Set("pool_max_conns", strconv.Itoa(schemaMaxConns))
	u.RawQuery = q.Encode()
	return u.String()
}
