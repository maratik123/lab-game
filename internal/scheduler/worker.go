package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/config"
)

// reasonMustBePositive is New's rejection reason for every non-positive
// config.Scheduler field.
const reasonMustBePositive = "must be positive"

// OptionError is returned by New when an Options field is invalid.
type OptionError struct {
	// Field names the invalid config.Scheduler field.
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
}

// Worker drains due scheduled_task rows: discover, execute, settle.
type Worker struct {
	pool     *pgxpool.Pool
	registry *Registry
	cfg      config.Scheduler
	observer Observer
}

// New builds a Worker from opts, refusing a nil Pool, a nil Registry, or
// any non-positive config.Scheduler field with an *OptionError naming the
// field.
func New(opts Options) (*Worker, error) {
	if opts.Pool == nil {
		return nil, &OptionError{Field: "Pool", Reason: "must not be nil"}
	}
	if opts.Registry == nil {
		return nil, &OptionError{Field: "Registry", Reason: "must not be nil"}
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
	return &Worker{
		pool:     opts.Pool,
		registry: opts.Registry,
		cfg:      opts.Config,
		observer: opts.Observer,
	}, nil
}

// RunOnce runs one discovery-then-execute cycle (design D2): one
// discovery statement, then each discovered id in its own transaction.
// It reports exactly one LoopObservation per call, whether it succeeds or
// fails — a discovery failure is reported through LoopObservation.Err
// (design D12) as well as returned, since this package has no logger and
// Run must not stop on a transient one.
func (w *Worker) RunOnce(ctx context.Context) error {
	start := time.Now()

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

// Run loops RunOnce at the configured poll interval until ctx is done,
// returning ctx.Err(). It runs one cycle immediately, then waits, so a
// freshly inserted due task need not wait a full poll interval to be
// picked up on start-up. Run does not stop on a RunOnce error — each
// cycle already reports it through ObserveLoop (design D12) — because a
// worker that stopped on a transient discovery failure would need an
// external restart for no reason.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_ = w.RunOnce(ctx)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
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
