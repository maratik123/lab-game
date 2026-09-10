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

	"github.com/prometheus/client_golang/prometheus"
)

// alwaysReady is a ReadyFunc that always reports ready — the default
// fixture for every test whose subject is not readiness itself.
func alwaysReady(context.Context) error { return nil }

// mustNewServer builds a Server from addr, gatherer and ready (nil
// defaults to alwaysReady), failing t on a rejected option — the
// shape every test in this file that is not itself testing NewServer's
// own validation uses.
func mustNewServer(t *testing.T, addr string, gatherer prometheus.Gatherer, ready ReadyFunc) *Server {
	t.Helper()
	if ready == nil {
		ready = alwaysReady
	}
	s, err := NewServer(ServerOptions{Addr: addr, Gatherer: gatherer, Ready: ready})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

func TestServer_StartBindsAndAddrReportsPort(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterRuntime(reg); err != nil {
		t.Fatalf("RegisterRuntime: %v", err)
	}
	s := mustNewServer(t, "127.0.0.1:0", reg, nil)
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
	s := mustNewServer(t, "127.0.0.1:0", reg, nil)
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
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), nil)
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

	s := mustNewServer(t, ln.Addr().String(), NewRegistry(), nil)
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
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), nil)
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
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), nil)
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
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), nil)

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
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), nil)
	if err := s.Shutdown(context.Background()); err == nil {
		t.Error("Shutdown: expected an error on a server never started")
	}
}

// TestNewServer_RefusesNilGatherer asserts the field-naming refusal.
func TestNewServer_RefusesNilGatherer(t *testing.T) {
	t.Parallel()
	_, err := NewServer(ServerOptions{Addr: "127.0.0.1:0", Ready: alwaysReady})
	var optErr *OptionError
	if !errors.As(err, &optErr) || optErr.Field != "Gatherer" {
		t.Fatalf("NewServer(nil Gatherer) = %v, want an *OptionError naming Gatherer", err)
	}
}

// TestNewServer_RefusesNilReady asserts the field-naming refusal.
func TestNewServer_RefusesNilReady(t *testing.T) {
	t.Parallel()
	_, err := NewServer(ServerOptions{Addr: "127.0.0.1:0", Gatherer: NewRegistry()})
	var optErr *OptionError
	if !errors.As(err, &optErr) || optErr.Field != "Ready" {
		t.Fatalf("NewServer(nil Ready) = %v, want an *OptionError naming Ready", err)
	}
}

// TestOptionError_Error renders the field and reason unconditionally,
// so the format itself is checked rather than only reached on an
// already-failing assertion's %v.
func TestOptionError_Error(t *testing.T) {
	t.Parallel()
	err := &OptionError{Field: "Ready", Reason: reasonMustNotBeNil}
	want := "health: Ready: must not be nil"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestServer_ReadyzServes200WhenReady asserts the happy path: Ready nil
// -> 200 with the fixed ready body.
func TestServer_ReadyzServes200WhenReady(t *testing.T) {
	t.Parallel()
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), alwaysReady)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	resp, err := http.Get("http://" + s.Addr() + readyzPath) //nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
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
	if string(body) != readyzReadyBody {
		t.Errorf("body = %q, want %q", body, readyzReadyBody)
	}
}

// TestServer_ReadyzServes503WhenNotReadyAndNeverLeaksTheCause asserts
// the not-ready path: the underlying error's own text never
// reaches the response body.
func TestServer_ReadyzServes503WhenNotReadyAndNeverLeaksTheCause(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("SENTINEL-DO-NOT-LEAK-THIS-CAUSE")
	notReady := func(context.Context) error { return sentinel }
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), notReady)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	resp, err := http.Get("http://" + s.Addr() + readyzPath) //nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != readyzNotReadyBody {
		t.Errorf("body = %q, want %q", body, readyzNotReadyBody)
	}
	if strings.Contains(string(body), "SENTINEL") {
		t.Fatalf("body leaked the underlying error's text: %q", body)
	}
}

// TestServer_ReadyzSurvivesABlockingReady proves the handler's own
// bound: a Ready that blocks past readyzTimeout still lets the request
// return (as not-ready), rather than hanging.
func TestServer_ReadyzSurvivesABlockingReady(t *testing.T) {
	t.Parallel()
	blocking := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	s := mustNewServer(t, "127.0.0.1:0", NewRegistry(), blocking)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	client := &http.Client{Timeout: readyzTimeout + 5*time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+s.Addr()+readyzPath, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v (the handler did not return within readyzTimeout plus a generous guard)", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", resp.StatusCode)
	}
}
