package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/storetest"
)

// panickingHandler always panics with a fixed, recognisable value.
type panickingHandler struct {
	value any
}

func (h *panickingHandler) Execute(_ context.Context, _ pgx.Tx, _ Task) (Outcome, error) {
	panic(h.value)
}

// panicOnceThenSucceedsHandler panics on its first call and reports
// OutcomeDone on every call after that.
type panicOnceThenSucceedsHandler struct {
	mu    sync.Mutex
	calls int
}

func (h *panicOnceThenSucceedsHandler) Execute(_ context.Context, _ pgx.Tx, _ Task) (Outcome, error) {
	h.mu.Lock()
	h.calls++
	first := h.calls == 1
	h.mu.Unlock()
	if first {
		panic("panicOnceThenSucceedsHandler: first-call panic")
	}
	return OutcomeDone, nil
}

// openRowsThenPanicHandler issues a query, deliberately never reads nor
// closes its returned rows, and then panics — the unusable-transaction
// fixture: a busy connection is what every later statement on tx (the
// savepoint rollback included) then fails against.
type openRowsThenPanicHandler struct {
	value any
}

func (h *openRowsThenPanicHandler) Execute(ctx context.Context, tx pgx.Tx, _ Task) (Outcome, error) {
	if _, err := tx.Query(ctx, "SELECT generate_series(1, 5)"); err != nil { //nolint:sqlclosecheck // deliberate: the rows are left open and unread so the panic below leaves the connection busy, which is exactly the fixture this test needs
		return OutcomeFailed, err
	}
	panic(h.value)
}

// blockThenPanicHandler blocks on release, ignoring ctx, then panics once
// released — the log-only surface fixture: released after its deadline
// has already been breached and RunOnce has already returned, so this
// goroutine is an orphan whose recovered panic has no settlement of its
// own to ride.
type blockThenPanicHandler struct {
	release chan struct{}
}

func (h *blockThenPanicHandler) Execute(_ context.Context, _ pgx.Tx, _ Task) (Outcome, error) {
	<-h.release
	panic("blockThenPanicHandler: panicked after release")
}

// recordedLogEntry is one Handle call's message, level and attributes,
// captured eagerly so the underlying slog.Record need not survive past
// Handle itself.
type recordedLogEntry struct {
	message string
	level   slog.Level
	attrs   map[string]string
}

// recordingLogHandler is a slog.Handler test double collecting every
// record behind a mutex, since the handler goroutine and the test
// goroutine both touch it.
type recordingLogHandler struct {
	mu      sync.Mutex
	entries []recordedLogEntry
}

func (h *recordingLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingLogHandler) Handle(_ context.Context, r slog.Record) error {
	entry := recordedLogEntry{message: r.Message, level: r.Level, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		entry.attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	h.entries = append(h.entries, entry)
	h.mu.Unlock()
	return nil
}

func (h *recordingLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingLogHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingLogHandler) Entries() []recordedLogEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]recordedLogEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

// findLogEntry returns the first entry in logs whose message matches,
// and whether one was found.
func findLogEntry(logs *recordingLogHandler, message string) (recordedLogEntry, bool) {
	for _, e := range logs.Entries() {
		if e.message == message {
			return e, true
		}
	}
	return recordedLogEntry{}, false
}

// waitForPanicLogEntry polls logs until it holds a "scheduler: recovered
// handler panic" entry or timeout elapses, failing the test on timeout —
// the log record is emitted from a goroutine the test does not otherwise
// synchronise with.
func waitForPanicLogEntry(t *testing.T, logs *recordingLogHandler, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, found := findLogEntry(logs, "scheduler: recovered handler panic"); found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("log entries = %+v after %v, want a recovered-handler-panic record", logs.Entries(), timeout)
}

// TestPanic_AC1_workerSurvivesAndNextCycleRunsNormally asserts that a
// panicking handler never ends the process, and the same worker's next
// cycle claims and runs a second, unrelated due task normally.
func TestPanic_AC1_workerSurvivesAndNextCycleRunsNormally(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := testConfig()

	panicker := &panickingHandler{value: "boom-ac1"}
	second := &fixedOutcomeHandler{outcome: OutcomeDone}
	reg, err := NewRegistry(
		Declaration{Type: "test.panic.ac1", Handler: panicker},
		Declaration{Type: "test.ok.ac1", Handler: second},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	dueNow(t, pool, reg, Request{Type: "test.panic.ac1"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (panicking task): %v", err)
	}

	dueNow(t, pool, reg, Request{Type: "test.ok.ac1"})
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (next cycle): %v", err)
	}
	second.mu.Lock()
	seen := len(second.seen)
	second.mu.Unlock()
	if seen != 1 {
		t.Fatalf("second handler seen = %d calls, want 1 — the worker's next cycle must run normally after a panic", seen)
	}
}

// TestPanic_AC2_oneShotAlwaysPanickingGivesUpWithinCap mirrors the
// existing deadline give-up test's shape: a one-shot whose handler
// panics on every attempt reaches the dead state within the configured
// attempt cap.
func TestPanic_AC2_oneShotAlwaysPanickingGivesUpWithinCap(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := testConfig()

	panicker := &panickingHandler{value: "boom-ac2"}
	reg, err := NewRegistry(Declaration{Type: "test.panic.ac2", Handler: panicker})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.panic.ac2"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for attempt := 1; attempt <= cfg.RetryMaxAttempts; attempt++ {
		if attempt > 1 {
			if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
				t.Fatalf("force due: %v", err)
			}
		}
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce (attempt %d): %v", attempt, err)
		}
	}

	state, failures, _, _, found := schedulerTaskRow(t, pool, id)
	if !found || state != "dead" || failures != cfg.RetryMaxAttempts {
		t.Fatalf("final row = state:%v failures:%d (found=%v), want dead/%d", state, failures, found, cfg.RetryMaxAttempts)
	}
}

// TestPanic_AC3_observationCarriesPanicFailureKind asserts that the
// panicking attempt's observation carries the panic failure kind, and a
// sibling case whose handler returns an error carries the handler
// failure kind — asserted apart, not one in isolation. It also covers
// the outranking rule as a sub-case: a handler that panics AND whose
// savepoint rollback succeeds still classifies as a panic, never as
// rolled-back.
func TestPanic_AC3_observationCarriesPanicFailureKind(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := testConfig()

	panicker := &panickingHandler{value: "boom-ac3"}
	erroring := &fixedOutcomeHandler{outcome: OutcomeFailed, err: errBoom}
	reg, err := NewRegistry(
		Declaration{Type: "test.panic.ac3", Handler: panicker},
		Declaration{Type: "test.err.ac3", Handler: erroring},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	dueNow(t, pool, reg, Request{Type: "test.panic.ac3"})
	dueNow(t, pool, reg, Request{Type: "test.err.ac3"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	tasks := obs.Tasks()
	panicObs, panicFound := findObservationByType(tasks, "test.panic.ac3")
	errObs, errFound := findObservationByType(tasks, "test.err.ac3")
	if len(tasks) != 2 || !panicFound || !errFound {
		t.Fatalf("observations = %+v, want exactly one test.panic.ac3 and one test.err.ac3", tasks)
	}
	if panicObs.Outcome != OutcomeFailed || panicObs.Failure != FailurePanic {
		t.Errorf("panic observation = %+v, want Failed/FailurePanic", panicObs)
	}
	if errObs.Outcome != OutcomeFailed || errObs.Failure != FailureHandler {
		t.Errorf("returned-error observation = %+v, want Failed/FailureHandler", errObs)
	}
}

// TestPanic_AC6_rowAndLogBothCarryStack asserts the row surface: the
// task row's last_error contains both the panic value and a frame
// naming the panicking fixture, and the recording logger holds a record
// whose stack attribute contains the same frame.
func TestPanic_AC6_rowAndLogBothCarryStack(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := testConfig()

	panicker := &panickingHandler{value: "boom-ac6-marker"}
	reg, err := NewRegistry(Declaration{Type: "test.panic.ac6", Handler: panicker})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.panic.ac6"})

	logs := &recordingLogHandler{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Logger: slog.New(logs)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	_, failures, lastError, _, found := schedulerTaskRow(t, pool, id)
	if !found || failures != 1 {
		t.Fatalf("row = failures:%d (found=%v), want 1", failures, found)
	}
	if lastError == nil || !strings.Contains(*lastError, "boom-ac6-marker") || !strings.Contains(*lastError, "panickingHandler") {
		t.Fatalf("last_error = %s, want it to contain the panic value and a frame naming panickingHandler", errText(lastError))
	}

	entries := logs.Entries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want exactly 1", len(entries))
	}
	if entries[0].level != slog.LevelError {
		t.Errorf("log level = %v, want error", entries[0].level)
	}
	if !strings.Contains(entries[0].attrs["stack"], "panickingHandler") {
		t.Errorf("log stack attribute = %q, want it to name panickingHandler", entries[0].attrs["stack"])
	}
}

// TestPanic_AC6_logOnlySurface_blockedPastDeadlineThenPanics asserts the
// narrowed log-only clause: a handler that blocks past its deadline and
// then panics, released only after RunOnce has already returned. The
// recording logger holds a record naming the panicking fixture, while
// the row — after the next cycle's drain — records the deadline breach
// that already settled the attempt, not the panic. RetryMaxAttempts is
// pinned to 1 so the drain's give-up branch always fires and leaves the
// row in "dead": without that cap, the drain's own re-armed run_at could
// still be in the past by the time the second RunOnce discovers pending
// rows, letting a legitimate retry re-claim and re-settle the row before
// the assertions run. The property under test is that the orphaned panic
// gets no settlement of its own, not the wall-clock race between the
// drain and the next discovery — an attempt budget of one removes the
// race instead of assuming it favors this test's ordering.
func TestPanic_AC6_logOnlySurface_blockedPastDeadlineThenPanics(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()
	cfg.RetryMaxAttempts = 1

	h := &blockThenPanicHandler{release: make(chan struct{})}
	reg, err := NewRegistry(Declaration{Type: "test.panic.ac6log", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.panic.ac6log"})

	logs := &recordingLogHandler{}
	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs, Logger: slog.New(logs)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (breach): %v", err)
	}

	tasks := obs.Tasks()
	if len(tasks) != 1 || tasks[0].Failure != FailureDeadline {
		t.Fatalf("observations = %+v, want one FailureDeadline observation", tasks)
	}

	close(h.release)
	waitLockFree(t, pool, id, 2*cfg.TaskTimeout+10*time.Second)
	waitForPanicLogEntry(t, logs, 10*time.Second)

	// The breach branch's own terminate may or may not itself log,
	// depending on which of the two clocks armed against the same
	// TaskTimeout reaches this backend first — this handler never
	// touches tx, so it is idle in transaction from right after the
	// savepoint statement, and the server's own
	// idle_in_transaction_session_timeout can beat the terminate to it.
	// Either ordering is benign and expected; the assertion below is on
	// the panic's own record, not on the total count.
	entry, found := findLogEntry(logs, "scheduler: recovered handler panic")
	if !found {
		t.Fatalf("log entries = %+v, want a recovered-handler-panic record", logs.Entries())
	}
	if !strings.Contains(entry.attrs["stack"], "blockThenPanicHandler") {
		t.Errorf("log stack attribute = %q, want it to name blockThenPanicHandler", entry.attrs["stack"])
	}

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain): %v", err)
	}
	state, failures, lastError, _, found := schedulerTaskRow(t, pool, id)
	if !found || state != "dead" || failures != 1 {
		t.Fatalf("row after drain: state=%q failures=%d (found=%v), want dead/1", state, failures, found)
	}
	if lastError == nil || !strings.Contains(*lastError, "deadline") {
		t.Fatalf("last_error = %s, want it to mention the deadline breach that settled this attempt, not the later panic", errText(lastError))
	}
}

// TestPanic_D4_rowsLeftOpen_unusableTransactionStillSettlesThroughDrain
// asserts that a handler that opens rows, leaves them open and panics
// leaves the connection unusable, so no statement on that transaction —
// including the savepoint's own rollback — can apply. The attempt is
// still counted (through the deferred drain), the row becomes claimable
// again, RunOnce returns no error, the observation carries the panic
// failure kind (not deadline or rolled-back), and the drain's last_error
// carries the panic value and a frame naming the fixture — the only
// route the stack has to the row on this branch.
func TestPanic_D4_rowsLeftOpen_unusableTransactionStillSettlesThroughDrain(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := testConfig()

	h := &openRowsThenPanicHandler{value: "boom-d4-marker"}
	reg, err := NewRegistry(Declaration{Type: "test.panic.d4", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.panic.d4"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (panic with rows open): %v", err)
	}

	tasks := obs.Tasks()
	if len(tasks) != 1 || tasks[0].Outcome != OutcomeFailed || tasks[0].Failure != FailurePanic {
		t.Fatalf("observations = %+v, want one Failed/FailurePanic observation", tasks)
	}

	// The transaction was unusable — nothing committed inline — so the
	// attempt is not yet visible on the row until the deferred drain
	// applies it, and the row is not claimable until pgxpool has
	// destroyed the busy connection on release, an eventual property of
	// its own goroutine rather than an instantaneous one.
	waitLockFree(t, pool, id, 10*time.Second)

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (drain): %v", err)
	}
	_, failures, lastError, _, found := schedulerTaskRow(t, pool, id)
	if !found || failures != 1 {
		t.Fatalf("row after drain: failures=%d (found=%v), want 1", failures, found)
	}
	if lastError == nil || !strings.Contains(*lastError, "boom-d4-marker") || !strings.Contains(*lastError, "openRowsThenPanicHandler") {
		t.Fatalf("last_error = %s, want it to contain the panic value and a frame naming openRowsThenPanicHandler", errText(lastError))
	}
}

// TestPanic_recoversOnRetryAfterOnePanic is a supplementary regression
// guard, not tied to one AC number: a one-shot handler that panics once
// and succeeds on its retry is counted as a failed attempt and then
// completes normally, so a recovered panic never leaves the task
// permanently stuck.
func TestPanic_recoversOnRetryAfterOnePanic(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := testConfig()

	h := &panicOnceThenSucceedsHandler{}
	reg, err := NewRegistry(Declaration{Type: "test.panic.recover", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.panic.recover"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (first attempt, panics): %v", err)
	}
	if _, failures, _, _, found := schedulerTaskRow(t, pool, id); !found || failures != 1 {
		t.Fatalf("row after first attempt: failures=%d (found=%v), want 1", failures, found)
	}

	if _, err := pool.Exec(ctx, `UPDATE scheduled_task SET run_at = now() WHERE id = $1`, int64(id)); err != nil {
		t.Fatalf("force due: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce (second attempt, succeeds): %v", err)
	}
	if _, _, _, _, found := schedulerTaskRow(t, pool, id); found {
		t.Fatalf("row still present after handler succeeded on retry, want it deleted (Done, one-shot)")
	}
}
