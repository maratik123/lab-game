package main

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Readiness reasons — never rendered to a probe response or a scrape;
// the readiness-func contract the metrics and readiness listener
// consumes keeps the underlying error out of both.
var (
	errNotReadyMigrating = errors.New("migrating")
	errNotReadyDraining  = errors.New("draining")
	errNotReadyDatabase  = errors.New("database unreachable")
)

// readiness is the one definition of "ready" this process exports
// through both the readiness path and the readiness gauge: two
// one-way latches — migrated, set by the migration start-up
// step, and draining, set as drain's first action — plus a Pool.Ping
// round trip. Readiness does not re-ask the database whether migrations
// are pending: after the migration step the answer is established by
// construction, so a set migrated latch is trusted rather than
// re-verified on every scrape.
type readiness struct {
	pool *pgxpool.Pool

	migrated atomic.Bool
	draining atomic.Bool
}

// newReadiness builds a readiness value over pool, with both latches
// unset.
func newReadiness(pool *pgxpool.Pool) *readiness {
	return &readiness{pool: pool}
}

// setMigrated latches migrated. One-way: never cleared.
func (r *readiness) setMigrated() {
	r.migrated.Store(true)
}

// setDraining latches draining. One-way: never cleared. Called as
// drain's first action, ahead of every runner.stop and every closer.
func (r *readiness) setDraining() {
	r.draining.Store(true)
}

// Ready is this process's one readiness-func implementation. Not ready
// with errNotReadyMigrating until the migration step has latched
// migrated; not ready with
// errNotReadyDraining once drain has begun — unconditionally, even when
// migrated and the pool still answers; otherwise a Pool.Ping round trip
// decides, reported as errNotReadyDatabase on failure.
func (r *readiness) Ready(ctx context.Context) error {
	if !r.migrated.Load() {
		return errNotReadyMigrating
	}
	if r.draining.Load() {
		return errNotReadyDraining
	}
	if err := r.pool.Ping(ctx); err != nil {
		return errNotReadyDatabase
	}
	return nil
}
