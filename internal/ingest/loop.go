package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tg"
)

// reasonMustNotBeNil and reasonMustBePositive are New's rejection
// reasons, mirroring this module's task-scheduler worker's own New.
const (
	reasonMustNotBeNil   = "must not be nil"
	reasonMustBePositive = "must be positive"
)

// OptionError is returned by New when an Options field is invalid.
type OptionError struct {
	// Field names the invalid Options field.
	Field string
	// Reason describes why the value was rejected.
	Reason string
}

// Error renders "ingest: <field>: <reason>".
func (e *OptionError) Error() string {
	return fmt.Sprintf("ingest: %s: %s", e.Field, e.Reason)
}

// Options configures a Loop.
type Options struct {
	// Client is the Telegram client this Loop polls getUpdates through and
	// hands to no other purpose. Its Options.Gate — installed at
	// construction, per this module's wiring order — is this task's chat
	// allowlist. Must not be nil.
	Client *tg.Client
	// Pool is the connection pool the loop begins each attempt's
	// transaction on. Must not be nil.
	Pool *pgxpool.Pool
	// Router dispatches a routed update to its Handler. Must not be nil;
	// an empty Router (no routes registered) is legal.
	Router *Router
	// Config holds the loop's polling, batch and retry tuning. Every
	// duration field must be strictly positive; BatchLimit and
	// RetryMaxAttempts must be strictly positive integers.
	Config config.Ingest
	// Observer optionally receives this loop's observations. A nil
	// Observer is checked, not called.
	Observer Observer
	// Logger receives the log record a recovered handler panic emits at
	// its recovery point, since a panic on an earlier attempt that a
	// later attempt supersedes leaves no give-up row of its own for that
	// stack to ride. A nil Logger is replaced with a discard handler,
	// not refused — the same treatment a nil Observer already gets.
	Logger *slog.Logger
}

// Loop is the update-ingestion front door: long-poll, dispatch, retry,
// idempotency and offset advance. Its structural
// model is this module's task-scheduler worker.
type Loop struct {
	client   *tg.Client
	pool     *pgxpool.Pool
	router   *Router
	cfg      config.Ingest
	observer Observer
	logger   *slog.Logger

	// allowedUpdates is transmitted on every getUpdates call, computed
	// once from Router.Kinds() at New time (Router is immutable). Never
	// empty: an empty route set carries a reserved sentinel instead, so
	// "send an empty list" — which encoding/json's omitempty would
	// silently erase — is never attempted.
	allowedUpdates []string

	// stopOnce/stopCh implement Stop: closed once, selected on beside
	// ctx.Done() in Run and, per cycle, watched to cancel the in-flight
	// getUpdates call without touching the batch it already returned —
	// the same drain lever Worker and Liveness carry.
	stopOnce sync.Once
	stopCh   chan struct{}
}

// New builds a Loop from opts, refusing a nil Client, Pool or Router, or
// any non-positive tuning field, with an *OptionError naming the field.
func New(opts Options) (*Loop, error) {
	if opts.Client == nil {
		return nil, &OptionError{Field: "Client", Reason: reasonMustNotBeNil}
	}
	if opts.Pool == nil {
		return nil, &OptionError{Field: "Pool", Reason: reasonMustNotBeNil}
	}
	if opts.Router == nil {
		return nil, &OptionError{Field: "Router", Reason: reasonMustNotBeNil}
	}

	positiveDurations := []struct {
		name  string
		value time.Duration
	}{
		{"Config.PollInterval", opts.Config.PollInterval},
		{"Config.LongPollTimeout", opts.Config.LongPollTimeout},
		{"Config.RetryBaseDelay", opts.Config.RetryBaseDelay},
		{"Config.RetryMaxDelay", opts.Config.RetryMaxDelay},
	}
	for _, f := range positiveDurations {
		if f.value <= 0 {
			return nil, &OptionError{Field: f.name, Reason: reasonMustBePositive}
		}
	}
	positiveInts := []struct {
		name  string
		value int
	}{
		{"Config.BatchLimit", opts.Config.BatchLimit},
		{"Config.RetryMaxAttempts", opts.Config.RetryMaxAttempts},
	}
	for _, f := range positiveInts {
		if f.value <= 0 {
			return nil, &OptionError{Field: f.name, Reason: reasonMustBePositive}
		}
	}
	if !backoff.ValidFactor(opts.Config.RetryFactor) {
		return nil, &OptionError{Field: "Config.RetryFactor", Reason: "must be finite and strictly greater than 1"}
	}

	kinds := opts.Router.Kinds()
	allowed := make([]string, 0, len(kinds))
	for _, k := range kinds {
		allowed = append(allowed, string(k))
	}
	if len(allowed) == 0 {
		// A reserved sentinel: an id space this bot can never
		// receive, present only while the route set is empty.
		allowed = append(allowed, string(telego.ShippingQueryUpdates))
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Loop{
		client:         opts.Client,
		pool:           opts.Pool,
		router:         opts.Router,
		cfg:            opts.Config,
		observer:       opts.Observer,
		logger:         logger,
		allowedUpdates: allowed,
		stopCh:         make(chan struct{}),
	}, nil
}

// PollOnce runs one long-poll cycle: read the offset to transmit, call
// getUpdates, and process every returned update in order. It reports
// exactly one LoopObservation per call, whether it succeeds or fails — a
// poll failure is reported through LoopObservation.Err as
// well as returned, since Run must not stop on a transient one.
//
// The getUpdates call alone runs on a per-cycle child context that Stop
// can cancel without touching ctx: readOffset and every processUpdate in
// the batch keep the parent ctx and run to completion, so a Stop that
// lands mid-batch settles every update already fetched rather than
// aborting it. When Stop cancels that child context while ctx itself is
// still live, the long poll's failure is a discarded cycle, not a failed
// one: the reported LoopObservation carries no Err, and PollOnce returns
// ErrPollDiscarded — a caller distinguishing "nothing happened" from
// "something went wrong" matches on it via errors.Is.
func (l *Loop) PollOnce(ctx context.Context) error {
	start := time.Now()

	offset, err := readOffset(ctx, l.pool)
	if err != nil {
		observeLoop(l.observer, LoopObservation{Duration: time.Since(start), Err: err})
		return err
	}

	params := &telego.GetUpdatesParams{
		Offset:         int(offset),
		Limit:          l.cfg.BatchLimit,
		Timeout:        int(l.cfg.LongPollTimeout / time.Second),
		AllowedUpdates: l.allowedUpdates,
	}

	updates, err := l.getUpdatesStoppable(ctx, params)
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.Canceled) {
			// Stop cancelled the poll while the parent context is still
			// live — a discarded cycle, not a failure.
			observeLoop(l.observer, LoopObservation{Duration: time.Since(start)})
			return ErrPollDiscarded
		}
		observeLoop(l.observer, LoopObservation{Duration: time.Since(start), Err: err})
		return err
	}

	for _, raw := range updates {
		if err := l.processUpdate(ctx, raw); err != nil {
			observeLoop(l.observer, LoopObservation{Duration: time.Since(start), BatchSize: len(updates), Err: err})
			return err
		}
	}
	observeLoop(l.observer, LoopObservation{Duration: time.Since(start), BatchSize: len(updates)})
	return nil
}

// getUpdatesStoppable calls GetUpdates on a child of ctx that is also
// cancelled the moment Stop is called, and joins the watcher goroutine
// that implements that before returning — so no goroutine outlives this
// call, and the batch is never touched until the watch is over.
func (l *Loop) getUpdatesStoppable(ctx context.Context, params *telego.GetUpdatesParams) ([]telego.Update, error) {
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-l.stopCh:
			cancel()
		case <-pollCtx.Done():
		}
	}()

	updates, err := l.client.API().GetUpdates(pollCtx, params)
	cancel()
	<-watchDone

	return updates, err
}

// Run loops PollOnce at the configured poll interval until Stop is
// called or ctx is done, returning nil when stopped and ctx.Err() when
// cancelled. Run does not stop on a PollOnce
// error — each cycle already reports it through ObserveLoop — because a
// loop that stopped on a transient poll failure against a self-hosted
// telegram-bot-api instance would need an external restart for no
// reason. Run runs one cycle immediately, then waits, so a fresh start
// need not wait a full poll interval for its first cycle.
func (l *Loop) Run(ctx context.Context) error {
	ticker := time.NewTicker(l.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_ = l.PollOnce(ctx)

		select {
		case <-l.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Stop makes Run return nil after the cycle in flight (if any)
// completes, cancelling that cycle's in-flight getUpdates call rather
// than waiting out the long-poll window. Safe to call more than once
// and from any goroutine.
func (l *Loop) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

// processUpdate derives raw's Update and dispatches it: to its Handler
// when one is registered, or to the unrouted settlement when none is.
// A non-nil return means ctx was cancelled mid-attempt
// — every other outcome is fully settled internally and
// reported through the observer, never returned as an error PollOnce
// must react to.
func (l *Loop) processUpdate(ctx context.Context, raw telego.Update) error {
	u, err := NewUpdate(raw)
	if err != nil {
		// A malformed update (an empty operation_id component) cannot be
		// settled meaningfully; treat it as unrouted rather than
		// stalling the whole batch on one bad payload. err is carried
		// into the observation rather than dropped.
		return l.settleUnrouted(ctx, Update{Raw: raw}, err)
	}

	handler, ok := l.router.Lookup(u.Kind)
	if !ok {
		return l.settleUnrouted(ctx, u, nil)
	}
	return l.runAttempts(ctx, handler, u)
}

// lagFor computes u's Observation.Lag/LagKnown pair: known
// only when u's derived kind carries its own date.
func lagFor(u Update) (time.Duration, bool) {
	date, ok := Date(&u.Raw)
	if !ok {
		return 0, false
	}
	return time.Since(date), true
}
