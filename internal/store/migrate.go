package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ProcessLockID is the advisory-lock id both production callers of
// Migrate pass to WithAdvisoryLock: the composition root's start-up
// migration step, and the migrate-only entry point. It is goose's own
// published DefaultLockID — the id the library documents for exactly
// this purpose — rather than a private number, so any other
// goose-driven tooling ever pointed at the same database contends on
// the same lock instead of applying migrations beside this process
// under a lock nothing else knows about.
const ProcessLockID = lock.DefaultLockID

// MigrateOption configures Migrate's optional behaviour. The zero value
// of every option's effect is "off" — Migrate's default call shape
// (no options) is unchanged from before options existed, so every
// existing call site and test fixture keeps its current behaviour.
type MigrateOption func(*migrateOptions)

// migrateOptions holds the options WithAdvisoryLock and any future
// MigrateOption populate.
type migrateOptions struct {
	lockID   int64
	haveLock bool
}

// WithAdvisoryLock makes Migrate take a PostgreSQL session-level
// advisory lock, identified by id, for the duration of the apply — so
// two processes calling Migrate concurrently against the same database
// are serialised rather than racing to apply the same migration. It is
// opt-in, with a caller-supplied id, because the lock is
// database-wide while this package's own test isolation is
// schema-wide: an id shared by unrelated fixtures on one shared test
// server would serialise them behind one lock for no reason. Production
// callers pass ProcessLockID; a test that is itself about concurrent
// migration must pass an id of its own, never ProcessLockID, so it
// cannot collide with a neighbouring package's run on the same shared
// server.
func WithAdvisoryLock(id int64) MigrateOption {
	return func(o *migrateOptions) {
		o.lockID = id
		o.haveLock = true
	}
}

// Migrate applies every pending migration this package embeds to
// pool's database, forward-only (no -- +goose Down section exists in this
// package). It is safe to call more than once: a database already at the
// latest migration is left unchanged. logger must be non-nil (goose rejects
// a nil *slog.Logger); tests pass slog.New(slog.DiscardHandler). With no
// options, no lock is taken — the package's original behaviour, unchanged.
// With WithAdvisoryLock, the apply is serialised against any other caller
// holding the same lock id: the loser waits, then finds nothing pending and
// returns nil rather than an error.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts ...MigrateOption) (err error) {
	var o migrateOptions
	for _, opt := range opts {
		opt(&o)
	}

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: sub filesystem: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		err = errors.Join(err, db.Close())
	}()

	providerOpts := []goose.ProviderOption{goose.WithSlog(logger)}
	if o.haveLock {
		locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(o.lockID))
		if err != nil {
			return fmt.Errorf("migrate: new session locker: %w", err)
		}
		providerOpts = append(providerOpts, goose.WithSessionLocker(locker))
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub, providerOpts...)
	if err != nil {
		return fmt.Errorf("migrate: new provider: %w", err)
	}

	if _, err = provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}

	return nil
}

// HasPendingMigrations reports whether pool's database has any
// migration this package embeds still to apply. It deliberately
// consults no lock even when the caller would otherwise configure one
// through Migrate — goose's own Provider.HasPending documents that it
// never blocks on, or is blocked by, a configured locker, which is
// exactly the property the auto-apply-off start-up refusal needs: the
// check itself never waits behind a concurrent apply. logger must be
// non-nil, matching Migrate.
func HasPendingMigrations(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (pending bool, err error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return false, fmt.Errorf("has pending migrations: sub filesystem: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		err = errors.Join(err, db.Close())
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSlog(logger))
	if err != nil {
		return false, fmt.Errorf("has pending migrations: new provider: %w", err)
	}

	pending, err = provider.HasPending(ctx)
	if err != nil {
		return false, fmt.Errorf("has pending migrations: %w", err)
	}
	return pending, nil
}
