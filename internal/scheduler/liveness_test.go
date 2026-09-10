package scheduler

import (
	"context"
	"errors"
	"go/ast"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/testdb"
)

// newLiveness builds a Liveness over pool with the given Interval and
// DowntimeThreshold, failing tb on a rejected option.
func newLiveness(tb testing.TB, pool *pgxpool.Pool, interval, threshold time.Duration) *Liveness {
	tb.Helper()
	l, err := NewLiveness(LivenessOptions{Pool: pool, Interval: interval, DowntimeThreshold: threshold})
	if err != nil {
		tb.Fatalf("NewLiveness: %v", err)
	}
	return l
}

// insertScheduledTask inserts one scheduled_task row with an explicit
// run_at expressed as a SQL expression relative to now() (never a Go
// instant, so the fixture stays honest about "the database's own
// clock"), returning its id.
func insertScheduledTask(tb testing.TB, pool *pgxpool.Pool, taskType, runAtExpr, state string) int64 {
	tb.Helper()
	ctx := context.Background()
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_task (type, payload, run_at, state) VALUES ($1, '{}'::jsonb, `+runAtExpr+`, $2) RETURNING id`,
		taskType, state,
	).Scan(&id)
	if err != nil {
		tb.Fatalf("insert scheduled_task: %v", err)
	}
	return id
}

// scheduledTaskRunAt reads back id's current run_at.
func scheduledTaskRunAt(tb testing.TB, pool *pgxpool.Pool, id int64) time.Time {
	tb.Helper()
	ctx := context.Background()
	var runAt time.Time
	if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE id = $1`, id).Scan(&runAt); err != nil {
		tb.Fatalf("read run_at for %d: %v", id, err)
	}
	return runAt
}

// setLivenessSeenAt sets the singleton liveness row's seen_at through a
// SQL expression relative to now(), never a Go instant.
func setLivenessSeenAt(tb testing.TB, pool *pgxpool.Pool, seenAtExpr string) {
	tb.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE process_liveness SET seen_at = `+seenAtExpr+` WHERE id = 1`); err != nil {
		tb.Fatalf("set process_liveness.seen_at: %v", err)
	}
}

func ledgerRowCounts(tb testing.TB, pool *pgxpool.Pool) map[string]int {
	tb.Helper()
	ctx := context.Background()
	counts := map[string]int{}
	for _, table := range []string{"deferred_task", "recurrent_task", "journal_entry", "posting"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			tb.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// TestAbsorbDowntime_NoStoredInstantSeedsAndMovesNothing covers the
// never-run-before state: the fresh migration seeds seen_at NULL.
func TestAbsorbDowntime_NoStoredInstantSeedsAndMovesNothing(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l := newLiveness(t, pool, time.Second, 5*time.Minute)

	id := insertScheduledTask(t, pool, "t", "now() - interval '1 hour'", "pending")
	before := scheduledTaskRunAt(t, pool, id)

	dt, err := l.AbsorbDowntime(context.Background())
	if err != nil {
		t.Fatalf("AbsorbDowntime: %v", err)
	}
	if !dt.Seeded {
		t.Errorf("Seeded = false, want true")
	}
	if dt.Shifted != 0 {
		t.Errorf("Shifted = %d, want 0", dt.Shifted)
	}
	if dt.Gap != 0 {
		t.Errorf("Gap = %v, want 0", dt.Gap)
	}

	after := scheduledTaskRunAt(t, pool, id)
	if !before.Equal(after) {
		t.Errorf("run_at changed on a seeding call: before %v, after %v", before, after)
	}

	var seenAt *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT seen_at FROM process_liveness WHERE id = 1`).Scan(&seenAt); err != nil {
		t.Fatalf("read seen_at: %v", err)
	}
	if seenAt == nil {
		t.Fatalf("seen_at still NULL after AbsorbDowntime")
	}
}

// TestAbsorbDowntime_OverdueRowsShiftedByTheGap covers the shift itself:
// a stale instant, two overdue rows and one future row — exactly the
// two overdue rows move.
func TestAbsorbDowntime_OverdueRowsShiftedByTheGap(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l := newLiveness(t, pool, time.Second, time.Minute)

	setLivenessSeenAt(t, pool, "now() - interval '1 hour'")

	overdue1 := insertScheduledTask(t, pool, "t", "now() - interval '10 minutes'", "pending")
	overdue2 := insertScheduledTask(t, pool, "t", "now() - interval '5 minutes'", "pending")
	future := insertScheduledTask(t, pool, "t", "now() + interval '1 hour'", "pending")

	before1 := scheduledTaskRunAt(t, pool, overdue1)
	before2 := scheduledTaskRunAt(t, pool, overdue2)
	beforeF := scheduledTaskRunAt(t, pool, future)

	dt, err := l.AbsorbDowntime(context.Background())
	if err != nil {
		t.Fatalf("AbsorbDowntime: %v", err)
	}
	if dt.Seeded {
		t.Errorf("Seeded = true, want false")
	}
	if dt.Shifted != 2 {
		t.Errorf("Shifted = %d, want 2", dt.Shifted)
	}
	if dt.Gap < 55*time.Minute || dt.Gap > 65*time.Minute {
		t.Errorf("Gap = %v, want roughly 1 hour", dt.Gap)
	}

	after1 := scheduledTaskRunAt(t, pool, overdue1)
	after2 := scheduledTaskRunAt(t, pool, overdue2)
	afterF := scheduledTaskRunAt(t, pool, future)

	if got, want := after1.Sub(before1), dt.Gap; absDuration(got-want) > time.Second {
		t.Errorf("overdue1 shifted by %v, want %v", got, want)
	}
	if got, want := after2.Sub(before2), dt.Gap; absDuration(got-want) > time.Second {
		t.Errorf("overdue2 shifted by %v, want %v", got, want)
	}
	if !afterF.Equal(beforeF) {
		t.Errorf("future row's run_at changed: before %v, after %v", beforeF, afterF)
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// TestAbsorbDowntime_DeadAndFutureRowsUntouched pins the predicate:
// pending-and-overdue only. A dead row, however overdue, and a pending
// row due in the future are both left alone.
func TestAbsorbDowntime_DeadAndFutureRowsUntouched(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l := newLiveness(t, pool, time.Second, time.Minute)

	setLivenessSeenAt(t, pool, "now() - interval '1 hour'")

	dead := insertScheduledTask(t, pool, "t", "now() - interval '10 minutes'", "dead")
	future := insertScheduledTask(t, pool, "t", "now() + interval '1 hour'", "pending")
	beforeDead := scheduledTaskRunAt(t, pool, dead)
	beforeFuture := scheduledTaskRunAt(t, pool, future)

	dt, err := l.AbsorbDowntime(context.Background())
	if err != nil {
		t.Fatalf("AbsorbDowntime: %v", err)
	}
	if dt.Shifted != 0 {
		t.Errorf("Shifted = %d, want 0 (no pending-and-overdue row exists)", dt.Shifted)
	}
	if got := scheduledTaskRunAt(t, pool, dead); !got.Equal(beforeDead) {
		t.Errorf("dead row's run_at changed: before %v, after %v", beforeDead, got)
	}
	if got := scheduledTaskRunAt(t, pool, future); !got.Equal(beforeFuture) {
		t.Errorf("future row's run_at changed: before %v, after %v", beforeFuture, got)
	}
}

// TestAbsorbDowntime_WithinThresholdMovesNothing covers the second,
// ordinary-restart case: a gap at or below the threshold leaves every
// row alone.
func TestAbsorbDowntime_WithinThresholdMovesNothing(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l := newLiveness(t, pool, time.Second, time.Hour)

	setLivenessSeenAt(t, pool, "now() - interval '1 minute'")
	id := insertScheduledTask(t, pool, "t", "now() - interval '10 seconds'", "pending")
	before := scheduledTaskRunAt(t, pool, id)

	dt, err := l.AbsorbDowntime(context.Background())
	if err != nil {
		t.Fatalf("AbsorbDowntime: %v", err)
	}
	if dt.Shifted != 0 {
		t.Errorf("Shifted = %d, want 0", dt.Shifted)
	}
	if got := scheduledTaskRunAt(t, pool, id); !got.Equal(before) {
		t.Errorf("run_at changed within threshold: before %v, after %v", before, got)
	}
}

// TestAbsorbDowntime_RunTwiceSecondCallMovesNothing: the first call
// refreshes the instant, so a second call immediately after measures a
// gap at or below the threshold.
func TestAbsorbDowntime_RunTwiceSecondCallMovesNothing(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l := newLiveness(t, pool, time.Second, time.Minute)

	setLivenessSeenAt(t, pool, "now() - interval '1 hour'")
	id := insertScheduledTask(t, pool, "t", "now() - interval '10 minutes'", "pending")

	if _, err := l.AbsorbDowntime(context.Background()); err != nil {
		t.Fatalf("first AbsorbDowntime: %v", err)
	}
	shifted := scheduledTaskRunAt(t, pool, id)

	dt, err := l.AbsorbDowntime(context.Background())
	if err != nil {
		t.Fatalf("second AbsorbDowntime: %v", err)
	}
	if dt.Shifted != 0 {
		t.Errorf("second call Shifted = %d, want 0", dt.Shifted)
	}
	if got := scheduledTaskRunAt(t, pool, id); !got.Equal(shifted) {
		t.Errorf("run_at changed on the second call: before %v, after %v", shifted, got)
	}
}

// TestAbsorbDowntime_LedgerAndBasisTablesUntouched asserts the invariant: the
// shift writes scheduled_task only.
func TestAbsorbDowntime_LedgerAndBasisTablesUntouched(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l := newLiveness(t, pool, time.Second, time.Minute)

	setLivenessSeenAt(t, pool, "now() - interval '1 hour'")
	insertScheduledTask(t, pool, "t", "now() - interval '10 minutes'", "pending")

	before := ledgerRowCounts(t, pool)
	dt, err := l.AbsorbDowntime(context.Background())
	if err != nil {
		t.Fatalf("AbsorbDowntime: %v", err)
	}
	if dt.Shifted == 0 {
		t.Fatalf("test fixture did not exercise a shift")
	}
	after := ledgerRowCounts(t, pool)
	for table, n := range before {
		if after[table] != n {
			t.Errorf("%s row count changed: before %d, after %d", table, n, after[table])
		}
	}
}

// TestAbsorbDowntime_TwoConcurrentCallsShiftOnce drives two
// AbsorbDowntime calls against one schema, released together, over a
// stale instant with overdue rows present: the row lock serialises
// them, so the rows move exactly once and by exactly one gap.
func TestAbsorbDowntime_TwoConcurrentCallsShiftOnce(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	l1 := newLiveness(t, pool, time.Second, time.Minute)
	l2 := newLiveness(t, pool, time.Second, time.Minute)

	setLivenessSeenAt(t, pool, "now() - interval '1 hour'")
	id := insertScheduledTask(t, pool, "t", "now() - interval '10 minutes'", "pending")
	before := scheduledTaskRunAt(t, pool, id)

	start := make(chan struct{})
	results := make(chan Downtime, 2)
	errs := make(chan error, 2)
	for _, l := range []*Liveness{l1, l2} {
		go func(l *Liveness) {
			<-start
			dt, err := l.AbsorbDowntime(context.Background())
			results <- dt
			errs <- err
		}(l)
	}
	close(start)

	dt1, dt2 := <-results, <-results
	err1, err2 := <-errs, <-errs
	if err1 != nil {
		t.Fatalf("call 1: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("call 2: %v", err2)
	}

	if dt1.Shifted+dt2.Shifted != 1 {
		t.Fatalf("Shifted totals = %d + %d, want exactly one call reporting the shift", dt1.Shifted, dt2.Shifted)
	}

	after := scheduledTaskRunAt(t, pool, id)
	winnerGap := dt1.Gap
	if dt1.Shifted == 0 {
		winnerGap = dt2.Gap
	}
	if got, want := after.Sub(before), winnerGap; absDuration(got-want) > time.Second {
		t.Errorf("row shifted by %v, want %v (exactly one gap)", got, want)
	}
}

// TestLiveness_RunWritesOncePerInterval asserts Run refreshes at the
// configured cadence by counting writes across a span of several
// intervals, not by timing a single one.
func TestLiveness_RunWritesOncePerInterval(t *testing.T) {
	t.Parallel()
	cfg := testdb.Schema(t)
	counter := &livenessWriteCounter{}
	cfg.ConnConfig.Tracer = counter

	pool, err := store.NewPool(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := store.Migrate(context.Background(), pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const interval = 50 * time.Millisecond
	l := newLiveness(t, pool, interval, time.Hour)

	runCtx, cancel := context.WithTimeout(context.Background(), 9*interval)
	defer cancel()

	err = l.Run(runCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run returned %v, want context.DeadlineExceeded", err)
	}
	if n := counter.Count(); n < 3 {
		t.Errorf("write count = %d, want at least 3 across ~9 intervals", n)
	}
}

// TestLiveness_StopSeam is the shared stop-contract scenario list this
// package's suite states once and applies to all three runners; this is
// the Liveness instance.
func TestLiveness_StopSeam(t *testing.T) {
	t.Parallel()

	t.Run("stop_before_run_returns_nil_without_a_cycle", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		l := newLiveness(t, pool, time.Hour, time.Hour)
		l.Stop()

		done := make(chan error, 1)
		go func() { done <- l.Run(context.Background()) }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run() = %v, want nil", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return promptly after Stop before Run")
		}
	})

	t.Run("stop_during_inter_heartbeat_wait_returns_nil_promptly", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		l := newLiveness(t, pool, time.Hour, time.Hour)

		done := make(chan error, 1)
		go func() { done <- l.Run(context.Background()) }()
		time.Sleep(20 * time.Millisecond)
		l.Stop()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run() = %v, want nil", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return promptly after Stop")
		}
	})

	t.Run("stop_called_twice_no_panic", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		l := newLiveness(t, pool, time.Hour, time.Hour)
		l.Stop()
		l.Stop()
	})

	t.Run("context_cancelled_returns_ctx_err", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		l := newLiveness(t, pool, time.Hour, time.Hour)
		runCtx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() { done <- l.Run(runCtx) }()
		time.Sleep(20 * time.Millisecond)
		cancel()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run() = %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return promptly after cancel")
		}
	})
}

// TestLiveness_FailingHeartbeatReturnsAfterToleranceSpent asserts the
// mid-life policy: Run against a pool whose liveness write always fails
// returns the last error once the derived tolerance is spent, and not
// before — and that a spent tolerance is distinct from a clean Stop
// (nil).
func TestLiveness_FailingHeartbeatReturnsAfterToleranceSpent(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)

	// Close the pool immediately so every subsequent Refresh fails —
	// the simplest reliable "always fails" fixture.
	pool.Close()

	const interval = 10 * time.Millisecond
	const threshold = 40 * time.Millisecond // tolerance = ceil(40/10) = 4
	l := newLiveness(t, pool, interval, threshold)

	start := time.Now()
	err := l.Run(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("Run() = nil, want the last Refresh error once tolerance is spent")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() = %v, want a Refresh error, not a context error", err)
	}
	// At least tolerance-1 intervals must have elapsed (four failing
	// heartbeats), proving Run did not give up on the first failure.
	if elapsed < 3*interval {
		t.Errorf("Run gave up after %v, want at least %v (tolerance not spent early)", elapsed, 3*interval)
	}
}

// clockBannedSelectors names the Go clock functions the liveness source
// must never call — every instant it reasons about must come from the
// database instead.
var clockBannedSelectors = map[string]bool{
	"Now":   true,
	"Since": true,
	"Until": true,
}

// clockOffenses parses path and returns one string per call to
// time.Now, time.Since or time.Until found in it.
func clockOffenses(t *testing.T, path string) []string {
	t.Helper()
	f := srcguard.ParseFile(t, path)
	var offenses []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "time" || !clockBannedSelectors[sel.Sel.Name] {
			return true
		}
		offenses = append(offenses, "time."+sel.Sel.Name)
		return true
	})
	return offenses
}

// TestGuard_LivenessSourceCallsNoGoClock is the structural proof:
// the liveness source calls none of time.Now, time.Since or
// time.Until — every instant it reasons about comes from the database.
// The walk is proven discriminating against a scratch file that does
// call time.Now, in its own t.TempDir(), never the working tree.
func TestGuard_LivenessSourceCallsNoGoClock(t *testing.T) {
	t.Parallel()

	path := repotest.RootPath(t, filepath.Join("internal", "scheduler", "liveness.go"))
	offenses := clockOffenses(t, path)
	if len(offenses) != 0 {
		t.Errorf("liveness source calls a Go clock function: %v (every instant must come from the database)", offenses)
	}
}

func TestGuard_LivenessSourceCallsNoGoClock_ProvenDiscriminating(t *testing.T) {
	t.Parallel()

	_, path := srcguard.WriteScratchFile(t, "scratch/scratch.go",
		"package scratch\n\nimport \"time\"\n\nfunc f() time.Time { return time.Now() }\n")
	if offenses := clockOffenses(t, path); len(offenses) == 0 {
		t.Fatal("expected the scratch time.Now call to be flagged")
	}
}
