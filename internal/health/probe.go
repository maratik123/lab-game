package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tg"
)

// ProbeResult carries one probe attempt's outcome: the whole-call
// latency and the status code the transport client recorded.
type ProbeResult struct {
	// Latency is the whole call's duration, as the transport client's
	// own observation reports it.
	Latency time.Duration
	// StatusCode is the last HTTP status code received, or 0 when none
	// was ever received.
	StatusCode int
}

// Prober performs one canary probe attempt against one Telegram Bot API
// endpoint. The runner calls Probe once per leg per tick and waits for
// it before the next.
type Prober interface {
	// Probe issues one attempt and reports its outcome.
	Probe(ctx context.Context) (ProbeResult, error)
}

// TelegramProberOptions configures NewTelegramProber. Token and BaseURL
// are one leg's own credential-endpoint pair — this type carries no
// notion of which leg it is building; the leg builder is where that
// pairing is decided.
type TelegramProberOptions struct {
	// Token is the bot token this leg's getMe call authenticates with.
	// Its type redacts it from a %v of this struct.
	Token config.Secret
	// BaseURL is the Bot API base URL this leg probes.
	BaseURL string
	// Transport is copied into the built client, with RetryMaxAttempts
	// forced to one by NewTelegramProber regardless of this field's own
	// value.
	Transport config.Transport
	// HTTPClient, when non-nil, is the client the built transport client
	// performs attempts with — a test's fake-server client. Nil means
	// the default transport.
	HTTPClient *http.Client
}

// statusRecorder is a transport observer that records the single
// observation of the one call in flight on the client it is installed
// on. Its precondition — at most one call in flight at a time — is
// guaranteed by the canary runner, which probes one leg per tick and
// awaits it before the next.
type statusRecorder struct {
	mu  sync.Mutex
	obs tg.Observation
}

// ObserveCall implements the transport client's Observer interface.
func (r *statusRecorder) ObserveCall(obs tg.Observation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.obs = obs
}

// take returns the last recorded Observation.
func (r *statusRecorder) take() tg.Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.obs
}

// TelegramProber probes one Telegram Bot API endpoint with getMe,
// through its own transport client and its own status recorder — never
// this package's transport adapter, so a canary call never reaches the
// transport series.
type TelegramProber struct {
	client   *tg.Client
	recorder *statusRecorder
}

// NewTelegramProber builds a TelegramProber against opts.BaseURL with
// opts.Token, forcing exactly one attempt per call regardless of
// opts.Transport.RetryMaxAttempts — the canary's own attempt count is
// structural, not configured.
func NewTelegramProber(opts TelegramProberOptions) (*TelegramProber, error) {
	transport := opts.Transport
	transport.RetryMaxAttempts = 1
	recorder := &statusRecorder{}
	client, err := tg.New(tg.Options{
		BaseURL:    opts.BaseURL,
		Token:      opts.Token.Reveal(),
		Transport:  transport,
		Observer:   recorder,
		HTTPClient: opts.HTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("health: build canary client: %w", err)
	}
	return &TelegramProber{client: client, recorder: recorder}, nil
}

// Probe issues one getMe call. Success is reported only when the call
// returned no error — which telego's caller guarantees only for a
// decoded envelope whose Ok field is true — and the recorded status
// code is exactly 200; a non-200 status paired with a nil error (an
// envelope claiming ok:true on a non-200 response) is still reported
// as a failure.
func (p *TelegramProber) Probe(ctx context.Context) (ProbeResult, error) {
	_, err := p.client.API().GetMe(ctx)
	obs := p.recorder.take()
	result := ProbeResult{Latency: obs.Latency, StatusCode: obs.StatusCode}
	if err != nil {
		return result, err
	}
	if obs.StatusCode != http.StatusOK {
		return result, fmt.Errorf("health: probe: status %d with no call error", obs.StatusCode)
	}
	return result, nil
}

// classifyFailure renders a failed probe's reason label, never from
// error text: a recorded status code above zero renders as its decimal
// form; a zero status code renders as one of the enumerated transport
// classes derived from the error's kind.
func classifyFailure(statusCode int, err error) string {
	if statusCode > 0 {
		return strconv.Itoa(statusCode)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "network"
	}
}
