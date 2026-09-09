package health

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// recordingFactory is a ProberFactory that records every
// TelegramProberOptions it was called with and returns fakeProber
// values — used by the leg-builder scenarios below, which must never
// touch the network.
type recordingFactory struct {
	mu    sync.Mutex
	calls []TelegramProberOptions
}

func (f *recordingFactory) build(opts TelegramProberOptions) (Prober, error) {
	f.mu.Lock()
	f.calls = append(f.calls, opts)
	f.mu.Unlock()
	return &fakeProber{}, nil
}

func (f *recordingFactory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestNewLegs_PairingNeverSwapped(t *testing.T) {
	t.Parallel()
	factory := &recordingFactory{}
	legs, err := NewLegs(LegsOptions{
		OwnToken:      "own-token",
		OwnBaseURL:    "https://own.invalid",
		CloudToken:    "cloud-token",
		CloudBaseURL:  "https://cloud.invalid",
		ProberFactory: factory.build,
	})
	if err != nil {
		t.Fatalf("NewLegs: %v", err)
	}
	if legs.Own == nil {
		t.Fatal("Own must not be nil")
	}
	if legs.Cloud == nil {
		t.Fatal("Cloud must not be nil when CloudToken is set")
	}
	if got := factory.callCount(); got != 2 {
		t.Fatalf("factory called %d times, want 2 (one per enabled leg)", got)
	}
	own, cloud := factory.calls[0], factory.calls[1]
	if own.Token != "own-token" || own.BaseURL != "https://own.invalid" {
		t.Errorf("own leg = (%q, %q), want (%q, %q)", own.Token, own.BaseURL, "own-token", "https://own.invalid")
	}
	if cloud.Token != "cloud-token" || cloud.BaseURL != "https://cloud.invalid" {
		t.Errorf("cloud leg = (%q, %q), want (%q, %q)", cloud.Token, cloud.BaseURL, "cloud-token", "https://cloud.invalid")
	}
}

// TestLegsOptions_RedactsTokensInDefaultVerb falsifies a %v rendering of
// LegsOptions that leaks either token: both fields carry a type that
// redacts its own rendering precisely so this never happens.
func TestLegsOptions_RedactsTokensInDefaultVerb(t *testing.T) {
	t.Parallel()
	opts := LegsOptions{
		OwnToken:     "own-secret-token",
		OwnBaseURL:   "https://own.invalid",
		CloudToken:   "cloud-secret-token",
		CloudBaseURL: "https://cloud.invalid",
	}
	rendered := fmt.Sprintf("%v", opts)
	if strings.Contains(rendered, "own-secret-token") || strings.Contains(rendered, "cloud-secret-token") {
		t.Errorf("%%v of LegsOptions leaked a token: %s", rendered)
	}
	if got := strings.Count(rendered, "[redacted]"); got != 2 {
		t.Errorf("%%v of LegsOptions carries %d redaction placeholders, want 2: %s", got, rendered)
	}
}

func TestNewLegs_EmptyCloudTokenDisablesCloudLeg(t *testing.T) {
	t.Parallel()
	factory := &recordingFactory{}
	legs, err := NewLegs(LegsOptions{
		OwnToken:      "own-token",
		OwnBaseURL:    "https://own.invalid",
		CloudToken:    "",
		ProberFactory: factory.build,
	})
	if err != nil {
		t.Fatalf("NewLegs: %v", err)
	}
	if legs.Cloud != nil {
		t.Fatal("Cloud must be nil when CloudToken is empty")
	}
	if got := factory.callCount(); got != 1 {
		t.Fatalf("factory called %d times, want 1 (own leg only)", got)
	}
}

// fakeProber is a Prober whose behaviour a canary runner test controls
// directly, with no network involved.
type fakeProber struct {
	mu    sync.Mutex
	calls int
	block time.Duration
	err   error
}

func (p *fakeProber) Probe(ctx context.Context) (ProbeResult, error) {
	p.mu.Lock()
	p.calls++
	block := p.block
	err := p.err
	p.mu.Unlock()
	if block > 0 {
		select {
		case <-time.After(block):
		case <-ctx.Done():
			return ProbeResult{}, ctx.Err()
		}
	}
	return ProbeResult{StatusCode: 200}, err
}

func (p *fakeProber) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func newTestCanary(t *testing.T, legs *Legs, interval time.Duration) *Canary {
	t.Helper()
	c, err := NewCanary(NewRegistry(), CanaryOptions{Legs: legs, Interval: interval})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	return c
}

func TestCanary_FirstProbeFiresAtStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		own := &fakeProber{}
		c := newTestCanary(t, &Legs{Own: own}, time.Minute)
		if err := c.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		synctest.Wait()
		if got := own.callCount(); got != 1 {
			t.Errorf("own.calls = %d, want 1 immediately after Start", got)
		}
		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	})
}

func TestCanary_BothLegsProbeEachTick(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		own := &fakeProber{}
		cloud := &fakeProber{}
		interval := time.Minute
		c := newTestCanary(t, &Legs{Own: own, Cloud: cloud}, interval)
		if err := c.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		synctest.Wait()
		if own.callCount() != 1 || cloud.callCount() != 1 {
			t.Fatalf("calls after Start = (own=%d, cloud=%d), want (1, 1)", own.callCount(), cloud.callCount())
		}

		time.Sleep(interval)
		synctest.Wait()
		if own.callCount() != 2 || cloud.callCount() != 2 {
			t.Errorf("calls after one interval = (own=%d, cloud=%d), want (2, 2)", own.callCount(), cloud.callCount())
		}

		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	})
}

func TestCanary_ShutdownReturnsPromptlyMidInterval(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		own := &fakeProber{}
		c := newTestCanary(t, &Legs{Own: own}, time.Hour)
		if err := c.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		synctest.Wait()

		before := time.Now()
		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		if elapsed := time.Since(before); elapsed >= time.Hour {
			t.Errorf("Shutdown took %v, want well under the 1h interval", elapsed)
		}
	})
}

func TestCanary_SecondStartErrors(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		own := &fakeProber{}
		c := newTestCanary(t, &Legs{Own: own}, time.Minute)
		if err := c.Start(); err != nil {
			t.Fatalf("first Start: %v", err)
		}
		if err := c.Start(); err == nil {
			t.Error("second Start: expected an error")
		}
		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	})
}

func TestCanary_ShutdownWithoutStartErrors(t *testing.T) {
	t.Parallel()
	c := newTestCanary(t, &Legs{Own: &fakeProber{}}, time.Minute)
	if err := c.Shutdown(context.Background()); err == nil {
		t.Error("Shutdown: expected an error on a canary never started")
	}
}

func TestCanary_ProberBlockingPastIntervalIsCancelledAndCountedFailure(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		interval := time.Minute
		own := &fakeProber{block: 2 * interval}
		c := newTestCanary(t, &Legs{Own: own}, interval)
		if err := c.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}

		// The first tick's own probe blocks for 2 intervals; its own
		// per-tick context is bounded at 1 interval, so it must be
		// cancelled and counted a failure rather than overlap the next
		// tick's probe.
		time.Sleep(interval)
		synctest.Wait()
		if got := own.callCount(); got != 1 {
			t.Fatalf("own.calls after 1 interval = %d, want 1 (still blocked, not overlapped)", got)
		}

		time.Sleep(interval)
		synctest.Wait()
		if got := own.callCount(); got != 2 {
			t.Errorf("own.calls after 2 intervals = %d, want 2 (second tick started only after the first was cancelled)", got)
		}

		if got := testutil.ToFloat64(c.failures.WithLabelValues(legOwn, "timeout")); got != 1 {
			t.Errorf("failures{leg=own,reason=timeout} = %v, want 1", got)
		}

		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	})
}

func TestCanary_DisabledCloudLegExportsNoSeries(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		own := &fakeProber{}
		c := newTestCanary(t, &Legs{Own: own}, time.Minute)
		if err := c.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		for range 3 {
			time.Sleep(time.Minute)
			synctest.Wait()
		}
		if err := c.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}

		mfs, err := gatherFrom(t, c.probes)
		if err != nil {
			t.Fatalf("gatherFrom: %v", err)
		}
		if values := observedLabelValues(mfs, labelLeg); values[legCloud] {
			t.Error("labgame_canary_probes_total carries a leg=\"cloud\" series with no cloud leg configured")
		}
	})
}

func TestNewCanary_RejectsNilOwnLegAndNonPositiveInterval(t *testing.T) {
	t.Parallel()
	if _, err := NewCanary(NewRegistry(), CanaryOptions{Legs: &Legs{}, Interval: time.Minute}); err == nil {
		t.Error("NewCanary: expected an error with a nil Own leg")
	}
	if _, err := NewCanary(NewRegistry(), CanaryOptions{Legs: &Legs{Own: &fakeProber{}}, Interval: 0}); err == nil {
		t.Error("NewCanary: expected an error with a non-positive Interval")
	}
}
