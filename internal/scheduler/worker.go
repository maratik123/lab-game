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
func (w *Worker) RunOnce(ctx context.Context) error {
	ids, err := discoverDue(ctx, w.pool, w.cfg.ClaimLimit)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := w.executeOne(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
