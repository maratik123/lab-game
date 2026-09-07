// Package testdb provisions a PostgreSQL instance for package tests that
// exercise real database behaviour (AGENTS.md § Go Test Conventions —
// "Postgres is tested against Postgres"). It never skips: with neither the
// LAB_GAME_TEST_DSN environment variable nor a reachable container runtime,
// Main returns a non-zero exit code instead of letting tests report a false
// pass.
package testdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image is the container image pulled when LAB_GAME_TEST_DSN is unset.
// Pinned to the major version only (never a patch tag) per the project's
// standing container-image policy.
const Image = "docker.io/library/postgres:18"

// dsnEnv is the environment variable that, when set, points the test suite
// at an existing PostgreSQL server instead of starting a container. It is a
// mechanism for CI or a developer with a server already running — it must
// never be pointed at a local development instance in any committed file.
const dsnEnv = "LAB_GAME_TEST_DSN"

// The cluster lives in RAM and is never synced: a test database that
// survives a crash buys nothing, and both costs are startup costs.
// initdb's final sync of the fresh cluster is 6 of the 8 seconds a
// container needs before it accepts connections on this project's
// development machine, and the tmpfs keeps every later write off the
// container storage driver. Durability of the data itself is already
// gone: the postgres module runs the server with fsync=off.
const (
	// tmpfsDir is the in-RAM mount PGDATA is created under. PGDATA is a
	// subdirectory rather than the mount point itself so that the image's
	// entrypoint creates it with the 0700 ownership initdb demands.
	tmpfsDir = "/var/lib/postgresql/tmpfs"
	// tmpfsPGDATA overrides the image's on-disk PGDATA.
	tmpfsPGDATA = tmpfsDir + "/data"
	// tmpfsOptions caps the mount. Measured: a full run of every
	// database-backed package against one server leaves a 193 MB cluster.
	tmpfsOptions = "rw,size=512m"
)

const (
	dbName = "labgame_test"
	dbUser = "labgame"
	dbPass = "labgame"
)

var (
	baseDSN     string
	schemaCount atomic.Uint64
)

// container holds the running PostgreSQL container so Main can terminate it
// after the test binary's m.Run() returns. Nil when LAB_GAME_TEST_DSN was
// used instead.
var container *postgres.PostgresContainer

// Main provisions a PostgreSQL server for the calling test binary and runs
// m.Run(), returning the exit code the caller's TestMain must pass to
// os.Exit. It never skips: if LAB_GAME_TEST_DSN is unset and no container
// runtime is reachable, it prints the error and returns 1 without running
// any test.
func Main(m *testing.M) int {
	ctx := context.Background()

	if dsn := os.Getenv(dsnEnv); dsn != "" {
		baseDSN = dsn
		return m.Run()
	}

	pgContainer, err := retryRun(startAttempts, startRetryDelay, func() (*postgres.PostgresContainer, error) {
		return postgres.Run(ctx, Image,
			postgres.WithDatabase(dbName),
			postgres.WithUsername(dbUser),
			postgres.WithPassword(dbPass),
			postgres.BasicWaitStrategies(),
			testcontainers.WithTmpfs(map[string]string{tmpfsDir: tmpfsOptions}),
			testcontainers.WithEnv(map[string]string{
				"PGDATA":               tmpfsPGDATA,
				"POSTGRES_INITDB_ARGS": "--no-sync",
			}),
		)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "testdb: starting %s: %v\n", Image, err)
		return 1
	}
	container = pgContainer

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testdb: connection string: %v\n", err)
		terminate(ctx)
		return 1
	}
	baseDSN = dsn

	code := m.Run()
	terminate(ctx)
	return code
}

// startAttempts and startRetryDelay bound the retry around the container
// start. testcontainers shares ONE Ryuk reaper between the test binaries
// `go test ./...` runs in parallel, and a binary that finds that container
// in the window between "created" and "running" fails its whole package
// with `wait for reaper …: unexpected container status "created"` —
// measured at one run in six on this project's development machine, on the
// container settings above and on the ones that preceded them alike. A
// retry turns the race into a delay: by the next attempt the process that
// created the reaper has started it.
//
// The retry is deliberately not conditioned on the error text. A container
// that failed to start is transient by nature, upstream wraps that message
// through three layers, and the price of retrying an error that is NOT
// transient — no container runtime reachable at all — is under a second
// before the same non-zero exit Main already returns.
const (
	startAttempts   = 3
	startRetryDelay = 400 * time.Millisecond
)

// retryRun calls run up to attempts times, waiting delay between attempts,
// and returns the first success or the last error. A failed attempt that
// still produced a container is terminated before the next one: postgres.Run
// reports both when the container was created but never became usable.
//
// It refuses a non-positive attempts rather than returning a nil container
// with a nil error, which every caller would dereference.
func retryRun(attempts int, delay time.Duration, run func() (*postgres.PostgresContainer, error)) (*postgres.PostgresContainer, error) {
	if attempts < 1 {
		return nil, fmt.Errorf("testdb: retryRun needs at least one attempt, got %d", attempts)
	}

	var err error
	for attempt := range attempts {
		var ctr *postgres.PostgresContainer
		ctr, err = run()
		if err == nil {
			return ctr, nil
		}
		// TerminateContainer is nil-safe, and a failed attempt may still
		// hand back a container that was created but never became usable.
		err = errors.Join(err, testcontainers.TerminateContainer(ctr))
		if attempt < attempts-1 {
			time.Sleep(delay)
		}
	}
	return nil, err
}

const (
	// terminateTimeout bounds the container's Terminate call at the end of
	// a package's TestMain; Ryuk remains the crash safety net.
	terminateTimeout = 30 * time.Second
	// schemaMaxConns caps each per-test pool so 16 parallel subtests stay
	// well under the server's connection limit (design D11: 16 × 4 = 64
	// against max_connections = 100).
	schemaMaxConns = 4
)

func terminate(ctx context.Context) {
	if container == nil {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, terminateTimeout)
	defer cancel()
	if err := container.Terminate(tctx); err != nil {
		fmt.Fprintf(os.Stderr, "testdb: terminating container: %v\n", err)
	}
}

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

	// Schema's signature is fixed by design (D11: Schema(tb) *pgxpool.Config,
	// no context parameter) — it necessarily starts its own background
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
