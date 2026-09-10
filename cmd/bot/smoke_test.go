package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/goleak"

	"github.com/maratik123/lab-game/internal/tgtest"
)

// smokeCapture records every request body the fake Bot API server
// sees, so the smoke test can inspect the getUpdates call the assembled
// ingest loop actually made.
type smokeCapture struct {
	mu       sync.Mutex
	requests []getUpdatesBody
}

type getUpdatesBody struct {
	AllowedUpdates []string `json:"allowed_updates"`
}

func (c *smokeCapture) handler() tgtest.Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			body, _ := io.ReadAll(r.Body)
			var req getUpdatesBody
			_ = json.Unmarshal(body, &req)
			c.mu.Lock()
			c.requests = append(c.requests, req)
			c.mu.Unlock()
		}
		tgtest.Success(nil)(w, r)
	}
}

func (c *smokeCapture) all() []getUpdatesBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]getUpdatesBody, len(c.requests))
	copy(out, c.requests)
	return out
}

// TestSmoke_AssembledProcessServesReadyzMetricsAndDrainsOnSIGTERM is the
// in-process smoke condition: assemble the real process against a real
// schema and a fake Bot API, start serving, wait for /readyz to answer
// 200, send this test process a real SIGTERM, observe /readyz answer
// 503 while draining, and confirm serve returns exit code 0 within the
// shutdown budget — reachable only when the ingest loop, the scheduler
// worker and the liveness heartbeat all returned on Stop rather than on
// the budget's own cancel. It goes through assemble + (*app).serve
// directly rather than through run's argv dispatch, so the test can
// read the health listener's bound address back — run's own dispatch
// is exercised separately in run_test.go. Not parallel: it sends a
// real signal to the test process, and the leak check is the last
// assertion.
// smokePatience is this file's patience budget: how long a poll waits
// for a condition it expects to become true. It is an instrument, not a
// subject — widening it changes no proposition this test asserts — and
// it is sized for the whole-module race gate, where sixteen packages
// contend for one Postgres server and the race detector stretches every
// goroutine handoff. The drain's own bound stays the configured
// shutdown budget, asserted separately below.
const smokePatience = 30 * time.Second

func TestSmoke_AssembledProcessServesReadyzMetricsAndDrainsOnSIGTERM(t *testing.T) {
	capture := &smokeCapture{}
	srv := tgtest.New(t, capture.handler())

	// Snapshot after the test's own fixtures (the fake server's
	// listener goroutine included) are already running, so the leak
	// check below is about what assemble/serve left running, not about
	// this test's own scaffolding.
	leakOpt := goleak.IgnoreCurrent()

	env := assembleTestEnv(t)
	env["LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT"] = "10s"

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:     mapLookup(env),
		Stderr:     &stderr,
		Version:    "smoke-test",
		StartedAt:  time.Now(),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("assemble: %v (stderr: %s)", err, stderr.String())
	}

	serveDone := make(chan int, 1)
	go func() { serveDone <- a.serve(context.Background(), a.cfg.Process.ShutdownTimeout, &stderr) }()

	addr := a.healthSrv.Addr()
	waitForReadyz(t, addr, http.StatusOK, smokePatience)

	// Step 1's own proof: /metrics carries a Go-runtime family, a
	// process collector family, a pool family and every
	// process-identity family.
	scrape := scrapeMetrics(t, addr)
	for _, want := range []string{"go_goroutines", "process_", "labgame_pgxpool_", "labgame_build_info", "labgame_start_time_seconds", "labgame_ready"} {
		if !strings.Contains(scrape, want) {
			t.Errorf("scrape missing %q family", want)
		}
	}

	// The assembled loop's getUpdates request carries the reserved
	// sentinel substituted for an empty route set.
	waitForCapture(t, capture, smokePatience)
	reqs := capture.all()
	if len(reqs) == 0 {
		t.Fatal("no getUpdates request observed")
	}
	if len(reqs[0].AllowedUpdates) != 1 || reqs[0].AllowedUpdates[0] != string(telego.ShippingQueryUpdates) {
		t.Errorf("AllowedUpdates = %v, want the empty-route-set sentinel [%q]", reqs[0].AllowedUpdates, telego.ShippingQueryUpdates)
	}

	start := time.Now()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	waitForReadyz(t, addr, http.StatusServiceUnavailable, smokePatience)

	select {
	case code := <-serveDone:
		elapsed := time.Since(start)
		if code != 0 {
			t.Errorf("serve() = %d, stderr = %q, want 0", code, stderr.String())
		}
		if elapsed >= a.cfg.Process.ShutdownTimeout {
			t.Errorf("serve took %s, want well under the %s shutdown budget", elapsed, a.cfg.Process.ShutdownTimeout)
		}
	case <-time.After(a.cfg.Process.ShutdownTimeout + smokePatience):
		t.Fatal("serve did not return within the shutdown budget plus slack")
	}

	goleak.VerifyNone(t, leakOpt)
}

// waitForReadyz polls addr's /readyz path until it answers wantStatus
// or the deadline expires.
func waitForReadyz(tb testing.TB, addr string, wantStatus int, timeout time.Duration) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		//nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
		resp, err := http.Get("http://" + addr + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	tb.Fatalf("/readyz never answered %d within %s", wantStatus, timeout)
}

// waitForCapture polls capture until at least one request has been
// recorded or the deadline expires.
func waitForCapture(tb testing.TB, capture *smokeCapture, timeout time.Duration) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(capture.all()) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	tb.Fatal("no getUpdates request observed within the timeout")
}
