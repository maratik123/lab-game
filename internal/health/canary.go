package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/maratik123/lab-game/internal/config"
)

// Metric family names for the canary — declared here, beside the
// families they name, rather than beside every other adapter's, since
// nothing else in this package registers a canary family.
const (
	familyCanaryProbes        = namePrefix + "canary_probes_total"
	familyCanaryProbeFailures = namePrefix + "canary_probe_failures_total"
	familyCanaryProbeDuration = namePrefix + "canary_probe_duration_seconds"
)

// Canary outcome label values.
const (
	canaryOutcomeSuccess = "success"
	canaryOutcomeFailure = "failure"
)

// Canary leg label values.
const (
	legOwn   = "own"
	legCloud = "cloud"
)

// LegsOptions carries each leg's own credential beside its own endpoint,
// and nothing else routes one to the other — the pairing this package
// supplies; wiring these fields from the running configuration is the
// composition root's own obligation.
type LegsOptions struct {
	// OwnToken is the own-instance leg's bot token. Its type redacts it
	// from a %v of this struct.
	OwnToken config.Secret
	// OwnBaseURL is the own-instance leg's Bot API base URL.
	OwnBaseURL string
	// CloudToken is the cloud reference leg's bot token. Empty disables
	// the cloud leg entirely. Its type redacts it from a %v of this
	// struct.
	CloudToken config.Secret
	// CloudBaseURL is the cloud reference leg's Bot API base URL.
	CloudBaseURL string
	// Transport is copied into each leg's prober, with
	// RetryMaxAttempts always forced to exactly one attempt per call —
	// the canary's own attempt count is structural, not configured.
	Transport config.Transport
	// HTTPClient, when non-nil, is threaded into each leg's
	// TelegramProberOptions.HTTPClient — the composition root's one
	// process-wide client, or a test's own.
	HTTPClient *http.Client
	// ProberFactory builds one leg's Prober. Nil means
	// NewTelegramProber. A test installs a recording factory to assert
	// the exact (Token, BaseURL) pair each leg is built from — the seam
	// exists for that assertion and no other reason.
	ProberFactory func(TelegramProberOptions) (Prober, error)
}

// Legs holds the two canary probers NewLegs built. Own is never nil.
// Cloud is nil when the cloud token is empty — no cloud client is built
// at all in that case.
type Legs struct {
	Own   Prober
	Cloud Prober
}

// NewLegs builds the own-instance leg and, when CloudToken is non-empty,
// the cloud reference leg — each leg's own credential paired with its
// own endpoint, never swapped.
func NewLegs(opts LegsOptions) (*Legs, error) {
	factory := opts.ProberFactory
	if factory == nil {
		factory = func(o TelegramProberOptions) (Prober, error) { return NewTelegramProber(o) }
	}

	own, err := factory(TelegramProberOptions{
		Token:      opts.OwnToken,
		BaseURL:    opts.OwnBaseURL,
		Transport:  opts.Transport,
		HTTPClient: opts.HTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("health: build own-instance canary leg: %w", err)
	}

	legs := &Legs{Own: own}
	if opts.CloudToken != "" {
		cloud, err := factory(TelegramProberOptions{
			Token:      opts.CloudToken,
			BaseURL:    opts.CloudBaseURL,
			Transport:  opts.Transport,
			HTTPClient: opts.HTTPClient,
		})
		if err != nil {
			return nil, fmt.Errorf("health: build cloud-reference canary leg: %w", err)
		}
		legs.Cloud = cloud
	}
	return legs, nil
}

// CanaryOptions configures NewCanary.
type CanaryOptions struct {
	// Legs is the pair of probers each tick drives. Own must be
	// non-nil; Cloud may be nil, meaning the cloud leg is disabled.
	Legs *Legs
	// Interval is the tick cadence driving both legs — one cadence
	// value for both.
	Interval time.Duration
}

// Canary is the two-leg Telegram canary runner: it ticks Interval,
// probing each configured leg once per tick and recording its outcome —
// never pre-initialising a label combination for a leg that never
// probes, so a disabled cloud leg exports no series of any kind.
type Canary struct {
	legs     *Legs
	interval time.Duration

	probes   *prometheus.CounterVec
	failures *prometheus.CounterVec
	duration *prometheus.HistogramVec

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewCanary registers the canary families on reg and returns the runner.
func NewCanary(reg prometheus.Registerer, opts CanaryOptions) (*Canary, error) {
	if opts.Legs == nil || opts.Legs.Own == nil {
		return nil, errors.New("health: canary: Legs.Own must not be nil")
	}
	if opts.Interval <= 0 {
		return nil, fmt.Errorf("health: canary: Interval must be positive, got %s", opts.Interval)
	}

	c := &Canary{
		legs:     opts.Legs,
		interval: opts.Interval,
		probes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familyCanaryProbes,
			Help: "Canary probes, by leg and outcome.",
		}, []string{labelLeg, labelOutcome}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familyCanaryProbeFailures,
			Help: "Failed canary probes, by leg and classified failure reason.",
		}, []string{labelLeg, labelReason}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    familyCanaryProbeDuration,
			Help:    "Canary probe latency in seconds, by leg and outcome.",
			Buckets: durationBuckets,
		}, []string{labelLeg, labelOutcome}),
	}
	for _, col := range []prometheus.Collector{c.probes, c.failures, c.duration} {
		if err := reg.Register(col); err != nil {
			return nil, fmt.Errorf("health: register canary family: %w", err)
		}
	}
	return c, nil
}

// Start begins ticking: the first probe fires immediately, and one more
// fires per Interval thereafter, until Shutdown. Returns an error rather
// than panicking on a second call.
func (c *Canary) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return errors.New("health: canary already started")
	}
	c.started = true
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	go c.run(ctx)
	return nil
}

// run drives the tick loop until ctx is done.
func (c *Canary) run(ctx context.Context) {
	defer close(c.done)
	c.tick(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick probes every configured leg once, concurrently, and waits for
// both before returning — so both legs probe on the same tick from one
// cadence value. A leg that blocks past Interval is cancelled and
// counted a failure rather than overlapping the next tick.
func (c *Canary) tick(ctx context.Context) {
	tickCtx, cancel := context.WithTimeout(ctx, c.interval)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.probeLeg(tickCtx, legOwn, c.legs.Own)
	}()
	if c.legs.Cloud != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.probeLeg(tickCtx, legCloud, c.legs.Cloud)
		}()
	}
	wg.Wait()
}

// probeLeg runs one probe attempt for leg and records its outcome.
func (c *Canary) probeLeg(ctx context.Context, leg string, p Prober) {
	result, err := p.Probe(ctx)
	outcome := canaryOutcomeSuccess
	if err != nil {
		outcome = canaryOutcomeFailure
	}
	c.probes.WithLabelValues(leg, outcome).Inc()
	c.duration.WithLabelValues(leg, outcome).Observe(result.Latency.Seconds())
	if err != nil {
		c.failures.WithLabelValues(leg, classifyFailure(result.StatusCode, err)).Inc()
	}
}

// Shutdown cancels the run context — which cancels any in-flight
// per-tick context with it — and waits for the probe goroutine to exit,
// so a shutdown never waits out the current interval. Returns an error
// rather than panicking when called on a canary never started.
func (c *Canary) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	started := c.started
	cancel := c.cancel
	done := c.done
	c.mu.Unlock()
	if !started {
		return errors.New("health: canary not started")
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
