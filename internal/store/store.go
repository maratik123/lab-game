// Package store implements the quantitative ledger's write path: the
// owner/scope/account catalog, its forward migrations, and Post, the only
// function that moves a balance. It implements the game's double-entry
// accounting design against a single global account; nothing here
// redesigns it.
package store

import (
	"context"
	"fmt"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool builds a connection pool whose connections decode PostgreSQL
// numeric values into shopspring/decimal.Decimal. cfg must come from
// pgxpool.ParseConfig — pgxpool itself panics on a hand-built config with a
// nil ConnConfig, so every caller in this codebase satisfies that
// precondition by construction.
func NewPool(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	prior := cfg.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		if prior != nil {
			return prior(ctx, conn)
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	return pool, nil
}
