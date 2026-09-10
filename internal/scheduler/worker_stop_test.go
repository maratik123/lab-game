package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestWorker_StopSeam is the shared stop-contract scenario list this
// package's suite states once, beside the Liveness instance, and
// applies to all three runners; this is the Worker instance.
func TestWorker_StopSeam(t *testing.T) {
	t.Parallel()

	t.Run("stop_before_run_returns_nil_without_a_cycle", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		reg, err := NewRegistry()
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		obs := &recordingObserver{}
		w, err := New(Options{Pool: pool, Registry: reg, Config: testConfig(), Observer: obs})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		w.Stop()

		done := make(chan error, 1)
		go func() { done <- w.Run(context.Background()) }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run() = %v, want nil", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return promptly after Stop before Run")
		}
		if len(obs.Loops()) != 0 {
			t.Errorf("Loops() = %v, want none — Stop before Run must start no cycle", obs.Loops())
		}
	})

	t.Run("stop_during_inter_cycle_wait_returns_nil_promptly", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		reg, err := NewRegistry()
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		cfg := testConfig()
		cfg.PollInterval = time.Hour // never lets the ticker itself fire
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		done := make(chan error, 1)
		go func() { done <- w.Run(context.Background()) }()
		time.Sleep(20 * time.Millisecond)
		w.Stop()

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
		w := newWorker(t, pool, mustRegistry(t))
		w.Stop()
		w.Stop()
	})

	t.Run("context_cancelled_returns_ctx_err", func(t *testing.T) {
		t.Parallel()
		pool := newScheduler(t)
		reg, err := NewRegistry()
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		cfg := testConfig()
		cfg.PollInterval = time.Hour
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		runCtx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() { done <- w.Run(runCtx) }()
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

// mustRegistry returns a fresh, empty *Registry, failing tb on error.
func mustRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestWorker_StopLetsInFlightCycleFinish drives Stop while a cycle is
// blocked mid-execution: Run must not return until that cycle's handler
// returns, and the returned error must be nil (a completed drain), not
// ctx.Err().
func TestWorker_StopLetsInFlightCycleFinish(t *testing.T) {
	t.Parallel()
	pool := newScheduler(t)
	h := newBlockingHandler()
	reg, err := NewRegistry(Declaration{Type: "block.stop", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	dueNow(t, pool, reg, Request{Type: "block.stop"})

	cfg := contentionSafeConfig()
	cfg.PollInterval = time.Hour
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- w.Run(context.Background()) }()

	// Give the first cycle time to discover the row and block inside
	// the handler before Stop is called.
	time.Sleep(200 * time.Millisecond)
	w.Stop()

	select {
	case <-done:
		t.Fatal("Run returned before the in-flight handler finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(h.release)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil (Stop, not cancel)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the in-flight handler finished")
	}
}
