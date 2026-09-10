package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
// 200, send this test process a real SIGTERM, observe the readiness
// value itself latch not-ready the moment the drain begins, and confirm
// serve returns exit code 0 within the shutdown budget — reachable only
// when the ingest loop, the scheduler worker and the liveness heartbeat
// all returned on Stop rather than on the budget's own cancel. It goes
// through assemble + (*app).serve directly rather than through run's
// argv dispatch, so the test can read the health listener's bound
// address back — run's own dispatch is exercised separately in
// run_test.go. Not parallel: it sends a real signal to the test
// process, and the leak check is the last assertion.
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

	// The property under test is the design's own claim: readiness is
	// false from the moment shutdown begins (drain's first action,
	// ahead of every runner.stop and every closer). The honest way to
	// observe that is the readiness value itself, in-process, not a
	// live HTTP poll of /readyz: this fixture's own real runners have
	// nothing in flight to wait out, so a clean drain here completes in
	// low tens of milliseconds — faster than a poll interval can
	// reliably land a request before the health listener (one of the
	// closers the same drain tears down) is already gone. Racing an
	// HTTP poll against its own listener's teardown was measured to
	// flake under exactly that shape: serve had already returned
	// (cleanly, exit 0) before any poll observed a 503, which a racing
	// read of serveDone is what surfaced rather than a bare timeout.
	// The HTTP-level mapping this replaces — /readyz answering 503 when
	// Ready returns an error — is proven directly by
	// the health package's own dedicated tests, and this same handler's
	// other branch is already exercised by this test's earlier 200
	// check; re-proving it here would only be racing a closing listener
	// for coverage that already exists elsewhere. sawDraining races the
	// same three outcomes waitForReadyz's replacement above avoided
	// conflating: the latch observed, serve already gone, or neither
	// within the patience budget.
	sawDraining, code, err := raceDrainingLatchAgainstServeDone(a, serveDone, smokePatience)
	switch {
	case err != nil:
		t.Fatalf("readiness never latched draining within %s, and serve had not returned either: %v; stderr so far:\n%s",
			smokePatience, err, stderr.String())
	case !sawDraining:
		t.Fatalf("serve already returned (code=%d) after %s, before the draining latch was ever observed set — "+
			"a runner most likely returned on its own before the signal reached it; stderr:\n%s",
			code, time.Since(start), stderr.String())
	}

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
		t.Fatalf("serve did not return within the shutdown budget plus slack; stderr so far:\n%s", stderr.String())
	}

	goleak.VerifyNone(t, leakOpt)
}

// raceDrainingLatchAgainstServeDone polls a.readiness's draining latch,
// in-process, until it is set, until serveDone fires (serve returned on
// its own, before the latch was ever observed set — the "exited early"
// outcome), or until timeout expires (the "never started" outcome —
// serve is presumably still running, but nothing confirms it). Exactly
// one of three shapes is returned: (true, 0, nil) — the latch was
// observed set; (false, code, nil) — serveDone fired first, code is
// what it carried; (false, 0, err) — the timeout expired with neither
// observed. Polling the latch directly, rather than the HTTP path that
// reads it, keeps this observation honest under a drain fast enough to
// have already torn down the listener before an HTTP round trip could
// land.
func raceDrainingLatchAgainstServeDone(a *app, serveDone <-chan int, timeout time.Duration) (sawDraining bool, code int, err error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if a.readiness.draining.Load() {
			return true, 0, nil
		}
		select {
		case code := <-serveDone:
			return false, code, nil
		default:
		}
		time.Sleep(time.Millisecond)
	}
	return false, 0, fmt.Errorf("timed out after %s", timeout)
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
