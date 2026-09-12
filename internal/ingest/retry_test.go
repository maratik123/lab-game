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
// rollback, rather than merely that the handler ran.
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

// panickingHandler always panics — a fixture for the panic-recovery path.
type panickingHandler struct{}

func (panickingHandler) Handle(context.Context, pgx.Tx, Update) error {
	panic("boom")
}

// txClosingHandler rolls its own attempt's transaction back and returns
// nil — a fixture for the handlerErr==nil/advanceOffset
// failure branch: attemptOnce's subsequent advanceOffset call runs
// against an already-closed tx and fails with pgx.ErrTxClosed.
type txClosingHandler struct{}

func (txClosingHandler) Handle(ctx context.Context, tx pgx.Tx, _ Update) error {
	_ = tx.Rollback(ctx)
	return nil
}

// deferredConstraintHandler returns nil having inserted two rows that
// violate a UNIQUE ... DEFERRABLE INITIALLY DEFERRED constraint on a
// temp table it creates itself — a fixture for the
// handlerErr==nil/Commit failure branch: the violation passes every
// statement inside the transaction (the check is deferred) and is
// caught only when tx.Commit runs the deferred check.
type deferredConstraintHandler struct{}

func (deferredConstraintHandler) Handle(ctx context.Context, tx pgx.Tx, _ Update) error {
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE r15_commit_probe (x int, CONSTRAINT r15_commit_probe_x_key UNIQUE (x) DEFERRABLE INITIALLY DEFERRED) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO r15_commit_probe (x) VALUES (1), (1)`); err != nil {
		return err
	}
	return nil
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
	if dead[0].Kind != KindMessage {
		t.Errorf("dead[0].Kind = %v, want %v (derived, not hardcoded)", dead[0].Kind, KindMessage)
	}
	if dead[0].ChatID == nil || *dead[0].ChatID != 1 {
		t.Errorf("dead[0].ChatID = %v, want a pointer to 1 (derived from the update's Chat)", dead[0].ChatID)
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
	// process.
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

// TestLoop_advanceOffsetFailureReportsFailed covers the gap where
// attemptOnce's handlerErr==nil branch must report OutcomeFailed when
// advanceOffset itself fails, not silently count the attempt without an
// observation.
func TestLoop_advanceOffsetFailureReportsFailed(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 60, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: txClosingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	cfg := testIngestConfig()
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
		t.Errorf("OutcomeFailed observations = %d, want %d (every attempt's advanceOffset failure must be reported)", failedCount, cfg.RetryMaxAttempts)
	}
	if givenUpCount != 1 {
		t.Errorf("OutcomeGivenUp observations = %d, want 1", givenUpCount)
	}
}

// TestLoop_commitFailureReportsFailed covers the companion gap where
// attemptOnce's handlerErr==nil branch must report OutcomeFailed when
// tx.Commit itself fails (advanceOffset having already succeeded), not
// silently count the attempt without an observation. deferredConstraintHandler
// forces the failure to surface at Commit specifically, via a UNIQUE
// ... DEFERRABLE INITIALLY DEFERRED violation whose check only runs at
// commit time.
func TestLoop_commitFailureReportsFailed(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 61, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: deferredConstraintHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	cfg := testIngestConfig()
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
		t.Errorf("OutcomeFailed observations = %d, want %d (every attempt's commit failure must be reported)", failedCount, cfg.RetryMaxAttempts)
	}
	if givenUpCount != 1 {
		t.Errorf("OutcomeGivenUp observations = %d, want 1", givenUpCount)
	}
}

// TestLoop_nonDefaultFactorReachesTheCallSite is the
// non-default-factor scenario for this package's between-attempt
// delay (runAttempts): a base and factor chosen so the
// summed delay at the configured factor is far above the summed delay
// the shared package's compiled-in default factor would produce,
// asserted as a LOWER bound on elapsed wall time so the case cannot
// flake on a slow machine — this package's paths touch a real database
// and cannot run inside a synctest bubble. Reds if runAttempts passes
// the compiled-in default instead of l.cfg.RetryFactor; no shipped test
// does.
func TestLoop_nonDefaultFactorReachesTheCallSite(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 40, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: writeAndFailHandler{err: errors.New("boom")}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	cfg := testIngestConfig()
	cfg.RetryMaxAttempts = 4
	cfg.RetryBaseDelay = 20 * time.Millisecond
	cfg.RetryMaxDelay = 10 * time.Second
	cfg.RetryFactor = 5

	l, err := New(Options{
		Client:   newTestClient(t, srv),
		Pool:     pool,
		Router:   router,
		Config:   cfg,
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Delays at factor 5, base 20ms: 20ms + 100ms + 500ms = 620ms. At
	// the compiled-in default factor (2) they would sum to 20ms + 40ms +
	// 80ms = 140ms — well below the floor asserted here.
	start := time.Now()
	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	elapsed := time.Since(start)

	const minElapsed = 550 * time.Millisecond
	if elapsed < minElapsed {
		t.Errorf("elapsed = %v, want at least %v (the configured RetryFactor must reach the between-attempt delay)", elapsed, minElapsed)
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
	// The handler adds a delay so the first attempt's failure lands well
	// past the few tens of milliseconds a fixed sleep would allow for on
	// a loaded machine.
	srv.SetHandler(tgtest.Delayed(200*time.Millisecond, tgtest.Success(updatesJSON(t, []telego.Update{raw}))))

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

	// Wait for the first attempt's failure to be observed, then cancel —
	// how long reaching that failure takes is not something the test can
	// guess, so it polls the observation rather than sleeping a fixed
	// duration.
	waitDeadline := time.Now().Add(10 * time.Second)
	for {
		observedFailure := false
		for _, o := range rec.Updates() {
			if o.Outcome == OutcomeFailed {
				observedFailure = true
				break
			}
		}
		if observedFailure {
			break
		}
		if time.Now().After(waitDeadline) {
			cancel()
			<-done
			t.Fatal("timed out waiting for an OutcomeFailed observation before cancelling")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancelAt := time.Now()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly after cancellation")
	}
	// The backoff wait's own select must react to ctx.Done() immediately
	// — not merely within the outer 2s test timeout, which the
	// configured backoff (base 300ms, max 500ms, up to 5 attempts) would
	// still clear even were that select's ctx.Done() case missing
	// entirely (each remaining attempt's Begin would then fail fast on
	// the already-cancelled ctx, but the loop would still wait out every
	// intervening full backoff delay — several seconds, not milliseconds).
	if sinceCancel := time.Since(cancelAt); sinceCancel > 250*time.Millisecond {
		t.Errorf("Run() returned %v after cancellation, want at most %v — the mid-wait select must react to ctx.Done() directly rather than waiting out the %v backoff delay", sinceCancel, 250*time.Millisecond, cfg.RetryBaseDelay)
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

func TestWaitOrCancel_returnsNilOnceDelayElapses(t *testing.T) {
	t.Parallel()

	start := time.Now()
	if err := waitOrCancel(context.Background(), 20*time.Millisecond); err != nil {
		t.Fatalf("waitOrCancel: %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("waitOrCancel returned after %v, want at least the 20ms delay", elapsed)
	}
}

func TestWaitOrCancel_returnsContextErrorWhenCancelledFirst(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := waitOrCancel(ctx, time.Hour)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitOrCancel: err = %v, want context.Canceled", err)
	}
	if elapsed > time.Second {
		t.Errorf("waitOrCancel took %v to return after cancellation, want a prompt return rather than waiting out the hour delay", elapsed)
	}
}

func TestWaitOrCancel_zeroOrNegativeDurationReturnsWithoutWaiting(t *testing.T) {
	t.Parallel()

	for _, delay := range []time.Duration{0, -time.Second} {
		start := time.Now()
		if err := waitOrCancel(context.Background(), delay); err != nil {
			t.Fatalf("waitOrCancel(%v): %v, want nil", delay, err)
		}
		if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
			t.Errorf("waitOrCancel(%v) took %v, want an immediate return", delay, elapsed)
		}
	}
}
