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
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every pending migration in internal/store/migrations to
// pool's database, forward-only (no -- +goose Down section exists in this
// package). It is safe to call more than once: a database already at the
// latest migration is left unchanged. logger must be non-nil (goose rejects
// a nil *slog.Logger); tests pass slog.New(slog.DiscardHandler).
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (err error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: sub filesystem: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		err = errors.Join(err, db.Close())
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSlog(logger))
	if err != nil {
		return fmt.Errorf("migrate: new provider: %w", err)
	}

	if _, err = provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}

	return nil
}
