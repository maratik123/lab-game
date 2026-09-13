package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/config"
)

// shortDeadlineConfig returns a Scheduler config tuned for the deadline
// tests: a short TaskTimeout so a breach happens quickly, and retry
// values short enough that a give-up test need not wait long.
func shortDeadlineConfig() config.Scheduler {
	cfg := testConfig()
	cfg.TaskTimeout = 100 * time.Millisecond
	cfg.RetryBaseDelay = 20 * time.Millisecond
	cfg.RetryMaxDelay = 500 * time.Millisecond
	cfg.RetryMaxAttempts = 3
	return cfg
}

// blockingHandler blocks until release is closed, then returns a fixed
// Outcome. It never touches ctx, so layer 1 (context.WithTimeout) is what
// stops the worker waiting on it.
type blockingHandler struct {
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{release: make(chan struct{})}
}

func (h *blockingHandler) Execute(_ context.Context, _ pgx.Tx, _ Task) (Outcome, error) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	<-h.release
	return OutcomeDone, nil
}

// ctxIgnoringHandler is the deadline defence's negative case: it never
// sees ctx's cancellation, issuing short statements on a context of its
// own until
// release is closed.
type ctxIgnoringHandler struct {
	release chan struct{}
}

func (h *ctxIgnoringHandler) Execute(_ context.Context, tx pgx.Tx, _ Task) (Outcome, error) {
	for {
		select {
		case <-h.release:
			return OutcomeDone, nil
		default:
		}
		_, _ = tx.Exec(context.Background(), "SELECT pg_sleep(0.02)") //nolint:contextcheck // deliberate: this handler is the ctx-ignoring negative case under test
	}
}

// waitLockFree polls, without depending on any scheduler internals,
// until id's row can be locked FOR NO KEY UPDATE SKIP LOCKED — i.e. the
// abandoned transaction's backend has released its locks, normally
// because the deadline branch's own terminate ended it, or otherwise
// because idle_in_transaction_session_timeout or the watchdog's close of
// the hijacked connection did. timeout is an instrument, a patience budget for the
// poll — never the deadline defence itself, which every caller here
// already asserts separately — so callers give it slack well past
// anything cross-package load can add to how long the drain takes.
func waitLockFree(t *testing.T, pool *pgxpool.Pool, id TaskID, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		func() {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin probe: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			var got int64
			err = tx.QueryRow(ctx,
				`SELECT id FROM scheduled_task WHERE id = $1 FOR NO KEY UPDATE SKIP LOCKED`, int64(id),
			).Scan(&got)
			if err == nil {
				deadline = time.Time{} // sentinel: found
			}
		}()
		if deadline.IsZero() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("row %d never became lock-free within %v", id, timeout)
}

// TestDeadline_blockedHandler_rowClaimableWithinBound asserts that the
// worker returns, the observation carries the deadline failure kind, and
// the row becomes claimable again within the deadline defence's own
// derived bound (~2x the configured deadline), not blocked forever.
func TestDeadline_blockedHandler_rowClaimableWithinBound(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	h := newBlockingHandler()
	defer close(h.release)

	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	tasks := obs.Tasks()
	if len(tasks) != 1 || tasks[0].Outcome != OutcomeFailed || tasks[0].Failure != FailureDeadline {
		t.Fatalf("observations = %+v, want one Failed/FailureDeadline observation", tasks)
	}

	waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)
}

// TestDeadline_breachIsSettledNotMerelyAbandoned is the round-6 finding:
// after the breach and the next cycle's drain, the row's
// consecutive_failures has risen by one, its last_error records the
// deadline, and its run_at is strictly later than the drain instant —
// never merely t + backoff(k), which could already be in the past.
func TestDeadline_breachIsSettledNotMerelyAbandoned(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	h := newBlockingHandler()
	defer close(h.release)

	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (breach): %v", err)
	}
	waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)

	var drainInstant time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&drainInstant); err != nil {
		t.Fatalf("read drain instant: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain): %v", err)
	}

	state, failures, lastError, runAt, found := schedulerTaskRow(t, pool, id)
	if !found || state != "pending" {
		t.Fatalf("row after drain = state:%v (found=%v), want pending", state, found)
	}
	if failures != 1 {
		t.Fatalf("consecutive_failures = %d, want 1", failures)
	}
	if lastError == nil || !strings.Contains(*lastError, "deadline") {
		t.Fatalf("last_error = %s, want it to mention the deadline", errText(lastError))
	}
	if !runAt.After(drainInstant) {
		t.Fatalf("run_at = %v, want strictly after the drain instant %v (not merely t + backoff, which can already be past)", runAt, drainInstant)
	}
}

// TestDeadline_successiveBreaches_growingDelay covers the breach
// class: driven attempt by attempt, each settled run_at falls within the
// exact backoff bracket computed around the drain instant that wrote it.
func TestDeadline_successiveBreaches_growingDelay(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	// RetryBaseDelay is raised locally to 200ms (the ceiling here is
	// 500ms, so 200ms then 400ms both fit): this test never waits for
	// the backoff to elapse, so the higher base costs nothing, and it
	// keeps the bracket well clear of drain/scheduling jitter. Do not
	// change the shared shortDeadlineConfig() TaskTimeout -- other tests
	// depend on its 100ms value.
	cfg.RetryBaseDelay = 200 * time.Millisecond

	for attempt := 1; attempt < cfg.RetryMaxAttempts; attempt++ {
		h := newBlockingHandler()
		reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var id TaskID
		if attempt == 1 {
			id = dueNow(t, pool, reg, Request{Type: "test.oneshot", InstanceKey: "growing-delay"})
		} else {
			var scanned int64
			if err := pool.QueryRow(ctx, `SELECT id FROM scheduled_task WHERE type = 'test.oneshot' AND instance_key = 'growing-delay'`).Scan(&scanned); err != nil {
				t.Fatalf("select row: %v", err)
			}
			id = TaskID(scanned)
			if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
				t.Fatalf("force due: %v", err)
			}
		}

		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (breach %d): %v", attempt, err)
		}
		waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)

		var drainInstant time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&drainInstant); err != nil {
			t.Fatalf("read drain instant: %v", err)
		}
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (drain %d): %v", attempt, err)
		}
		after := captureNow(t, pool)
		close(h.release)

		_, failures, _, runAt, found := schedulerTaskRow(t, pool, id)
		if !found || failures != attempt {
			t.Fatalf("attempt %d: consecutive_failures = %d (found=%v), want %d", attempt, failures, found, attempt)
		}
		// Same before <= s <= after argument as
		// TestFailurePolicy_oneShotAttemptsGrowAndGiveUp, anchored to the
		// drain instant: the backoff base for this path is read inside
		// drainPending, strictly between drainInstant and
		// after.
		//
		// The bracket's delay is a LITERAL one-based ramp, not a call to
		// the shared backoff package's ramp function: 200ms then 400ms is
		// exactly what the shipped one-based backoff(attempt, 200ms, 500ms)
		// computes at attempts 1 and 2 -- verified green against the
		// still-shipped backoff before the shared package's ramp was ever
		// re-pointed. Pinning it as a literal is what lets this assertion
		// catch an omitted one-based-to-zero-based translation at the
		// settlement code's own drain-settlement call site.
		literalOneBasedRamp := map[int]time.Duration{
			1: 200 * time.Millisecond,
			2: 400 * time.Millisecond,
		}
		want, ok := literalOneBasedRamp[attempt]
		if !ok {
			t.Fatalf("attempt %d: no literal ramp entry (want one for every attempt < RetryMaxAttempts)", attempt)
		}
		lo, hi := drainInstant.Add(want), after.Add(want)
		if runAt.Before(lo) || runAt.After(hi) {
			t.Fatalf("attempt %d: run_at = %v, want within [%v, %v] (drainInstant=%v after=%v backoff=%v)", attempt, runAt, lo, hi, drainInstant, after, want)
		}
	}
}

// TestDeadline_nonDefaultFactorReachesTheCallSite is the
// non-default-factor scenario for the deferred-drain settlement
// (deferredFailedStatement's path via drainPending): the only
// instrument that discriminates a call site passing the configured
// cfg.RetryFactor from one passing the shared package's compiled-in
// default — the literal
// one-based ramp above stays at the default and cannot see it.
func TestDeadline_nonDefaultFactorReachesTheCallSite(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	cfg.RetryBaseDelay = 200 * time.Millisecond
	cfg.RetryFactor = 3

	// factor=3: k-1=0 at attempt 1 (always base, 200ms — already pinned by
	// the default-factor test above) and k-1=1 at attempt 2, where
	// base*3 = 600ms exceeds the 500ms ceiling and clamps to it — distinct
	// from both the default factor's 400ms and an unclamped 600ms, so a
	// call site passing the wrong factor OR skipping the clamp both red.
	literalRampAtFactorThree := map[int]time.Duration{
		1: 200 * time.Millisecond,
		2: 500 * time.Millisecond,
	}

	for attempt := 1; attempt < cfg.RetryMaxAttempts; attempt++ {
		h := newBlockingHandler()
		reg, err := NewRegistry(Declaration{Type: "test.oneshot.factor", Handler: h})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var id TaskID
		if attempt == 1 {
			id = dueNow(t, pool, reg, Request{Type: "test.oneshot.factor", InstanceKey: "growing-delay-factor"})
		} else {
			var scanned int64
			if err := pool.QueryRow(ctx, `SELECT id FROM scheduled_task WHERE type = 'test.oneshot.factor' AND instance_key = 'growing-delay-factor'`).Scan(&scanned); err != nil {
				t.Fatalf("select row: %v", err)
			}
			id = TaskID(scanned)
			if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
				t.Fatalf("force due: %v", err)
			}
		}

		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (breach %d): %v", attempt, err)
		}
		waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)

		var drainInstant time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&drainInstant); err != nil {
			t.Fatalf("read drain instant: %v", err)
		}
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (drain %d): %v", attempt, err)
		}
		after := captureNow(t, pool)
		close(h.release)

		_, _, _, runAt, found := schedulerTaskRow(t, pool, id)
		if !found {
			t.Fatalf("attempt %d: row not found", attempt)
		}
		want := literalRampAtFactorThree[attempt]
		lo, hi := drainInstant.Add(want), after.Add(want)
		if runAt.Before(lo) || runAt.After(hi) {
			t.Fatalf("attempt %d: run_at = %v, want within [%v, %v] (drainInstant=%v after=%v factor-3 backoff=%v)", attempt, runAt, lo, hi, drainInstant, after, want)
		}
	}
}

// TestDeadline_recurrenceSettlesIntoFuture is the recurrent branch of the
// round-7 correction: with a cadence measured in hundreds of
// milliseconds (so the test need not sleep for long), a breaching
// handler's settled run_at is still strictly after the drain instant —
// the case a stale execution instant would fail and a production-length
// cadence would hide.
func TestDeadline_recurrenceSettlesIntoFuture(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	h := newBlockingHandler()
	defer close(h.release)

	period := 200 * time.Millisecond
	reg, err := NewRegistry(Declaration{
		Type: "test.recurrent", Handler: h,
		Recurrence: &Recurrence{Cadence: Every(period), ConfigKey: "k"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (breach): %v", err)
	}
	waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)

	var drainInstant time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&drainInstant); err != nil {
		t.Fatalf("read drain instant: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain): %v", err)
	}

	state, failures, _, runAt, found := schedulerTaskRow(t, pool, id)
	if !found || state != "pending" || failures != 1 {
		t.Fatalf("row after drain = state:%v failures:%d (found=%v), want pending/1", state, failures, found)
	}
	if !runAt.After(drainInstant) {
		t.Fatalf("run_at = %v, want strictly after the drain instant %v", runAt, drainInstant)
	}
}

// TestDeadline_oneShotGivesUpWithinCap is the regression guard for the
// unbounded re-claim loop: a one-shot whose handler always breaches
// reaches give-up within the configured cap.
func TestDeadline_oneShotGivesUpWithinCap(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()

	var id TaskID
	for attempt := 1; attempt <= cfg.RetryMaxAttempts; attempt++ {
		h := newBlockingHandler()
		reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if attempt == 1 {
			id = dueNow(t, pool, reg, Request{Type: "test.oneshot", InstanceKey: "giveup-deadline"})
		} else {
			if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
				t.Fatalf("force due: %v", err)
			}
		}
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (breach %d): %v", attempt, err)
		}
		waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (drain %d): %v", attempt, err)
		}
		close(h.release)
	}

	state, failures, _, _, found := schedulerTaskRow(t, pool, id)
	if !found || state != "dead" || failures != cfg.RetryMaxAttempts {
		t.Fatalf("final row = state:%v failures:%d (found=%v), want dead/%d", state, failures, found, cfg.RetryMaxAttempts)
	}
}

// TestDeadline_recurrenceNeverGivesUp covers the recurrence breach case:
// a recurrence whose handler always breaches never reaches a terminal
// state, staying
// pending and advancing by cadence, with a rising failure count.
func TestDeadline_recurrenceNeverGivesUp(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	period := 200 * time.Millisecond

	var id TaskID
	attempts := cfg.RetryMaxAttempts + 2
	for attempt := 1; attempt <= attempts; attempt++ {
		h := newBlockingHandler()
		reg, err := NewRegistry(Declaration{
			Type: "test.recurrent", Handler: h,
			Recurrence: &Recurrence{Cadence: Every(period), ConfigKey: "k"},
		})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if attempt == 1 {
			id = dueNow(t, pool, reg, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"})
		} else {
			if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
				t.Fatalf("force due: %v", err)
			}
		}
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (breach %d): %v", attempt, err)
		}
		waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (drain %d): %v", attempt, err)
		}
		close(h.release)

		state, failures, _, _, found := schedulerTaskRow(t, pool, id)
		if !found || state != "pending" {
			t.Fatalf("attempt %d: state = %v (found=%v), want pending (never terminal)", attempt, state, found)
		}
		if failures != attempt {
			t.Fatalf("attempt %d: consecutive_failures = %d, want %d", attempt, failures, attempt)
		}
	}
}

// TestDeadline_deferredGuard_rowAlreadyMoved covers the deferred-drain
// guard: with the breached row's run_at changed by another party before
// the drain runs
// (simulating a second worker having claimed and settled it), the drain
// updates nothing and drops the id rather than counting an attempt
// against work it did not do.
func TestDeadline_deferredGuard_rowAlreadyMoved(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	h := newBlockingHandler()
	defer close(h.release)

	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (breach): %v", err)
	}
	waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)

	// Simulate a second worker having finished the row entirely.
	if _, err := pool.Exec(ctx, `DELETE FROM scheduled_task WHERE id = $1`, int64(id)); err != nil {
		t.Fatalf("simulate completion: %v", err)
	}

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain): %v", err)
	}
	if _, _, _, _, found := schedulerTaskRow(t, pool, id); found {
		t.Fatalf("row reappeared after the drain touched a row someone else finished")
	}
}

// TestDeadline_drainDoesNotBlockOnLockedRow asserts that with a pending
// settlement's row still held by an open transaction, the drain's
// statement returns having updated nothing and the cycle proceeds
// without blocking; the id is retried once the lock is released. This
// drives Worker.enqueuePending and RunOnce's drain directly, holding the
// lock in a separate transaction under this test's own control — a
// breach-driven version of this test would race the server's own
// idle-in-transaction release against the assertion window, since both
// timers start from approximately the same instant.
func TestDeadline_drainDoesNotBlockOnLockedRow(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	// RetryBaseDelay/RetryMaxDelay are both raised locally so that the
	// drain's persisted run_at = s + backoff(1) is far in the future.
	// Without this, backoff(1) with the shared 20ms base can already have
	// elapsed by the time discovery's next WHERE run_at <= now() runs,
	// re-claiming the row within the same test before the lock-release
	// assertion below; the fixedOutcomeHandler{OutcomeDone} would then
	// succeed and delete the one-shot row, producing found=false instead
	// of the expected failures=1. Raising RetryMaxDelay too is required:
	// the shared 500ms ceiling would otherwise clamp the 30s base back
	// down. This costs zero wall-clock time -- the test never waits for
	// that run_at to arrive.
	cfg.RetryBaseDelay = 30 * time.Second
	cfg.RetryMaxDelay = time.Minute
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: &fixedOutcomeHandler{outcome: OutcomeDone}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	var runAt time.Time
	if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE id = $1`, int64(id)).Scan(&runAt); err != nil {
		t.Fatalf("select run_at: %v", err)
	}

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	if _, err := holder.Exec(ctx, `SELECT id FROM scheduled_task WHERE id = $1 FOR NO KEY UPDATE`, int64(id)); err != nil {
		t.Fatalf("hold lock: %v", err)
	}

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w.enqueuePending(pendingSettlement{id: id, runAt: runAt, consecutiveFailures: 0, reason: "deadline exceeded"})

	start := time.Now()
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain while locked): %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("RunOnce took %v while draining a locked row, want it non-blocking", elapsed)
	}
	if _, failures, _, _, found := schedulerTaskRow(t, pool, id); !found || failures != 0 {
		t.Fatalf("row while still locked = failures:%d (found=%v), want 0 (not yet settled)", failures, found)
	}

	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain after release): %v", err)
	}
	if _, failures, _, _, found := schedulerTaskRow(t, pool, id); !found || failures != 1 {
		t.Fatalf("row after lock release = failures:%d (found=%v), want 1 (settled on retry)", failures, found)
	}
}

// TestDeadline_neighboursSurvive asserts that a cycle containing a task
// that breaches and a task that succeeds leaves the successful task's
// effects committed, because they were never in the same transaction.
func TestDeadline_neighboursSurvive(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	// TaskTimeout is raised locally to 1s: blockingHandler blocks on
	// <-h.release, which is closed only by this test's own deferred
	// close(blocker.release) at exit, so the blocker breaches ANY finite
	// TaskTimeout by construction -- the 100ms value never made the
	// blocker breach, it only set how long the test waits for that
	// breach to settle. The raise IS required, though: the same value
	// is also the succeeding neighbour's budget, and the neighbour's own
	// SAVEPOINT + INSERT + RELEASE was measured at 96-112ms under
	// parallel -race load -- the same order as 100ms, which is what made
	// this test flaky in the past. Do not change the shared
	// shortDeadlineConfig() TaskTimeout -- other tests depend on its
	// 100ms value.
	cfg.TaskTimeout = time.Second
	blocker := newBlockingHandler()
	defer close(blocker.release)
	succeeder := &writingHandler{outcome: OutcomeDone, reason: "neighbour-survives"}

	reg, err := NewRegistry(
		Declaration{Type: "test.oneshot.block", Handler: blocker},
		Declaration{Type: "test.oneshot.ok", Handler: succeeder},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	dueNow(t, pool, reg, Request{Type: "test.oneshot.block"})
	okID := dueNow(t, pool, reg, Request{Type: "test.oneshot.ok"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	tasks := obs.Tasks()
	block, blockFound := findObservationByType(tasks, "test.oneshot.block")
	neighbour, okFound := findObservationByType(tasks, "test.oneshot.ok")
	if len(tasks) != 2 || !blockFound || !okFound {
		t.Errorf("observations = %+v, want exactly one test.oneshot.block and one test.oneshot.ok", tasks)
	}
	if block.Outcome != OutcomeFailed || block.Failure != FailureDeadline {
		t.Errorf("observations = %+v, want test.oneshot.block Failed/FailureDeadline", tasks)
	}
	if neighbour.Outcome != OutcomeDone || neighbour.Failure != FailureNone {
		t.Errorf("observations = %+v, want test.oneshot.ok Done/FailureNone", tasks)
	}

	if manualCorrectionCount(t, pool, "neighbour-survives") != 1 {
		t.Errorf("the succeeding neighbour's effects were not committed")
	}
	if _, _, _, _, found := schedulerTaskRow(t, pool, okID); found {
		t.Fatalf("succeeding neighbour's row still present, want it deleted (Done, one-shot)")
	}
}

// findObservationByType returns the first Observation in tasks whose Type
// matches typ, and whether one was found. Keyed on Type rather than a
// positional index, so the assertion does not silently depend on
// discoverDue's ORDER BY run_at, id matching insertion order.
func findObservationByType(tasks []Observation, typ Type) (Observation, bool) {
	for _, task := range tasks {
		if task.Type == typ {
			return task, true
		}
	}
	return Observation{}, false
}

// TestDeadline_ctxIgnoringHandler_rowReclaimedDespiteIgnoredCtx covers a
// handler that never sees the deadline's cancellation and issues short
// statements on a context of its own. The worker still reports the
// deadline failure kind and proceeds within the deadline; its backend is
// terminated regardless of whether the handler ever notices, so the row
// becomes claimable within the existing lock-free poll helper's own
// patience budget even while the handler keeps running.
func TestDeadline_ctxIgnoringHandler_rowReclaimedDespiteIgnoredCtx(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	h := &ctxIgnoringHandler{release: make(chan struct{})}
	// Deferred, not called inline: once the backend is terminated, every
	// further Exec inside the handler's loop returns instantly, turning
	// it from pg_sleep(0.02)-paced into a hot spin for as long as the
	// test takes to reach this deferred close — so no slow work belongs
	// between the claim assertion below and here.
	defer close(h.release)

	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	start := time.Now()
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("RunOnce took %v, want it to return promptly at the deadline even though the handler keeps running", elapsed)
	}

	tasks := obs.Tasks()
	if len(tasks) != 1 || tasks[0].Failure != FailureDeadline {
		t.Fatalf("observations = %+v, want one FailureDeadline observation", tasks)
	}

	waitLockFree(t, pool, id, 10*time.Second)
}
