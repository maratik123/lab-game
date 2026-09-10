package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/tgtest"
)

// blockingHandler blocks until release is closed, recording that it was
// entered so a test can synchronise on "the handler is now running"
// without a fixed sleep budget standing in for it.
type blockingHandler struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
}

func (h *blockingHandler) Handle(_ context.Context, _ pgx.Tx, _ Update) error {
	close(h.entered)
	<-h.release
	return nil
}

// TestLoop_StopSeam is the shared stop-contract scenario list this
// module's task-scheduler package states once, beside its own Liveness
// and Worker instances; this is the Loop instance.
func TestLoop_StopSeam(t *testing.T) {
	t.Parallel()

	t.Run("stop_before_run_returns_nil_without_a_cycle", func(t *testing.T) {
		t.Parallel()
		pool := newIngestPool(t)
		srv := tgtest.New(t, tgtest.Success(updatesJSON(t, nil)))
		router, err := NewRouter()
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		rec := &recordingObserver{}
		l := newLoop(t, srv, pool, router, rec)
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
		if len(rec.Loops()) != 0 {
			t.Errorf("Loops() = %v, want none — Stop before Run must start no cycle", rec.Loops())
		}
	})

	t.Run("stop_during_inter_cycle_wait_returns_nil_promptly", func(t *testing.T) {
		t.Parallel()
		pool := newIngestPool(t)
		srv := tgtest.New(t, tgtest.Success(updatesJSON(t, nil)))
		router, err := NewRouter()
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		l := newLoop(t, srv, pool, router, nil)
		l.cfg.PollInterval = time.Hour // never lets the ticker itself fire

		done := make(chan error, 1)
		go func() { done <- l.Run(context.Background()) }()
		time.Sleep(50 * time.Millisecond)
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
		pool := newIngestPool(t)
		srv := tgtest.New(t, tgtest.Success(updatesJSON(t, nil)))
		router, err := NewRouter()
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		l := newLoop(t, srv, pool, router, nil)
		l.Stop()
		l.Stop()
	})

	t.Run("context_cancelled_returns_ctx_err", func(t *testing.T) {
		t.Parallel()
		pool := newIngestPool(t)
		srv := tgtest.New(t, tgtest.Success(updatesJSON(t, nil)))
		router, err := NewRouter()
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		l := newLoop(t, srv, pool, router, nil)
		l.cfg.PollInterval = time.Hour
		runCtx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() { done <- l.Run(runCtx) }()
		time.Sleep(50 * time.Millisecond)
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

// TestPollOnce_StopDiscardsAParkedLongPoll drives Stop while PollOnce is
// parked in getUpdates: the call returns promptly rather than waiting
// out the long-poll window, PollOnce reports ErrPollDiscarded, the
// reported LoopObservation carries no Err (a discarded poll must not
// mint a spurious poll-error metric), and the offset row — read by the
// same PollOnce that was cut short — is unchanged.
func TestPollOnce_StopDiscardsAParkedLongPoll(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, tgtest.Delayed(5*time.Second, tgtest.Success(updatesJSON(t, nil))))
	router, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	before, err := readOffset(context.Background(), pool)
	if err != nil {
		t.Fatalf("readOffset (before): %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- l.PollOnce(context.Background()) }()
	time.Sleep(100 * time.Millisecond)
	l.Stop()

	select {
	case err := <-done:
		if !errors.Is(err, ErrPollDiscarded) {
			t.Errorf("PollOnce() = %v, want ErrPollDiscarded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PollOnce did not return promptly after Stop — it waited out the long-poll window")
	}

	loops := rec.Loops()
	if len(loops) != 1 || loops[0].Err != nil {
		t.Fatalf("Loops() = %+v, want exactly one observation with no Err", loops)
	}

	after, err := readOffset(context.Background(), pool)
	if err != nil {
		t.Fatalf("readOffset (after): %v", err)
	}
	if after != before {
		t.Errorf("offset changed on a discarded poll: before %d, after %d", before, after)
	}
}

// TestPollOnce_StopMidBatchSettlesEveryUpdate is the other half of the
// two-context split: Stop landing while a batch is mid-processUpdate
// must not cancel the parent context. Every update already fetched
// settles — its handler runs to completion and the offset advances —
// and PollOnce returns nil, not an error.
func TestPollOnce_StopMidBatchSettlesEveryUpdate(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	h := newBlockingHandler()
	router, err := NewRouter(Route{Kind: KindMessage, Handler: h})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	raw := telego.Update{UpdateID: 7, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv := tgtest.New(t, tgtest.Success(updatesJSON(t, []telego.Update{raw})))
	l := newLoop(t, srv, pool, router, nil)

	done := make(chan error, 1)
	go func() { done <- l.PollOnce(context.Background()) }()

	select {
	case <-h.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handler was never entered")
	}
	l.Stop()

	select {
	case <-done:
		t.Fatal("PollOnce returned before the in-flight handler finished — Stop must not cancel the batch")
	case <-time.After(100 * time.Millisecond):
	}

	close(h.release)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("PollOnce() = %v, want nil (the batch settled)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PollOnce did not return after the in-flight handler finished")
	}

	after, err := readOffset(context.Background(), pool)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if after != int64(raw.UpdateID)+1 {
		t.Errorf("offset after settlement = %d, want %d", after, raw.UpdateID+1)
	}
}
