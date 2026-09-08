package scheduler

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

// newScheduler builds a fresh, migrated schema-scoped pool for tb and
// closes it on cleanup — this package's own fixture, shaped like the
// ledger package's own newStore.
func newScheduler(tb testing.TB) *pgxpool.Pool {
	tb.Helper()

	ctx := context.Background()
	cfg := testdb.Schema(tb)

	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		tb.Fatalf("new pool: %v", err)
	}
	tb.Cleanup(pool.Close)

	if err := store.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		tb.Fatalf("migrate: %v", err)
	}

	return pool
}

// dueNow inserts req through reg.Schedule inside a committed transaction
// with zero delay, returning the new TaskID.
func dueNow(tb testing.TB, pool *pgxpool.Pool, reg *Registry, req Request) TaskID {
	tb.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		tb.Fatalf("begin: %v", err)
	}
	id, err := reg.Schedule(ctx, tx, req)
	if err != nil {
		_ = tx.Rollback(ctx)
		tb.Fatalf("schedule: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tb.Fatalf("commit: %v", err)
	}
	return id
}

// fixedOutcomeHandler is a Handler test double that always returns a
// fixed Outcome/error pair, optionally recording each Task it was
// handed.
type fixedOutcomeHandler struct {
	mu      sync.Mutex
	outcome Outcome
	err     error
	seen    []Task
}

func (h *fixedOutcomeHandler) Execute(_ context.Context, _ pgx.Tx, task Task) (Outcome, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seen = append(h.seen, task)
	return h.outcome, h.err
}

// recordingObserver collects every Observation and LoopObservation
// behind a mutex, so an assertion on the seam is exact and the collector
// is race-clean.
type recordingObserver struct {
	mu    sync.Mutex
	tasks []Observation
	loops []LoopObservation
}

func (o *recordingObserver) ObserveTask(obs Observation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tasks = append(o.tasks, obs)
}

func (o *recordingObserver) ObserveLoop(obs LoopObservation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.loops = append(o.loops, obs)
}

func (o *recordingObserver) Tasks() []Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Observation, len(o.tasks))
	copy(out, o.tasks)
	return out
}

func (o *recordingObserver) Loops() []LoopObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]LoopObservation, len(o.loops))
	copy(out, o.loops)
	return out
}
