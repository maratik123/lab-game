package ingest

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"
	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// writeAndFailHandler posts a trivial balanced batch under u's canonical
// operation_id, then returns err unconditionally — the fixture that lets
// a failure/retry scenario assert an attempt's writes do not survive its
// rollback (design D24), rather than merely that the handler ran.
type writeAndFailHandler struct{ err error }

func (h writeAndFailHandler) Handle(ctx context.Context, tx pgx.Tx, u Update) error {
	if err := store.Post(ctx, tx,
		&store.PlayerOperation{Source: store.SourceTelegram, OperationID: u.OperationID},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.RequireFromString("1")},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.RequireFromString("-1")},
	); err != nil {
		return err
	}
	return h.err
}

// panickingHandler always panics — design D7/D10's fixture.
type panickingHandler struct{}

func (panickingHandler) Handle(context.Context, pgx.Tx, Update) error {
	panic("boom")
}

func TestLoop_failureAndRetryGivesUp(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 20, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	wantErr := errors.New("boom")
	router, err := NewRouter(Route{Kind: KindMessage, Handler: writeAndFailHandler{err: wantErr}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	cfg := testIngestConfig()
	if got := countRows(t, pool, "player_operation"); got != 0 {
		t.Errorf("player_operation rows after give-up = %d, want 0 (every attempt's writes were rolled back)", got)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	next, err := readOffset(context.Background(), tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if next != 21 {
		t.Errorf("offset after give-up on update_id=20 = %d, want 21 (the offset still advances)", next)
	}

	dead, err := DeadUpdates(context.Background(), tx, 10)
	if err != nil {
		t.Fatalf("DeadUpdates: %v", err)
	}
	if len(dead) != 1 {
		t.Fatalf("DeadUpdates = %+v, want exactly one row", dead)
	}
	if dead[0].UpdateID != 20 || dead[0].ConsecutiveFailures != cfg.RetryMaxAttempts || dead[0].LastError != wantErr.Error() {
		t.Errorf("dead row = %+v, want UpdateID=20 ConsecutiveFailures=%d LastError=%q", dead[0], cfg.RetryMaxAttempts, wantErr.Error())
	}

	var failedCount, givenUpCount int
	for _, o := range rec.Updates() {
		switch o.Outcome {
		case OutcomeFailed:
			failedCount++
		case OutcomeGivenUp:
			givenUpCount++
		default:
		}
	}
	if failedCount != cfg.RetryMaxAttempts {
		t.Errorf("OutcomeFailed observations = %d, want %d", failedCount, cfg.RetryMaxAttempts)
	}
	if givenUpCount != 1 {
		t.Errorf("OutcomeGivenUp observations = %d, want 1", givenUpCount)
	}
}

func TestLoop_panicIsRecoveredAndRetried(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 25, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: panickingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	// The whole point: a panicking Handler must never bring down the test
	// process (design D7).
	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	cfg := testIngestConfig()
	var panicCount, givenUpCount, failedCount int
	for _, o := range rec.Updates() {
		switch o.Outcome {
		case OutcomePanic:
			panicCount++
		case OutcomeGivenUp:
			givenUpCount++
		case OutcomeFailed:
			failedCount++
		default:
		}
	}
	if panicCount != cfg.RetryMaxAttempts {
		t.Errorf("OutcomePanic observations = %d, want %d", panicCount, cfg.RetryMaxAttempts)
	}
	if failedCount != 0 {
		t.Errorf("OutcomeFailed observations = %d, want 0 (a panic is reported distinctly)", failedCount)
	}
	if givenUpCount != 1 {
		t.Errorf("OutcomeGivenUp observations = %d, want 1", givenUpCount)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	dead, err := DeadUpdates(context.Background(), tx, 10)
	if err != nil {
		t.Fatalf("DeadUpdates: %v", err)
	}
	if len(dead) != 1 || dead[0].UpdateID != 25 {
		t.Fatalf("DeadUpdates = %+v, want exactly one row for update_id=25", dead)
	}
}

func TestRun_pollFailureDoesNotStopTheLoop(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}

	const failCycles = 3
	var callCount atomic.Int32
	raw := telego.Update{UpdateID: 30, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}

	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= failCycles {
			tgtest.ServerError(http.StatusInternalServerError)(w, r)
			return
		}
		tgtest.Success(updatesJSON(t, []telego.Update{raw}))(w, r)
	})

	router, err := NewRouter(Route{Kind: KindMessage, Handler: postingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()

	// Wait for at least one successful cycle beyond the failing ones —
	// proof Run kept polling through every failure instead of exiting.
	waitStart := time.Now()
	deadline := waitStart.Add(5 * time.Second)
	for callCount.Load() <= failCycles+1 {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("timed out waiting for the loop to recover from poll failures")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Not a busy loop: failCycles failing cycles paced at PollInterval
	// take at least (failCycles-1)*PollInterval of wall time. A
	// continue-with-no-wait would blow through this floor instantly.
	elapsed := time.Since(waitStart)
	minElapsed := time.Duration(failCycles-1) * testIngestConfig().PollInterval
	if elapsed < minElapsed {
		t.Errorf("elapsed = %v reaching cycle %d, want at least %v (the poll interval must pace the failing cycles)", elapsed, failCycles+1, minElapsed)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly after cancellation")
	}

	var pollErrs int
	for _, lo := range rec.Loops() {
		if lo.Err != nil {
			pollErrs++
		}
	}
	if pollErrs < failCycles {
		t.Errorf("LoopObservations with Err != nil = %d, want at least %d", pollErrs, failCycles)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	next, err := readOffset(context.Background(), tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if next != 31 {
		t.Errorf("offset after the recovering cycle = %d, want 31 (update_id 30 settled once the poll succeeded)", next)
	}
}

func TestRun_cancellationLeavesTheUpdateUnsettled(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 40, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	cfg := testIngestConfig()
	cfg.RetryBaseDelay = 300 * time.Millisecond
	cfg.RetryMaxDelay = 500 * time.Millisecond
	cfg.RetryMaxAttempts = 5

	router, err := NewRouter(Route{Kind: KindMessage, Handler: writeAndFailHandler{err: errors.New("boom")}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l, err := New(Options{Client: newTestClient(t, srv), Pool: pool, Router: router, Config: cfg, Observer: rec})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()

	// Let the first attempt fail and enter its backoff wait, then cancel
	// mid-wait (design D25).
	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly after cancellation")
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	next, err := readOffset(context.Background(), tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if next != 0 {
		t.Errorf("offset after a mid-retry cancellation = %d, want unchanged 0 (the update never settled)", next)
	}
	if got := countRows(t, pool, "player_operation"); got != 0 {
		t.Errorf("player_operation rows = %d, want 0 (the last attempt's writes were rolled back)", got)
	}
}

func TestPollOnce_cancellationDuringLongPollReturnsPromptly(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	srv := tgtest.New(t, tgtest.Delayed(5*time.Second, tgtest.Success(updatesJSON(t, nil))))

	router, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err = l.PollOnce(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("PollOnce: want an error from a cancelled long poll, got nil")
	}
	if elapsed > time.Second {
		t.Errorf("PollOnce took %v to return after cancellation, want a prompt return", elapsed)
	}
}
