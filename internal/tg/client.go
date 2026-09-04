package tg

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/config"
)

// Options configures a Client (design D2). Every outbound-call method
// Client.API() exposes is telego's own generated method, taking
// ctx context.Context first — Options and Client themselves store no
// context.Context field (AC1).
type Options struct {
	// BaseURL is the Bot API server's base URL — the self-hosted
	// instance, api.telegram.org, or a fake server's URL in a test — all
	// three are values of this one field, never a branch in code
	// (design D2, AC20).
	BaseURL string
	// Token is the Telegram bot token.
	Token string
	// Transport holds the retry, backoff and rate-limit tuning this
	// client is built from (design D9, D10).
	Transport config.Transport
	// Gate, when non-nil, is consulted before every outbound call
	// (design D12). Nil means no gate: every call proceeds to the
	// limiter.
	Gate Gate
	// Observer, when non-nil, receives one Observation per outbound call
	// (design D11). Nil means no observation.
	Observer Observer
	// Jitter, when non-nil, supplies the backoff formula's random factor
	// in [0,1) (design D6). Nil means the default math/rand/v2 source.
	Jitter func() float64
	// HTTPClient, when non-nil, is the *http.Client this package's caller
	// performs attempts with. Nil means http.DefaultClient's transport
	// settings are not assumed — a client is constructed with sane
	// defaults.
	HTTPClient *http.Client
	// Logger, when non-nil, is installed as telego's own logger. Nil
	// means telego.WithDiscardLogger() — this package's typed Error and
	// Observer are the transport's real output, and telego's own default
	// stderr logger is a duplicate, token-redacted-but-still-noisy
	// channel this package does not want to own by default (design D2).
	Logger telego.Logger
}

// OptionError names the Options field that failed New's validation.
type OptionError struct {
	// Field is the Options field name that failed validation.
	Field string
	// Err is the reason it failed.
	Err error
}

// Error renders "tg: options: <field>: <cause>".
func (e *OptionError) Error() string {
	return fmt.Sprintf("tg: options: %s: %s", e.Field, e.Err)
}

// Unwrap returns e.Err.
func (e *OptionError) Unwrap() error {
	return e.Err
}

// optionErrorf builds an *OptionError for field with a formatted cause.
func optionErrorf(field, format string, args ...any) *OptionError {
	return &OptionError{Field: field, Err: fmt.Errorf(format, args...)}
}

// Client is lab-game's one Telegram Bot API client (design D2). Every
// outbound call in the project is a call on the *telego.Bot Client.API()
// returns, and every one of them is routed through this package's own
// telegoapi.Caller — the retry loop, the retry_after wait, both rate
// limiters, the outbound gate and the observation point.
type Client struct {
	bot           *telego.Bot
	transport     config.Transport
	gate          Gate
	observer      Observer
	jitter        func() float64
	httpClient    *http.Client
	limiter       *Limiter
	tokenReplacer *strings.Replacer
}

// New validates opts and constructs a Client. It returns an *OptionError
// naming the offending field rather than falling back to a compiled-in
// value — internal/config (design D10) is the one place defaults live
// (design D2).
func New(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, optionErrorf("BaseURL", "must not be empty")
	}
	if opts.Token == "" {
		return nil, optionErrorf("Token", "must not be empty")
	}
	if opts.Transport.RetryMaxAttempts < 1 {
		return nil, optionErrorf("Transport.RetryMaxAttempts", "must be at least 1, got %d", opts.Transport.RetryMaxAttempts)
	}
	if opts.Transport.RetryBaseDelay <= 0 {
		return nil, optionErrorf("Transport.RetryBaseDelay", "must be positive, got %s", opts.Transport.RetryBaseDelay)
	}
	if opts.Transport.RetryMaxDelay <= 0 {
		return nil, optionErrorf("Transport.RetryMaxDelay", "must be positive, got %s", opts.Transport.RetryMaxDelay)
	}
	if opts.Transport.AttemptTimeout <= 0 {
		return nil, optionErrorf("Transport.AttemptTimeout", "must be positive, got %s", opts.Transport.AttemptTimeout)
	}

	jitter := opts.Jitter
	if jitter == nil {
		jitter = defaultJitter
	}

	c := &Client{
		transport:     opts.Transport,
		gate:          opts.Gate,
		observer:      opts.Observer,
		jitter:        jitter,
		httpClient:    opts.HTTPClient,
		limiter:       newLimiter(opts.Transport.Limits),
		tokenReplacer: strings.NewReplacer(opts.Token, "[REDACTED_TOKEN]"),
	}

	// WithAPICaller and WithRequestConstructor are the whole net/http +
	// encoding/json swap (design D3): telego's own bot never gets a
	// chance to install its default fasthttp caller or constructor, so no
	// call issued through the returned Client can reach them (design D2's
	// accessor guarantee, AC2, AC27).
	botOptions := []telego.BotOption{
		telego.WithAPIServer(opts.BaseURL),
		telego.WithAPICaller(&caller{client: c}),
		telego.WithRequestConstructor(jsonConstructor{}),
	}
	if opts.Logger != nil {
		botOptions = append(botOptions, telego.WithLogger(opts.Logger))
	} else {
		botOptions = append(botOptions, telego.WithDiscardLogger())
	}

	bot, err := telego.NewBot(opts.Token, botOptions...)
	if err != nil {
		return nil, optionErrorf("Token", "%s", err)
	}
	c.bot = bot

	return c, nil
}

// API returns the configured *telego.Bot every outbound Bot API call goes
// through. Across the generated-method surface — every Bot.SendMessage-
// shaped method, which is the whole Bot API — there is no path that skips
// this package's own caller (design D2, AC27).
func (c *Client) API() *telego.Bot {
	return c.bot
}
