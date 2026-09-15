package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/storetest"
)

// sqlstateTerminatingConnection is the SQLSTATE Postgres reports when
// pg_terminate_backend ends the statement in flight: "terminating
// connection due to administrator command".
const sqlstateTerminatingConnection = "57P01"

// reclaimFixtureHandler is the whole instrument for the reclaim suite: it
// neutralises the transaction's own statement_timeout before issuing a
// long sleep on a context of its own, so only a terminated backend — never
// the server's per-statement timeout, and never ctx cancellation — can end
// that sleep. Without the neutralisation, statement_timeout would end the
// sleep on its own well before any terminate, and either scenario below
// would pass with the terminate removed: a green instrument measuring
// nothing.
type reclaimFixtureHandler struct {
	// blockAfterSleep selects the variant: true blocks on release after
	// the sleep errors, demonstrating the handler has not returned;
	// false returns immediately after recording sleepErr.
	blockAfterSleep bool
	release         chan struct{}
	// returned is closed just before Execute returns, so a test can
	// assert the handler demonstrably has, or has not, returned yet.
	returned chan struct{}

	mu       sync.Mutex
	pid      uint32
	sleepErr error
}

func (h *reclaimFixtureHandler) Execute(ctx context.Context, tx pgx.Tx, _ Task) (Outcome, error) {
	defer close(h.returned)

	var pid uint32
	if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		return OutcomeFailed, err
	}
	h.mu.Lock()
	h.pid = pid
	h.mu.Unlock()

	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
		return OutcomeFailed, err
	}

	_, sleepErr := tx.Exec(context.Background(), "SELECT pg_sleep(30)") //nolint:contextcheck // deliberate: the fixture issues this on a context of its own so only the terminate ends it, never ctx's cancellation
	h.mu.Lock()
	h.sleepErr = sleepErr
	h.mu.Unlock()

	if h.blockAfterSleep {
		<-h.release
	}
	return OutcomeFailed, sleepErr
}

// backendPID and lastSleepErr read h's fields behind its mutex, for a
// test to inspect after Execute has run.
func (h *reclaimFixtureHandler) backendPID() uint32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pid
}

func (h *reclaimFixtureHandler) lastSleepErr() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sleepErr
}

// waitBackendGone polls pg_stat_activity until pid is absent or timeout
// elapses — the connection-released assertion, exact rather than a fixed
// sleep, since the backend's death is asynchronous to the terminate call
// by construction.
func waitBackendGone(t *testing.T, pool *pgxpool.Pool, pid uint32, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE pid = $1`, int64(pid)).Scan(&n); err != nil {
			t.Fatalf("count pg_stat_activity: %v", err)
		}
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("backend pid %d still present in pg_stat_activity after %v", pid, timeout)
}

// TestReclaim_AC7_rowClaimableWithoutWaitingForHandlerReturn asserts the
// pair that "claimable without waiting for the handler to return"
// actually states: after RunOnce returns, a claim on the breached row
// succeeds within a bound far below the fixture's own sleep, AND the
// handler has demonstrably not returned yet. Without the second half
// this would also pass once the handler eventually returns on its own,
// which is a weaker claim.
func TestReclaim_AC7_rowClaimableWithoutWaitingForHandlerReturn(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()

	h := &reclaimFixtureHandler{
		blockAfterSleep: true,
		release:         make(chan struct{}),
		returned:        make(chan struct{}),
	}
	defer close(h.release)

	reg, err := NewRegistry(Declaration{Type: "test.reclaim.ac7", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "test.reclaim.ac7"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// The bound is an instrument, far below the fixture's own 30s sleep,
	// using the package's existing lock-free poll helper rather than a
	// bespoke probe shape.
	waitLockFree(t, pool, id, 2*time.Second)

	select {
	case <-h.returned:
		t.Fatal("handler already returned by the time the claim succeeded — want it still blocked on release, proving the claim did not wait for it")
	default:
	}

	tasks := obs.Tasks()
	if len(tasks) != 1 || tasks[0].Failure != FailureDeadline {
		t.Fatalf("observations = %+v, want one FailureDeadline observation, unchanged", tasks)
	}
}

// TestReclaim_AC8_handlerReturnsAfterTerminate asserts that a handler
// blocked in the database returns promptly once its backend is
// terminated: the error it recorded carries the terminate's own SQLSTATE
// (57P01), distinguished from a context cancellation — the error the
// pre-change tree would produce here — and the backend is gone from
// pg_stat_activity, the connection-released assertion in a form the
// test can make exactly.
func TestReclaim_AC8_handlerReturnsAfterTerminate(t *testing.T) {
	t.Parallel()

	pool := storetest.Pool(t)
	ctx := context.Background()
	cfg := shortDeadlineConfig()

	h := &reclaimFixtureHandler{
		blockAfterSleep: false,
		returned:        make(chan struct{}),
	}

	reg, err := NewRegistry(Declaration{Type: "test.reclaim.ac8", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	dueNow(t, pool, reg, Request{Type: "test.reclaim.ac8"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	select {
	case <-h.returned:
	case <-time.After(2 * time.Second):
		t.Fatal("handler's Execute did not return within the bound, want the terminate to have released it")
	}

	sleepErr := h.lastSleepErr()
	var pgErr *pgconn.PgError
	if !errors.As(sleepErr, &pgErr) || pgErr.Code != sqlstateTerminatingConnection {
		t.Fatalf("sleep error = %v, want a *pgconn.PgError with SQLSTATE %s", sleepErr, sqlstateTerminatingConnection)
	}
	if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
		t.Fatalf("sleep error = %v, want the terminate's own class, never a context cancellation", sleepErr)
	}

	waitBackendGone(t, pool, h.backendPID(), 2*time.Second)

	tasks := obs.Tasks()
	if len(tasks) != 1 || tasks[0].Failure != FailureDeadline {
		t.Fatalf("observations = %+v, want one FailureDeadline observation, unchanged", tasks)
	}
}
