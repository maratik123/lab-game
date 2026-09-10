package main

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/store"
)

// migrateOnly assembles just enough to apply this process's pending
// migrations under the same production advisory-lock id the serving
// path's own migration step uses, and returns the process exit code —
// no signal registration (a one-shot command has no serve to buffer
// one for; the default disposition is the right answer here), no
// readiness/metrics listener, no Telegram client, no ingest loop, no
// scheduler worker, no canary. A second invocation against an
// already-migrated database is a no-op: applying is safe to call more
// than once.
func migrateOnly(ctx context.Context, opts assembleOptions) int {
	cfg, err := config.Load(opts.Lookup)
	if err != nil {
		logStep(opts.Stderr, "configuration", err)
		return 1
	}
	logger := slog.New(slog.NewTextHandler(opts.Stderr, nil))

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN.Reveal())
	if err != nil {
		logStep(opts.Stderr, "database", err)
		return 1
	}
	pool, err := store.NewPool(ctx, poolCfg)
	if err != nil {
		logStep(opts.Stderr, "database", err)
		return 1
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		logStep(opts.Stderr, "database", err)
		return 1
	}

	if err := store.Migrate(ctx, pool, logger, store.WithAdvisoryLock(store.ProcessLockID)); err != nil {
		logStep(opts.Stderr, "migrations", err)
		return 1
	}
	return 0
}
