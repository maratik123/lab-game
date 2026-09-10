package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

// patience is a generous-but-bounded wait for an assertion that should
// resolve promptly (well inside the test's own shutdownTimeout budget).
const patience = 2 * time.Second

// recordingCloser builds a closer that appends its own name to log
// (guarded by mu) when called, and returns closeErr.
func recordingCloser(mu *sync.Mutex, log *[]string, name string, closeErr error) closer {
	return closer{
		name: name,
		close: func(context.Context) error {
			mu.Lock()
			*log = append(*log, "close:"+name)
			mu.Unlock()
			return closeErr
		},
	}
}

// stoppingRunner returns a runner that returns nil promptly once
// stopped, or ctx.Err() if ctx is cancelled first.
func stoppingRunner(mu *sync.Mutex, log *[]string, name string) runner {
	stopCh := make(chan struct{})
	var once sync.Once
	return runner{
		name: name,
		run: func(ctx context.Context) error {
			select {
			case <-stopCh:
				mu.Lock()
				*log = append(*log, "run-returned:"+name)
				mu.Unlock()
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		stop: func() {
			mu.Lock()
			*log = append(*log, "stop:"+name)
			mu.Unlock()
			once.Do(func() { close(stopCh) })
		},
	}
}

// stubbornRunner returns a runner whose stop is a no-op: it only
// returns on ctx cancellation.
func stubbornRunner(name string) runner {
	return runner{
		name: name,
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		stop: func() {},
	}
}

func TestServe_SignalCleanStopExitZero(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var log []string

	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
		runners: []runner{
			stoppingRunner(&mu, &log, "ingest loop"),
			stoppingRunner(&mu, &log, "scheduler worker"),
		},
		closers: []closer{
			recordingCloser(&mu, &log, "pool", nil),
			recordingCloser(&mu, &log, "health listener", nil),
		},
	}

	done := make(chan int, 1)
	go func() { done <- a.serve(context.Background(), patience, &bytes.Buffer{}) }()

	a.signals <- syscall.SIGTERM

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("serve() = %d, want 0", code)
		}
	case <-time.After(patience):
		t.Fatal("serve did not return in time")
	}

	if !a.readiness.draining.Load() {
		t.Error("readiness.draining not latched")
	}

	mu.Lock()
	defer mu.Unlock()
	wantPrefix := []string{"stop:ingest loop", "stop:scheduler worker"}
	if len(log) < len(wantPrefix) {
		t.Fatalf("log = %v, too short", log)
	}
	seen := map[string]bool{}
	for _, l := range log[:len(wantPrefix)] {
		seen[l] = true
	}
	for _, w := range wantPrefix {
		if !seen[w] {
			t.Errorf("log %v missing %q among the first stops", log, w)
		}
	}
	// The closers must run AFTER both runners have stopped, and in
	// reverse of the order they were appended to a.closers — derived
	// from that same list rather than hand-written, so the assertion
	// tracks the closer list by construction.
	wantTail := make([]string, len(a.closers))
	for i, c := range a.closers {
		wantTail[len(a.closers)-1-i] = "close:" + c.name
	}
	if len(log) < len(wantTail) {
		t.Fatalf("log = %v, too short for closer tail %v", log, wantTail)
	}
	gotTail := log[len(log)-len(wantTail):]
	for i, w := range wantTail {
		if gotTail[i] != w {
			t.Errorf("closer order = %v, want tail %v", log, wantTail)
		}
	}
}

// drainObservingRunner returns a runner whose stop records, into
// *observed (guarded by mu), whether a's readiness was already latched
// draining at the moment stop was called — before returning promptly
// once stopped.
func drainObservingRunner(a *app, mu *sync.Mutex, observed *bool, name string) runner {
	stopCh := make(chan struct{})
	var once sync.Once
	return runner{
		name: name,
		run: func(ctx context.Context) error {
			select {
			case <-stopCh:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		stop: func() {
			mu.Lock()
			*observed = a.readiness.draining.Load()
			mu.Unlock()
			once.Do(func() { close(stopCh) })
		},
	}
}

// TestServe_DrainLatchesReadinessBeforeStoppingRunners asserts the
// ORDER drain's own doc comment names — readiness.draining is latched
// before any runner.stop is called — by observing the latch from
// inside a runner's own stop, not by re-checking the latch after serve
// has already returned (which cannot distinguish "latched first" from
// "latched last": both leave the latch set by the time serve returns).
func TestServe_DrainLatchesReadinessBeforeStoppingRunners(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var observedDraining bool

	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
	}
	a.runners = []runner{drainObservingRunner(a, &mu, &observedDraining, "ingest loop")}

	done := make(chan int, 1)
	go func() { done <- a.serve(context.Background(), patience, &bytes.Buffer{}) }()

	a.signals <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(patience):
		t.Fatal("serve did not return in time")
	}

	mu.Lock()
	defer mu.Unlock()
	if !observedDraining {
		t.Error("runner.stop observed readiness.draining not yet latched — the latch must be set before any runner.stop is called")
	}
}

// TestServe_RunnerIgnoresStop_BudgetExpires_ExitNonZero asserts a
// duration — the drain must abandon exactly at the shutdown budget,
// not merely "eventually" — so it runs inside a synctest bubble
// against a virtual clock: a wall-clock budget racing a shared test
// server's neighbouring binaries (this package shares one Postgres
// server with several sibling packages) is exactly the shape the
// bubble exists to remove, per this module's own subject/instrument
// convention for a duration under test.
func TestServe_RunnerIgnoresStop_BudgetExpires_ExitNonZero(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := &app{
			readiness: newReadiness(nil),
			signals:   make(chan os.Signal, 2),
			runners:   []runner{stubbornRunner("scheduler worker")},
		}

		budget := 100 * time.Millisecond
		a.signals <- syscall.SIGTERM

		start := time.Now()
		code := a.serve(context.Background(), budget, &bytes.Buffer{})
		elapsed := time.Since(start)

		if code == 0 {
			t.Error("serve() = 0, want non-zero (abandoned drain)")
		}
		if elapsed != budget {
			t.Errorf("serve returned after exactly %s of virtual time, want exactly the budget %s", elapsed, budget)
		}
	})
}

// TestServe_SecondSignalMidDrain_EndsAtOnce asserts a duration too —
// the second signal must end the wait at exactly the instant it
// arrives, not merely "well before" a large budget — so it runs inside
// a synctest bubble for the same reason as the case above.
func TestServe_SecondSignalMidDrain_EndsAtOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := &app{
			readiness: newReadiness(nil),
			signals:   make(chan os.Signal, 2),
			runners:   []runner{stubbornRunner("scheduler worker")},
		}

		budget := 10 * time.Second // large — the second signal must end it well before this
		secondSignalAt := 50 * time.Millisecond

		a.signals <- syscall.SIGTERM
		go func() {
			time.Sleep(secondSignalAt)
			a.signals <- syscall.SIGTERM
		}()

		start := time.Now()
		code := a.serve(context.Background(), budget, &bytes.Buffer{})
		elapsed := time.Since(start)

		if code == 0 {
			t.Error("serve() = 0, want non-zero (abandoned drain)")
		}
		if elapsed != secondSignalAt {
			t.Errorf("serve took exactly %s of virtual time, want exactly %s (the instant the second signal arrived)", elapsed, secondSignalAt)
		}
	})
}

func TestServe_RunnerReturnsOnOwnWithNoSignal(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var log []string
	wantErr := errors.New("reconcile failed")

	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
		runners: []runner{
			{
				name: "scheduler worker",
				run:  func(context.Context) error { return wantErr },
				stop: func() {},
			},
			stoppingRunner(&mu, &log, "ingest loop"),
		},
	}

	var stderr bytes.Buffer
	code := a.serve(context.Background(), patience, &stderr)

	if code == 0 {
		t.Error("serve() = 0, want non-zero — a runner returned on its own")
	}
	if !strings.Contains(stderr.String(), "scheduler worker") || !strings.Contains(stderr.String(), wantErr.Error()) {
		t.Errorf("stderr = %q, want it to name the runner and its error", stderr.String())
	}
}

func TestServe_FailingFinalLivenessWrite_ReportedNotFatal(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var log []string

	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
		runners:   []runner{stoppingRunner(&mu, &log, "liveness heartbeat")},
		closers: []closer{
			recordingCloser(&mu, &log, "liveness final write", errors.New("write failed")),
		},
	}

	var stderr bytes.Buffer
	go func() { a.signals <- syscall.SIGTERM }()
	code := a.serve(context.Background(), patience, &stderr)

	if code != 0 {
		t.Errorf("serve() = %d, want 0 — a failing final liveness write must not flip the exit code", code)
	}
	if !strings.Contains(stderr.String(), "liveness final write") {
		t.Errorf("stderr = %q, want it to report the failing closer by name", stderr.String())
	}
}

func TestServe_OtherClosingFailure_IsFatal(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var log []string

	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
		runners:   []runner{stoppingRunner(&mu, &log, "ingest loop")},
		closers: []closer{
			recordingCloser(&mu, &log, "pool", errors.New("close failed")),
		},
	}

	go func() { a.signals <- syscall.SIGTERM }()
	code := a.serve(context.Background(), patience, &bytes.Buffer{})

	if code == 0 {
		t.Error("serve() = 0, want non-zero — a non-liveness closer failed")
	}
}
