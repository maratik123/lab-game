package health

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServer_StartBindsAndAddrReportsPort(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterRuntime(reg); err != nil {
		t.Fatalf("RegisterRuntime: %v", err)
	}
	s := NewServer("127.0.0.1:0", reg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	addr := s.Addr()
	if addr == "" {
		t.Fatal("Addr() is empty after Start")
	}
	if _, port, err := net.SplitHostPort(addr); err != nil || port == "0" || port == "" {
		t.Fatalf("Addr() = %q, want a concrete bound port", addr)
	}
}

func TestServer_MetricsPathServes200(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterRuntime(reg); err != nil {
		t.Fatalf("RegisterRuntime: %v", err)
	}
	s := NewServer("127.0.0.1:0", reg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	resp, err := http.Get("http://" + s.Addr() + metricsPath) //nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(body), "go_goroutines") {
		t.Errorf("body does not name a registered family; body = %q", string(body))
	}
}

func TestServer_OtherPathIs404(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", NewRegistry())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	resp, err := http.Get("http://" + s.Addr() + "/other") //nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", resp.StatusCode)
	}
}

func TestServer_StartOnAlreadyBoundAddressReturnsErrorAndStartsNoGoroutine(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	s := NewServer(ln.Addr().String(), NewRegistry())
	if err := s.Start(); err == nil {
		t.Fatal("Start: expected a bind error against an already-bound address")
	}
	// A failed Start must not have transitioned to started — Shutdown on
	// it must still report "not started", not attempt to stop a
	// goroutine that was never spawned.
	if err := s.Shutdown(context.Background()); err == nil {
		t.Error("Shutdown after a failed Start: expected an error (not started)")
	}
}

func TestServer_ShutdownReturnsNilOnCleanStop(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", NewRegistry())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown: %v, want nil (http.ErrServerClosed must not be reported as a failure)", err)
	}
}

// TestServer_SecondShutdownReturnsRatherThanHanging falsifies a
// second-call deadlock: without the fix, the first Shutdown drains the
// single-value serve-error channel and a second call blocks on it
// forever. A deadline on the second call's own context makes a
// regression fail the test instead of stalling the whole suite.
func TestServer_SecondShutdownReturnsRatherThanHanging(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", NewRegistry())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		done <- s.Shutdown(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("second Shutdown: %v, want nil (the latched first-call result)", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK: second Shutdown never returned within a 3s test guard")
	}
}

// TestServer_ShutdownMidFlightBoundsSecondCallerByItsOwnCtx falsifies a
// second-call block that is bounded only once the first call has already
// finished: with an active connection held in http.StateActive, the
// first Shutdown call is still inside the graceful stop when the second
// arrives. Without the fix, the second call blocks inside
// sync.Once.Do — which consults no context — rather than on the select
// that follows it, so it outlives its own short deadline. A guard
// deadline on the whole test makes a regression fail rather than stall
// the suite.
func TestServer_ShutdownMidFlightBoundsSecondCallerByItsOwnCtx(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", NewRegistry())

	handlerEntered := make(chan struct{})
	release := make(chan struct{})
	s.httpSrv.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerEntered)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	reqDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + s.Addr() + "/") //nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
		if err == nil {
			_ = resp.Body.Close()
		}
		reqDone <- err
	}()

	select {
	case <-handlerEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never entered — request did not reach the server")
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- s.Shutdown(context.Background())
	}()

	// Give the first call a head start into shutdownOnce.Do so the
	// second call below genuinely arrives while the first is in
	// progress, not before it.
	time.Sleep(100 * time.Millisecond)

	secondDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		secondDone <- s.Shutdown(ctx)
	}()

	select {
	case err := <-secondDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("second Shutdown = %v, want context.DeadlineExceeded from its own 300ms deadline", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK: second Shutdown never returned within its own deadline plus a 3s test guard")
	}

	close(release)

	if err := <-firstDone; err != nil {
		t.Errorf("first Shutdown: %v", err)
	}
	if err := <-reqDone; err != nil {
		t.Errorf("held request: %v", err)
	}
}

func TestServer_ShutdownOnNeverStartedReturnsError(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", NewRegistry())
	if err := s.Shutdown(context.Background()); err == nil {
		t.Error("Shutdown: expected an error on a server never started")
	}
}
