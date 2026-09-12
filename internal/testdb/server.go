package testdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

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
	// perClientMountMB is the mount cap this package grants per concurrent
	// client: the value this package has granted a single client all along,
	// kept byte-for-byte so the one-client default is unchanged. It has to
	// hold one client's peak cluster size under sustained load, plus up to
	// two checkpoint intervals of write-ahead log beside it.
	perClientMountMB = 512
	// walRetentionMB is the server's max_wal_size, in the same const block
	// as perClientMountMB because the two numbers are one decision, not two:
	// max_wal_size bounds how much WAL the server lets accumulate before a
	// checkpoint recycles it, and that WAL lives on the same fixed-size
	// tmpfs as the cluster itself. A retention target picked without regard
	// to the mount cap can let sustained write load fill the mount out from
	// under the cluster, however small the cluster is — measured, the
	// server then PANICs, goes into recovery and exits, and every
	// connection open at that instant dies mid-statement. 64 MB leaves a
	// one-client mount enough headroom for the cluster's own measured size
	// at up to two checkpoint intervals of WAL, and the mount grows with the
	// client count while this retention target stays fixed, which only
	// widens that headroom for every additional client.
	walRetentionMB = 64
)

// MountOptions returns the tmpfs mount-option string sized for clients
// concurrent clients, at perClientMountMB each. A count below one sizes for
// one client rather than zero, so the zero value of a caller's options
// still reproduces a usable mount.
func MountOptions(clients int) string {
	if clients < 1 {
		clients = 1
	}
	return fmt.Sprintf("rw,size=%dm", clients*perClientMountMB)
}

const (
	dbName = "labgame_test"
	dbUser = "labgame"
	dbPass = "labgame"
)

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
// before the same non-zero exit the caller already returns.
const (
	startAttempts   = 3
	startRetryDelay = 400 * time.Millisecond
)

// terminateTimeout bounds a Server's Stop call. Ryuk remains the crash
// safety net for a process that is killed outright.
const terminateTimeout = 30 * time.Second

// ServerOptions configures StartServer. The zero value reproduces the
// fallback container path exactly: an anonymous container — only this
// invocation's session can find it, which is what makes it the reaper's to
// remove — at the image's own default connection ceiling.
type ServerOptions struct {
	// ContainerName names the container so a later invocation's locator can
	// find it, and reuses one already running under that name instead of
	// creating a second. Empty starts an anonymous container.
	ContainerName string
	// ConnCeiling overrides the image's default max_connections. Zero keeps
	// the image default.
	ConnCeiling int
	// Clients is the number of concurrent clients the provisioned server's
	// PGDATA mount must hold at once. Zero sizes the mount for one client,
	// the same as the fallback container path provisions.
	Clients int
}

// Server is a provisioned PostgreSQL server: its connection string, and a
// Stop method that terminates the underlying container.
type Server struct {
	dsn       string
	container *postgres.PostgresContainer
}

// DSN returns the server's connection string.
func (s *Server) DSN() string {
	return s.dsn
}

// Stop terminates the server's container, bounded by terminateTimeout. It
// is safe to call on a nil *Server, which happens for nothing to do.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.container == nil {
		return nil
	}
	tctx, cancel := context.WithTimeout(ctx, terminateTimeout)
	defer cancel()
	return s.container.Terminate(tctx)
}

// StartServer provisions a PostgreSQL container using this project's image,
// tmpfs and wait-strategy settings, applying opts. The zero ServerOptions
// reproduces Main's container branch exactly: same image, same tmpfs, same
// wait strategy, same start retry, same connection ceiling. It retries the
// start per the schedule startAttempts and startRetryDelay describe.
func StartServer(ctx context.Context, opts ServerOptions) (*Server, error) {
	pgContainer, err := retryRun(startAttempts, startRetryDelay, func() (*postgres.PostgresContainer, error) {
		moduleOpts := []testcontainers.ContainerCustomizer{
			postgres.WithDatabase(dbName),
			postgres.WithUsername(dbUser),
			postgres.WithPassword(dbPass),
			postgres.BasicWaitStrategies(),
			testcontainers.WithTmpfs(map[string]string{tmpfsDir: MountOptions(opts.Clients)}),
			testcontainers.WithEnv(map[string]string{
				"PGDATA":               tmpfsPGDATA,
				"POSTGRES_INITDB_ARGS": "--no-sync",
			}),
			testcontainers.WithCmdArgs("-c", fmt.Sprintf("max_wal_size=%dMB", walRetentionMB)),
		}
		if opts.ContainerName != "" {
			moduleOpts = append(moduleOpts, testcontainers.WithReuseByName(opts.ContainerName))
		}
		if opts.ConnCeiling > 0 {
			moduleOpts = append(moduleOpts, testcontainers.WithCmdArgs("-c", fmt.Sprintf("max_connections=%d", opts.ConnCeiling)))
		}
		return postgres.Run(ctx, Image, moduleOpts...)
	})
	if err != nil {
		return nil, fmt.Errorf("testdb: starting %s: %w", Image, err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("testdb: connection string: %w", errors.Join(err, testcontainers.TerminateContainer(pgContainer)))
	}

	return &Server{dsn: dsn, container: pgContainer}, nil
}

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

// Probe opens the connection a caller would have opened anyway and reads
// the server's own max_connections setting, so one round trip answers both
// "does this server accept connections" and "how many can it admit". A
// syntactically invalid dsn returns an error without dialling.
func Probe(ctx context.Context, dsn string) (int, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return 0, fmt.Errorf("testdb: probe connect: %w", err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	var maxConns int
	if err := conn.QueryRow(ctx, "SELECT setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&maxConns); err != nil {
		return 0, fmt.Errorf("testdb: probe max_connections: %w", err)
	}
	return maxConns, nil
}

// Binaries is the number of test binaries in this module that provision a
// database through Main, and therefore can run concurrently against a
// shared server under go test ./...'s default binary parallelism (`go help
// build`'s -p). A new database-backed package must update this constant; a
// manifest test keeps it honest against the tree rather than against
// memory.
const Binaries = 5

const (
	// imageDefaultCeiling is Image's own max_connections default — the floor
	// below which a computed ceiling would only give back capacity the
	// server already offers.
	imageDefaultCeiling = 100
	// ceilingMax is the largest ceiling this project will ask a container to
	// start with, measured against Image with this project's tmpfs and
	// fsync settings: `podman run` with `-c max_connections=1000` starts and
	// serves on those settings.
	ceilingMax = 1000
	// ceilingSlack covers the server's own reserved connection slots
	// (superuser_reserved_connections, measured at 3 on Image) plus the one
	// test that raises its pool cap above schemaMaxConns.
	ceilingSlack = 32
)

// Ceiling computes the connection ceiling a provisioned server needs to
// admit clients concurrent whole-module test runs, each running Binaries
// database-backed test binaries at up to parallel parallel tests, against
// the per-pool cap Schema sets:
//
//	ceiling = clients × Binaries × parallel × (schemaMaxConns + 1) + ceilingSlack
//
// The result is floored at imageDefaultCeiling — never returning less than
// the image's own default — and refuses (a non-nil error) above ceilingMax,
// naming the computed value, the terms it came from, and the flags that
// lower it, rather than silently granting less than the formula asks for.
// clients and parallel below 1 are treated as 1.
func Ceiling(clients, parallel int) (int, error) {
	if clients < 1 {
		clients = 1
	}
	if parallel < 1 {
		parallel = 1
	}

	computed := clients*Binaries*parallel*(schemaMaxConns+1) + ceilingSlack

	if computed > ceilingMax {
		return 0, fmt.Errorf(
			"testdb: computed connection ceiling %d (clients=%d, binaries=%d, parallel=%d, schemaMaxConns=%d, slack=%d) exceeds ceilingMax %d; lower --clients, --parallel, or the child's own -parallel",
			computed, clients, Binaries, parallel, schemaMaxConns, ceilingSlack, ceilingMax,
		)
	}
	if computed < imageDefaultCeiling {
		return imageDefaultCeiling, nil
	}
	return computed, nil
}
