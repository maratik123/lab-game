package scheduler

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/config"
)

// alwaysFailHandler always returns OutcomeFailed with a fixed error.
type alwaysFailHandler struct {
	err error
}

func (h *alwaysFailHandler) Execute(context.Context, pgx.Tx, Task) (Outcome, error) {
	return OutcomeFailed, h.err
}

// decodeFailHandler simulates AC26's "cannot decode its payload" case: it
// always fails to json.Unmarshal the payload into an int and returns that
// decode error, classified as an ordinary failure.
type decodeFailHandler struct{}

func (decodeFailHandler) Execute(_ context.Context, _ pgx.Tx, task Task) (Outcome, error) {
	var n int
	if err := json.Unmarshal(task.Payload, &n); err != nil {
		return OutcomeFailed, err
	}
	return OutcomeDone, nil
}

// slowFirstAttemptHandler fails every attempt like alwaysFailHandler, but
// makes the FIRST attempt take firstDelay longer than the others. That
// asymmetry is what turns the run_at anchoring into a deterministic
// assertion instead of a load-dependent one: a measurement anchored
// outside RunOnce carries this delay, a measurement anchored to the
// settlement instant does not.
type slowFirstAttemptHandler struct {
	mu         sync.Mutex
	seen       int
	firstDelay time.Duration
	err        error
}

func (h *slowFirstAttemptHandler) Execute(ctx context.Context, _ pgx.Tx, _ Task) (Outcome, error) {
	h.mu.Lock()
	h.seen++
	first := h.seen == 1
	h.mu.Unlock()
	if first {
		select {
		case <-time.After(h.firstDelay):
		case <-ctx.Done():
			return OutcomeFailed, ctx.Err()
		}
	}
	return OutcomeFailed, h.err
}

// backoffProbeConfig is testConfig with a task deadline wide enough that
// slowFirstAttemptHandler's deliberate delay cannot breach it -- this test
// is about backoff anchoring, not about deadlines. RetryBaseDelay is
// raised to 200ms (RetryMaxDelay stays at 1s, so backoff is 200ms then
// 400ms, both under the ceiling): this costs nothing in wall-clock time,
// since the row is forced due on every iteration and the backoff is
// never waited on. The base must also stay strictly below
// slowFirstAttemptHandler's 400ms first-attempt delay, and that is what
// keeps this test's guard deterministic rather than probabilistic: a
// regression that anchors the measurement outside RunOnce again would
// measure backoff(1)+~402ms = ~602ms for attempt 1 but backoff(2)+~1ms
// = ~401ms for attempt 2, so its "the delay grew" check fails on every
// run instead of once in three.
func backoffProbeConfig() config.Scheduler {
	cfg := testConfig()
	cfg.TaskTimeout = 10 * time.Second
	cfg.RetryBaseDelay = 200 * time.Millisecond
	return cfg
}

// captureNow returns the server's clock_timestamp(), for bracketing a
// settlement instant that is read inside a transaction sometime between
// two of these calls.
func captureNow(t *testing.T, pool *pgxpool.Pool) time.Time {
	t.Helper()
	var now time.Time
	if err := pool.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatalf("capture now: %v", err)
	}
	return now
}

// forceDue sets id's run_at to now(), so the next RunOnce claims it
// without waiting for a persisted backoff to elapse.
func forceDue(t *testing.T, pool *pgxpool.Pool, id TaskID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
		t.Fatalf("force due: %v", err)
	}
}

// TestFailurePolicy_oneShotAttemptsGrowAndGiveUp is AC9: driven attempt
// by attempt (each cycle preceded by making the row due), each persisted
// run_at falls within the exact backoff bracket computed around the
// settlement instant, the exact attempt count at give-up equals the
// configured cap, the state is terminal, and a subsequent discovery does
// not return it.
func TestFailurePolicy_oneShotAttemptsGrowAndGiveUp(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := backoffProbeConfig()
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: &slowFirstAttemptHandler{firstDelay: 400 * time.Millisecond, err: errBoom}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for attempt := 1; attempt <= cfg.RetryMaxAttempts; attempt++ {
		before := forceDueAndCapture(t, pool, id)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce attempt %d: %v", attempt, err)
		}
		after := captureNow(t, pool)

		state, failures, lastError, runAt, found := schedulerTaskRow(t, pool, id)
		if attempt < cfg.RetryMaxAttempts {
			if !found || state != "pending" {
				t.Fatalf("attempt %d: state = %v (found=%v), want pending", attempt, state, found)
			}
			if failures != attempt {
				t.Fatalf("attempt %d: consecutive_failures = %d, want %d", attempt, failures, attempt)
			}
			// The settlement instant s (settle.go's clock_timestamp() read,
			// after the handler returned) satisfies before <= s <= after by
			// construction: all three are server clock reads, and before and
			// after bracket the RunOnce call that reads s. run_at = s +
			// backoff(attempt), so it must land in
			// [before+backoff, after+backoff] -- an exact bracket, not merely
			// a "grew" check, and independent of how long RunOnce itself
			// takes to execute.
			want := backoff(attempt, cfg.RetryBaseDelay, cfg.RetryMaxDelay)
			lo, hi := before.Add(want), after.Add(want)
			if runAt.Before(lo) || runAt.After(hi) {
				t.Fatalf("attempt %d: run_at = %v, want within [%v, %v] (before=%v after=%v backoff=%v)", attempt, runAt, lo, hi, before, after, want)
			}
			if lastError == nil || !strings.Contains(*lastError, "boom") {
				t.Fatalf("attempt %d: last_error = %v, want it to mention the handler error", attempt, lastError)
			}
		} else {
			if !found || state != "dead" {
				t.Fatalf("final attempt: state = %v (found=%v), want dead", state, found)
			}
			if failures != cfg.RetryMaxAttempts {
				t.Fatalf("final attempt: consecutive_failures = %d, want %d", failures, cfg.RetryMaxAttempts)
			}
		}
	}

	// A subsequent discovery must not return the dead row, even when
	// forced due.
	forceDue(t, pool, id)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce after give-up: %v", err)
	}
	state, failures, _, _, found := schedulerTaskRow(t, pool, id)
	if !found || state != "dead" || failures != cfg.RetryMaxAttempts {
		t.Fatalf("row after a further cycle = state:%v failures:%d (found=%v), want it untouched at dead/%d", state, failures, found, cfg.RetryMaxAttempts)
	}
}

// forceDueAndCapture forces id due and returns the server's now() at that
// moment, so callers can compare a later run_at against it.
func forceDueAndCapture(t *testing.T, pool *pgxpool.Pool, id TaskID) time.Time {
	t.Helper()
	var now time.Time
	if err := pool.QueryRow(context.Background(),
		`UPDATE scheduled_task SET run_at = now() WHERE id = $1 RETURNING run_at`, int64(id),
	).Scan(&now); err != nil {
		t.Fatalf("force due: %v", err)
	}
	return now
}

// TestFailurePolicy_decodeFailure is AC26.
func TestFailurePolicy_decodeFailure(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: decodeFailHandler{}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot", Payload: json.RawMessage(`{"not":"an int"}`)})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for attempt := 1; attempt <= cfg.RetryMaxAttempts; attempt++ {
		forceDue(t, pool, id)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce attempt %d: %v", attempt, err)
		}
	}

	state, failures, lastError, _, found := schedulerTaskRow(t, pool, id)
	if !found || state != "dead" || failures != cfg.RetryMaxAttempts {
		t.Fatalf("row after cap decode failures = state:%v failures:%d (found=%v), want dead/%d", state, failures, found, cfg.RetryMaxAttempts)
	}
	if lastError == nil || !strings.Contains(*lastError, "json") && !strings.Contains(*lastError, "cannot unmarshal") {
		t.Fatalf("last_error = %v, want it to record the decode failure", lastError)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	dead, err := DeadTasks(ctx, tx, 10)
	if err != nil {
		t.Fatalf("DeadTasks: %v", err)
	}
	found = false
	for _, d := range dead {
		if d.Type == "test.oneshot" {
			found = true
			if d.LastError == "" {
				t.Fatalf("DeadTasks row has empty LastError, want the decode message")
			}
		}
	}
	if !found {
		t.Fatalf("DeadTasks did not enumerate the given-up decode-failure row")
	}
}

// TestFailurePolicy_recurrenceNeverTerminal is AC32: a recurrence whose
// handler fails more times than the one-shot cap stays pending, still
// advances by its cadence, and never becomes terminal.
func TestFailurePolicy_recurrenceNeverTerminal(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()
	period := 20 * time.Millisecond
	reg, err := NewRegistry(Declaration{
		Type: "test.recurrent", Handler: &alwaysFailHandler{err: errBoom},
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

	attempts := cfg.RetryMaxAttempts + 3 // strictly more than the one-shot cap
	var prevRunAt time.Time
	for attempt := 1; attempt <= attempts; attempt++ {
		forceDue(t, pool, id)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce attempt %d: %v", attempt, err)
		}
		state, failures, _, runAt, found := schedulerTaskRow(t, pool, id)
		if !found || state != "pending" {
			t.Fatalf("attempt %d: state = %v (found=%v), want pending (a recurrence never becomes dead)", attempt, state, found)
		}
		if failures != attempt {
			t.Fatalf("attempt %d: consecutive_failures = %d, want %d", attempt, failures, attempt)
		}
		if !prevRunAt.IsZero() && !runAt.After(prevRunAt) {
			t.Fatalf("attempt %d: run_at %v did not advance past the previous %v", attempt, runAt, prevRunAt)
		}
		prevRunAt = runAt
	}
}

// TestFailurePolicy_counterRisesAndResets is AC34: the counter rises
// across successive failures and returns to zero after a success, and
// after a no-op.
func TestFailurePolicy_counterRisesAndResets(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()
	h := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "counter-rises"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Two failures, then a success.
	for attempt := 1; attempt <= 2; attempt++ {
		forceDue(t, pool, id)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		_, failures, _, _, found := schedulerTaskRow(t, pool, id)
		if !found || failures != attempt {
			t.Fatalf("after failure %d: consecutive_failures = %d (found=%v), want %d", attempt, failures, found, attempt)
		}
	}

	h.mu.Lock()
	h.outcome = OutcomeDone
	h.err = nil
	h.mu.Unlock()
	forceDue(t, pool, id)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (success): %v", err)
	}
	if _, _, _, _, found := schedulerTaskRow(t, pool, id); found {
		t.Fatalf("one-shot row still present after success, want it deleted")
	}

	// A recurrence resets to zero after a no-op too.
	recH := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "counter-resets-recurrent"}
	reg2, err := NewRegistry(Declaration{
		Type: "test.recurrent", Handler: recH,
		Recurrence: &Recurrence{Cadence: Every(time.Hour), ConfigKey: "k"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id2 := dueNow(t, pool, reg2, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"})
	w2, err := New(Options{Pool: pool, Registry: reg2, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w2.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (recurrent failure): %v", err)
	}
	if _, failures, _, _, found := schedulerTaskRow(t, pool, id2); !found || failures != 1 {
		t.Fatalf("recurrent row after one failure: failures=%d found=%v, want 1/true", failures, found)
	}

	recH.mu.Lock()
	recH.outcome = OutcomeNoop
	recH.err = nil
	recH.mu.Unlock()
	forceDue(t, pool, id2)
	if err := w2.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (recurrent no-op): %v", err)
	}
	if _, failures, lastError, _, found := schedulerTaskRow(t, pool, id2); !found || failures != 0 || lastError != nil {
		t.Fatalf("recurrent row after a no-op: failures=%d lastError=%v found=%v, want 0/nil/true", failures, lastError, found)
	}
}

// TestFailurePolicy_undeclaredType_keepsComingDue is AC21's execution-time
// half: an undeclared row's refusal repeats at the configured ceiling
// rather than stopping — driven more times than the one-shot cap, it
// stays pending with its starting failure count untouched.
func TestFailurePolicy_undeclaredType_keepsComingDue(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()
	// An empty registry: "undeclared.type" has no Declaration.
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: &alwaysFailHandler{err: errBoom}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at) VALUES ('undeclared.type', 'u1', '{}'::jsonb, now()) RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatalf("seed undeclared row: %v", err)
	}
	taskID := TaskID(id)

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	attempts := cfg.RetryMaxAttempts + 2
	var prevRunAt time.Time
	for i := 1; i <= attempts; i++ {
		forceDue(t, pool, taskID)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
		state, failures, lastError, runAt, found := schedulerTaskRow(t, pool, taskID)
		if !found || state != "pending" {
			t.Fatalf("attempt %d: state = %v (found=%v), want pending (an undeclared row never reaches dead)", i, state, found)
		}
		if failures != 0 {
			t.Fatalf("attempt %d: consecutive_failures = %d, want 0 (untouched)", i, failures)
		}
		if lastError != nil {
			t.Fatalf("attempt %d: last_error = %v, want nil (untouched)", i, *lastError)
		}
		if !prevRunAt.IsZero() && !runAt.After(prevRunAt) {
			t.Fatalf("attempt %d: run_at %v did not advance past the previous %v", i, runAt, prevRunAt)
		}
		prevRunAt = runAt
	}
}
