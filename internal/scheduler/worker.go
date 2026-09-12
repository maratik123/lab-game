package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
)

// reasonMustBePositive is New's rejection reason for every non-positive
// Scheduler config field.
const reasonMustBePositive = "must be positive"

// reasonMustNotBeNil is the rejection reason for every required nil
// pointer/interface field across this package's option structs.
const reasonMustNotBeNil = "must not be nil"

// OptionError is returned by New when an Options field is invalid.
type OptionError struct {
	// Field names the invalid Scheduler config field.
	Field string
	// Reason describes why the value was rejected.
	Reason string
}

// Error renders "scheduler: <field>: <reason>".
func (e *OptionError) Error() string {
	return fmt.Sprintf("scheduler: %s: %s", e.Field, e.Reason)
}

// Options configures a Worker.
type Options struct {
	// Pool is the connection pool the worker claims and executes tasks
	// on. Must not be nil.
	Pool *pgxpool.Pool
	// Registry is the immutable set of declared task types. Must not be
	// nil.
	Registry *Registry
	// Config is the scheduler's polling, claim-batch and retry tuning.
	// Every field must be strictly positive.
	Config config.Scheduler
	// Observer optionally receives this worker's observations. A nil
	// Observer is checked, not called.
	Observer Observer
	// Logger receives the log record a recovered handler panic emits at
	// its boundary, since not every attempt leaves a settled row to
	// carry that stack. A nil Logger is replaced with a discard handler,
	// not refused — the same treatment a nil Observer already gets.
	Logger *slog.Logger
}

// Worker drains due scheduled_task rows: discover, execute, settle.
type Worker struct {
	pool     *pgxpool.Pool
	registry *Registry
	cfg      config.Scheduler
	observer Observer
	logger   *slog.Logger

	// pendingMu guards pending, the set of tasks whose transaction died
	// before it could settle them — a deadline breach or a failed COMMIT.
	// Run and a caller's RunOnce may both touch it.
	pendingMu sync.Mutex
	pending   map[TaskID]pendingSettlement

	// stopOnce/stopCh implement Stop: closed once, selected on beside
	// ctx.Done() in Run, so Run returns nil when stopped and ctx.Err()
	// when cancelled — the drain's lever, shared in shape with Liveness
	// and (*Loop).Stop.
	stopOnce sync.Once
	stopCh   chan struct{}
}

// New builds a Worker from opts, refusing a nil Pool, a nil Registry, or
// any non-positive Scheduler config field with an *OptionError naming the
// field.
func New(opts Options) (*Worker, error) {
	if opts.Pool == nil {
		return nil, &OptionError{Field: "Pool", Reason: reasonMustNotBeNil}
	}
	if opts.Registry == nil {
		return nil, &OptionError{Field: "Registry", Reason: reasonMustNotBeNil}
	}
	positiveFields := []struct {
		name  string
		value time.Duration
	}{
		{"PollInterval", opts.Config.PollInterval},
		{"ClaimLimit", time.Duration(opts.Config.ClaimLimit)},
		{"RetryMaxAttempts", time.Duration(opts.Config.RetryMaxAttempts)},
		{"RetryBaseDelay", opts.Config.RetryBaseDelay},
		{"RetryMaxDelay", opts.Config.RetryMaxDelay},
		{"TaskTimeout", opts.Config.TaskTimeout},
	}
	for _, f := range positiveFields {
		if f.value <= 0 {
			return nil, &OptionError{Field: f.name, Reason: reasonMustBePositive}
		}
	}
	if !backoff.ValidFactor(opts.Config.RetryFactor) {
		return nil, &OptionError{Field: "RetryFactor", Reason: "must be finite and strictly greater than 1"}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Worker{
		pool:     opts.Pool,
		registry: opts.Registry,
		cfg:      opts.Config,
		observer: opts.Observer,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}, nil
}

// RunOnce runs one discovery-then-execute cycle: one
// discovery statement, then each discovered id in its own transaction.
// It reports exactly one LoopObservation per call, whether it succeeds or
// fails — a discovery failure is reported through LoopObservation.Err
// as well as returned, since Logger records only a recovered handler
// panic, never a discovery failure, and Run must not stop on a
// transient one.
func (w *Worker) RunOnce(ctx context.Context) error {
	start := time.Now()

	if err := w.drainPending(ctx); err != nil {
		w.observeLoop(LoopObservation{Duration: time.Since(start), Err: err})
		return err
	}

	ids, err := discoverDue(ctx, w.pool, w.cfg.ClaimLimit)
	if err != nil {
		w.observeLoop(LoopObservation{Duration: time.Since(start), Err: err})
		return err
	}

	for _, id := range ids {
		if err := w.executeOne(ctx, id, len(ids)); err != nil {
			w.observeLoop(LoopObservation{Duration: time.Since(start), BatchSize: len(ids), Err: err})
			return err
		}
	}
	w.observeLoop(LoopObservation{Duration: time.Since(start), BatchSize: len(ids)})
	return nil
}

// Run calls Reconcile once, then loops RunOnce at the configured poll
// interval until ctx is done, returning ctx.Err(). If Reconcile fails,
// Run returns its error without entering the loop at all — a worker
// that could not reconcile is one whose recurrences may be missing, and
// running anyway would hide that behind a quiet loop. When ctx is
// already done at the point Reconcile fails, Run reports ctx.Err()
// instead of Reconcile's error, since a failure observed after
// cancellation is a cancellation, not a reconcile defect, however the
// underlying driver happened to phrase it.
// RunOnce does not call Reconcile: it is the single-cycle primitive the
// tests drive, and an implicit reconcile inside it would make every
// cycle test a reconcile test too.
//
// After Reconcile, Run runs one cycle immediately, then waits, so a
// freshly inserted due task need not wait a full poll interval to be
// picked up on start-up. Run does not stop on a RunOnce error — each
// cycle already reports it through ObserveLoop — because a
// worker that stopped on a transient discovery failure would need an
// external restart for no reason.
//
// Stop makes Run return nil after the cycle in flight (if any)
// completes, rather than waiting for ctx to be done: Run returns nil
// when stopped and ctx.Err() when cancelled, which is what lets a
// caller tell a completed drain from an abandoned one. Stop called
// before Run's first cycle makes Run return nil without starting one.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.Reconcile(ctx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("scheduler: run: %w", err)
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_ = w.RunOnce(ctx)

		select {
		case <-w.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Stop makes Run return nil after the cycle in flight (if any)
// completes, rather than waiting for ctx to be done. Safe to call more
// than once and from any goroutine.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
}

// observeTask reports obs to w's Observer, when one is installed.
func (w *Worker) observeTask(obs Observation) {
	if w.observer != nil {
		w.observer.ObserveTask(obs)
	}
}

// observeLoop reports obs to w's Observer, when one is installed.
func (w *Worker) observeLoop(obs LoopObservation) {
	if w.observer != nil {
		w.observer.ObserveLoop(obs)
	}
}
