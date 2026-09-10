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
	// reverse of their append order: health listener, then pool.
	wantTail := []string{"close:health listener", "close:pool"}
	gotTail := log[len(log)-2:]
	for i, w := range wantTail {
		if gotTail[i] != w {
			t.Errorf("closer order = %v, want tail %v", log, wantTail)
		}
	}
}

func TestServe_RunnerIgnoresStop_BudgetExpires_ExitNonZero(t *testing.T) {
	t.Parallel()
	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
		runners:   []runner{stubbornRunner("scheduler worker")},
	}

	budget := 100 * time.Millisecond
	go func() { a.signals <- syscall.SIGTERM }()
	start := time.Now()
	code := a.serve(context.Background(), budget, &bytes.Buffer{})
	elapsed := time.Since(start)

	if code == 0 {
		t.Error("serve() = 0, want non-zero (abandoned drain)")
	}
	if elapsed < budget {
		t.Errorf("serve returned after %s, want at least the budget %s", elapsed, budget)
	}
	if elapsed > budget+patience {
		t.Errorf("serve returned after %s, want close to the budget %s", elapsed, budget)
	}
}

func TestServe_SecondSignalMidDrain_EndsAtOnce(t *testing.T) {
	t.Parallel()
	a := &app{
		readiness: newReadiness(nil),
		signals:   make(chan os.Signal, 2),
		runners:   []runner{stubbornRunner("scheduler worker")},
	}

	budget := 10 * time.Second // large — the second signal must end it well before this

	go func() {
		a.signals <- syscall.SIGTERM
		time.Sleep(50 * time.Millisecond)
		a.signals <- syscall.SIGTERM
	}()

	start := time.Now()
	code := a.serve(context.Background(), budget, &bytes.Buffer{})
	elapsed := time.Since(start)

	if code == 0 {
		t.Error("serve() = 0, want non-zero (abandoned drain)")
	}
	if elapsed >= budget {
		t.Errorf("serve took %s, want well under the %s budget (second signal should end it)", elapsed, budget)
	}
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
