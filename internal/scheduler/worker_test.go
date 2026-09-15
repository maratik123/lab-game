package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/storetest"
)

// errBoom is a fixed handler-error test double.
var errBoom = errors.New("boom")

// testConfig returns a Scheduler config tuned for fast, deterministic
// tests: a short poll interval, a small claim limit, and retry values
// that make backoff assertions quick without sleeping for long.
func testConfig() config.Scheduler {
	return config.Scheduler{
		PollInterval:     50 * time.Millisecond,
		ClaimLimit:       10,
		RetryMaxAttempts: 3,
		RetryBaseDelay:   10 * time.Millisecond,
		RetryMaxDelay:    time.Second,
		RetryFactor:      backoff.DefaultFactor,
		TaskTimeout:      time.Second,
	}
}

// contentionSafeConfig returns testConfig with its TaskTimeout raised well
// past anything cross-package load can add to one task's own execution: the
// worker re-applies TaskTimeout server-side as statement_timeout and
// idle_in_transaction_session_timeout for the task's transaction, so a
// value tuned for an uncontended server is an instrument that must not
// fire once another package's tests are hammering the same one. Tests that
// assert the deadline itself firing use shortDeadlineConfig instead and
// never call this function, so its own value stays exactly as documented.
func contentionSafeConfig() config.Scheduler {
	cfg := testConfig()
	cfg.TaskTimeout = 30 * time.Second
	return cfg
}

// TestNew_RetryFactorRefusal covers the constructor's factor check:
// exactly 1, +Inf and NaN are each refused naming RetryFactor — the same
// values that would pass a wrong predicate borrowed from the duration
// checks beside it (<= 0) — and an invalid factor beside an
// earlier-invalid field names the EARLIER field, which is the only row
// that can red a factor check inserted anywhere but last in New's chain.
// No migrated schema is needed: New only nil-checks the pool and never
// dials it.
func TestNew_RetryFactorRefusal(t *testing.T) {
	t.Parallel()

	pool, err := pgxpool.New(context.Background(), "postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	cases := []struct {
		name      string
		mutate    func(*config.Scheduler)
		wantField string
	}{
		{"exactly one", func(c *config.Scheduler) { c.RetryFactor = 1 }, "RetryFactor"},
		{"positive infinity", func(c *config.Scheduler) { c.RetryFactor = math.Inf(1) }, "RetryFactor"},
		{"NaN", func(c *config.Scheduler) { c.RetryFactor = math.NaN() }, "RetryFactor"},
		{
			"invalid retry factor beside an earlier-invalid field names the earlier field",
			func(c *config.Scheduler) {
				c.RetryBaseDelay = 0
				c.RetryFactor = 1
			},
			"RetryBaseDelay",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			tc.mutate(&cfg)
			_, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
			if err == nil {
				t.Fatal("New: expected an error, got nil")
			}
			var optErr *OptionError
			if !errors.As(err, &optErr) {
				t.Fatalf("New: error %v is not an *OptionError", err)
			}
			if optErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", optErr.Field, tc.wantField)
			}
		})
	}
}

// TestNew_LoggerNilAcceptedAndSubstituted covers Options.Logger: a nil
// Logger is accepted, not refused, and New substitutes a discard handler
// rather than leaving the field nil; a non-nil Logger passes through as
// given.
func TestNew_LoggerNilAcceptedAndSubstituted(t *testing.T) {
	t.Parallel()

	pool, err := pgxpool.New(context.Background(), "postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	w, err := New(Options{Pool: pool, Registry: reg, Config: testConfig()})
	if err != nil {
		t.Fatalf("New with nil Logger: %v", err)
	}
	if w.logger == nil {
		t.Fatal("logger = nil, want a discard handler substituted for the nil Options.Logger")
	}

	given := slog.New(slog.DiscardHandler)
	w, err = New(Options{Pool: pool, Registry: reg, Config: testConfig(), Logger: given})
	if err != nil {
		t.Fatalf("New with a given Logger: %v", err)
	}
	if w.logger != given {
		t.Fatalf("logger = %v, want the given Logger passed through unchanged", w.logger)
	}
}

func newWorker(t *testing.T, pool *pgxpool.Pool, reg *Registry) *Worker {
	t.Helper()
	w, err := New(Options{Pool: pool, Registry: reg, Config: contentionSafeConfig()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return w
}

// writingHandler inserts a manual_correction row (a real table write, so
// "its writes are absent" is asserted against the migrated schema) and
// then returns a fixed Outcome/error.
type writingHandler struct {
	mu      sync.Mutex
	outcome Outcome
	err     error
	reason  string
	seen    int
}

func (h *writingHandler) Execute(ctx context.Context, tx pgx.Tx, _ Task) (Outcome, error) {
	h.mu.Lock()
	h.seen++
	h.mu.Unlock()
	if _, err := tx.Exec(ctx, `INSERT INTO manual_correction (actor, reason) VALUES ('scheduler-test', $1)`, h.reason); err != nil {
		return OutcomeFailed, err
	}
	return h.outcome, h.err
}

func (h *writingHandler) seenCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seen
}

// swallowingHandler triggers a real unique-violation, swallows it, and
// reports OutcomeDone — a regression guard for the raw-SQL
// savepoint: written against
// pgx.Tx.Begin's pseudo-nested transaction this class of handler would
// silently report success.
type swallowingHandler struct {
	instanceKey string
}

func (h *swallowingHandler) Execute(ctx context.Context, tx pgx.Tx, _ Task) (Outcome, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO player_operation (source, operation_id) VALUES ('telegram', $1)`, h.instanceKey); err != nil {
		return OutcomeFailed, err
	}
	// The duplicate below violates player_operation's unique constraint;
	// the error is deliberately swallowed.
	_, _ = tx.Exec(ctx, `INSERT INTO player_operation (source, operation_id) VALUES ('telegram', $1)`, h.instanceKey)
	return OutcomeDone, nil
}

func manualCorrectionCount(t *testing.T, pool *pgxpool.Pool, reason string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM manual_correction WHERE reason = $1`, reason).Scan(&n); err != nil {
		t.Fatalf("count manual_correction: %v", err)
	}
	return n
}

// schedulerTaskRow reads one scheduled_task row's mutable columns.
// found is false when the row does not exist (deleted on done).
func schedulerTaskRow(t *testing.T, pool *pgxpool.Pool, id TaskID) (state string, failures int, lastError *string, runAt time.Time, found bool) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT state, consecutive_failures, last_error, run_at FROM scheduled_task WHERE id = $1`, int64(id),
	).Scan(&state, &failures, &lastError, &runAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil, time.Time{}, false
	}
	if err != nil {
		t.Fatalf("select scheduled_task %d: %v", id, err)
	}
	return state, failures, lastError, runAt, true
}

// errText renders a nullable last_error column for a failure message: the
// quoted string when the column is set, the literal <nil> when it is not.
// A *string formatted with %v prints an address, which tells a reader
// nothing about why the assertion failed.
func errText(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return strconv.Quote(*s)
}

func TestRunOnce_batchBoundedAndSkipNotWait(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "batch-bound"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Insert three due tasks; hold two of them locked in a concurrent,
	// uncommitted transaction, exactly the exclusion protocol this test
	// covers.
	var ids [3]TaskID
	for i := range ids {
		ids[i] = dueNow(t, pool, reg, Request{Type: "test.oneshot"})
	}

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx,
		`SELECT id FROM scheduled_task WHERE id = ANY($1) FOR NO KEY UPDATE SKIP LOCKED`,
		[]int64{int64(ids[0]), int64(ids[1])},
	); err != nil {
		t.Fatalf("hold rows: %v", err)
	}

	w := newWorker(t, pool, reg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := h.seenCount(); got != 1 {
		t.Fatalf("handler ran %d times, want 1 (the two held rows must be skipped, not waited on)", got)
	}
}

func TestRunOnce_twoWorkersConcurrent_exactlyOnceUnderRace(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "concurrent"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	const n = 20
	for range n {
		dueNow(t, pool, reg, Request{Type: "test.oneshot"})
	}

	w1 := newWorker(t, pool, reg)
	w2 := newWorker(t, pool, reg)

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	for _, w := range []*Worker{w1, w2} {
		go func(w *Worker) {
			defer wg.Done()
			for range 5 {
				if err := w.RunOnce(ctx); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := h.seenCount(); got != n {
		t.Fatalf("handler ran %d times across two workers, want exactly %d", got, n)
	}
}

func TestRunOnce_rowTakenBeforeReclaim_skippedSilently(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "taken-before-reclaim"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	// Delete the row out from under the worker before it re-claims —
	// simulating another worker having already finished it.
	if _, err := pool.Exec(ctx, `DELETE FROM scheduled_task WHERE id = $1`, int64(id)); err != nil {
		t.Fatalf("delete: %v", err)
	}

	w := newWorker(t, pool, reg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if got := h.seenCount(); got != 0 {
		t.Fatalf("handler ran %d times on a row deleted before re-claim, want 0", got)
	}
}

// TestRunOnce_failedHandler_writesRolledBack_attemptRecorded covers a
// failed handler's rollback and attempt recording.
func TestRunOnce_failedHandler_writesRolledBack_attemptRecorded(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	reason := "failed-handler"
	h := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: reason}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w := newWorker(t, pool, reg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := manualCorrectionCount(t, pool, reason); got != 0 {
		t.Fatalf("manual_correction rows with reason %q = %d, want 0 (rolled back)", reason, got)
	}
	state, failures, lastError, _, found := schedulerTaskRow(t, pool, id)
	if !found {
		t.Fatalf("row deleted after a failure, want it to remain pending")
	}
	if state != "pending" || failures != 1 || lastError == nil || *lastError == "" {
		t.Fatalf("row after failure = state:%s failures:%d lastError:%s, want pending/1/non-empty", state, failures, errText(lastError))
	}
}

// TestRunOnce_noop_writesAbsent_rowSettled covers a no-op outcome's
// settlement.
func TestRunOnce_noop_writesAbsent_rowSettled(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	reason := "noop"
	h := &writingHandler{outcome: OutcomeNoop, reason: reason}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w := newWorker(t, pool, reg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if got := manualCorrectionCount(t, pool, reason); got != 0 {
		t.Fatalf("manual_correction rows with reason %q = %d, want 0 (a no-op's writes must be absent)", reason, got)
	}
	_, _, _, _, found := schedulerTaskRow(t, pool, id)
	if found {
		t.Fatalf("one-shot row still present after a no-op, want it deleted (settled exactly as Done)")
	}
}

// TestRunOnce_swallowedDatabaseError_settlesAsFailure is a
// regression guard: a handler that swallows a real database error and
// reports OutcomeDone must have its attempt settled as a failure, because
// the savepoint release fails.
func TestRunOnce_swallowedDatabaseError_settlesAsFailure(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	h := &swallowingHandler{instanceKey: "swallow-1"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w := newWorker(t, pool, reg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	state, failures, lastError, _, found := schedulerTaskRow(t, pool, id)
	if !found {
		t.Fatalf("row deleted after a swallowed-error attempt, want it to remain pending as a recorded failure")
	}
	if state != "pending" || failures != 1 || lastError == nil || *lastError == "" {
		t.Fatalf("row after swallowed error = state:%s failures:%d lastError:%s, want pending/1/non-empty", state, failures, errText(lastError))
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM player_operation WHERE operation_id = 'swallow-1'`).Scan(&count); err != nil {
		t.Fatalf("count player_operation: %v", err)
	}
	if count != 0 {
		t.Fatalf("player_operation rows for swallow-1 = %d, want 0 (ROLLBACK TO SAVEPOINT undoes the whole subtransaction, both inserts included)", count)
	}
}

// TestRunOnce_deleteOnDone_bothDirections covers delete-on-done in both
// directions.
func TestRunOnce_deleteOnDone_bothDirections(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "delete-on-done"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w := newWorker(t, pool, reg)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if _, _, _, _, found := schedulerTaskRow(t, pool, id); found {
		t.Fatalf("a committed one-shot Done must leave no row")
	}

	// The rollback direction: a task whose transaction is rolled back
	// (simulated here by re-claiming and rolling back directly) leaves
	// the row present and still due.
	id2 := dueNow(t, pool, reg, Request{Type: "test.oneshot"})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, _, err := reclaim(ctx, tx, id2); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_task WHERE id = $1`, int64(id2)); err != nil {
		t.Fatalf("delete inside tx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	state, _, _, _, found := schedulerTaskRow(t, pool, id2)
	if !found || state != "pending" {
		t.Fatalf("a rolled-back deletion must leave the row present and pending, found=%v state=%s", found, state)
	}
}

// TestRunOnce_recurrence_singleLiveRow asserts that the row's id is
// unchanged and the live-row count for the recurrence is exactly one at
// every
// commit boundary, across a committed execution, a rolled-back
// execution, and a failed execution.
func TestRunOnce_recurrence_singleLiveRow(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "recurrence-single-row"}
	reg, err := NewRegistry(Declaration{
		Type: "test.recurrent", Handler: h,
		Recurrence: &Recurrence{Cadence: Every(time.Hour), ConfigKey: "k"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"})

	countLive := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_task WHERE type = 'test.recurrent'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	w := newWorker(t, pool, reg)

	// Committed execution.
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (committed): %v", err)
	}
	if got := countLive(); got != 1 {
		t.Fatalf("live rows after committed execution = %d, want 1", got)
	}
	_, _, _, _, found := schedulerTaskRow(t, pool, id)
	if !found {
		t.Fatalf("recurrence row %d must survive its own Done settlement", id)
	}

	// Make it due again and roll back the execution directly (simulating
	// a crash mid-transaction).
	if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
		t.Fatalf("make due: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, _, err := reclaim(ctx, tx, id); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := countLive(); got != 1 {
		t.Fatalf("live rows after rolled-back execution = %d, want 1", got)
	}

	// A failed execution.
	if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
		t.Fatalf("make due: %v", err)
	}
	failingReg, err := NewRegistry(Declaration{
		Type: "test.recurrent", Handler: &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "recurrence-fail"},
		Recurrence: &Recurrence{Cadence: Every(time.Hour), ConfigKey: "k"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	fw := newWorker(t, pool, failingReg)
	if err := fw.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (failed): %v", err)
	}
	if got := countLive(); got != 1 {
		t.Fatalf("live rows after a failed execution = %d, want 1 (a recurrence never becomes dead)", got)
	}
	var gotID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM scheduled_task WHERE type = 'test.recurrent'`).Scan(&gotID); err != nil {
		t.Fatalf("select id: %v", err)
	}
	if TaskID(gotID) != id {
		t.Fatalf("recurrence id changed: got %d, want %d (one row moved forward in place)", gotID, id)
	}
}
