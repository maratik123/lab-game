package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// probeTransport returns a transport configuration valid enough to build
// a client, with a short attempt timeout so a deadline scenario
// resolves quickly.
func probeTransport() config.Transport {
	return config.Transport{
		RetryMaxAttempts: 3,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    10 * time.Millisecond,
		RetryFactor:      backoff.DefaultFactor,
		AttemptTimeout:   30 * time.Second,
	}
}

func newTestProber(t *testing.T, srv *tgtest.Server) *TelegramProber {
	t.Helper()
	p, err := NewTelegramProber(TelegramProberOptions{
		Token:      tgtest.Token,
		BaseURL:    tgtest.BaseURL,
		Transport:  probeTransport(),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewTelegramProber: %v", err)
	}
	return p
}

func TestTelegramProber_Success(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(json.RawMessage(`{"id":1,"is_bot":true,"first_name":"x"}`)))
	p := newTestProber(t, srv)

	result, err := p.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
}

// TestTelegramProberOptions_RedactsTokenInDefaultVerb falsifies a %v
// rendering of TelegramProberOptions that leaks the token: the field
// carries a type that redacts its own rendering precisely so this never
// happens.
func TestTelegramProberOptions_RedactsTokenInDefaultVerb(t *testing.T) {
	t.Parallel()
	opts := TelegramProberOptions{
		Token:   "own-secret-token",
		BaseURL: "https://own.invalid",
	}
	rendered := fmt.Sprintf("%v", opts)
	if strings.Contains(rendered, "own-secret-token") {
		t.Errorf("%%v of TelegramProberOptions leaked the token: %s", rendered)
	}
	if !strings.Contains(rendered, "[redacted]") {
		t.Errorf("%%v of TelegramProberOptions carries no redaction placeholder: %s", rendered)
	}
}

func TestTelegramProber_OkFalse(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"bad"}`))
	})
	p := newTestProber(t, srv)

	_, err := p.Probe(context.Background())
	if err == nil {
		t.Fatal("Probe: expected an error from ok:false")
	}
}

func TestTelegramProber_ServerError(t *testing.T) {
	t.Parallel()
	var calls int32
	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		tgtest.ServerError(http.StatusInternalServerError)(w, r)
	})
	p := newTestProber(t, srv)

	result, err := p.Probe(context.Background())
	if err == nil {
		t.Fatal("Probe: expected an error from the 500")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("handler invoked %d times, want exactly 1 (one attempt per tick)", got)
	}
	if got := classifyFailure(result.StatusCode, err); got != "500" {
		t.Errorf("classifyFailure = %q, want %q", got, "500")
	}
}

// TestTelegramProber_OkTrueWithServerError falsifies a probe that trusts
// the decoded envelope alone: a handler answering a 500 status while
// still claiming ok:true in its body must not count as a successful
// probe.
func TestTelegramProber_OkTrueWithServerError(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"x"}}`))
	})
	p := newTestProber(t, srv)

	result, err := p.Probe(context.Background())
	if err == nil {
		t.Fatal("Probe: expected an error from a 500 status carrying ok:true")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
}

func TestTelegramProber_DialFailure(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(nil))
	srv.FailNextDial()
	p := newTestProber(t, srv)

	result, err := p.Probe(context.Background())
	if err == nil {
		t.Fatal("Probe: expected an error from the failed dial")
	}
	if result.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 (no response received)", result.StatusCode)
	}
	if got := classifyFailure(result.StatusCode, err); got != "network" {
		t.Errorf("classifyFailure = %q, want %q", got, "network")
	}
}

func TestTelegramProber_Timeout(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Delayed(time.Hour, tgtest.Success(nil)))
	p := newTestProber(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// A pause before the call, as a descheduled goroutine takes on a
	// loaded machine: the deadline has to outlast it, so that it breaches
	// during the HTTP wait this test classifies rather than before the
	// call reaches the transport at all.
	time.Sleep(50 * time.Millisecond)
	result, err := p.Probe(ctx)
	if err == nil {
		t.Fatal("Probe: expected an error from the deadline")
	}
	if got := classifyFailure(result.StatusCode, err); got != "timeout" {
		t.Errorf("classifyFailure = %q, want %q (err=%v)", got, "timeout", err)
	}
}

func TestTelegramProber_NeverWritesToATransportRegistry(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(json.RawMessage(`{"id":1,"is_bot":true,"first_name":"x"}`)))
	p := newTestProber(t, srv)

	// A transport observer registered on a separate registry must
	// gather zero transport families after a probe — a canary call has
	// no path to the transport series.
	reg := NewRegistry()
	if _, err := NewTransportObserver(reg); err != nil {
		t.Fatalf("NewTransportObserver: %v", err)
	}

	if _, err := p.Probe(context.Background()); err != nil {
		t.Fatalf("Probe: %v", err)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if len(mf.GetMetric()) > 0 {
			t.Errorf("family %s carries %d metrics after a probe — a canary call must never reach the transport registry", mf.GetName(), len(mf.GetMetric()))
		}
	}
}

// pathRecorder records the last HTTP request path a fake server's handler
// observed, under a mutex — the handler runs on the server goroutine and
// the assertion reads it from the test goroutine, so a plain field would
// race even though the HTTP round trip itself happened to complete first.
type pathRecorder struct {
	mu   sync.Mutex
	path string
}

func (r *pathRecorder) record(p string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = p
}

func (r *pathRecorder) get() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path
}

// TestNewTelegramProber_TransmitsItsOwnToken asserts the pairing at the
// wire, not at the constructor argument: NewLegs's own recording-factory
// test (TestNewLegs_PairingNeverSwapped) never issues a real HTTP request,
// so it cannot catch a swap or a hard-coded credential introduced inside
// NewTelegramProber itself. Two probers, built with two distinguishable
// (but format-valid) tokens against two independent fake servers, each
// assert their own recorded request path — proving no crossover, not only
// that some token arrived.
func TestNewTelegramProber_TransmitsItsOwnToken(t *testing.T) {
	t.Parallel()

	// Both tokens match telego's own token shape (a digit run, a colon,
	// then 35 word/hyphen characters), the same shape this package's other
	// fake-token constant uses, so telego's construction-time validation
	// accepts them; the repeated "AAAA-"/"BBBB-" bodies make each
	// unmistakably not a credential.
	const tokenA = "111:AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-"
	const tokenB = "222:BBBB-BBBB-BBBB-BBBB-BBBB-BBBB-BBBB-"

	var pathA, pathB pathRecorder
	successBody := json.RawMessage(`{"id":1,"is_bot":true,"first_name":"x"}`)
	srvA := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		pathA.record(r.URL.Path)
		tgtest.Success(successBody)(w, r)
	})
	srvB := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		pathB.record(r.URL.Path)
		tgtest.Success(successBody)(w, r)
	})

	pA, err := NewTelegramProber(TelegramProberOptions{
		Token:      tokenA,
		BaseURL:    tgtest.BaseURL,
		Transport:  probeTransport(),
		HTTPClient: srvA.Client(),
	})
	if err != nil {
		t.Fatalf("NewTelegramProber (leg A): %v", err)
	}
	pB, err := NewTelegramProber(TelegramProberOptions{
		Token:      tokenB,
		BaseURL:    tgtest.BaseURL,
		Transport:  probeTransport(),
		HTTPClient: srvB.Client(),
	})
	if err != nil {
		t.Fatalf("NewTelegramProber (leg B): %v", err)
	}

	if _, err := pA.Probe(context.Background()); err != nil {
		t.Fatalf("Probe (leg A): %v", err)
	}
	if _, err := pB.Probe(context.Background()); err != nil {
		t.Fatalf("Probe (leg B): %v", err)
	}

	wantA := "/bot" + tokenA + "/getMe"
	wantB := "/bot" + tokenB + "/getMe"
	if got := pathA.get(); got != wantA {
		t.Errorf("leg A transmitted path %q, want %q", got, wantA)
	}
	if got := pathB.get(); got != wantB {
		t.Errorf("leg B transmitted path %q, want %q", got, wantB)
	}
	if pathA.get() == pathB.get() {
		t.Errorf("legs A and B transmitted the same request path %q — the token was not distinguished at the wire", pathA.get())
	}
}

// classifyFailure_TableTest exercises every branch directly, independent
// of a real probe.
func TestClassifyFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		statusCode int
		err        error
		want       string
	}{
		{"status code wins", 500, errors.New("ignored"), "500"},
		{"deadline exceeded", 0, context.DeadlineExceeded, "timeout"},
		{"canceled", 0, context.Canceled, "canceled"},
		{"other", 0, errors.New("dial refused"), "network"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyFailure(tc.statusCode, tc.err); got != tc.want {
				t.Errorf("classifyFailure(%d, %v) = %q, want %q", tc.statusCode, tc.err, got, tc.want)
			}
		})
	}
}
